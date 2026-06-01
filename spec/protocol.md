# Kimi Code Agent SDK Protocol

This document defines the v1 wire protocol used by `kimi-code-agent-sdk`
clients to control a local Kimi Code runtime.

The protocol is designed for standalone Node, Python, and Go SDKs. Every
language client talks to the same runtime entrypoint and must pass the same
conformance fixtures.

## Goals

- Provide a language-neutral SDK boundary for Kimi Code.
- Reuse the user's local Kimi Code configuration, auth, sessions, MCP, and
  profile state under `~/.kimi-code`.
- Keep runtime behavior in `kimi-code`; SDK clients are thin process and
  protocol adapters.
- Make turn streaming, approval, questions, cancellation, and errors explicit.

## Non-Goals

- Compatibility with the legacy `kimi-cli --wire` protocol.
- In-process embedding of the TypeScript runtime by Python or Go.
- Client-side session serialization or context persistence.
- SDK-managed login UI, OAuth browser flows, MCP auth, plugin install, or
  config mutation.
- External tool registration in v1.
- Remote runtime transport. v1 is local process over stdio only.

## Runtime Entrypoint

Kimi Code must expose a stable headless SDK server:

```bash
kimi-code sdk-server --stdio
```

An alias such as `kimi --wire` may exist for ergonomics, but the canonical
documentation should use `sdk-server --stdio` to avoid confusion with the
legacy `kimi-cli --wire` protocol.

The server is responsible for creating and owning the Kimi Code runtime
(`KimiHarness` / `agent-core`). SDK clients only spawn the process and speak
this protocol.

## Transport

Transport is JSON-RPC 2.0 over line-delimited stdio.

Each message is one JSON object followed by `\n`.

This protocol intentionally uses string error codes in the JSON-RPC
`error.code` field (for example, `"SESSION_NOT_FOUND"`). Strict JSON-RPC 2.0
uses integer error codes; SDK clients for this protocol must accept the string
codes defined in this document.

Client request:

```json
{"jsonrpc":"2.0","id":"1","method":"initialize","params":{"supportedVersions":["1.0"],"client":{"name":"example","version":"0.1.0"}}}
```

Server response:

```json
{"jsonrpc":"2.0","id":"1","result":{"protocolVersion":"1.0","supportedVersions":["1.0"],"server":{"name":"kimi-code","version":"0.6.0"},"capabilities":{}}}
```

Server event notification:

```json
{"jsonrpc":"2.0","method":"event","params":{"sessionId":"ses_...","agentId":"main","type":"assistant.delta","turnId":1,"delta":"Hello"}}
```

Server reverse request:

```json
{"jsonrpc":"2.0","id":"srv_1","method":"approval/request","params":{"sessionId":"ses_...","turnId":1,"toolCallId":"tool_1","toolName":"Bash","action":"execute","display":{"kind":"shell","command":"rm -rf build"}}}
```

Client reverse response:

```json
{"jsonrpc":"2.0","id":"srv_1","result":{"decision":"approved"}}
```

## Version Negotiation

The first client request must be `initialize`.

Request:

```ts
interface InitializeRequest {
  supportedVersions: string[];
  client?: {
    name?: string;
    version?: string;
  };
}
```

Response:

```ts
interface InitializeResult {
  protocolVersion: string;
  supportedVersions: string[];
  server: {
    name: "kimi-code";
    version: string;
  };
  capabilities: Record<string, unknown>;
}
```

Rules:

- The server chooses the highest mutually supported version.
- v1 clients must support `"1.0"`.
- If there is no overlap, the server returns
  `UNSUPPORTED_PROTOCOL_VERSION`.
- Clients must not send any non-`initialize` request before initialization.

## Method Registry

### `createSession`

Create a new session.

```ts
interface CreateSessionRequest {
  id?: string;
  workDir: string;
  model?: string;
  thinking?: string | boolean;
  permission?: "manual" | "auto" | "yolo";
  metadata?: Record<string, unknown>;
}

interface SessionSummary {
  id: string;
  title?: string;
  lastPrompt?: string;
  workDir: string;
  sessionDir: string;
  createdAt: number;
  updatedAt: number;
  archived?: boolean;
  metadata?: Record<string, unknown>;
}
```

`workDir` is interpreted by the server process. Relative paths are resolved by
the server.

### `resumeSession`

Resume an existing session.

```ts
interface ResumeSessionRequest {
  id: string;
}

type ResumeSessionResult = SessionSummary & {
  // Server may include additional resume state in future versions.
  warning?: string;
};
```

### `closeSession`

Close a session.

```ts
interface CloseSessionRequest {
  sessionId: string;
}

type CloseSessionResult = Record<string, never>;
```

