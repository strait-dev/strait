"""Tests for composition wait_for_run."""

import pytest

from strait._errors import StraitTimeoutError
from strait.composition._wait import WaitForRunOptions, wait_for_run


class TestWaitForRun:
    def test_immediate_terminal(self):
        run = {"id": "r1", "status": "completed"}
        result = wait_for_run(
            get_run=lambda rid: run,
            get_status=lambda r: r["status"],
            run_id="r1",
        )
        assert result["status"] == "completed"

    def test_transitions_to_terminal(self):
        statuses = iter(["executing", "executing", "completed"])

        def get_run(rid):
            return {"id": rid, "status": next(statuses)}

        result = wait_for_run(
            get_run=get_run,
            get_status=lambda r: r["status"],
            run_id="r1",
            opts=WaitForRunOptions(initial_delay_ms=1, timeout_ms=5000),
        )
        assert result["status"] == "completed"

    def test_timeout_raises(self):
        def get_run(rid):
            return {"id": rid, "status": "executing"}

        with pytest.raises(StraitTimeoutError) as exc_info:
            wait_for_run(
                get_run=get_run,
                get_status=lambda r: r["status"],
                run_id="r1",
                opts=WaitForRunOptions(timeout_ms=50, initial_delay_ms=10),
            )
        assert exc_info.value.run_id == "r1"

    def test_custom_is_terminal(self):
        run = {"id": "r1", "status": "custom_done"}
        result = wait_for_run(
            get_run=lambda rid: run,
            get_status=lambda r: r["status"],
            run_id="r1",
            opts=WaitForRunOptions(is_terminal=lambda s: s == "custom_done"),
        )
        assert result["status"] == "custom_done"

    def test_all_default_terminal_statuses(self):
        for status in [
            "completed", "failed", "timed_out", "crashed",
            "system_failed", "canceled", "expired", "dead_letter",
        ]:
            run = {"id": "r1", "status": status}
            result = wait_for_run(
                get_run=lambda rid: run,
                get_status=lambda r: r["status"],
                run_id="r1",
            )
            assert result["status"] == status
