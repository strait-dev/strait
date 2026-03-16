"""Tests for test harness: create_test_context."""

from __future__ import annotations

from strait.authoring._run_context import RunContext, RunContextState
from strait.authoring._test_client import TestRunRecord, create_test_context


class TestCreateTestContext:
    def test_returns_context_and_record(self):
        ctx, record = create_test_context()
        assert isinstance(ctx, RunContext)
        assert isinstance(record, TestRunRecord)

    def test_default_run_id(self):
        ctx, _ = create_test_context()
        assert ctx.run_id == "test-run"

    def test_custom_run_id(self):
        ctx, _ = create_test_context(run_id="custom-123")
        assert ctx.run_id == "custom-123"

    def test_default_attempt(self):
        ctx, _ = create_test_context()
        assert ctx.attempt == 1

    def test_custom_attempt(self):
        ctx, _ = create_test_context(attempt=3)
        assert ctx.attempt == 3

    def test_all_methods_wired(self):
        ctx, _ = create_test_context()
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

    def test_record_starts_empty(self):
        _, record = create_test_context()
        assert record.checkpoints == []
        assert record.logs == []
        assert record.usage_reports == []
        assert record.tool_calls == []
        assert record.outputs == []
        assert record.progress_updates == []
        assert record.state_store == {}
        assert record.stream_chunks == []
        assert record.heartbeats == 0
        assert record.spawns == []
        assert record.events == []
        assert record.annotations == []
        assert record.continuations == []
        assert record.completed is False
        assert record.failed is False
        assert record.fail_error is None
        assert record.result is None


class TestTestContextOperations:
    async def test_checkpoint_records(self):
        ctx, record = create_test_context()
        await ctx.checkpoint({"step": 5})
        assert len(record.checkpoints) == 1
        assert record.checkpoints[0] == {"step": 5}

    async def test_heartbeat_records(self):
        ctx, record = create_test_context()
        await ctx.heartbeat()
        await ctx.heartbeat()
        assert record.heartbeats == 2

    async def test_progress_records(self):
        ctx, record = create_test_context()
        await ctx.report_progress(50.0, "halfway")
        assert len(record.progress_updates) == 1
        assert record.progress_updates[0]["percent"] == 50.0

    async def test_report_usage_records(self):
        ctx, record = create_test_context()
        await ctx.report_usage(provider="openai", model="gpt-4", cost_microusd=100)
        assert len(record.usage_reports) == 1
        assert record.usage_reports[0]["provider"] == "openai"

    async def test_log_tool_call_records(self):
        ctx, record = create_test_context()
        await ctx.log_tool_call("search", input={"q": "test"})
        assert len(record.tool_calls) == 1
        assert record.tool_calls[0]["tool_name"] == "search"

    async def test_save_output_records(self):
        ctx, record = create_test_context()
        await ctx.save_output("result", {"data": 42})
        assert len(record.outputs) == 1
        assert record.outputs[0]["key"] == "result"

    async def test_wait_for_event_records(self):
        ctx, record = create_test_context()
        result = await ctx.wait_for_event("order.paid")
        assert len(record.events) == 1
        assert result["event_key"] == "order.paid"
        assert result["status"] == "waiting"

    async def test_spawn_records(self):
        ctx, record = create_test_context()
        result = await ctx.spawn("worker", "proj-1")
        assert len(record.spawns) == 1
        assert result["id"] == "spawn_1"

    async def test_continue_run_records(self):
        ctx, record = create_test_context()
        result = await ctx.continue_run(payload={"next": True})
        assert len(record.continuations) == 1
        assert record.continuations[0]["payload"] == {"next": True}
        assert result["id"] == "continue_1"

    async def test_annotate_records(self):
        ctx, record = create_test_context()
        await ctx.annotate({"env": "prod"})
        assert len(record.annotations) == 1
        assert record.annotations[0] == {"env": "prod"}

    async def test_complete_records(self):
        ctx, record = create_test_context()
        await ctx.complete(result={"status": "done"})
        assert record.completed is True
        assert record.result == {"status": "done"}

    async def test_complete_no_result(self):
        ctx, record = create_test_context()
        await ctx.complete()
        assert record.completed is True
        assert record.result is None

    async def test_fail_records(self):
        ctx, record = create_test_context()
        await ctx.fail("something broke")
        assert record.failed is True
        assert record.fail_error == "something broke"

    async def test_stream_chunk_records(self):
        ctx, record = create_test_context()
        await ctx.stream_chunk("hello ", stream_id="s1", done=False)
        await ctx.stream_chunk("world", stream_id="s1", done=True)
        assert len(record.stream_chunks) == 2
        assert record.stream_chunks[0]["chunk"] == "hello "
        assert record.stream_chunks[1]["done"] is True


class TestTestContextStateStore:
    async def test_state_set_and_get(self):
        ctx, record = create_test_context()
        await ctx.state.set("key1", {"value": 42})
        assert record.state_store["key1"] == {"value": 42}

    async def test_state_get(self):
        ctx, record = create_test_context()
        await ctx.state.set("key1", "hello")
        result = await ctx.state.get("key1")
        assert result == "hello"

    async def test_state_get_missing_key(self):
        ctx, _ = create_test_context()
        result = await ctx.state.get("nonexistent")
        assert result is None

    async def test_state_delete(self):
        ctx, record = create_test_context()
        await ctx.state.set("key1", "value1")
        await ctx.state.delete("key1")
        assert "key1" not in record.state_store

    async def test_state_delete_nonexistent(self):
        ctx, _ = create_test_context()
        # Should not raise
        await ctx.state.delete("nonexistent")

    async def test_state_list(self):
        ctx, _ = create_test_context()
        await ctx.state.set("a", 1)
        await ctx.state.set("b", 2)
        items = await ctx.state.list()
        assert len(items) == 2
        keys = {item["key"] for item in items}
        assert keys == {"a", "b"}

    async def test_state_list_empty(self):
        ctx, _ = create_test_context()
        items = await ctx.state.list()
        assert items == []

    async def test_state_overwrite(self):
        ctx, record = create_test_context()
        await ctx.state.set("key1", "first")
        await ctx.state.set("key1", "second")
        assert record.state_store["key1"] == "second"
