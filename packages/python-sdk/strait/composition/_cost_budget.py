"""Cost budget tracking and enforcement."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Awaitable, Callable, TypeVar

from strait._errors import CostBudgetExceededError

T = TypeVar("T")


@dataclass
class CostBudgetOptions:
    max_cost_microusd: int
    on_warning: Callable[[int, int], None] | None = None
    warning_threshold: float = 0.8


class CostTracker:
    def __init__(self, options: CostBudgetOptions) -> None:
        self._current_cost = 0
        self._options = options
        self._warning_fired = False

    def add(self, cost_microusd: int) -> None:
        self._current_cost += cost_microusd

        if (
            not self._warning_fired
            and self._options.on_warning is not None
            and self._current_cost >= self._options.max_cost_microusd * self._options.warning_threshold
        ):
            self._warning_fired = True
            self._options.on_warning(self._current_cost, self._options.max_cost_microusd)

        if self._current_cost >= self._options.max_cost_microusd:
            raise CostBudgetExceededError(
                f"Cost budget exceeded: {self._current_cost} >= "
                f"{self._options.max_cost_microusd} microusd",
                current_cost_microusd=self._current_cost,
                max_cost_microusd=self._options.max_cost_microusd,
            )

    def current(self) -> int:
        return self._current_cost

    def remaining(self) -> int:
        return max(0, self._options.max_cost_microusd - self._current_cost)

    def is_exceeded(self) -> bool:
        return self._current_cost >= self._options.max_cost_microusd


def create_cost_tracker(options: CostBudgetOptions) -> CostTracker:
    return CostTracker(options)


async def with_cost_budget(
    fn: Callable[[CostTracker], Awaitable[T]],
    options: CostBudgetOptions,
) -> T:
    tracker = create_cost_tracker(options)
    return await fn(tracker)
