from __future__ import annotations

from collections.abc import Iterator, Mapping
from dataclasses import dataclass
from typing import Any

JsonObject = dict[str, Any]
PromptInput = str | list[JsonObject]


class Event(Mapping[str, Any]):
    def __init__(self, data: Mapping[str, Any]) -> None:
        self._data = dict(data)

    @property
    def type(self) -> str:
        value = self._data.get("type")
        return value if isinstance(value, str) else ""

    @property
    def session_id(self) -> str:
        value = self._data.get("sessionId")
        return value if isinstance(value, str) else ""

    @property
    def agent_id(self) -> str:
        value = self._data.get("agentId")
        return value if isinstance(value, str) else ""

    @property
    def turn_id(self) -> int | None:
        value = self._data.get("turnId")
        return value if isinstance(value, int) else None

    def __getitem__(self, key: str) -> Any:
        return self._data[key]

    def __iter__(self) -> Iterator[str]:
        return iter(self._data)

    def __len__(self) -> int:
        return len(self._data)

    def __getattr__(self, name: str) -> Any:
        try:
            return self._data[name]
        except KeyError as error:
            raise AttributeError(name) from error

    def to_dict(self) -> JsonObject:
        return dict(self._data)


@dataclass(frozen=True)
class SessionSummary:
    id: str
    work_dir: str
    session_dir: str
    created_at: int | float
    updated_at: int | float
    title: str | None = None
    last_prompt: str | None = None
    archived: bool | None = None
    metadata: JsonObject | None = None


@dataclass(frozen=True)
class SessionStatus:
    thinking_level: str
    permission: str
    plan_mode: bool
    context_tokens: int | float
    max_context_tokens: int | float
    context_usage: int | float
    model: str | None = None
    usage: Any | None = None

