# kimi-code-agent-sdk

Standalone multi-language SDK for controlling Kimi Code from applications.

This repository is intentionally based on the new `kimi-code` runtime, not the
legacy `kimi-cli` runtime used by `MoonshotAI/kimi-agent-sdk`.

Current status: protocol and client API design.

## Design

- [Protocol](spec/protocol.md)
- [Client API](spec/client-api.md)

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

