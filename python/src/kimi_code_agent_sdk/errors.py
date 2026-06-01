from __future__ import annotations

from typing import Any


class KimiCodeAgentError(Exception):
    def __init__(
        self,
        message: str,
        *,
        code: str,
        retryable: bool = False,
        details: Any | None = None,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.retryable = retryable
        self.details = details


class ProtocolError(KimiCodeAgentError):
    pass


class TransportError(KimiCodeAgentError):
    def __init__(self, message: str, *, details: Any | None = None) -> None:
        super().__init__(message, code="TRANSPORT_ERROR", retryable=True, details=details)

