"""Tests for composition trigger_and_wait."""

from strait.composition._trigger import trigger_and_wait
from strait.composition._wait import WaitForRunOptions


class TestTriggerAndWait:
    def test_trigger_then_wait(self):
        def trigger_fn(input):
            return {"id": "run-1", "status": "queued"}

        def get_run(run_id):
            return {"id": run_id, "status": "completed"}

        result = trigger_and_wait(
            trigger_fn=trigger_fn,
            get_run=get_run,
            get_id=lambda r: r["id"],
            get_status=lambda r: r["status"],
            input={"payload": {}},
            opts=WaitForRunOptions(initial_delay_ms=1),
        )
        assert result["status"] == "completed"
        assert result["id"] == "run-1"
