"""Job definition DSL."""

from __future__ import annotations

import dataclasses
import json
from dataclasses import dataclass
from typing import Any, Callable, Generic, Protocol, TypeVar

from strait.authoring._run_context import RunContext

TPayload = TypeVar("TPayload")


class JobDSLClient(Protocol):
    def create_job(self, body: dict[str, Any]) -> dict[str, Any]: ...
    def trigger_job(self, job_id: str, body: dict[str, Any]) -> dict[str, Any]: ...
    def bulk_trigger_job(self, job_id: str, body: dict[str, Any]) -> dict[str, Any]: ...
    def get_run(self, run_id: str) -> dict[str, Any]: ...


@dataclass
class TriggerJobInput(Generic[TPayload]):
    payload: TPayload
    job_id: str = ""
    idempotency_key: str = ""
    priority: int | None = None
    dry_run: bool | None = None
    metadata: dict[str, str] | None = None
    scheduled_at: str = ""


@dataclass
class JobOptions(Generic[TPayload]):
    name: str = ""
    slug: str = ""
    endpoint_url: str = ""
    project_id: str = ""
    description: str = ""
    group_id: str = ""
    tags: dict[str, str] | None = None
    environment_id: str = ""
    cron: str = ""
    timezone: str = ""
    execution_window_cron: str = ""
    max_concurrency: int | None = None
    rate_limit_max: int | None = None
    rate_limit_window_secs: int | None = None
    max_attempts: int | None = None
    retry_strategy: str = ""
    retry_delays_secs: list[int] | None = None
    timeout_secs: int | None = None
    run_ttl_secs: int | None = None
    dedup_window_secs: int | None = None
    webhook_url: str = ""
    webhook_secret: str = ""
    fallback_endpoint_url: str = ""
    run: Callable[[TPayload, RunContext], Any] | None = None
    on_success: Callable[[TPayload, Any, RunContext], None] | None = None
    on_failure: Callable[[TPayload, Exception, RunContext], None] | None = None
    on_start: Callable[[TPayload, RunContext], None] | None = None


def _payload_to_dict(payload: Any) -> Any:
    if dataclasses.is_dataclass(payload) and not isinstance(payload, type):
        return dataclasses.asdict(payload)
    if isinstance(payload, dict):
        return payload
    data = json.dumps(payload)
    return json.loads(data)


def _build_trigger_body(input: TriggerJobInput[Any]) -> dict[str, Any]:
    body: dict[str, Any] = {"payload": _payload_to_dict(input.payload)}
    if input.idempotency_key:
        body["idempotency_key"] = input.idempotency_key
    if input.priority is not None:
        body["priority"] = input.priority
    if input.dry_run is not None:
        body["dry_run"] = input.dry_run
    if input.metadata is not None:
        body["metadata"] = input.metadata
    if input.scheduled_at:
        body["scheduled_at"] = input.scheduled_at
    return body


class JobDefinition(Generic[TPayload]):
    def __init__(self, opts: JobOptions[TPayload]) -> None:
        self.kind = "job"
        self.slug = opts.slug
        self._opts = opts
        self.run = opts.run
        self.on_success = opts.on_success
        self.on_failure = opts.on_failure
        self.on_start = opts.on_start
        self._last_registered_job_id = ""

    def to_registration_body(self, project_id: str = "") -> dict[str, Any]:
        pid = project_id or self._opts.project_id
        if not pid:
            raise ValueError(f"define_job({self.slug}) requires project_id")

        body: dict[str, Any] = {
            "project_id": pid,
            "name": self._opts.name,
            "slug": self._opts.slug,
            "endpoint_url": self._opts.endpoint_url,
        }

        _set_opt_str(body, "description", self._opts.description)
        _set_opt_str(body, "group_id", self._opts.group_id)
        _set_opt_map(body, "tags", self._opts.tags)
        _set_opt_str(body, "environment_id", self._opts.environment_id)
        _set_opt_str(body, "cron", self._opts.cron)
        _set_opt_str(body, "timezone", self._opts.timezone)
        _set_opt_str(body, "execution_window_cron", self._opts.execution_window_cron)
        _set_opt_int(body, "max_concurrency", self._opts.max_concurrency)
        _set_opt_int(body, "rate_limit_max", self._opts.rate_limit_max)
        _set_opt_int(body, "rate_limit_window_secs", self._opts.rate_limit_window_secs)
        _set_opt_int(body, "max_attempts", self._opts.max_attempts)
        _set_opt_str(body, "retry_strategy", self._opts.retry_strategy)
        if self._opts.retry_delays_secs:
            body["retry_delays_secs"] = self._opts.retry_delays_secs
        _set_opt_int(body, "timeout_secs", self._opts.timeout_secs)
        _set_opt_int(body, "run_ttl_secs", self._opts.run_ttl_secs)
        _set_opt_int(body, "dedup_window_secs", self._opts.dedup_window_secs)
        _set_opt_str(body, "webhook_url", self._opts.webhook_url)
        _set_opt_str(body, "webhook_secret", self._opts.webhook_secret)
        _set_opt_str(body, "fallback_endpoint_url", self._opts.fallback_endpoint_url)

        return body

    def register(
        self, client: JobDSLClient, project_id: str = "",
    ) -> dict[str, Any]:
        body = self.to_registration_body(project_id)
        result = client.create_job(body)
        if isinstance(result.get("id"), str) and result["id"]:
            self._last_registered_job_id = result["id"]
        return result

    def trigger(
        self, client: JobDSLClient, input: TriggerJobInput[TPayload],
    ) -> dict[str, Any]:
        job_id = input.job_id or self._last_registered_job_id
        if not job_id:
            raise ValueError(
                f"define_job({self.slug}) trigger requires job_id or prior successful register()"
            )
        body = _build_trigger_body(input)
        return client.trigger_job(job_id, body)

    def batch_trigger(
        self,
        client: JobDSLClient,
        items: list[TriggerJobInput[TPayload]],
        job_id: str = "",
    ) -> dict[str, Any]:
        jid = job_id or self._last_registered_job_id
        if not jid:
            raise ValueError(f"define_job({self.slug}) batch_trigger requires job_id")
        trigger_items = [_build_trigger_body(item) for item in items]
        return client.bulk_trigger_job(jid, {"items": trigger_items})


def define_job(opts: JobOptions[TPayload]) -> JobDefinition[TPayload]:
    return JobDefinition(opts)


def _set_opt_str(m: dict[str, Any], key: str, val: str) -> None:
    if val:
        m[key] = val


def _set_opt_int(m: dict[str, Any], key: str, val: int | None) -> None:
    if val is not None:
        m[key] = val


def _set_opt_map(m: dict[str, Any], key: str, val: dict[str, Any] | None) -> None:
    if val:
        m[key] = val
