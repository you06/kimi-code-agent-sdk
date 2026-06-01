import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';

import { TransportError } from './errors.js';
import { JsonRpcStdioClient } from './protocol.js';
import type {
  ClientOptions,
  CreateSessionOptions,
  Event,
  JsonObject,
  ListSessionsOptions,
  PromptInput,
  SessionStatus,
  SessionSummary,
} from './types.js';

const PROTOCOL_VERSION = '1.0';

export async function connect(options: ClientOptions = {}): Promise<Client> {
  const executable = options.executable ?? 'kimi-code';
  const args = options.args ?? ['sdk-server', '--stdio'];
  const child = spawn(executable, args, {
    cwd: options.cwd,
    env: options.env,
    stdio: ['pipe', 'pipe', 'pipe'],
  });
  child.stderr.resume();
  const rpc = new JsonRpcStdioClient(child);
  const client = new Client(rpc, child);
  await client.initialize({
    name: options.clientName ?? '@moonshot-ai/kimi-code-agent-sdk',
    version: options.clientVersion,
  });
  return client;
}

export const createClient = connect;

export class Client {
  private readonly sessions = new Map<string, Session>();
  private readonly eventListeners = new Set<(event: Event) => void>();
  private readonly unsubscribeNotifications: () => void;

  constructor(
    private readonly rpc: JsonRpcStdioClient,
    private readonly child: ChildProcessWithoutNullStreams,
  ) {
    this.unsubscribeNotifications = rpc.onNotification((method, params) => {
      if (method !== 'event') return;
      if (!isEvent(params)) return;
      this.dispatchEvent(params);
    });
  }

  async initialize(client: { readonly name: string; readonly version?: string }): Promise<void> {
    await this.rpc.request('initialize', {
      supportedVersions: [PROTOCOL_VERSION],
      client,
    });
  }

  async createSession(options: CreateSessionOptions): Promise<Session> {
    const summary = asSessionSummary(await this.rpc.request('createSession', options));
    return this.bindSession(summary);
  }

  async resumeSession(options: { readonly id: string }): Promise<Session> {
    const summary = asSessionSummary(await this.rpc.request('resumeSession', options));
    return this.bindSession(summary);
  }

  async listSessions(options: ListSessionsOptions = {}): Promise<readonly SessionSummary[]> {
    const result = await this.rpc.request('listSessions', options);
    if (!Array.isArray(result)) {
      throw new TransportError('listSessions result must be an array.', result);
    }
    return result.map(asSessionSummary);
  }

  async close(): Promise<void> {
    this.unsubscribeNotifications();
    await this.rpc.request('shutdown', {}).catch(() => {});
    await this.rpc.close();
  }

  onEvent(listener: (event: Event) => void): () => void {
    this.eventListeners.add(listener);
    return () => {
      this.eventListeners.delete(listener);
    };
  }

  async closeSession(sessionId: string): Promise<void> {
    await this.rpc.request('closeSession', { sessionId });
    this.sessions.delete(sessionId);
  }

  prompt(sessionId: string, input: PromptInput): AsyncIterable<Event> {
    return this.streamTurn(sessionId, 'prompt', { sessionId, input });
  }

  async steer(sessionId: string, input: PromptInput): Promise<void> {
    await this.rpc.request('steer', { sessionId, input });
  }

  async cancel(sessionId: string, turnId?: number): Promise<void> {
    await this.rpc.request('cancel', { sessionId, turnId });
  }

  async getStatus(sessionId: string): Promise<SessionStatus> {
    return asSessionStatus(await this.rpc.request('getStatus', { sessionId }));
  }

  private bindSession(summary: SessionSummary): Session {
    const existing = this.sessions.get(summary.id);
    if (existing !== undefined) return existing;
    const session = new Session(this, summary);
    this.sessions.set(session.id, session);
    return session;
  }

  private dispatchEvent(event: Event): void {
    for (const listener of this.eventListeners) {
      listener(event);
    }
  }

  private async *streamTurn(
    sessionId: string,
    method: string,
    params: unknown,
  ): AsyncIterable<Event> {
    const queue = new EventQueue();
    const unsubscribe = this.onEvent((event) => {
      if (event.sessionId === sessionId) queue.push(event);
    });
    try {
      const result = await this.rpc.request(method, params);
      const turnId = asTurnId(result);
      for (;;) {
        const event = await queue.next();
        yield event;
        if (event.type === 'turn.ended' && event.turnId === turnId) return;
      }
    } finally {
      unsubscribe();
      queue.close();
    }
  }
}

export class Session {
  readonly id: string;
  readonly workDir: string;

