"""Tests for defineAgent and AgentRunContext."""

from __future__ import annotations

from typing import Any

from strait.authoring._agent import AgentOptions, AgentRunContext, define_agent
from strait.authoring._job import JobDefinition
from strait.authoring._run_context import RunContext


class TestDefineAgent:
    def test_returns_job_definition(self):
        agent = define_agent(AgentOptions(
            name="Test Agent",
            slug="test-agent",
            project_id="proj-1",
            endpoint_url="http://localhost:8080",
        ))
        assert isinstance(agent, JobDefinition)
        assert agent.slug == "test-agent"

    def test_agent_tag_set(self):
        agent = define_agent(AgentOptions(
            name="Test Agent",
            slug="test-agent",
            project_id="proj-1",
            endpoint_url="http://localhost:8080",
        ))
        body = agent.to_registration_body()
        assert body["tags"]["strait.kind"] == "agent"

    def test_custom_tags_preserved(self):
        agent = define_agent(AgentOptions(
            name="Test Agent",
            slug="test-agent",
            project_id="proj-1",
            endpoint_url="http://localhost:8080",
            tags={"env": "prod"},
        ))
        body = agent.to_registration_body()
        assert body["tags"]["env"] == "prod"
        assert body["tags"]["strait.kind"] == "agent"

    def test_default_timeout(self):
        agent = define_agent(AgentOptions(
            name="Test Agent",
            slug="test-agent",
            project_id="proj-1",
            endpoint_url="http://localhost:8080",
        ))
        body = agent.to_registration_body()
        assert body.get("timeout_secs") == 600

    def test_custom_timeout(self):
        agent = define_agent(AgentOptions(
            name="Test Agent",
            slug="test-agent",
            project_id="proj-1",
            endpoint_url="http://localhost:8080",
            timeout_secs=1200,
        ))
        body = agent.to_registration_body()
        assert body.get("timeout_secs") == 1200

    def test_default_max_attempts(self):
        agent = define_agent(AgentOptions(
            name="Test Agent",
            slug="test-agent",
            project_id="proj-1",
            endpoint_url="http://localhost:8080",
        ))
        body = agent.to_registration_body()
        assert body.get("max_attempts") == 5

    def test_default_retry_strategy(self):
        agent = define_agent(AgentOptions(
            name="Test Agent",
            slug="test-agent",
            project_id="proj-1",
            endpoint_url="http://localhost:8080",
        ))
        body = agent.to_registration_body()
        assert body.get("retry_strategy") == "exponential"

    def test_run_handler_attached(self):
        def my_handler(payload: Any, ctx: Any) -> Any:
            return {"done": True}

        agent = define_agent(AgentOptions(
            name="Test Agent",
            slug="test-agent",
            project_id="proj-1",
            endpoint_url="http://localhost:8080",
            run=my_handler,
        ))
        assert agent.run is not None

    def test_lifecycle_hooks_attached(self):
        def on_success(p: Any, r: Any, c: Any) -> None:
            pass

        def on_failure(p: Any, e: Any, c: Any) -> None:
            pass

        def on_start(p: Any, c: Any) -> None:
            pass

        agent = define_agent(AgentOptions(
            name="Test Agent",
            slug="test-agent",
            project_id="proj-1",
            endpoint_url="http://localhost:8080",
            on_success=on_success,
            on_failure=on_failure,
            on_start=on_start,
        ))
        assert agent.on_success is on_success
        assert agent.on_failure is on_failure
        assert agent.on_start is on_start

    def test_agent_run_wraps_context(self):
        captured_ctx: list[Any] = []

        def my_handler(payload: Any, ctx: Any) -> Any:
            captured_ctx.append(ctx)
            return "ok"

        agent = define_agent(AgentOptions(
            name="Test",
            slug="test",
            project_id="p1",
            endpoint_url="http://localhost",
            run=my_handler,
        ))

        base_ctx = RunContext(run_id="run-1", attempt=2)
        result = agent.run({"data": 1}, base_ctx)
        assert result == "ok"
        assert len(captured_ctx) == 1
        assert isinstance(captured_ctx[0], AgentRunContext)
        assert captured_ctx[0].run_id == "run-1"
        assert captured_ctx[0].attempt == 2

    def test_agent_run_returns_none_without_handler(self):
        agent = define_agent(AgentOptions(
            name="Test",
            slug="test",
            project_id="p1",
            endpoint_url="http://localhost",
        ))
        base_ctx = RunContext(run_id="run-1")
        result = agent.run({}, base_ctx)
        assert result is None


