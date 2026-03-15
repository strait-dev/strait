"""Retry with exponential backoff."""

from __future__ import annotations

import asyncio
import random
import time
from dataclasses import dataclass
from enum import StrEnum
from typing import Callable, TypeVar

T = TypeVar("T")


class JitterStrategy(StrEnum):
    FULL = "full"
    NONE = "none"


@dataclass
class RetryOptions:
    attempts: int = 3
    delay_ms: int = 250
    factor: float = 2.0
    max_delay_ms: int = 30_000
    jitter: JitterStrategy = JitterStrategy.FULL
    should_retry: Callable[[Exception, int, int], bool] | None = None


def _compute_delay(base_delay: int, jitter: JitterStrategy) -> int:
    if jitter == JitterStrategy.FULL:
        return int(round(random.random() * base_delay))
    return base_delay


def with_retry(fn: Callable[[], T], opts: RetryOptions | None = None) -> T:
    o = opts or RetryOptions()
    delay_ms = o.delay_ms

    for attempt in range(1, o.attempts + 1):
        try:
            return fn()
        except Exception as err:
            if attempt >= o.attempts:
                raise
            if o.should_retry is not None and not o.should_retry(err, attempt, o.attempts):
                raise

            wait_ms = _compute_delay(delay_ms, o.jitter)
            time.sleep(wait_ms / 1000.0)
            delay_ms = int(min(o.max_delay_ms, max(1, round(delay_ms * o.factor))))

    raise RuntimeError("unreachable")  # pragma: no cover


async def with_retry_async(
    fn: Callable[[], T], opts: RetryOptions | None = None,
) -> T:
    o = opts or RetryOptions()
    delay_ms = o.delay_ms

    for attempt in range(1, o.attempts + 1):
        try:
            result = fn()
            if asyncio.iscoroutine(result):
                return await result  # type: ignore[misc]
            return result  # type: ignore[return-value]
        except Exception as err:
            if attempt >= o.attempts:
                raise
            if o.should_retry is not None and not o.should_retry(err, attempt, o.attempts):
                raise

            wait_ms = _compute_delay(delay_ms, o.jitter)
            await asyncio.sleep(wait_ms / 1000.0)
            delay_ms = int(min(o.max_delay_ms, max(1, round(delay_ms * o.factor))))

    raise RuntimeError("unreachable")  # pragma: no cover
