"""Trigger and wait helper."""

from __future__ import annotations

import asyncio
from typing import Any, Callable, TypeVar

from strait.composition._wait import WaitForRunOptions, wait_for_run, wait_for_run_async

TInput = TypeVar("TInput")
TRun = TypeVar("TRun")


def trigger_and_wait(
    trigger_fn: Callable[[TInput], TRun],
    get_run: Callable[[str], TRun],
    get_id: Callable[[TRun], str],
    get_status: Callable[[TRun], str],
    input: TInput,
    opts: WaitForRunOptions | None = None,
) -> TRun:
    run = trigger_fn(input)
    return wait_for_run(get_run, get_status, get_id(run), opts)


async def trigger_and_wait_async(
    trigger_fn: Callable[[TInput], Any],
    get_run: Callable[[str], Any],
    get_id: Callable[[Any], str],
    get_status: Callable[[Any], str],
    input: TInput,
    opts: WaitForRunOptions | None = None,
) -> Any:
    result = trigger_fn(input)
    if asyncio.iscoroutine(result):
        run = await result
    else:
        run = result
    return await wait_for_run_async(get_run, get_status, get_id(run), opts)