After `closeSession`, future calls for the same session must return
`SESSION_NOT_FOUND` or `SESSION_CLOSED`.

### `prompt`

Start a new turn.

```ts
interface PromptRequest {
  sessionId: string;
  input: PromptInput;
}

interface PromptResult {
  turnId: number;
}
```

Rules:

- `prompt` starts a new turn.
- A session may have at most one active turn.
- If a turn is already active, the server returns `TURN_ALREADY_ACTIVE`.
- The server returns `PromptResult` after the turn has been accepted. Turn
  output is delivered through `event` notifications.
- The terminal event for a turn is `turn.ended`.

### `steer`

Append user input to the current active turn.

```ts
interface SteerRequest {
  sessionId: string;
  input: PromptInput;
}

type SteerResult = Record<string, never>;
```

Rules:

- `steer` does not start a new turn.
- If no active turn exists, the server returns `TURN_NOT_ACTIVE`.
- Steered input is ordered after all already-accepted input for the active turn.

### `cancel`

Request cancellation of a turn.

```ts
interface CancelRequest {
  sessionId: string;
  turnId?: number;
}

type CancelResult = Record<string, never>;
```

Rules:

- `cancel` is best effort. Success means the signal was delivered, not that the
  turn has already stopped.
- The final state is reported by `turn.ended.reason`.
- If `turnId` is omitted, the server cancels the active turn for the session.

### `getStatus`

Return session status.

```ts
interface GetStatusRequest {
  sessionId: string;
}

interface SessionStatus {
  model?: string;
  thinkingLevel: string;
  permission: "manual" | "auto" | "yolo";
  planMode: boolean;
  contextTokens: number;
  maxContextTokens: number;
  contextUsage: number;
  usage?: unknown;
}
```

### `listSessions`

List sessions.

```ts
interface ListSessionsRequest {
  workDir?: string;
  sessionId?: string;
}

type ListSessionsResult = SessionSummary[];
```

### `shutdown`

Request graceful server shutdown.

```ts
type ShutdownRequest = Record<string, never>;
type ShutdownResult = Record<string, never>;
```

The server should close all active sessions, flush logs, and exit.

## Prompt Input

```ts
type PromptInput = string | PromptPart[];

type PromptPart =
  | { type: "text"; text: string }
  | { type: "image_url"; image_url: { url: string } }
  | { type: "video_url"; video_url: { url: string } };
```

v1 media support is URL-only. `file://` URLs assume the server process can
access the same filesystem path. Remote runtimes, containers, and multi-host
file transfer are not covered by v1.

Inline base64 media is intentionally out of scope for v1.

## Event Envelope

Every server event notification uses:

```ts
interface EventEnvelope {
  sessionId: string;
  agentId: string; // v1 usually "main"
  type: string;
  turnId?: number;
}
```

The server adapter must attach `sessionId` and `agentId`, even if the internal
agent-core event did not include them.

## Core Events

The following event names are part of v1:

```ts
type CoreEvent =
  | TurnStartedEvent
  | AssistantDeltaEvent
  | ThinkingDeltaEvent
  | ToolCallDeltaEvent
  | ToolCallStartedEvent
  | ToolProgressEvent
  | ToolResultEvent
  | AgentStatusUpdatedEvent
  | TurnStepStartedEvent
  | TurnStepCompletedEvent
  | TurnStepRetryingEvent
  | TurnStepInterruptedEvent
  | TurnEndedEvent
  | ErrorEvent
  | WarningEvent;
```

Minimum payloads:

```ts
interface TurnStartedEvent extends EventEnvelope {
  type: "turn.started";
  turnId: number;
  origin?: string;
}

interface AssistantDeltaEvent extends EventEnvelope {
  type: "assistant.delta";
  turnId: number;
  delta: string;
}

interface ThinkingDeltaEvent extends EventEnvelope {
  type: "thinking.delta";
  turnId: number;
  delta: string;
}

interface ToolCallDeltaEvent extends EventEnvelope {
  type: "tool.call.delta";
  turnId: number;
  toolCallId: string;
  name?: string;
  argumentsPart?: string;
}

interface ToolCallStartedEvent extends EventEnvelope {
  type: "tool.call.started";
  turnId: number;
  toolCallId: string;
  name: string;
  args: unknown;
  description?: string;
  display?: unknown;
}

interface ToolProgressEvent extends EventEnvelope {
  type: "tool.progress";
  turnId: number;
  toolCallId: string;
  update: unknown;
}

interface ToolResultEvent extends EventEnvelope {
  type: "tool.result";
  turnId: number;
  toolCallId: string;
  output: unknown;
  isError?: boolean;
  synthetic?: boolean;
}

interface TurnEndedEvent extends EventEnvelope {
  type: "turn.ended";
  turnId: number;
  reason: "completed" | "cancelled" | "failed" | "interrupted";
  error?: ProtocolErrorPayload;
}

interface ErrorEvent extends EventEnvelope {
  type: "error";
  code: string;
  message: string;
  retryable?: boolean;
  details?: unknown;
}

interface WarningEvent extends EventEnvelope {
  type: "warning";
  message: string;
  code?: string;
}
```

