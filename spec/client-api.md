# Kimi Code Agent SDK Client API

This document defines the v1 public client API shape for the standalone
`kimi-code-agent-sdk` repository.

The SDK has Node, Python, and Go implementations. All implementations use the
same protocol defined in `spec/protocol.md`.

## Overview

Each client:

1. Spawns `kimi-code sdk-server --stdio` or a user-provided executable.
2. Negotiates protocol version with `initialize`.
3. Wraps protocol methods and events in idiomatic language APIs.
4. Maps protocol errors into language-specific error types.

Clients do not import `@moonshot-ai/agent-core`, `@moonshot-ai/kimi-code-sdk`,
or any `kimi-code` internal package. The runtime is always the spawned Kimi
Code SDK server.

## Cross-Language Shape

All languages expose the same flow:

```text
connect client
  -> create or resume session
  -> prompt returns event stream
  -> close session
  -> close client
```

The stream abstraction is language-specific:

- Node: async iterable
- Python: async generator
- Go: receive-only channel

`PromptInput` is defined by `spec/protocol.md`. In v1 it is either a string or
an ordered array of text/image/video URL parts.

## Node API

The main constructor is `connect`. `createClient` may exist as an alias.

```ts
import { connect } from "@moonshot-ai/kimi-code-agent-sdk";

const client = await connect({
  executable: "kimi-code",
  cwd: process.cwd(),
  env: process.env,
});

const session = await client.createSession({
  workDir: "/path/to/project",
  model: "kimi-latest",
  thinking: false,
  permission: "manual",
});

for await (const event of session.prompt("Hello")) {
  switch (event.type) {
    case "assistant.delta":
      process.stdout.write(event.delta);
      break;
    case "turn.ended":
      console.log("done:", event.reason);
      break;
  }
}

await session.close();
await client.close();
```

v0 API:

```ts
function connect(options?: ClientOptions): Promise<Client>;
function createClient(options?: ClientOptions): Promise<Client>;

interface Client {
  createSession(options: CreateSessionOptions): Promise<Session>;
  close(): Promise<void>;
}

interface Session {
  readonly id: string;
  readonly workDir: string;
  prompt(input: PromptInput, options?: PromptOptions): AsyncIterable<Event>;
  close(): Promise<void>;
}
```

v1 additions:

```ts
interface Client {
  resumeSession(options: { id: string }): Promise<Session>;
  listSessions(options?: ListSessionsOptions): Promise<readonly SessionSummary[]>;
}

interface Session {
  cancel(options?: { turnId?: number }): Promise<void>;
  steer(input: PromptInput): Promise<void>;
  getStatus(): Promise<SessionStatus>;
  setApprovalHandler(handler: ApprovalHandler | undefined): void;
  setQuestionHandler(handler: QuestionHandler | undefined): void;
}
```

## Python API

```python
from kimi_code_agent_sdk import KimiCodeAgentClient

async with KimiCodeAgentClient.connect(executable="kimi-code") as client:
    async with await client.create_session(
        work_dir="/path/to/project",
        model="kimi-latest",
        thinking=False,
        permission="manual",
    ) as session:
        async for event in session.prompt("Hello"):
            if event.type == "assistant.delta":
                print(event.delta, end="")
            elif event.type == "turn.ended":
                print(f"\ndone: {event.reason}")
```

v0 API:

```python
class KimiCodeAgentClient:
    @classmethod
    async def connect(
        cls,
        executable: str | None = None,
        cwd: str | None = None,
        env: dict[str, str] | None = None,
    ) -> "KimiCodeAgentClient": ...

    async def create_session(
        self,
        *,
        work_dir: str,
        model: str | None = None,
        thinking: str | bool | None = None,
        permission: str | None = None,
        metadata: dict[str, object] | None = None,
    ) -> "Session": ...

    async def close(self) -> None: ...

class Session:
    @property
    def id(self) -> str: ...

    @property
    def work_dir(self) -> str: ...

    def prompt(self, input: PromptInput) -> AsyncIterator[Event]: ...
    async def close(self) -> None: ...
```

