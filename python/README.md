# kimi-code-agent-sdk for Python

Python client for the `kimi-code sdk-server --stdio` protocol.

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

The client requires a `kimi-code` executable that supports:

```bash
kimi-code sdk-server --stdio
```