The server may include additional fields copied from agent-core. Clients must
preserve unknown fields in raw event access, but typed convenience models may
ignore them.

## Optional Events

The following are optional in v1 and may be treated as unknown events by simple
clients:

- `session.meta.updated`
- `skill.activated`
- `mcp.server.status`
- `tool.list.updated`
- `subagent.spawned`
- `subagent.completed`
- `subagent.failed`
- `compaction.started`
- `compaction.blocked`
- `compaction.cancelled`
- `compaction.completed`
- `background.task.started`
- `background.task.updated`
- `background.task.terminated`

Clients must not fail when receiving an unknown event type.

## Reverse Requests

### `approval/request`

Server request:

```ts
interface ApprovalRequest {
  sessionId: string;
  turnId?: number;
  toolCallId: string;
  toolName: string;
  action: string;
  display: unknown;
}
```

Client response:

```ts
interface ApprovalResponse {
  decision: "approved" | "rejected" | "cancelled";
  scope?: "session";
  feedback?: string;
  selectedLabel?: string;
}
```

### `question/request`

Server request:

```ts
interface QuestionRequest {
  sessionId: string;
  turnId?: number;
  toolCallId?: string;
  questions: QuestionItem[];
}
```

Client response:

```ts
type QuestionResult = null | Record<string, string | true> | {
  answers: Record<string, string | true>;
  method?: "enter" | "space" | "number_key";
};
```

If a client does not install a handler, it should return a cancellation-style
response rather than hanging indefinitely.

## Ordering and Lifecycle Invariants

1. `initialize` must happen before all other requests.
2. A session may have at most one active turn.
3. `prompt` starts a new turn; `steer` appends to an active turn.
4. Events for the same `sessionId` and `turnId` must be emitted in server
   order.
5. `turn.ended` is terminal. No subsequent event for that `turnId` may contain
   assistant deltas, tool results, or step progress.
6. Breaking out of a client-side event stream does not cancel the server turn.
   Users must explicitly call `cancel()` or use a language-specific abort
   helper that calls `cancel()`.
7. `cancel()` is best-effort; terminal status is determined by `turn.ended`.
8. A server crash, SIGKILL, or stdio EOF must be surfaced by clients as a
   transport error. Active sessions on that client become unusable.

## Error Shape

JSON-RPC errors use:

```ts
interface ProtocolErrorPayload {
  code: ErrorCode;
  message: string;
  retryable?: boolean;
  details?: unknown;
}
```

JSON-RPC error object:

```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "error": {
    "code": "SESSION_NOT_FOUND",
    "message": "Session not found",
    "data": { "retryable": false }
  }
}
```

v1 error code registry:

| Code | Meaning |
|---|---|
| `INVALID_REQUEST` | Malformed JSON-RPC message or missing required field |
| `INVALID_INPUT` | Request shape is valid JSON-RPC but semantically invalid |
| `UNSUPPORTED_PROTOCOL_VERSION` | No compatible protocol version |
| `SESSION_NOT_FOUND` | Session id is unknown or closed |
| `SESSION_CLOSED` | Session is known but no longer usable |
| `TURN_NOT_FOUND` | Turn id is unknown |
| `TURN_NOT_ACTIVE` | Request requires an active turn but none exists |
| `TURN_ALREADY_ACTIVE` | `prompt` called while another turn is active |
| `AUTH_REQUIRED` | Kimi Code runtime requires login/auth |
| `CONFIG_ERROR` | Kimi Code config is missing or invalid |
| `CANCELLED` | Operation was cancelled |
| `SERVER_ERROR` | Unclassified server-side failure |

SDK languages must expose these as idiomatic errors while preserving the
original code.

## Conformance

The standalone SDK repo must include conformance fixtures before or alongside
client implementations.

Minimum v0 fixtures:

- initialize success and unsupported version failure
- create session success
- prompt emits `turn.started`, `assistant.delta`, `turn.ended`
- close session success
- server crash / stdio EOF maps to transport error

Minimum v1 fixtures:

- approval request/response round trip
- question request/response round trip
- cancel active turn
- steer active turn
- resume session
- `TURN_ALREADY_ACTIVE` on concurrent prompt
- `TURN_NOT_ACTIVE` on steer without active turn

Node, Python, and Go clients must pass the same fixture set.