  constructor(
    private readonly client: Client,
    readonly summary: SessionSummary,
  ) {
    this.id = summary.id;
    this.workDir = summary.workDir;
  }

  prompt(input: PromptInput): AsyncIterable<Event> {
    return this.client.prompt(this.id, input);
  }

  async close(): Promise<void> {
    await this.client.closeSession(this.id);
  }

  async steer(input: PromptInput): Promise<void> {
    await this.client.steer(this.id, input);
  }

  async cancel(turnId?: number): Promise<void> {
    await this.client.cancel(this.id, turnId);
  }

  async getStatus(): Promise<SessionStatus> {
    return this.client.getStatus(this.id);
  }
}

class EventQueue {
  private readonly values: Event[] = [];
  private readonly waiters: Array<(event: Event) => void> = [];
  private closed = false;

  push(event: Event): void {
    if (this.closed) return;
    const waiter = this.waiters.shift();
    if (waiter !== undefined) {
      waiter(event);
      return;
    }
    this.values.push(event);
  }

  next(): Promise<Event> {
    const value = this.values.shift();
    if (value !== undefined) return Promise.resolve(value);
    if (this.closed) return Promise.reject(new TransportError('Event stream closed.'));
    return new Promise((resolve) => {
      this.waiters.push(resolve);
    });
  }

  close(): void {
    this.closed = true;
  }
}

function asTurnId(value: unknown): number {
  if (!isRecord(value) || typeof value['turnId'] !== 'number') {
    throw new TransportError('prompt result must include turnId.', value);
  }
  return value['turnId'];
}

function asSessionSummary(value: unknown): SessionSummary {
  if (!isRecord(value)) throw new TransportError('session summary must be an object.', value);
  return {
    id: requiredString(value['id'], 'id'),
    workDir: requiredString(value['workDir'], 'workDir'),
    sessionDir: requiredString(value['sessionDir'], 'sessionDir'),
    createdAt: requiredNumber(value['createdAt'], 'createdAt'),
    updatedAt: requiredNumber(value['updatedAt'], 'updatedAt'),
    title: optionalString(value['title'], 'title'),
    lastPrompt: optionalString(value['lastPrompt'], 'lastPrompt'),
    archived: optionalBoolean(value['archived'], 'archived'),
    metadata: optionalObject(value['metadata'], 'metadata'),
  };
}

function asSessionStatus(value: unknown): SessionStatus {
  if (!isRecord(value)) throw new TransportError('session status must be an object.', value);
  return {
    model: optionalString(value['model'], 'model'),
    thinkingLevel: requiredString(value['thinkingLevel'], 'thinkingLevel'),
    permission: requiredPermission(value['permission']),
    planMode: requiredBoolean(value['planMode'], 'planMode'),
    contextTokens: requiredNumber(value['contextTokens'], 'contextTokens'),
    maxContextTokens: requiredNumber(value['maxContextTokens'], 'maxContextTokens'),
    contextUsage: requiredNumber(value['contextUsage'], 'contextUsage'),
    usage: value['usage'],
  };
}

function isEvent(value: unknown): value is Event {
  return (
    isRecord(value) &&
    typeof value['type'] === 'string' &&
    typeof value['sessionId'] === 'string' &&
    typeof value['agentId'] === 'string'
  );
}

function requiredString(value: unknown, field: string): string {
  if (typeof value !== 'string') throw new TransportError(`${field} must be a string.`, value);
  return value;
}

function optionalString(value: unknown, field: string): string | undefined {
  if (value === undefined) return undefined;
  return requiredString(value, field);
}

function requiredNumber(value: unknown, field: string): number {
  if (typeof value !== 'number') throw new TransportError(`${field} must be a number.`, value);
  return value;
}

function requiredBoolean(value: unknown, field: string): boolean {
  if (typeof value !== 'boolean') throw new TransportError(`${field} must be a boolean.`, value);
  return value;
}

function optionalBoolean(value: unknown, field: string): boolean | undefined {
  if (value === undefined) return undefined;
  return requiredBoolean(value, field);
}

function requiredPermission(value: unknown): 'manual' | 'auto' | 'yolo' {
  if (value === 'manual' || value === 'auto' || value === 'yolo') return value;
  throw new TransportError('permission must be manual, auto, or yolo.', value);
}

function optionalObject(value: unknown, field: string): JsonObject | undefined {
  if (value === undefined) return undefined;
  if (!isJsonObject(value)) throw new TransportError(`${field} must be an object.`, value);
  return value;
}

function isJsonObject(value: unknown): value is JsonObject {
  return isRecord(value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
