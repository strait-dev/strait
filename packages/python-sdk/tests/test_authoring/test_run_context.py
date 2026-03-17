"""Tests for RunContext factory and all wired methods."""

from __future__ import annotations

from typing import Any

from strait.authoring._run_context import RunContext, RunContextState
from strait.authoring._run_context_client import create_run_context


class _RecordingClient:
    """Mock client that records all calls."""

    def __init__(self) -> None:
        self.calls: list[tuple[str, Any]] = []

    async def checkpoint_run(self, run_id: str, body: Any) -> None:
        self.calls.append(("checkpoint_run", {"run_id": run_id, "body": body}))

    async def heartbeat_run(self, run_id: str) -> None:
        self.calls.append(("heartbeat_run", {"run_id": run_id}))

    async def progress_run(self, run_id: str, body: Any) -> None:
        self.calls.append(("progress_run", {"run_id": run_id, "body": body}))

    async def log_run(self, run_id: str, body: Any) -> None:
        self.calls.append(("log_run", {"run_id": run_id, "body": body}))

    async def usage_run(self, run_id: str, body: Any) -> None:
        self.calls.append(("usage_run", {"run_id": run_id, "body": body}))

    async def tool_call_run(self, run_id: str, body: Any) -> None:
        self.calls.append(("tool_call_run", {"run_id": run_id, "body": body}))

    async def output_run(self, run_id: str, body: Any) -> None:
        self.calls.append(("output_run", {"run_id": run_id, "body": body}))

    async def wait_for_event_run(self, run_id: str, body: Any) -> dict[str, Any]:
        self.calls.append(("wait_for_event_run", {"run_id": run_id, "body": body}))
        return {"status": "waiting", "event_key": body["event_key"]}

    async def spawn_run(self, run_id: str, body: Any) -> dict[str, Any]:
        self.calls.append(("spawn_run", {"run_id": run_id, "body": body}))
        return {"id": "spawn_1"}

    async def continue_run(self, run_id: str, body: Any | None) -> dict[str, Any]:
        self.calls.append(("continue_run", {"run_id": run_id, "body": body}))
        return {"id": "continue_1"}

    async def annotate_run(self, run_id: str, body: Any) -> None:
        self.calls.append(("annotate_run", {"run_id": run_id, "body": body}))

    async def complete_run(self, run_id: str, body: Any) -> None:
        self.calls.append(("complete_run", {"run_id": run_id, "body": body}))

    async def fail_run(self, run_id: str, body: Any) -> None:
        self.calls.append(("fail_run", {"run_id": run_id, "body": body}))

    async def set_state(self, run_id: str, body: Any) -> None:
        self.calls.append(("set_state", {"run_id": run_id, "body": body}))

    async def list_state(self, run_id: str) -> list[dict[str, Any]]:
        self.calls.append(("list_state", {"run_id": run_id}))
        return [{"key": "k", "value": "v"}]

    async def get_state(self, run_id: str, key: str) -> Any:
        self.calls.append(("get_state", {"run_id": run_id, "key": key}))
        return {"value": "test"}

    async def delete_state(self, run_id: str, key: str) -> None:
        self.calls.append(("delete_state", {"run_id": run_id, "key": key}))

    async def stream_run(self, run_id: str, body: Any) -> None:
        self.calls.append(("stream_run", {"run_id": run_id, "body": body}))


