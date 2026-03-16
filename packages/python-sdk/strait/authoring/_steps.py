"""Workflow step types and builders."""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import StrEnum
from typing import Any, Protocol, runtime_checkable


class StepType(StrEnum):
    JOB = "job"
    APPROVAL = "approval"
    SUB_WORKFLOW = "sub_workflow"
    WAIT_FOR_EVENT = "wait_for_event"
    SLEEP = "sleep"


class OnFailureAction(StrEnum):
    FAIL_WORKFLOW = "fail_workflow"
    SKIP_DEPENDENTS = "skip_dependents"
    CONTINUE = "continue"


class ResourceClass(StrEnum):
    SMALL = "small"
    MEDIUM = "medium"
    LARGE = "large"


class RetryBackoff(StrEnum):
    EXPONENTIAL = "exponential"
    FIXED = "fixed"


@dataclass
class BaseStepOptions:
    depends_on: list[str] = field(default_factory=list)
    condition: dict[str, Any] | None = None
    on_failure: OnFailureAction | None = None
    payload: dict[str, Any] | None = None
    retry_max_attempts: int | None = None
    retry_backoff: RetryBackoff | None = None
    retry_initial_delay_secs: int | None = None
    retry_max_delay_secs: int | None = None
    timeout_secs_override: int | None = None
    output_transform: str | None = None
    concurrency_key: str | None = None
    resource_class: ResourceClass | None = None


@runtime_checkable
class Step(Protocol):
    def step_ref(self) -> str: ...
    def type(self) -> StepType: ...
    def base_options(self) -> BaseStepOptions: ...


@dataclass
class JobStep:
    ref: str
    job_id: str
    options: BaseStepOptions = field(default_factory=BaseStepOptions)

    def step_ref(self) -> str:
        return self.ref

    def type(self) -> StepType:
        return StepType.JOB

    def base_options(self) -> BaseStepOptions:
        return self.options


@dataclass
class ApprovalStep:
    ref: str
    approval_timeout_secs: int | None = None
    approvers: list[str] = field(default_factory=list)
    options: BaseStepOptions = field(default_factory=BaseStepOptions)

    def step_ref(self) -> str:
        return self.ref

    def type(self) -> StepType:
        return StepType.APPROVAL

    def base_options(self) -> BaseStepOptions:
        return self.options


@dataclass
class SubWorkflowStep:
    ref: str
    sub_workflow_id: str
    max_nesting_depth: int | None = None
    options: BaseStepOptions = field(default_factory=BaseStepOptions)

    def step_ref(self) -> str:
        return self.ref

    def type(self) -> StepType:
        return StepType.SUB_WORKFLOW

    def base_options(self) -> BaseStepOptions:
        return self.options


@dataclass
class WaitForEventStep:
    ref: str
    event_key: str
    event_timeout_secs: int | None = None
    event_notify_url: str | None = None
    options: BaseStepOptions = field(default_factory=BaseStepOptions)

    def step_ref(self) -> str:
        return self.ref

    def type(self) -> StepType:
        return StepType.WAIT_FOR_EVENT

    def base_options(self) -> BaseStepOptions:
        return self.options


@dataclass
class SleepStep:
    ref: str
    sleep_duration_secs: int
    options: BaseStepOptions = field(default_factory=BaseStepOptions)

    def step_ref(self) -> str:
        return self.ref

    def type(self) -> StepType:
        return StepType.SLEEP

    def base_options(self) -> BaseStepOptions:
        return self.options


def job_step(
    ref: str,
    job_id: str,
    *,
    depends_on: list[str] | None = None,
    on_failure: OnFailureAction | None = None,
    payload: dict[str, Any] | None = None,
    condition: dict[str, Any] | None = None,
    retry_max_attempts: int | None = None,
    retry_backoff: RetryBackoff | None = None,
    retry_initial_delay_secs: int | None = None,
    retry_max_delay_secs: int | None = None,
    timeout_secs_override: int | None = None,
    output_transform: str | None = None,
    concurrency_key: str | None = None,
    resource_class: ResourceClass | None = None,
) -> JobStep:
    return JobStep(
        ref=ref,
        job_id=job_id,
        options=BaseStepOptions(
            depends_on=depends_on or [],
            condition=condition,
            on_failure=on_failure,
            payload=payload,
            retry_max_attempts=retry_max_attempts,
            retry_backoff=retry_backoff,
            retry_initial_delay_secs=retry_initial_delay_secs,
            retry_max_delay_secs=retry_max_delay_secs,
            timeout_secs_override=timeout_secs_override,
            output_transform=output_transform,
            concurrency_key=concurrency_key,
            resource_class=resource_class,
        ),
    )


def approval_step(
    ref: str,
    *,
    approval_timeout_secs: int | None = None,
    approvers: list[str] | None = None,
    depends_on: list[str] | None = None,
    on_failure: OnFailureAction | None = None,
    condition: dict[str, Any] | None = None,
) -> ApprovalStep:
    return ApprovalStep(
        ref=ref,
        approval_timeout_secs=approval_timeout_secs,
        approvers=approvers or [],
        options=BaseStepOptions(
            depends_on=depends_on or [],
            condition=condition,
            on_failure=on_failure,
        ),
    )


