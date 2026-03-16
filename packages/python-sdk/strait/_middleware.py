"""Middleware hooks for HTTP request/response interception."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Callable


@dataclass
class MiddlewareRequestContext:
    method: str
    url: str
    headers: dict[str, str]
    body: bytes | None = None


@dataclass
class MiddlewareResponseContext:
    method: str
    url: str
    status: int
    duration_ms: int


@dataclass
class MiddlewareErrorContext:
    method: str
    url: str
    error: BaseException


@dataclass
class Middleware:
    on_request: Callable[[MiddlewareRequestContext], None] | None = None
    on_response: Callable[[MiddlewareResponseContext], None] | None = None
    on_error: Callable[[MiddlewareErrorContext], None] | None = None
