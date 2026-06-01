# @moonshot-ai/kimi-code-agent-sdk

Initial Node client for the Kimi Code Agent SDK stdio protocol.

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

The client spawns `kimi-code sdk-server --stdio` by default. Pass
`executable` and `args` to point at a local development build.
