"""Wait for run to reach terminal status with exponential backoff polling."""

from __future__ import annotations

import asyncio
import time
from dataclasses import dataclass
from typing import Any, Callable, TypeVar

from strait._errors import StraitTimeoutError

T = TypeVar("T")

_DEFAULT_TERMINAL_STATUSES = frozenset({
    "completed", "failed", "timed_out", "crashed",
    "system_failed", "canceled", "expired", "dead_letter",
})


@dataclass
class WaitForRunOptions:
    timeout_ms: int = 60_000
    initial_delay_ms: int = 500
    max_delay_ms: int = 10_000
    factor: float = 1.5
    is_terminal: Callable[[str], bool] | None = None


def wait_for_run(
    get_run: Callable[[str], T],
    get_status: Callable[[T], str],
    run_id: str,
    opts: WaitForRunOptions | None = None,
) -> T:
    o = opts or WaitForRunOptions()
    delay_ms = o.initial_delay_ms
    started_at = time.monotonic()

    while True:
        run = get_run(run_id)
        status = get_status(run)

        is_terminal = (
            o.is_terminal(status)
            if o.is_terminal is not None
            else status in _DEFAULT_TERMINAL_STATUSES
        )

        if is_terminal:
            return run

        elapsed = int((time.monotonic() - started_at) * 1000)
        if elapsed > o.timeout_ms:
            raise StraitTimeoutError(
                f"waitForRun timed out after {o.timeout_ms}ms for run {run_id}",
                run_id=run_id,
                elapsed_ms=elapsed,
            )

        time.sleep(delay_ms / 1000.0)
        delay_ms = int(min(o.max_delay_ms, max(1, round(delay_ms * o.factor))))


async def wait_for_run_async(
    get_run: Callable[[str], Any],
    get_status: Callable[[Any], str],
    run_id: str,
    opts: WaitForRunOptions | None = None,
) -> Any:
    o = opts or WaitForRunOptions()
    delay_ms = o.initial_delay_ms
    started_at = time.monotonic()

    while True:
        result = get_run(run_id)
        if asyncio.iscoroutine(result):
            run = await result
        else:
            run = result

        status = get_status(run)

        is_terminal = (
            o.is_terminal(status)
            if o.is_terminal is not None
            else status in _DEFAULT_TERMINAL_STATUSES
        )

        if is_terminal:
            return run

        elapsed = int((time.monotonic() - started_at) * 1000)
        if elapsed > o.timeout_ms:
            raise StraitTimeoutError(
                f"waitForRun timed out after {o.timeout_ms}ms for run {run_id}",
                run_id=run_id,
                elapsed_ms=elapsed,
            )

        await asyncio.sleep(delay_ms / 1000.0)
        delay_ms = int(min(o.max_delay_ms, max(1, round(delay_ms * o.factor))))
