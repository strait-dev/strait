"""Workflow definition DSL."""

from __future__ import annotations

import dataclasses
import json
from dataclasses import dataclass, field
from typing import Any, Callable, Generic, Protocol, TypeVar

from strait.authoring._dag_validation import validate_dag
from strait.authoring._run_context import RunContext
from strait.authoring._steps import Step, step_to_api

TPayload = TypeVar("TPayload")


class WorkflowDSLClient(Protocol):
    def create_workflow(self, body: dict[str, Any]) -> dict[str, Any]: ...
    def trigger_workflow(
        self, workflow_id: str, body: dict[str, Any],
    ) -> dict[str, Any]: ...
    def get_run(self, run_id: str) -> dict[str, Any]: ...


@dataclass
class TriggerWorkflowInput(Generic[TPayload]):
    payload: TPayload
    workflow_id: str = ""
    idempotency_key: str = ""
    priority: int | None = None
    dry_run: bool | None = None
    metadata: dict[str, str] | None = None
    step_overrides: dict[str, Any] | None = None


@dataclass
class WorkflowOptions(Generic[TPayload]):
    name: str = ""
    slug: str = ""
    steps: list[Step] = field(default_factory=list)
    project_id: str = ""
    description: str = ""
    tags: dict[str, str] | None = None
    environment_id: str = ""
    max_concurrent_runs: int | None = None
    max_parallel_steps: int | None = None
    timeout_secs: int | None = None
    max_attempts: int | None = None
    retry_strategy: str = ""
    cron: str = ""
    timezone: str = ""
    webhook_url: str = ""
    webhook_secret: str = ""
    run: Callable[[TPayload, RunContext], Any] | None = None
    on_success: Callable[[TPayload, Any, RunContext], None] | None = None
    on_failure: Callable[[TPayload, Exception, RunContext], None] | None = None


def _payload_to_dict(payload: Any) -> Any:
    if dataclasses.is_dataclass(payload) and not isinstance(payload, type):
        return dataclasses.asdict(payload)
    if isinstance(payload, dict):
        return payload
    data = json.dumps(payload)
    return json.loads(data)


class WorkflowDefinition(Generic[TPayload]):
    def __init__(self, opts: WorkflowOptions[TPayload]) -> None:
        self.kind = "workflow"
        self.slug = opts.slug
        self._opts = opts
        self.run = opts.run
        self.on_success = opts.on_success
        self.on_failure = opts.on_failure
        self._last_registered_workflow_id = ""

    def to_registration_body(self, project_id: str = "") -> dict[str, Any]:
        pid = project_id or self._opts.project_id
        if not pid:
            raise ValueError(f"define_workflow({self.slug}) requires project_id")

        if self._opts.steps:
            validate_dag(self._opts.steps)

        api_steps = [step_to_api(s) for s in self._opts.steps]

        body: dict[str, Any] = {
            "project_id": pid,
            "name": self._opts.name,
            "slug": self._opts.slug,
            "steps": api_steps,
        }

        if self._opts.description:
            body["description"] = self._opts.description
        if self._opts.tags:
            body["tags"] = self._opts.tags
        if self._opts.environment_id:
            body["environment_id"] = self._opts.environment_id
        if self._opts.max_concurrent_runs is not None:
            body["max_concurrent_runs"] = self._opts.max_concurrent_runs
        if self._opts.max_parallel_steps is not None:
            body["max_parallel_steps"] = self._opts.max_parallel_steps
        if self._opts.timeout_secs is not None:
            body["timeout_secs"] = self._opts.timeout_secs
        if self._opts.max_attempts is not None:
            body["max_attempts"] = self._opts.max_attempts
        if self._opts.retry_strategy:
            body["retry_strategy"] = self._opts.retry_strategy
        if self._opts.cron:
            body["cron"] = self._opts.cron
        if self._opts.timezone:
            body["timezone"] = self._opts.timezone
        if self._opts.webhook_url:
            body["webhook_url"] = self._opts.webhook_url
        if self._opts.webhook_secret:
            body["webhook_secret"] = self._opts.webhook_secret

        return body

    def register(
        self, client: WorkflowDSLClient, project_id: str = "",
    ) -> dict[str, Any]:
        body = self.to_registration_body(project_id)
        result = client.create_workflow(body)
        if isinstance(result.get("id"), str) and result["id"]:
            self._last_registered_workflow_id = result["id"]
        return result

    def trigger(
        self, client: WorkflowDSLClient, input: TriggerWorkflowInput[TPayload],
    ) -> dict[str, Any]:
        wf_id = input.workflow_id or self._last_registered_workflow_id
        if not wf_id:
            raise ValueError(
                f"define_workflow({self.slug}) trigger requires workflow_id "
                "or prior successful register()"
            )

        body: dict[str, Any] = {"payload": _payload_to_dict(input.payload)}
        if input.idempotency_key:
            body["idempotency_key"] = input.idempotency_key
        if input.priority is not None:
            body["priority"] = input.priority
        if input.dry_run is not None:
            body["dry_run"] = input.dry_run
        if input.metadata is not None:
            body["metadata"] = input.metadata
        if input.step_overrides is not None:
            body["step_overrides"] = input.step_overrides

        return client.trigger_workflow(wf_id, body)


def define_workflow(opts: WorkflowOptions[TPayload]) -> WorkflowDefinition[TPayload]:
    return WorkflowDefinition(opts)
