"""Run context passed to job/workflow handlers."""

from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass, field
from typing import Any, Callable, Awaitable, Protocol


class RunContextState:
    def __init__(
        self,
        get: Callable[[str], Awaitable[Any]],
        set: Callable[[str, Any], Awaitable[None]],
        delete: Callable[[str], Awaitable[None]],
        list: Callable[[], Awaitable[list[dict[str, Any]]]],
    ) -> None:
        self.get = get
        self.set = set
        self.delete = delete
        self.list = list


@dataclass
class RunContext:
    run_id: str
    attempt: int = 1
    logger: logging.Logger = field(default_factory=lambda: logging.getLogger("strait"))
    checkpoint: Callable[[dict[str, Any]], Any] | None = None
    report_progress: Callable[..., Any] | None = None
    heartbeat: Callable[[], Any] | None = None
    report_usage: Callable[..., Any] | None = None
    log_tool_call: Callable[..., Any] | None = None
    save_output: Callable[..., Any] | None = None
    state: RunContextState | None = None
    stream_chunk: Callable[..., Any] | None = None
    wait_for_event: Callable[..., Any] | None = None
    spawn: Callable[..., Any] | None = None
    continue_run: Callable[..., Any] | None = None
    annotate: Callable[..., Any] | None = None
    complete: Callable[..., Any] | None = None
    fail: Callable[..., Any] | None = None
