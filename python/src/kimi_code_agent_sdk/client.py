from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator, Callable
from typing import Any

from .errors import TransportError
from .protocol import JsonRpcStdioClient
from .types import Event, JsonObject, PromptInput, SessionStatus, SessionSummary

PROTOCOL_VERSION = "1.0"


async def connect(
    *,
    executable: str = "kimi-code",
    args: list[str] | None = None,
    cwd: str | None = None,
    env: dict[str, str] | None = None,
    client_name: str = "kimi-code-agent-sdk-python",
    client_version: str | None = None,
) -> "KimiCodeAgentClient":
    return await KimiCodeAgentClient.connect(
        executable=executable,
        args=args,
        cwd=cwd,
        env=env,
        client_name=client_name,
        client_version=client_version,
    )


class KimiCodeAgentClient:
    def __init__(self, rpc: JsonRpcStdioClient) -> None:
        self._rpc = rpc
        self._sessions: dict[str, Session] = {}
        self._event_listeners: set[Callable[[Event], None]] = set()
        self._unsubscribe_notifications = rpc.on_notification(self._handle_notification)

    @classmethod
    async def connect(
        cls,
        *,
        executable: str = "kimi-code",
        args: list[str] | None = None,
        cwd: str | None = None,
        env: dict[str, str] | None = None,
        client_name: str = "kimi-code-agent-sdk-python",
        client_version: str | None = None,
    ) -> "KimiCodeAgentClient":
        rpc = await JsonRpcStdioClient.connect(
            executable,
            args or ["sdk-server", "--stdio"],
            cwd=cwd,
            env=env,
        )
        client = cls(rpc)
        await client.initialize(client_name=client_name, client_version=client_version)
        return client

    async def __aenter__(self) -> "KimiCodeAgentClient":
        return self

    async def __aexit__(self, *_exc: object) -> None:
        await self.close()

    async def initialize(self, *, client_name: str, client_version: str | None = None) -> None:
        await self._rpc.request(
            "initialize",
            {
                "supportedVersions": [PROTOCOL_VERSION],
                "client": {"name": client_name, "version": client_version},
            },
        )

    async def create_session(
        self,
        *,
        work_dir: str,
        id: str | None = None,
        model: str | None = None,
        thinking: str | bool | None = None,
        permission: str | None = None,
        metadata: JsonObject | None = None,
    ) -> "Session":
        summary = _session_summary(
            await self._rpc.request(
                "createSession",
                _omit_none({
                    "id": id,
                    "workDir": work_dir,
                    "model": model,
                    "thinking": thinking,
                    "permission": permission,
                    "metadata": metadata,
                }),
            )
        )
        return self._bind_session(summary)

    async def resume_session(self, id: str) -> "Session":
        summary = _session_summary(await self._rpc.request("resumeSession", {"id": id}))
        return self._bind_session(summary)

    async def list_sessions(
        self, *, work_dir: str | None = None, session_id: str | None = None
    ) -> list[SessionSummary]:
        result = await self._rpc.request(
            "listSessions",
            _omit_none({"workDir": work_dir, "sessionId": session_id}),
        )
        if not isinstance(result, list):
            raise TransportError("listSessions result must be an array.", details=result)
        return [_session_summary(item) for item in result]

    async def close(self) -> None:
        self._unsubscribe_notifications()
        try:
            await self._rpc.request("shutdown", {})
        except Exception:
            pass
        await self._rpc.close()

    def on_event(self, listener: Callable[[Event], None]) -> Callable[[], None]:
        self._event_listeners.add(listener)

        def unsubscribe() -> None:
            self._event_listeners.discard(listener)

        return unsubscribe

    async def close_session(self, session_id: str) -> None:
        await self._rpc.request("closeSession", {"sessionId": session_id})
        self._sessions.pop(session_id, None)

    async def prompt(self, session_id: str, input: PromptInput) -> AsyncIterator[Event]:
        async for event in self._stream_turn(session_id, "prompt", {"sessionId": session_id, "input": input}):
            yield event

    async def steer(self, session_id: str, input: PromptInput) -> None:
        await self._rpc.request("steer", {"sessionId": session_id, "input": input})

    async def cancel(self, session_id: str, turn_id: int | None = None) -> None:
        await self._rpc.request("cancel", _omit_none({"sessionId": session_id, "turnId": turn_id}))

    async def get_status(self, session_id: str) -> SessionStatus:
        return _session_status(await self._rpc.request("getStatus", {"sessionId": session_id}))

    def _bind_session(self, summary: SessionSummary) -> "Session":
        existing = self._sessions.get(summary.id)
        if existing is not None:
            return existing
        session = Session(self, summary)
        self._sessions[summary.id] = session
        return session

    def _handle_notification(self, method: str, params: Any) -> None:
        if method != "event" or not isinstance(params, dict):
            return
        if not isinstance(params.get("type"), str):
            return
        if not isinstance(params.get("sessionId"), str):
            return
        if not isinstance(params.get("agentId"), str):
            return
        event = Event(params)
        for listener in tuple(self._event_listeners):
            listener(event)

    async def _stream_turn(self, session_id: str, method: str, params: Any) -> AsyncIterator[Event]:
        queue: _EventQueue = _EventQueue()

        def listener(event: Event) -> None:
            if event.session_id == session_id:
                queue.push(event)

        unsubscribe = self.on_event(listener)
        try:
            result = await self._rpc.request(method, params)
            turn_id = _turn_id(result)
            while True:
                event = await queue.next()
                yield event
                if event.type == "turn.ended" and event.turn_id == turn_id:
                    return
        finally:
            unsubscribe()
            queue.close()


