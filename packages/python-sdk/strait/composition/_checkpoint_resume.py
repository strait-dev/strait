"""Checkpoint resume helper for resumable runs."""

from __future__ import annotations

from typing import Any, Awaitable, Callable, TypeVar

from strait.authoring._run_context import RunContext

T = TypeVar("T")


async def with_checkpoint_resume(
    ctx: RunContext,
    last_checkpoint: dict[str, Any] | None,
    fn: Callable[[dict[str, Any], Callable[[dict[str, Any]], None]], Awaitable[T]],
    initial_state: dict[str, Any],
    checkpoint_interval: int = 1,
) -> T:
    current_state = last_checkpoint if last_checkpoint is not None else initial_state
    step_count = 0

    def update_state(new_state: dict[str, Any]) -> None:
        nonlocal current_state, step_count
        current_state = new_state
        step_count += 1
        if step_count % checkpoint_interval == 0 and ctx.checkpoint is not None:
            try:
                import asyncio
                coro = ctx.checkpoint(current_state)
                if asyncio.iscoroutine(coro):
                    asyncio.ensure_future(coro)
            except Exception:
                pass

    result = await fn(current_state, update_state)

    if ctx.checkpoint is not None:
        await ctx.checkpoint(current_state)

    return result
