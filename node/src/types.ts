export type JsonPrimitive = string | number | boolean | null;
export type JsonValue = JsonPrimitive | JsonValue[] | { readonly [key: string]: JsonValue };
export type JsonObject = { readonly [key: string]: JsonValue };

export type PermissionMode = 'manual' | 'auto' | 'yolo';

export type PromptInput = string | readonly PromptPart[];

export type PromptPart =
  | { readonly type: 'text'; readonly text: string }
  | { readonly type: 'image_url'; readonly image_url: { readonly url: string } }
  | { readonly type: 'video_url'; readonly video_url: { readonly url: string } };

export interface ClientOptions {
  readonly executable?: string;
  readonly args?: readonly string[];
  readonly cwd?: string;
  readonly env?: NodeJS.ProcessEnv;
  readonly clientName?: string;
  readonly clientVersion?: string;
}

export interface CreateSessionOptions {
  readonly id?: string;
  readonly workDir: string;
  readonly model?: string;
  readonly thinking?: string | boolean;
  readonly permission?: PermissionMode;
  readonly metadata?: JsonObject;
}

export interface ListSessionsOptions {
  readonly workDir?: string;
  readonly sessionId?: string;
}

export interface SessionSummary {
  readonly id: string;
  readonly title?: string;
  readonly lastPrompt?: string;
  readonly workDir: string;
  readonly sessionDir: string;
  readonly createdAt: number;
  readonly updatedAt: number;
  readonly archived?: boolean;
  readonly metadata?: JsonObject;
}

export interface SessionStatus {
  readonly model?: string;
  readonly thinkingLevel: string;
  readonly permission: PermissionMode;
  readonly planMode: boolean;
  readonly contextTokens: number;
  readonly maxContextTokens: number;
  readonly contextUsage: number;
  readonly usage?: unknown;
}

export type TurnEndReason = 'completed' | 'cancelled' | 'failed' | 'interrupted';

export interface Event {
  readonly type: string;
  readonly sessionId: string;
  readonly agentId: string;
  readonly turnId?: number;
  readonly [key: string]: unknown;
}

export interface AssistantDeltaEvent extends Event {
  readonly type: 'assistant.delta';
  readonly turnId: number;
  readonly delta: string;
}

export interface TurnEndedEvent extends Event {
  readonly type: 'turn.ended';
  readonly turnId: number;
  readonly reason: TurnEndReason;
  readonly error?: unknown;
}
