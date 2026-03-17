"""Tests for checkpoint resume helper."""

from __future__ import annotations

from typing import Any, Callable

from strait.authoring._run_context import RunContext
from strait.composition._checkpoint_resume import with_checkpoint_resume


class TestWithCheckpointResume:
    async def test_uses_initial_state_when_no_checkpoint(self):
        received_states: list[dict[str, Any]] = []

        async def fn(
            state: dict[str, Any],
            update: Callable[[dict[str, Any]], None],
        ) -> str:
            received_states.append(state)
            return "done"

        checkpoints: list[dict[str, Any]] = []

        async def mock_checkpoint(state: dict[str, Any]) -> None:
            checkpoints.append(state)

        ctx = RunContext(run_id="r1", checkpoint=mock_checkpoint)
        result = await with_checkpoint_resume(
            ctx, last_checkpoint=None, fn=fn, initial_state={"step": 0},
        )
        assert result == "done"
        assert received_states[0] == {"step": 0}

    async def test_uses_last_checkpoint_when_provided(self):
        received_states: list[dict[str, Any]] = []

        async def fn(
            state: dict[str, Any],
            update: Callable[[dict[str, Any]], None],
        ) -> str:
            received_states.append(state)
            return "resumed"

        checkpoints: list[dict[str, Any]] = []

        async def mock_checkpoint(state: dict[str, Any]) -> None:
            checkpoints.append(state)

        ctx = RunContext(run_id="r1", checkpoint=mock_checkpoint)
        result = await with_checkpoint_resume(
            ctx, last_checkpoint={"step": 5}, fn=fn, initial_state={"step": 0},
        )
        assert result == "resumed"
        assert received_states[0] == {"step": 5}

    async def test_final_checkpoint_saved(self):
        checkpoints: list[dict[str, Any]] = []

        async def mock_checkpoint(state: dict[str, Any]) -> None:
            checkpoints.append(state)

        async def fn(
            state: dict[str, Any],
            update: Callable[[dict[str, Any]], None],
        ) -> str:
            update({"step": 1})
            update({"step": 2})
            return "done"

        ctx = RunContext(run_id="r1", checkpoint=mock_checkpoint)
        await with_checkpoint_resume(
            ctx, last_checkpoint=None, fn=fn, initial_state={"step": 0},
        )
        # Final checkpoint should be saved with latest state
        assert checkpoints[-1] == {"step": 2}

    async def test_update_state_changes_current(self):
        checkpoints: list[dict[str, Any]] = []

        async def mock_checkpoint(state: dict[str, Any]) -> None:
            checkpoints.append(state)

        captured_states: list[dict[str, Any]] = []

        async def fn(
            state: dict[str, Any],
            update: Callable[[dict[str, Any]], None],
        ) -> str:
            captured_states.append(dict(state))
            update({"step": 1, "data": "a"})
            update({"step": 2, "data": "b"})
            return "done"

        ctx = RunContext(run_id="r1", checkpoint=mock_checkpoint)
        await with_checkpoint_resume(
            ctx, last_checkpoint=None, fn=fn, initial_state={"step": 0},
        )
        assert captured_states[0] == {"step": 0}

    async def test_no_checkpoint_function_still_works(self):
        async def fn(
            state: dict[str, Any],
            update: Callable[[dict[str, Any]], None],
        ) -> str:
            update({"step": 1})
            return "done"

        ctx = RunContext(run_id="r1", checkpoint=None)
        result = await with_checkpoint_resume(
            ctx, last_checkpoint=None, fn=fn, initial_state={"step": 0},
        )
        assert result == "done"

    async def test_checkpoint_interval_batching(self):
        checkpoints: list[dict[str, Any]] = []

        async def mock_checkpoint(state: dict[str, Any]) -> None:
            checkpoints.append(dict(state))

        async def fn(
            state: dict[str, Any],
            update: Callable[[dict[str, Any]], None],
        ) -> str:
            update({"step": 1})
            update({"step": 2})
            update({"step": 3})
            update({"step": 4})
            update({"step": 5})
            return "done"

        ctx = RunContext(run_id="r1", checkpoint=mock_checkpoint)
        await with_checkpoint_resume(
            ctx, last_checkpoint=None, fn=fn, initial_state={"step": 0},
            checkpoint_interval=3,
        )
        # The final checkpoint is always saved.
        # Intermediate checkpoints fire at step_count % interval == 0.
        # step_count goes 1,2,3,4,5 -> fires at 3 (step_count=3) and final
        assert checkpoints[-1] == {"step": 5}

    async def test_result_returned(self):
        async def fn(
            state: dict[str, Any],
            update: Callable[[dict[str, Any]], None],
        ) -> dict[str, str]:
            return {"result": "success"}

        checkpoints: list[dict[str, Any]] = []

        async def mock_checkpoint(state: dict[str, Any]) -> None:
            checkpoints.append(state)

        ctx = RunContext(run_id="r1", checkpoint=mock_checkpoint)
        result = await with_checkpoint_resume(
            ctx, last_checkpoint=None, fn=fn, initial_state={},
        )
        assert result == {"result": "success"}

    async def test_initial_state_not_mutated(self):
        initial = {"step": 0, "items": []}

        async def fn(
            state: dict[str, Any],
            update: Callable[[dict[str, Any]], None],
        ) -> str:
            update({"step": 1, "items": ["a"]})
            return "done"

        checkpoints: list[dict[str, Any]] = []

        async def mock_checkpoint(state: dict[str, Any]) -> None:
            checkpoints.append(state)

        ctx = RunContext(run_id="r1", checkpoint=mock_checkpoint)
        await with_checkpoint_resume(
            ctx, last_checkpoint=None, fn=fn, initial_state=initial,
        )
        # initial_state should not be affected since update replaces the whole state
        assert initial == {"step": 0, "items": []}

    async def test_empty_fn_still_checkpoints(self):
        checkpoints: list[dict[str, Any]] = []

        async def mock_checkpoint(state: dict[str, Any]) -> None:
            checkpoints.append(state)

        async def fn(
            state: dict[str, Any],
            update: Callable[[dict[str, Any]], None],
        ) -> str:
            return "done"

        ctx = RunContext(run_id="r1", checkpoint=mock_checkpoint)
        await with_checkpoint_resume(
            ctx, last_checkpoint=None, fn=fn, initial_state={"init": True},
        )
        assert len(checkpoints) == 1
        assert checkpoints[0] == {"init": True}
