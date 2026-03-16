"""Agent definition DSL — wraps defineJob with agent conventions."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable

from strait.authoring._job import JobDefinition, JobOptions, define_job
from strait.authoring._run_context import RunContext


@dataclass
class AgentRunContext(RunContext):
    iteration: int = 0
    _accumulated_cost_microusd: int = 0
    _max_cost_microusd: float = float("inf")

    def accumulated_cost_microusd(self) -> int:
        return self._accumulated_cost_microusd

    def is_budget_exceeded(self) -> bool:
        return self._accumulated_cost_microusd >= self._max_cost_microusd


@dataclass
class AgentOptions:
    name: str = ""
    slug: str = ""
    endpoint_url: str = ""
    project_id: str = ""
    description: str = ""
    tags: dict[str, str] | None = None
    max_iterations: int | None = None
    max_cost_microusd: int | None = None
    auto_checkpoint: bool = True
    timeout_secs: int | None = None
    max_attempts: int | None = None
    retry_strategy: str = ""
    run: Callable[..., Any] | None = None
    on_success: Callable[..., Any] | None = None
    on_failure: Callable[..., Any] | None = None
    on_start: Callable[..., Any] | None = None


def define_agent(opts: AgentOptions) -> JobDefinition[Any]:
    max_cost = opts.max_cost_microusd if opts.max_cost_microusd is not None else float("inf")
    auto_checkpoint = opts.auto_checkpoint
    user_run = opts.run

    def agent_run(payload: Any, ctx: RunContext) -> Any:
        accumulated_cost = 0
        iteration = 0

        agent_ctx = AgentRunContext(
            run_id=ctx.run_id,
            attempt=ctx.attempt,
            logger=ctx.logger,
            checkpoint=None,
            report_progress=ctx.report_progress,
            heartbeat=ctx.heartbeat,
            report_usage=None,
            log_tool_call=ctx.log_tool_call,
            save_output=ctx.save_output,
            state=ctx.state,
            stream_chunk=ctx.stream_chunk,
            wait_for_event=ctx.wait_for_event,
            spawn=ctx.spawn,
            continue_run=ctx.continue_run,
            annotate=ctx.annotate,
            complete=ctx.complete,
            fail=ctx.fail,
            iteration=iteration,
            _accumulated_cost_microusd=0,
            _max_cost_microusd=max_cost,
        )

        original_report_usage = ctx.report_usage

        async def wrapped_report_usage(
            provider: str,
            model: str,
            prompt_tokens: int | None = None,
            completion_tokens: int | None = None,
            total_tokens: int | None = None,
            cost_microusd: int | None = None,
            **kwargs: Any,
        ) -> None:
            nonlocal accumulated_cost
            if cost_microusd is not None:
                accumulated_cost += cost_microusd
                agent_ctx._accumulated_cost_microusd = accumulated_cost
            if original_report_usage is not None:
                await original_report_usage(
                    provider=provider,
                    model=model,
                    prompt_tokens=prompt_tokens,
                    completion_tokens=completion_tokens,
                    total_tokens=total_tokens,
                    cost_microusd=cost_microusd,
                )

        if ctx.report_usage is not None:
            agent_ctx.report_usage = wrapped_report_usage

        original_checkpoint = ctx.checkpoint

        async def wrapped_checkpoint(state: dict[str, Any]) -> None:
            nonlocal iteration
            iteration += 1
            agent_ctx.iteration = iteration
            if auto_checkpoint and original_checkpoint is not None:
                await original_checkpoint(state)

        agent_ctx.checkpoint = wrapped_checkpoint

        if user_run is not None:
            return user_run(payload, agent_ctx)
        return None

    tags = dict(opts.tags or {})
    tags["strait.kind"] = "agent"

    return define_job(JobOptions(
        name=opts.name,
        slug=opts.slug,
        endpoint_url=opts.endpoint_url,
        project_id=opts.project_id,
        description=opts.description,
        tags=tags,
        timeout_secs=opts.timeout_secs or 600,
        max_attempts=opts.max_attempts or 5,
        retry_strategy=opts.retry_strategy or "exponential",
        run=agent_run,
        on_success=opts.on_success,
        on_failure=opts.on_failure,
        on_start=opts.on_start,
    ))
