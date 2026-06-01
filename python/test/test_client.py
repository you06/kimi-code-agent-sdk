from __future__ import annotations

import asyncio
import json
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path

from kimi_code_agent_sdk import KimiCodeAgentClient, ProtocolError


class PythonClientTest(unittest.IsolatedAsyncioTestCase):
    async def test_creates_session_and_streams_prompt_events(self) -> None:
        server = await write_fake_server()
        client = await KimiCodeAgentClient.connect(
            executable=sys.executable,
            args=[str(server)],
            client_name="python-client-test",
        )
        try:
            session = await client.create_session(
                id="ses_python_test",
                work_dir="/tmp/project",
                thinking=False,
                permission="auto",
            )

            self.assertEqual(session.id, "ses_python_test")
            self.assertEqual(session.work_dir, "/tmp/project")

            events = [event async for event in session.prompt("hello")]

            self.assertEqual([event.type for event in events], ["turn.started", "assistant.delta", "turn.ended"])
            self.assertEqual(events[1].delta, "hello from fake server")
            self.assertEqual(events[2].reason, "completed")
        finally:
            await client.close()

    async def test_maps_protocol_errors(self) -> None:
        server = await write_fake_server()
        client = await KimiCodeAgentClient.connect(executable=sys.executable, args=[str(server)])
        try:
            with self.assertRaises(ProtocolError) as raised:
                await client.create_session(work_dir="/tmp/missing")
            self.assertEqual(raised.exception.code, "INVALID_INPUT")
        finally:
            await client.close()


async def write_fake_server() -> Path:
    directory = Path(tempfile.mkdtemp(prefix="kimi-code-agent-sdk-python-"))
    file = directory / "fake_server.py"
    file.write_text(
        textwrap.dedent(
            r'''
            import json
            import sys


            def write(message):
                sys.stdout.write(json.dumps(message, separators=(",", ":")) + "\n")
                sys.stdout.flush()


            for line in sys.stdin:
                if not line.strip():
                    continue
                request = json.loads(line)
                if request["method"] == "initialize":
                    write({
                        "jsonrpc": "2.0",
                        "id": request["id"],
                        "result": {
                            "protocolVersion": "1.0",
                            "supportedVersions": ["1.0"],
                            "server": {"name": "kimi-code", "version": "0.0.0-test"},
                            "capabilities": {},
                        },
                    })
                    continue
                if request["method"] == "createSession":
                    if request["params"].get("id") != "ses_python_test":
                        write({
                            "jsonrpc": "2.0",
                            "id": request["id"],
                            "error": {
                                "code": "INVALID_INPUT",
                                "message": "id must be ses_python_test",
                                "data": {"retryable": False},
                            },
                        })
                        continue
                    write({
                        "jsonrpc": "2.0",
                        "id": request["id"],
                        "result": {
                            "id": "ses_python_test",
                            "workDir": request["params"]["workDir"],
                            "sessionDir": "/tmp/session",
                            "createdAt": 1,
                            "updatedAt": 2,
                        },
                    })
                    continue
                if request["method"] == "prompt":
                    write({
                        "jsonrpc": "2.0",
                        "method": "event",
                        "params": {
                            "type": "turn.started",
                            "sessionId": request["params"]["sessionId"],
                            "agentId": "main",
                            "turnId": 0,
                        },
                    })
                    write({
                        "jsonrpc": "2.0",
                        "method": "event",
                        "params": {
                            "type": "assistant.delta",
                            "sessionId": request["params"]["sessionId"],
                            "agentId": "main",
                            "turnId": 0,
                            "delta": "hello from fake server",
                        },
                    })
                    write({
                        "jsonrpc": "2.0",
                        "method": "event",
                        "params": {
                            "type": "turn.ended",
                            "sessionId": request["params"]["sessionId"],
                            "agentId": "main",
                            "turnId": 0,
                            "reason": "completed",
                        },
                    })
                    write({"jsonrpc": "2.0", "id": request["id"], "result": {"turnId": 0}})
                    continue
                if request["method"] == "shutdown":
                    write({"jsonrpc": "2.0", "id": request["id"], "result": {}})
                    raise SystemExit(0)
            '''
        ),
        encoding="utf-8",
    )
    return file


if __name__ == "__main__":
    unittest.main()