class TestAgentRunContext:
    def test_initial_iteration(self):
        ctx = AgentRunContext(run_id="r1")
        assert ctx.iteration == 0

    def test_initial_cost(self):
        ctx = AgentRunContext(run_id="r1")
        assert ctx.accumulated_cost_microusd() == 0

    def test_is_budget_exceeded_false(self):
        ctx = AgentRunContext(run_id="r1", _max_cost_microusd=1000)
        assert ctx.is_budget_exceeded() is False

    def test_is_budget_exceeded_true(self):
        ctx = AgentRunContext(run_id="r1", _accumulated_cost_microusd=1000, _max_cost_microusd=1000)
        assert ctx.is_budget_exceeded() is True

    def test_is_budget_exceeded_over(self):
        ctx = AgentRunContext(run_id="r1", _accumulated_cost_microusd=1500, _max_cost_microusd=1000)
        assert ctx.is_budget_exceeded() is True

    def test_default_budget_is_infinite(self):
        ctx = AgentRunContext(run_id="r1")
        assert ctx.is_budget_exceeded() is False
        assert ctx._max_cost_microusd == float("inf")

    async def test_wrapped_checkpoint_increments_iteration(self):
        checkpoints: list[Any] = []

        async def mock_checkpoint(state: dict[str, Any]) -> None:
            checkpoints.append(state)

        captured_ctx: list[AgentRunContext] = []

        def handler(payload: Any, ctx: AgentRunContext) -> Any:
            captured_ctx.append(ctx)
            return None

        agent = define_agent(AgentOptions(
            name="Test", slug="test", project_id="p1", endpoint_url="http://localhost",
            run=handler, auto_checkpoint=True,
        ))

        base_ctx = RunContext(run_id="run-1", checkpoint=mock_checkpoint)
        agent.run({}, base_ctx)

        agent_ctx = captured_ctx[0]
        assert agent_ctx.iteration == 0
        await agent_ctx.checkpoint({"step": 1})
        assert agent_ctx.iteration == 1
        await agent_ctx.checkpoint({"step": 2})
        assert agent_ctx.iteration == 2
        assert len(checkpoints) == 2

    async def test_auto_checkpoint_false_skips_checkpoint(self):
        checkpoints: list[Any] = []

        async def mock_checkpoint(state: dict[str, Any]) -> None:
            checkpoints.append(state)

        captured_ctx: list[AgentRunContext] = []

        def handler(payload: Any, ctx: AgentRunContext) -> Any:
            captured_ctx.append(ctx)
            return None

        agent = define_agent(AgentOptions(
            name="Test", slug="test", project_id="p1", endpoint_url="http://localhost",
            run=handler, auto_checkpoint=False,
        ))

        base_ctx = RunContext(run_id="run-1", checkpoint=mock_checkpoint)
        agent.run({}, base_ctx)

        agent_ctx = captured_ctx[0]
        await agent_ctx.checkpoint({"step": 1})
        assert agent_ctx.iteration == 1
        assert len(checkpoints) == 0  # auto_checkpoint=False means no forwarding

    async def test_wrapped_report_usage_accumulates_cost(self):
        usage_reports: list[Any] = []

        async def mock_usage(**kwargs: Any) -> None:
            usage_reports.append(kwargs)

        captured_ctx: list[AgentRunContext] = []

        def handler(payload: Any, ctx: AgentRunContext) -> Any:
            captured_ctx.append(ctx)
            return None

        agent = define_agent(AgentOptions(
            name="Test", slug="test", project_id="p1", endpoint_url="http://localhost",
            run=handler, max_cost_microusd=10000,
        ))

        base_ctx = RunContext(run_id="run-1", report_usage=mock_usage)
        agent.run({}, base_ctx)

        agent_ctx = captured_ctx[0]
        await agent_ctx.report_usage(provider="openai", model="gpt-4", cost_microusd=500)
        assert agent_ctx.accumulated_cost_microusd() == 500

        await agent_ctx.report_usage(provider="openai", model="gpt-4", cost_microusd=300)
        assert agent_ctx.accumulated_cost_microusd() == 800
