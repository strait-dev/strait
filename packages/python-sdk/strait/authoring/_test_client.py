"""In-memory test context for unit-testing jobs and agents without HTTP."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any

from strait.authoring._run_context import RunContext
from strait.authoring._run_context_client import create_run_context


@dataclass
class TestRunRecord:
    checkpoints: list[dict[str, Any]] = field(default_factory=list)
    logs: list[dict[str, Any]] = field(default_factory=list)
    usage_reports: list[dict[str, Any]] = field(default_factory=list)
    tool_calls: list[dict[str, Any]] = field(default_factory=list)
    outputs: list[dict[str, Any]] = field(default_factory=list)
    progress_updates: list[dict[str, Any]] = field(default_factory=list)
    state_store: dict[str, Any] = field(default_factory=dict)
    stream_chunks: list[dict[str, Any]] = field(default_factory=list)
    heartbeats: int = 0
    spawns: list[dict[str, Any]] = field(default_factory=list)
    events: list[dict[str, Any]] = field(default_factory=list)
    annotations: list[dict[str, str]] = field(default_factory=list)
    continuations: list[dict[str, Any]] = field(default_factory=list)
    completed: bool = False
    failed: bool = False
    fail_error: str | None = None
    result: dict[str, Any] | None = None


def create_test_context(
    run_id: str = "test-run",
    attempt: int = 1,
) -> tuple[RunContext, TestRunRecord]:
    record = TestRunRecord()

    class MockClient:
        async def checkpoint_run(self, rid: str, body: Any) -> None:
            record.checkpoints.append(body.get("state", body))

        async def heartbeat_run(self, rid: str) -> None:
            record.heartbeats += 1

        async def progress_run(self, rid: str, body: Any) -> None:
            record.progress_updates.append(body)

        async def log_run(self, rid: str, body: Any) -> None:
            record.logs.append(body)

        async def usage_run(self, rid: str, body: Any) -> None:
            record.usage_reports.append(body)

        async def tool_call_run(self, rid: str, body: Any) -> None:
            record.tool_calls.append(body)

        async def output_run(self, rid: str, body: Any) -> None:
            record.outputs.append(body)

        async def wait_for_event_run(self, rid: str, body: Any) -> dict[str, Any]:
            record.events.append(body)
            return {
                "status": "waiting",
                "event_key": body["event_key"],
                "trigger_id": "trigger_test",
                "expires_at": datetime.now(timezone.utc).isoformat(),
            }

        async def spawn_run(self, rid: str, body: Any) -> dict[str, Any]:
            record.spawns.append(body)
            return {"id": f"spawn_{len(record.spawns)}"}

        async def continue_run(self, rid: str, body: Any) -> dict[str, Any]:
            payload = body.get("payload") if body else None
            record.continuations.append({"payload": payload})
            return {"id": f"continue_{len(record.continuations)}"}

        async def annotate_run(self, rid: str, body: Any) -> None:
            record.annotations.append(body.get("annotations", body))

        async def complete_run(self, rid: str, body: Any) -> None:
            record.completed = True
            if body:
                record.result = body.get("result")

        async def fail_run(self, rid: str, body: Any) -> None:
            record.failed = True
            record.fail_error = body.get("error")

        async def set_state(self, rid: str, body: Any) -> None:
            record.state_store[body["key"]] = body["value"]

        async def list_state(self, rid: str) -> list[dict[str, Any]]:
            return [
                {"key": k, "value": v, "updated_at": datetime.now(timezone.utc).isoformat()}
                for k, v in record.state_store.items()
            ]

        async def get_state(self, rid: str, key: str) -> Any:
            return record.state_store.get(key)

        async def delete_state(self, rid: str, key: str) -> None:
            record.state_store.pop(key, None)

        async def stream_run(self, rid: str, body: Any) -> None:
            record.stream_chunks.append(body)

    ctx = create_run_context(MockClient(), run_id, attempt)
    return ctx, record
