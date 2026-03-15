"""Run context passed to job/workflow handlers."""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import Any, Callable


@dataclass
class RunContext:
    run_id: str
    attempt: int = 1
    logger: logging.Logger = field(default_factory=lambda: logging.getLogger("strait"))
    checkpoint: Callable[[dict[str, Any]], None] | None = None
    report_progress: Callable[[float], None] | None = None
    heartbeat: Callable[[], None] | None = None
