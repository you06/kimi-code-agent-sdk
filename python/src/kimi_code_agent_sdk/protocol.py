from __future__ import annotations

import asyncio
import json
from collections.abc import Awaitable, Callable
from typing import Any

from .errors import ProtocolError, TransportError

JsonObject = dict[str, Any]
NotificationListener = Callable[[str, Any], None]


class JsonRpcStdioClient:
    def __init__(self, process: asyncio.subprocess.Process) -> None:
        if process.stdin is None or process.stdout is None:
            raise TransportError("SDK server subprocess must expose stdin and stdout.")
        self._process = process
        self._stdin = process.stdin
        self._stdout = process.stdout
        self._next_id = 1
        self._closed = False
        self._pending: dict[str, asyncio.Future[Any]] = {}
        self._notification_listeners: set[NotificationListener] = set()
        self._reader_task = asyncio.create_task(self._read_stdout())
        self._stderr_task = asyncio.create_task(self._drain_stderr())

    @classmethod
    async def connect(
        cls,
        executable: str,
        args: list[str],
        *,
        cwd: str | None = None,
        env: dict[str, str] | None = None,
    ) -> "JsonRpcStdioClient":
        process = await asyncio.create_subprocess_exec(
            executable,
            *args,
            cwd=cwd,
            env=env,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        return cls(process)

    def on_notification(self, listener: NotificationListener) -> Callable[[], None]:
        self._notification_listeners.add(listener)

        def unsubscribe() -> None:
            self._notification_listeners.discard(listener)

        return unsubscribe

    async def request(self, method: str, params: Any) -> Any:
        if self._closed:
            raise TransportError("Kimi Code SDK server is closed.")
        request_id = str(self._next_id)
        self._next_id += 1
        loop = asyncio.get_running_loop()
        future: asyncio.Future[Any] = loop.create_future()
        self._pending[request_id] = future
        line = json.dumps(
            {"jsonrpc": "2.0", "id": request_id, "method": method, "params": params},
            separators=(",", ":"),
        )
        self._stdin.write(f"{line}\n".encode())
        await self._stdin.drain()
        return await future

    async def close(self) -> None:
        if self._closed:
            return
        self._closed = True
        self._reject_pending(TransportError("Kimi Code SDK server is closed."))
        self._stdin.close()
        try:
            await self._stdin.wait_closed()
        except (BrokenPipeError, ConnectionResetError):
            pass
        if self._process.returncode is None:
            self._process.terminate()
            try:
                await asyncio.wait_for(self._process.wait(), timeout=2)
            except asyncio.TimeoutError:
                self._process.kill()
                await self._process.wait()
        await _cancel_and_wait(self._reader_task)
        await _cancel_and_wait(self._stderr_task)

    async def _read_stdout(self) -> None:
        try:
            while not self._closed:
                raw_line = await self._stdout.readline()
                if raw_line == b"":
                    break
                self._handle_line(raw_line.decode().rstrip("\n"))
        except Exception as error:
            self._close_with_error(TransportError(str(error), details=error))
            return
        if not self._closed:
            self._close_with_error(
                TransportError(
                    "Kimi Code SDK server exited.",
                    details={"returncode": self._process.returncode},
                )
            )

    async def _drain_stderr(self) -> None:
        stderr = self._process.stderr
        if stderr is None:
            return
        while await stderr.readline():
            pass

    def _handle_line(self, line: str) -> None:
        try:
            message = json.loads(line)
        except json.JSONDecodeError as error:
            self._close_with_error(
                ProtocolError("Malformed JSON-RPC response.", code="INVALID_REQUEST", details=error)
            )
            return
        if not isinstance(message, dict):
            self._close_with_error(
                ProtocolError("JSON-RPC message must be an object.", code="INVALID_REQUEST")
            )
            return
        if isinstance(message.get("method"), str) and "id" not in message:
            self._handle_notification(message)
            return
        request_id = message.get("id")
        if not isinstance(request_id, str):
            return
        if "error" in message:
            self._handle_error_response(request_id, message.get("error"))
            return
        if "result" in message:
            self._handle_result_response(request_id, message.get("result"))

    def _handle_notification(self, message: JsonObject) -> None:
        method = message["method"]
        params = message.get("params")
        for listener in tuple(self._notification_listeners):
            listener(method, params)

    def _handle_result_response(self, request_id: str, result: Any) -> None:
        future = self._pending.pop(request_id, None)
        if future is not None and not future.done():
            future.set_result(result)

    def _handle_error_response(self, request_id: str, error: Any) -> None:
        future = self._pending.pop(request_id, None)
        if future is None or future.done():
            return
        if isinstance(error, dict):
            data = error.get("data")
            retryable = bool(data.get("retryable")) if isinstance(data, dict) else False
            details = data.get("details") if isinstance(data, dict) else None
            future.set_exception(
                ProtocolError(
                    str(error.get("message", "Protocol error.")),
                    code=str(error.get("code", "SERVER_ERROR")),
                    retryable=retryable,
                    details=details,
                )
            )
            return
        future.set_exception(ProtocolError("Protocol error.", code="SERVER_ERROR"))

    def _close_with_error(self, error: Exception) -> None:
        if self._closed:
            return
        self._closed = True
        self._reject_pending(error)

    def _reject_pending(self, error: Exception) -> None:
        for future in self._pending.values():
            if not future.done():
                future.set_exception(error)
        self._pending.clear()


async def _cancel_and_wait(task: Awaitable[Any]) -> None:
    if not isinstance(task, asyncio.Task) or task.done():
        return
    task.cancel()
    try:
        await task
    except asyncio.CancelledError:
        pass