def sub_workflow_step(
    ref: str,
    sub_workflow_id: str,
    *,
    max_nesting_depth: int | None = None,
    depends_on: list[str] | None = None,
    on_failure: OnFailureAction | None = None,
    payload: dict[str, Any] | None = None,
    condition: dict[str, Any] | None = None,
) -> SubWorkflowStep:
    return SubWorkflowStep(
        ref=ref,
        sub_workflow_id=sub_workflow_id,
        max_nesting_depth=max_nesting_depth,
        options=BaseStepOptions(
            depends_on=depends_on or [],
            condition=condition,
            on_failure=on_failure,
            payload=payload,
        ),
    )


def wait_for_event_step(
    ref: str,
    event_key: str,
    *,
    event_timeout_secs: int | None = None,
    event_notify_url: str | None = None,
    depends_on: list[str] | None = None,
    on_failure: OnFailureAction | None = None,
    condition: dict[str, Any] | None = None,
) -> WaitForEventStep:
    return WaitForEventStep(
        ref=ref,
        event_key=event_key,
        event_timeout_secs=event_timeout_secs,
        event_notify_url=event_notify_url,
        options=BaseStepOptions(
            depends_on=depends_on or [],
            condition=condition,
            on_failure=on_failure,
        ),
    )


def sleep_step(
    ref: str,
    duration_secs: int,
    *,
    depends_on: list[str] | None = None,
    on_failure: OnFailureAction | None = None,
    condition: dict[str, Any] | None = None,
) -> SleepStep:
    return SleepStep(
        ref=ref,
        sleep_duration_secs=duration_secs,
        options=BaseStepOptions(
            depends_on=depends_on or [],
            condition=condition,
            on_failure=on_failure,
        ),
    )


def step_to_api(step: Step) -> dict[str, Any]:
    """Convert a Step to the snake_case API format."""
    out: dict[str, Any] = {
        "step_ref": step.step_ref(),
        "type": str(step.type()),
    }

    base = step.base_options()
    if base.depends_on:
        out["depends_on"] = base.depends_on
    if base.condition is not None:
        out["condition"] = base.condition
    if base.on_failure is not None:
        out["on_failure"] = str(base.on_failure)
    if base.payload is not None:
        out["payload"] = base.payload
    if base.retry_max_attempts is not None:
        out["retry_max_attempts"] = base.retry_max_attempts
    if base.retry_backoff is not None:
        out["retry_backoff"] = str(base.retry_backoff)
    if base.retry_initial_delay_secs is not None:
        out["retry_initial_delay_secs"] = base.retry_initial_delay_secs
    if base.retry_max_delay_secs is not None:
        out["retry_max_delay_secs"] = base.retry_max_delay_secs
    if base.timeout_secs_override is not None:
        out["timeout_secs_override"] = base.timeout_secs_override
    if base.output_transform is not None:
        out["output_transform"] = base.output_transform
    if base.concurrency_key is not None:
        out["concurrency_key"] = base.concurrency_key
    if base.resource_class is not None:
        out["resource_class"] = str(base.resource_class)

    if isinstance(step, JobStep):
        out["job_id"] = step.job_id
    elif isinstance(step, ApprovalStep):
        if step.approval_timeout_secs is not None:
            out["approval_timeout_secs"] = step.approval_timeout_secs
        if step.approvers:
            out["approvers"] = step.approvers
    elif isinstance(step, SubWorkflowStep):
        out["sub_workflow_id"] = step.sub_workflow_id
        if step.max_nesting_depth is not None:
            out["max_nesting_depth"] = step.max_nesting_depth
    elif isinstance(step, WaitForEventStep):
        out["event_key"] = step.event_key
        if step.event_timeout_secs is not None:
            out["event_timeout_secs"] = step.event_timeout_secs
        if step.event_notify_url is not None:
            out["event_notify_url"] = step.event_notify_url
    elif isinstance(step, SleepStep):
        out["sleep_duration_secs"] = step.sleep_duration_secs

    return out


def ai_step(
    ref: str,
    job_id: str,
    *,
    depends_on: list[str] | None = None,
    on_failure: OnFailureAction | None = None,
    payload: dict[str, Any] | None = None,
    condition: dict[str, Any] | None = None,
    retry_max_attempts: int | None = None,
    retry_backoff: RetryBackoff | None = None,
    retry_initial_delay_secs: int | None = None,
    retry_max_delay_secs: int | None = None,
    timeout_secs_override: int | None = None,
    output_transform: str | None = None,
    concurrency_key: str | None = None,
    resource_class: ResourceClass | None = None,
) -> JobStep:
    """Create a job step with LLM-tuned defaults."""
    return JobStep(
        ref=ref,
        job_id=job_id,
        options=BaseStepOptions(
            depends_on=depends_on or [],
            condition=condition,
            on_failure=on_failure,
            payload=payload,
            retry_max_attempts=retry_max_attempts if retry_max_attempts is not None else 5,
            retry_backoff=retry_backoff or RetryBackoff.EXPONENTIAL,
            retry_initial_delay_secs=(
                retry_initial_delay_secs if retry_initial_delay_secs is not None else 2
            ),
            retry_max_delay_secs=(
                retry_max_delay_secs if retry_max_delay_secs is not None else 120
            ),
            timeout_secs_override=(
                timeout_secs_override if timeout_secs_override is not None else 600
            ),
            output_transform=output_transform,
            concurrency_key=concurrency_key,
            resource_class=resource_class or ResourceClass.LARGE,
        ),
    )
