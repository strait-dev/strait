"""Factory for creating a RunContext wired to HTTP endpoints."""

from __future__ import annotations

import asyncio
import logging
from typing import Any, Protocol

from strait.authoring._run_context import RunContext, RunContextState


class RunContextClient(Protocol):
    def checkpoint_run(self, run_id: str, body: Any) -> Any: ...
    def heartbeat_run(self, run_id: str) -> Any: ...
    def progress_run(self, run_id: str, body: Any) -> Any: ...
    def log_run(self, run_id: str, body: Any) -> Any: ...
    def usage_run(self, run_id: str, body: Any) -> Any: ...
    def tool_call_run(self, run_id: str, body: Any) -> Any: ...
    def output_run(self, run_id: str, body: Any) -> Any: ...
    def wait_for_event_run(self, run_id: str, body: Any) -> Any: ...
    def spawn_run(self, run_id: str, body: Any) -> Any: ...
    def continue_run(self, run_id: str, body: Any | None) -> Any: ...
    def annotate_run(self, run_id: str, body: Any) -> Any: ...
    def complete_run(self, run_id: str, body: Any) -> Any: ...
    def fail_run(self, run_id: str, body: Any) -> Any: ...
    def set_state(self, run_id: str, body: Any) -> Any: ...
    def list_state(self, run_id: str) -> Any: ...
    def get_state(self, run_id: str, key: str) -> Any: ...
    def delete_state(self, run_id: str, key: str) -> Any: ...
    def stream_run(self, run_id: str, body: Any) -> Any: ...


def _fire_and_forget(coro: Any) -> None:
    try:
        loop = asyncio.get_running_loop()
        loop.create_task(coro) if asyncio.iscoroutine(coro) else None
    except RuntimeError:
        pass


def create_run_context(
    client: RunContextClient,
    run_id: str,
    attempt: int = 1,
) -> RunContext:
    async def checkpoint(state: dict[str, Any]) -> None:
        await client.checkpoint_run(run_id, {"state": state, "source": "sdk"})

    async def report_progress(percent: float, message: str | None = None) -> None:
        body: dict[str, Any] = {"percent": percent}
        if message is not None:
            body["message"] = message
        await client.progress_run(run_id, body)

    async def heartbeat() -> None:
        await client.heartbeat_run(run_id)

    async def report_usage(
        provider: str,
        model: str,
        prompt_tokens: int | None = None,
        completion_tokens: int | None = None,
        total_tokens: int | None = None,
        cost_microusd: int | None = None,
    ) -> None:
        body: dict[str, Any] = {"provider": provider, "model": model}
        if prompt_tokens is not None:
            body["prompt_tokens"] = prompt_tokens
        if completion_tokens is not None:
            body["completion_tokens"] = completion_tokens
        if total_tokens is not None:
            body["total_tokens"] = total_tokens
        if cost_microusd is not None:
            body["cost_microusd"] = cost_microusd
        await client.usage_run(run_id, body)

    async def log_tool_call(
        tool_name: str,
        input: dict[str, Any] | None = None,
        output: dict[str, Any] | None = None,
        duration_ms: int | None = None,
        status: str | None = None,
    ) -> None:
        body: dict[str, Any] = {"tool_name": tool_name}
        if input is not None:
            body["input"] = input
        if output is not None:
            body["output"] = output
        if duration_ms is not None:
            body["duration_ms"] = duration_ms
        if status is not None:
            body["status"] = status
        await client.tool_call_run(run_id, body)

    async def save_output(
        key: str,
        value: dict[str, Any],
        schema: dict[str, Any] | None = None,
    ) -> None:
        body: dict[str, Any] = {"key": key, "value": value}
        if schema is not None:
            body["schema"] = schema
        await client.output_run(run_id, body)

    async def wait_for_event(
        event_key: str,
        timeout_secs: int | None = None,
        notify_url: str | None = None,
    ) -> dict[str, Any]:
        body: dict[str, Any] = {"event_key": event_key}
        if timeout_secs is not None:
            body["timeout_secs"] = timeout_secs
        if notify_url is not None:
            body["notify_url"] = notify_url
        return await client.wait_for_event_run(run_id, body)

    async def spawn(
        job_slug: str,
        project_id: str,
        payload: dict[str, Any] | None = None,
        priority: int | None = None,
    ) -> dict[str, Any]:
        body: dict[str, Any] = {"job_slug": job_slug, "project_id": project_id}
        if payload is not None:
            body["payload"] = payload
        if priority is not None:
            body["priority"] = priority
        return await client.spawn_run(run_id, body)

    async def continue_run_fn(payload: dict[str, Any] | None = None) -> dict[str, Any]:
        body = {"payload": payload} if payload else None
        return await client.continue_run(run_id, body)

    async def annotate(annotations: dict[str, str]) -> None:
        await client.annotate_run(run_id, {"annotations": annotations})

    async def complete(result: dict[str, Any] | None = None) -> None:
        body = {"result": result} if result else None
        await client.complete_run(run_id, body)

    async def fail(error: str) -> None:
        await client.fail_run(run_id, {"error": error})

    async def stream_chunk(
        chunk: str,
        stream_id: str | None = None,
        done: bool | None = None,
    ) -> None:
        body: dict[str, Any] = {"chunk": chunk}
        if stream_id is not None:
            body["stream_id"] = stream_id
        if done is not None:
            body["done"] = done
        await client.stream_run(run_id, body)

    # State KV store
    async def state_get(key: str) -> Any:
        return await client.get_state(run_id, key)

    async def state_set(key: str, value: Any) -> None:
        await client.set_state(run_id, {"key": key, "value": value})

    async def state_delete(key: str) -> None:
        await client.delete_state(run_id, key)

    async def state_list() -> list[dict[str, Any]]:
        return await client.list_state(run_id)

    state = RunContextState(
        get=state_get,
        set=state_set,
        delete=state_delete,
        list=state_list,
    )

    # Logger with fire-and-forget
    ctx_logger = logging.getLogger(f"strait.run.{run_id}")

    class _LogHandler(logging.Handler):
        def emit(self, record: logging.LogRecord) -> None:
            level = record.levelname.lower()
            if level == "warning":
                level = "warn"
            _fire_and_forget(
                client.log_run(run_id, {"level": level, "message": record.getMessage()})
            )

    ctx_logger.addHandler(_LogHandler())

    return RunContext(
        run_id=run_id,
        attempt=attempt,
        logger=ctx_logger,
        checkpoint=checkpoint,
        report_progress=report_progress,
        heartbeat=heartbeat,
        report_usage=report_usage,
        log_tool_call=log_tool_call,
        save_output=save_output,
        state=state,
        stream_chunk=stream_chunk,
        wait_for_event=wait_for_event,
        spawn=spawn,
        continue_run=continue_run_fn,
        annotate=annotate,
        complete=complete,
        fail=fail,
    )
