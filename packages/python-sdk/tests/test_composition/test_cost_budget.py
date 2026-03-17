"""Tests for cost budget tracking and enforcement."""

from __future__ import annotations

import pytest

from strait._errors import CostBudgetExceededError
from strait.composition._cost_budget import (
    CostBudgetOptions,
    CostTracker,
    create_cost_tracker,
    with_cost_budget,
)


class TestCostTracker:
    def test_initial_current_is_zero(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        assert tracker.current() == 0

    def test_initial_remaining(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        assert tracker.remaining() == 1000

    def test_initial_is_exceeded_false(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        assert tracker.is_exceeded() is False

    def test_add_updates_current(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        tracker.add(300)
        assert tracker.current() == 300

    def test_add_updates_remaining(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        tracker.add(300)
        assert tracker.remaining() == 700

    def test_add_multiple(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        tracker.add(200)
        tracker.add(300)
        assert tracker.current() == 500
        assert tracker.remaining() == 500

    def test_add_raises_at_exact_limit(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        with pytest.raises(CostBudgetExceededError) as exc_info:
            tracker.add(1000)
        assert exc_info.value.current_cost_microusd == 1000
        assert exc_info.value.max_cost_microusd == 1000

    def test_add_raises_over_limit(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        tracker.add(800)
        with pytest.raises(CostBudgetExceededError):
            tracker.add(300)

    def test_is_exceeded_after_exceeding(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        try:
            tracker.add(1000)
        except CostBudgetExceededError:
            pass
        assert tracker.is_exceeded() is True

    def test_remaining_at_zero(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        try:
            tracker.add(1000)
        except CostBudgetExceededError:
            pass
        assert tracker.remaining() == 0

    def test_remaining_never_negative(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        try:
            tracker.add(1500)
        except CostBudgetExceededError:
            pass
        assert tracker.remaining() == 0


class TestCostBudgetWarning:
    def test_warning_fires_at_threshold(self):
        warnings: list[tuple[int, int]] = []

        def on_warning(current: int, max_val: int) -> None:
            warnings.append((current, max_val))

        tracker = create_cost_tracker(CostBudgetOptions(
            max_cost_microusd=1000, on_warning=on_warning, warning_threshold=0.8,
        ))
        tracker.add(800)
        assert len(warnings) == 1
        assert warnings[0] == (800, 1000)

    def test_warning_fires_once(self):
        warnings: list[tuple[int, int]] = []

        def on_warning(current: int, max_val: int) -> None:
            warnings.append((current, max_val))

        tracker = create_cost_tracker(CostBudgetOptions(
            max_cost_microusd=1000, on_warning=on_warning, warning_threshold=0.5,
        ))
        tracker.add(500)
        tracker.add(100)
        assert len(warnings) == 1

    def test_warning_not_fired_below_threshold(self):
        warnings: list[tuple[int, int]] = []

        def on_warning(current: int, max_val: int) -> None:
            warnings.append((current, max_val))

        tracker = create_cost_tracker(CostBudgetOptions(
            max_cost_microusd=1000, on_warning=on_warning, warning_threshold=0.8,
        ))
        tracker.add(700)
        assert len(warnings) == 0

    def test_no_warning_callback(self):
        tracker = create_cost_tracker(CostBudgetOptions(max_cost_microusd=1000))
        # Should not raise
        tracker.add(900)

    def test_custom_warning_threshold(self):
        warnings: list[tuple[int, int]] = []

        def on_warning(current: int, max_val: int) -> None:
            warnings.append((current, max_val))

        tracker = create_cost_tracker(CostBudgetOptions(
            max_cost_microusd=1000, on_warning=on_warning, warning_threshold=0.5,
        ))
        tracker.add(400)
        assert len(warnings) == 0
        tracker.add(100)
        assert len(warnings) == 1


class TestWithCostBudget:
    async def test_basic_usage(self):
        async def fn(tracker: CostTracker) -> str:
            tracker.add(100)
            return "done"

        result = await with_cost_budget(fn, CostBudgetOptions(max_cost_microusd=1000))
        assert result == "done"

    async def test_raises_on_budget_exceeded(self):
        async def fn(tracker: CostTracker) -> str:
            tracker.add(2000)
            return "done"

        with pytest.raises(CostBudgetExceededError):
            await with_cost_budget(fn, CostBudgetOptions(max_cost_microusd=1000))

    async def test_tracker_state_in_callback(self):
        async def fn(tracker: CostTracker) -> int:
            tracker.add(300)
            tracker.add(200)
            return tracker.current()

        result = await with_cost_budget(fn, CostBudgetOptions(max_cost_microusd=1000))
        assert result == 500


class TestCostBudgetExceededError:
    def test_is_strait_error(self):
        from strait._errors import StraitError
        err = CostBudgetExceededError("test", 100, 50)
        assert isinstance(err, StraitError)

    def test_attributes(self):
        err = CostBudgetExceededError("budget exceeded", 1500, 1000)
        assert err.current_cost_microusd == 1500
        assert err.max_cost_microusd == 1000
        assert str(err) == "budget exceeded"

    def test_message(self):
        err = CostBudgetExceededError("custom message", 200, 100)
        assert "custom message" in str(err)
