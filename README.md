# kimi-code-agent-sdk

Standalone multi-language SDK for controlling Kimi Code from applications.

This repository is intentionally based on the new `kimi-code` runtime, not the
legacy `kimi-cli` runtime used by `MoonshotAI/kimi-agent-sdk`.

Current status: v0 runtime protocol plus initial Node, Python, and Go clients.

v0 supports the core prompt loop:

```text
connect -> create/resume session -> prompt event stream -> close
```

Interactive approval/question handling, custom tools, plugin installation, and
advanced session serialization are intentionally deferred.

## Design

- [Protocol](spec/protocol.md)
- [Client API](spec/client-api.md)
- [Node client](node/)
- [Python client](python/)
- [Go client](go/)
- [Conformance scenarios](conformance/)

## Target Shape

```text
kimi-code-agent-sdk
  spec/       shared protocol and conformance contract
  node/       TypeScript client
  python/     Python client
  go/         Go client
  examples/   examples for all SDKs
```

All clients will talk to a local Kimi Code SDK server over stdio:

```bash
kimi-code sdk-server --stdio
```

The SDK clients do not embed the TypeScript runtime and do not depend on the
legacy Python `kimi-cli` package.

## Runtime Requirement

The clients expect a `kimi-code` executable that supports:

```bash
kimi-code sdk-server --stdio
```

During local development you can point a client at a built source runtime:

```bash
node /path/to/kimi-code/apps/kimi-code/dist/main.mjs sdk-server --stdio
```

## Quick Start

### Node

```ts
import { connect } from "@moonshot-ai/kimi-code-agent-sdk";

const client = await connect({ executable: "kimi-code" });
const session = await client.createSession({ workDir: process.cwd() });

for await (const event of session.prompt("Hello")) {
  if (event.type === "assistant.delta") {
    process.stdout.write(String(event.delta));
  }
}

await session.close();
await client.close();
```

### Python

```python
import asyncio

from kimi_code_agent_sdk import KimiCodeAgentClient


async def main() -> None:
    async with await KimiCodeAgentClient.connect() as client:
        async with await client.create_session(work_dir="/path/to/project") as session:
            async for event in session.prompt("Hello"):
                if event.type == "assistant.delta":
                    print(event.delta, end="")


asyncio.run(main())
```

### Go

```go
ctx := context.Background()
client, err := kimi.Connect(ctx)
if err != nil {
    panic(err)
}
defer client.Close(ctx)

session, err := client.CreateSession(ctx, kimi.WithWorkDir("/path/to/project"))
if err != nil {
    panic(err)
}
defer session.Close(ctx)

events, err := session.Prompt(ctx, "Hello")
if err != nil {
    panic(err)
}
for event := range events {
    if event.Type == "assistant.delta" {
        fmt.Print(event.Delta)
    }
}
```

## Validation

```bash
pnpm install
pnpm run typecheck
pnpm run test
pnpm run build

cd go
go test ./...
```

The root `pnpm` scripts cover Node and Python. Go is a separate module and is
validated with `go test ./...` from `go/`.

## v1 Follow-Ups

- Generate language-specific fake servers from `conformance/fixtures`.
- Add approval/question reverse-request handlers.
- Add richer transport diagnostics for subprocess stderr.
- Add Go stream cancellation helpers so consumers can stop before draining the
  event channel without relying only on context cancellation.