class TestCreateRunContext:
    def test_returns_run_context(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        assert isinstance(ctx, RunContext)
        assert ctx.run_id == "run-1"

    def test_default_attempt(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        assert ctx.attempt == 1

    def test_custom_attempt(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1", attempt=3)
        assert ctx.attempt == 3

    def test_has_all_methods(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        assert ctx.checkpoint is not None
        assert ctx.report_progress is not None
        assert ctx.heartbeat is not None
        assert ctx.report_usage is not None
        assert ctx.log_tool_call is not None
        assert ctx.save_output is not None
        assert ctx.state is not None
        assert ctx.stream_chunk is not None
        assert ctx.wait_for_event is not None
        assert ctx.spawn is not None
        assert ctx.continue_run is not None
        assert ctx.annotate is not None
        assert ctx.complete is not None
        assert ctx.fail is not None

    def test_logger_is_configured(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        assert ctx.logger is not None
        assert "run-1" in ctx.logger.name

    async def test_checkpoint(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.checkpoint({"step": 5})
        assert len(client.calls) == 1
        assert client.calls[0][0] == "checkpoint_run"
        assert client.calls[0][1]["body"]["state"] == {"step": 5}
        assert client.calls[0][1]["body"]["source"] == "sdk"

    async def test_heartbeat(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.heartbeat()
        assert client.calls[0][0] == "heartbeat_run"

    async def test_report_progress(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.report_progress(50.0, "halfway")
        assert client.calls[0][0] == "progress_run"
        assert client.calls[0][1]["body"]["percent"] == 50.0
        assert client.calls[0][1]["body"]["message"] == "halfway"

    async def test_report_progress_without_message(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.report_progress(75.0)
        body = client.calls[0][1]["body"]
        assert body["percent"] == 75.0
        assert "message" not in body

    async def test_report_usage(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.report_usage(
            provider="openai", model="gpt-4", prompt_tokens=100, cost_microusd=500,
        )
        body = client.calls[0][1]["body"]
        assert body["provider"] == "openai"
        assert body["model"] == "gpt-4"
        assert body["prompt_tokens"] == 100
        assert body["cost_microusd"] == 500

    async def test_report_usage_minimal(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.report_usage(provider="anthropic", model="claude-3")
        body = client.calls[0][1]["body"]
        assert body == {"provider": "anthropic", "model": "claude-3"}

    async def test_log_tool_call(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.log_tool_call(
            "search", input={"q": "test"}, output={"results": []}, duration_ms=42, status="ok",
        )
        body = client.calls[0][1]["body"]
        assert body["tool_name"] == "search"
        assert body["input"] == {"q": "test"}
        assert body["duration_ms"] == 42

    async def test_save_output(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.save_output("result", {"data": 1}, schema={"type": "object"})
        body = client.calls[0][1]["body"]
        assert body["key"] == "result"
        assert body["value"] == {"data": 1}
        assert body["schema"] == {"type": "object"}

    async def test_wait_for_event(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        result = await ctx.wait_for_event("order.paid", timeout_secs=300)
        assert result["status"] == "waiting"
        assert result["event_key"] == "order.paid"

    async def test_spawn(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        result = await ctx.spawn("worker", "proj-1", payload={"x": 1}, priority=5)
        assert result["id"] == "spawn_1"
        body = client.calls[0][1]["body"]
        assert body["job_slug"] == "worker"
        assert body["priority"] == 5

    async def test_continue_run(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        result = await ctx.continue_run(payload={"next": True})
        assert result["id"] == "continue_1"

    async def test_continue_run_no_payload(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.continue_run()
        assert client.calls[0][1]["body"] is None

    async def test_annotate(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.annotate({"env": "prod"})
        body = client.calls[0][1]["body"]
        assert body["annotations"] == {"env": "prod"}

    async def test_complete(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.complete(result={"status": "done"})
        body = client.calls[0][1]["body"]
        assert body["result"] == {"status": "done"}

    async def test_complete_no_result(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.complete()
        assert client.calls[0][1]["body"] is None

    async def test_fail(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.fail("something broke")
        body = client.calls[0][1]["body"]
        assert body["error"] == "something broke"

    async def test_stream_chunk(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.stream_chunk("hello ", stream_id="s1", done=False)
        body = client.calls[0][1]["body"]
        assert body["chunk"] == "hello "
        assert body["stream_id"] == "s1"
        assert body["done"] is False

    async def test_stream_chunk_minimal(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.stream_chunk("data")
        body = client.calls[0][1]["body"]
        assert body == {"chunk": "data"}

    async def test_state_get(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        result = await ctx.state.get("my-key")
        assert result == {"value": "test"}
        assert client.calls[0][0] == "get_state"

    async def test_state_set(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.state.set("my-key", {"foo": "bar"})
        assert client.calls[0][0] == "set_state"
        assert client.calls[0][1]["body"]["key"] == "my-key"
        assert client.calls[0][1]["body"]["value"] == {"foo": "bar"}

    async def test_state_delete(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        await ctx.state.delete("my-key")
        assert client.calls[0][0] == "delete_state"
        assert client.calls[0][1]["key"] == "my-key"

    async def test_state_list(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        result = await ctx.state.list()
        assert len(result) == 1
        assert result[0]["key"] == "k"

    async def test_state_is_run_context_state(self):
        client = _RecordingClient()
        ctx = create_run_context(client, "run-1")
        assert isinstance(ctx.state, RunContextState)