v1 additions:

```python
class KimiCodeAgentClient:
    async def resume_session(self, id: str) -> "Session": ...
    async def list_sessions(self, work_dir: str | None = None) -> list[SessionSummary]: ...

class Session:
    async def cancel(self, turn_id: int | None = None) -> None: ...
    async def steer(self, input: PromptInput) -> None: ...
    async def get_status(self) -> SessionStatus: ...
    def set_approval_handler(self, handler: ApprovalHandler | None) -> None: ...
    def set_question_handler(self, handler: QuestionHandler | None) -> None: ...
```

## Go API

```go
import kimi "github.com/you06/kimi-code-agent-sdk/go"

client, err := kimi.Connect(ctx, kimi.WithExecutable("kimi-code"))
if err != nil {
    return err
}
defer client.Close()

session, err := client.CreateSession(ctx,
    kimi.WithWorkDir("/path/to/project"),
    kimi.WithModel("kimi-latest"),
    kimi.WithPermission(kimi.PermissionManual),
)
if err != nil {
    return err
}
defer session.Close()

events, err := session.Prompt(ctx, kimi.Text("Hello"))
if err != nil {
    return err
}
for ev := range events {
    switch ev.Type {
    case kimi.EventAssistantDelta:
        fmt.Print(ev.AssistantDelta().Delta)
    case kimi.EventTurnEnded:
        fmt.Printf("\ndone: %s\n", ev.TurnEnded().Reason)
    }
}
```

v0 API:

```go
func Connect(ctx context.Context, opts ...ClientOption) (Client, error)

type Client interface {
    CreateSession(ctx context.Context, opts ...SessionOption) (Session, error)
    Close() error
}

type Session interface {
    ID() string
    WorkDir() string
    Prompt(ctx context.Context, input PromptInput) (<-chan Event, error)
    Close() error
}
```

v1 additions:

```go
type Client interface {
    ResumeSession(ctx context.Context, id string) (Session, error)
    ListSessions(ctx context.Context, opts ...ListSessionsOption) ([]SessionSummary, error)
}

type Session interface {
    Cancel(ctx context.Context, opts ...CancelOption) error
    Steer(ctx context.Context, input PromptInput) error
    Status(ctx context.Context) (SessionStatus, error)
    SetApprovalHandler(ApprovalHandler)
    SetQuestionHandler(QuestionHandler)
}
```

## API to Protocol Mapping

| Client API | Protocol method | Phase |
|---|---|---|
| `connect` / `KimiCodeAgentClient.connect` / `Connect` | spawn + `initialize` | v0 |
| `createSession` / `create_session` / `CreateSession` | `createSession` | v0 |
| `Session.prompt` / `prompt` / `Prompt` | `prompt` + event stream | v0 |
| `Session.close` / `close` / `Close` | `closeSession` | v0 |
| `Client.close` / `close` / `Close` | `shutdown` + process exit | v0 |
| `resumeSession` / `resume_session` / `ResumeSession` | `resumeSession` | v1 |
| `listSessions` / `list_sessions` / `ListSessions` | `listSessions` | v1 |
| `Session.cancel` / `cancel` / `Cancel` | `cancel` | v1 |
| `Session.steer` / `steer` / `Steer` | `steer` | v1 |
| approval handler | reverse request `approval/request` | v1 |
| question handler | reverse request `question/request` | v1 |
| `getStatus` / `get_status` / `Status` | `getStatus` | v1 |

## Event Types

Event names match the protocol exactly. Clients must not rename events.
Canonical payload fields are also defined by `spec/protocol.md`; for example,
`assistant.delta` uses the field name `delta`, not `text`.

Node exports a discriminated union:

```ts
type Event =
  | { type: "assistant.delta"; delta: string; ... }
  | { type: "turn.ended"; reason: "completed" | "cancelled" | "failed" | "interrupted"; ... }
  | ...
```

Python should expose typed event models with the `type` field as discriminator.

Go should expose concrete structs or a tagged event wrapper. The exact Go
implementation may choose ergonomics, but the wire `type` value must remain
available.

Unknown events must not crash clients. They should be surfaced as raw/unknown
events or ignored only by explicit user choice.

## Error Handling

All languages preserve protocol error codes from `spec/protocol.md`.

Node:

```ts
class KimiCodeAgentError extends Error {
  code: string;
  retryable: boolean;
  details?: unknown;
}

class TransportError extends KimiCodeAgentError {}
class ProtocolError extends KimiCodeAgentError {}
class SessionError extends KimiCodeAgentError {}
```

Python:

```python
class KimiCodeAgentError(Exception):
    code: str
    retryable: bool
    details: object | None

class TransportError(KimiCodeAgentError): ...
class ProtocolError(KimiCodeAgentError): ...
class SessionError(KimiCodeAgentError): ...
```

Go:

```go
type Error struct {
    Code      string
    Message   string
    Retryable bool
    Details   any
}

func (e *Error) Error() string
```

Go may expose sentinel errors or helper predicates, but code-preserving errors
are the compatibility requirement.

## Behavioral Rules

- `prompt()` starts a new turn.
- `steer()` appends to the current active turn.
- One session may have only one active turn.
- Breaking out of a prompt event stream does not cancel the turn.
- If the SDK offers abort helpers, they must call protocol `cancel`.
- The server process crashing or closing stdio must make the client raise a
  transport error and mark active sessions unusable.
- A prompt stream ending does not close the session.
- Multiple sessions per client are allowed, but v1 clients do not need to offer
  a mutable session registry.
- Client-side session serialization is out of scope. Persist only session IDs
  if the application wants to resume later.

## Non-Goals and Legacy Surfaces Not Inherited

The new SDK replaces the old `kimi-agent-sdk`, but it does not copy every
legacy surface.

| Legacy surface | Reason not inherited in v1 |
|---|---|
| `createExternalTool`, `external-tool.ts`, or Go/Python external tool helpers | External tools are deferred until protocol v2. |
| Raw `ProtocolClient`, `wire/jsonrpc2`, transport internals | Users should use idiomatic clients, not raw wire. |
| `kimi_cli.*` re-exports (`Config`, `MCPConfig`, `soul.*`) | Old runtime internals; protocol is self-contained. |
| `kosong.*` provider re-exports | Provider errors and messages are normalized by the server. |
| `authMCP`, `testMCP`, `resetAuthMCP` | MCP management is host/runtime responsibility in v1. |
| `login`, `logout`, `isLoggedIn` | v1 reuses existing `~/.kimi-code` auth/config. |
| `parseConfig`, `saveDefaultModel`, config mutation helpers | Config mutation is deferred. |
| `parseSessionEvents` and history mining helpers | History/replay/export are deferred. |
| `KimiPaths` and path utilities | Not needed by SDK users. |
| Logging sink APIs | Each language should use idiomatic logging. |
| Client-side session serialization/export | Server owns session state; client only holds IDs. |
| Plugin install/uninstall/enablement | Deferred until a later protocol version. |

## Milestones

### v0: Core Smoke

- Runtime entrypoint starts and initializes.
- Node, Python, and Go can create a session.
- A prompt returns an event stream with `turn.started`, `assistant.delta`, and
  `turn.ended`.
- Sessions and client close cleanly.
- Server crash / stdio EOF maps to transport error in all languages.
- All clients pass the same conformance fixtures.

### v1: GA

- Resume session.
- Approval reverse requests.
- Question reverse requests.
- Cancel active turn.
- Steer active turn.
- Get session status.
- Error code mapping in all languages.
- Unknown events handled safely.
