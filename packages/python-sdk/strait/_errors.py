"""Strait SDK exception hierarchy."""

from __future__ import annotations

from typing import Any


class StraitError(Exception):
    """Base exception for all Strait SDK errors."""


class TransportError(StraitError):
    """Network or transport-level failure."""

    def __init__(self, message: str, cause: BaseException | None = None) -> None:
        super().__init__(message)
        self.__cause__ = cause


class DecodeError(StraitError):
    """JSON decode failure."""

    def __init__(self, message: str, body: str, cause: BaseException | None = None) -> None:
        super().__init__(message)
        self.body = body
        self.__cause__ = cause


class ValidationError(StraitError):
    """Config or input validation failure."""

    def __init__(self, message: str, issues: list[str] | None = None) -> None:
        super().__init__(message)
        self.issues = issues or []


class UnauthorizedError(StraitError):
    """401 or 403 HTTP error."""

    def __init__(self, status: int, message: str, body: Any = None) -> None:
        super().__init__(message)
        self.status = status
        self.body = body


class NotFoundError(StraitError):
    """404 HTTP error."""

    def __init__(self, status: int, message: str, body: Any = None) -> None:
        super().__init__(message)
        self.status = status
        self.body = body


class ConflictError(StraitError):
    """409 HTTP error."""

    def __init__(self, status: int, message: str, body: Any = None) -> None:
        super().__init__(message)
        self.status = status
        self.body = body


class RateLimitedError(StraitError):
    """429 HTTP error."""

    def __init__(self, status: int, message: str, body: Any = None) -> None:
        super().__init__(message)
        self.status = status
        self.body = body


class ApiError(StraitError):
    """Generic HTTP error not covered by specific types."""

    def __init__(self, status: int, message: str, body: Any = None) -> None:
        super().__init__(message)
        self.status = status
        self.body = body


class StraitTimeoutError(StraitError):
    """Polling timeout."""

    def __init__(self, message: str, run_id: str, elapsed_ms: int) -> None:
        super().__init__(message)
        self.run_id = run_id
        self.elapsed_ms = elapsed_ms


TimeoutError = StraitTimeoutError  # Alias avoiding builtin shadow


class DagValidationError(StraitError):
    """DAG validation failure."""

    def __init__(
        self,
        message: str,
        *,
        cycles: list[str] | None = None,
        missing_refs: list[str] | None = None,
        duplicate_refs: list[str] | None = None,
    ) -> None:
        super().__init__(message)
        self.cycles = cycles or []
        self.missing_refs = missing_refs or []
        self.duplicate_refs = duplicate_refs or []


def map_http_error(status: int, message: str, body: Any = None) -> StraitError:
    """Map an HTTP status code to the appropriate SDK error type."""
    if not message:
        message = f"HTTP {status}"
    if status in (401, 403):
        return UnauthorizedError(status, message, body)
    if status == 404:
        return NotFoundError(status, message, body)
    if status == 409:
        return ConflictError(status, message, body)
    if status == 429:
        return RateLimitedError(status, message, body)
    return ApiError(status, message, body)