class Session:
    def __init__(self, client: KimiCodeAgentClient, summary: SessionSummary) -> None:
        self._client = client
        self.summary = summary
        self.id = summary.id
        self.work_dir = summary.work_dir

    async def __aenter__(self) -> "Session":
        return self

    async def __aexit__(self, *_exc: object) -> None:
        await self.close()

    async def prompt(self, input: PromptInput) -> AsyncIterator[Event]:
        async for event in self._client.prompt(self.id, input):
            yield event

    async def close(self) -> None:
        await self._client.close_session(self.id)

    async def steer(self, input: PromptInput) -> None:
        await self._client.steer(self.id, input)

    async def cancel(self, turn_id: int | None = None) -> None:
        await self._client.cancel(self.id, turn_id)

    async def get_status(self) -> SessionStatus:
        return await self._client.get_status(self.id)


class _EventQueue:
    def __init__(self) -> None:
        self._queue: list[Event] = []
        self._waiter: asyncio.Future[Event] | None = None
        self._closed = False

    def push(self, event: Event) -> None:
        if self._closed:
            return
        if self._waiter is not None:
            waiter = self._waiter
            self._waiter = None
            if not waiter.done():
                waiter.set_result(event)
            return
        self._queue.append(event)

    async def next(self) -> Event:
        if self._queue:
            return self._queue.pop(0)
        if self._closed:
            raise TransportError("Event stream closed.")
        loop = asyncio.get_running_loop()
        future: asyncio.Future[Event] = loop.create_future()
        self._waiter = future
        return await future

    def close(self) -> None:
        self._closed = True
        if self._waiter is not None and not self._waiter.done():
            self._waiter.set_exception(TransportError("Event stream closed."))
        self._waiter = None


def _turn_id(value: Any) -> int:
    if not isinstance(value, dict) or not isinstance(value.get("turnId"), int):
        raise TransportError("prompt result must include turnId.", details=value)
    return value["turnId"]


def _omit_none(value: dict[str, Any]) -> dict[str, Any]:
    return {key: item for key, item in value.items() if item is not None}


def _session_summary(value: Any) -> SessionSummary:
    if not isinstance(value, dict):
        raise TransportError("session summary must be an object.", details=value)
    return SessionSummary(
        id=_required_string(value, "id"),
        work_dir=_required_string(value, "workDir"),
        session_dir=_required_string(value, "sessionDir"),
        created_at=_required_number(value, "createdAt"),
        updated_at=_required_number(value, "updatedAt"),
        title=_optional_string(value, "title"),
        last_prompt=_optional_string(value, "lastPrompt"),
        archived=_optional_bool(value, "archived"),
        metadata=value.get("metadata") if isinstance(value.get("metadata"), dict) else None,
    )


def _session_status(value: Any) -> SessionStatus:
    if not isinstance(value, dict):
        raise TransportError("session status must be an object.", details=value)
    return SessionStatus(
        model=_optional_string(value, "model"),
        thinking_level=_required_string(value, "thinkingLevel"),
        permission=_required_string(value, "permission"),
        plan_mode=_required_bool(value, "planMode"),
        context_tokens=_required_number(value, "contextTokens"),
        max_context_tokens=_required_number(value, "maxContextTokens"),
        context_usage=_required_number(value, "contextUsage"),
        usage=value.get("usage"),
    )


def _required_string(value: dict[str, Any], field: str) -> str:
    item = value.get(field)
    if not isinstance(item, str):
        raise TransportError(f"{field} must be a string.", details=value)
    return item


def _optional_string(value: dict[str, Any], field: str) -> str | None:
    item = value.get(field)
    if item is None:
        return None
    if not isinstance(item, str):
        raise TransportError(f"{field} must be a string.", details=value)
    return item


def _required_bool(value: dict[str, Any], field: str) -> bool:
    item = value.get(field)
    if not isinstance(item, bool):
        raise TransportError(f"{field} must be a boolean.", details=value)
    return item


def _optional_bool(value: dict[str, Any], field: str) -> bool | None:
    item = value.get(field)
    if item is None:
        return None
    if not isinstance(item, bool):
        raise TransportError(f"{field} must be a boolean.", details=value)
    return item


def _required_number(value: dict[str, Any], field: str) -> int | float:
    item = value.get(field)
    if not isinstance(item, int | float) or isinstance(item, bool):
        raise TransportError(f"{field} must be a number.", details=value)
    return item
