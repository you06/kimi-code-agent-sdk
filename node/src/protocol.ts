import { createInterface } from 'node:readline';
import type { ChildProcessWithoutNullStreams } from 'node:child_process';

import { ProtocolError, TransportError } from './errors.js';

type JsonRpcId = string;

interface JsonRpcSuccess {
  readonly jsonrpc: '2.0';
  readonly id: JsonRpcId;
  readonly result: unknown;
}

interface JsonRpcFailure {
  readonly jsonrpc: '2.0';
  readonly id: JsonRpcId | null;
  readonly error: {
    readonly code: string;
    readonly message: string;
    readonly data?: {
      readonly retryable?: boolean;
      readonly details?: unknown;
    };
  };
}

interface JsonRpcNotification {
  readonly jsonrpc: '2.0';
  readonly method: string;
  readonly params?: unknown;
}

type PendingRequest = {
  resolve(value: unknown): void;
  reject(error: Error): void;
};

export class JsonRpcStdioClient {
  private nextId = 1;
  private closed = false;
  private readonly pending = new Map<JsonRpcId, PendingRequest>();
  private readonly notificationListeners = new Set<(method: string, params: unknown) => void>();

  constructor(private readonly child: ChildProcessWithoutNullStreams) {
    const lines = createInterface({ input: child.stdout, crlfDelay: Infinity });
    lines.on('line', (line) => {
      this.handleLine(line);
    });
    child.once('error', (error) => {
      this.closeWithError(new TransportError(error.message, error));
    });
    child.once('exit', (code, signal) => {
      if (this.closed) return;
      this.closeWithError(
        new TransportError('Kimi Code SDK server exited.', {
          code,
          signal,
        }),
      );
    });
  }

  onNotification(listener: (method: string, params: unknown) => void): () => void {
    this.notificationListeners.add(listener);
    return () => {
      this.notificationListeners.delete(listener);
    };
  }

  request(method: string, params: unknown): Promise<unknown> {
    if (this.closed) {
      return Promise.reject(new TransportError('Kimi Code SDK server is closed.'));
    }
    const id = String(this.nextId);
    this.nextId += 1;
    const message = { jsonrpc: '2.0', id, method, params };
    const promise = new Promise<unknown>((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
    });
    this.child.stdin.write(`${JSON.stringify(message)}\n`);
    return promise;
  }

  async close(): Promise<void> {
    if (this.closed) return;
    this.closed = true;
    this.rejectPending(new TransportError('Kimi Code SDK server is closed.'));
    this.child.stdin.end();
    if (this.child.exitCode === null && this.child.signalCode === null) {
      this.child.kill();
    }
  }

  private handleLine(line: string): void {
    let message: unknown;
    try {
      message = JSON.parse(line) as unknown;
    } catch {
      this.closeWithError(new ProtocolError('Malformed JSON-RPC response.', { code: 'INVALID_REQUEST' }));
      return;
    }
    if (!isRecord(message)) {
      this.closeWithError(new ProtocolError('JSON-RPC message must be an object.', { code: 'INVALID_REQUEST' }));
      return;
    }
    if (typeof message['method'] === 'string' && message['id'] === undefined) {
      this.handleNotification(message as unknown as JsonRpcNotification);
      return;
    }
    if (typeof message['id'] !== 'string') return;
    if (isFailure(message)) {
      this.handleFailure(message);
      return;
    }
    if (isSuccess(message)) {
      this.handleSuccess(message);
    }
  }

  private handleNotification(message: JsonRpcNotification): void {
    for (const listener of this.notificationListeners) {
      listener(message.method, message.params);
    }
  }

  private handleSuccess(message: JsonRpcSuccess): void {
    const pending = this.pending.get(message.id);
    if (pending === undefined) return;
    this.pending.delete(message.id);
    pending.resolve(message.result);
  }

  private handleFailure(message: JsonRpcFailure): void {
    const pending = message.id === null ? undefined : this.pending.get(message.id);
    if (pending === undefined) return;
    this.pending.delete(message.id!);
    pending.reject(
      new ProtocolError(message.error.message, {
        code: message.error.code,
        retryable: message.error.data?.retryable,
        details: message.error.data?.details,
      }),
    );
  }

  private closeWithError(error: Error): void {
    if (this.closed) return;
    this.closed = true;
    this.rejectPending(error);
  }

  private rejectPending(error: Error): void {
    for (const pending of this.pending.values()) {
      pending.reject(error);
    }
    this.pending.clear();
  }
}

function isSuccess(value: unknown): value is JsonRpcSuccess {
  if (!isRecord(value)) return false;
  return value['jsonrpc'] === '2.0' && typeof value['id'] === 'string' && 'result' in value;
}

function isFailure(value: unknown): value is JsonRpcFailure {
  if (!isRecord(value)) return false;
  if (value['jsonrpc'] !== '2.0') return false;
  if (typeof value['id'] !== 'string' && value['id'] !== null) return false;
  const error = value['error'];
  if (!isRecord(error)) return false;
  return typeof error['code'] === 'string' && typeof error['message'] === 'string';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}
