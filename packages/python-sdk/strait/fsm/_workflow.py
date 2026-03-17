"""Workflow run lifecycle finite state machine."""

from __future__ import annotations

from enum import StrEnum


class WorkflowRunStatus(StrEnum):
    PENDING = "pending"
    RUNNING = "running"
    PAUSED = "paused"
    COMPLETED = "completed"
    FAILED = "failed"
    TIMED_OUT = "timed_out"
    CANCELED = "canceled"


class WorkflowRunEvent(StrEnum):
    START = "START"
    PAUSE = "PAUSE"
    RESUME = "RESUME"
    COMPLETE = "COMPLETE"
    FAIL = "FAIL"
    TIMEOUT = "TIMEOUT"
    CANCEL = "CANCEL"


_workflow_run_transitions: dict[WorkflowRunStatus, dict[WorkflowRunEvent, WorkflowRunStatus]] = {
    WorkflowRunStatus.PENDING: {
        WorkflowRunEvent.START: WorkflowRunStatus.RUNNING,
        WorkflowRunEvent.CANCEL: WorkflowRunStatus.CANCELED,
    },
    WorkflowRunStatus.RUNNING: {
        WorkflowRunEvent.PAUSE: WorkflowRunStatus.PAUSED,
        WorkflowRunEvent.COMPLETE: WorkflowRunStatus.COMPLETED,
        WorkflowRunEvent.FAIL: WorkflowRunStatus.FAILED,
        WorkflowRunEvent.TIMEOUT: WorkflowRunStatus.TIMED_OUT,
        WorkflowRunEvent.CANCEL: WorkflowRunStatus.CANCELED,
    },
    WorkflowRunStatus.PAUSED: {
        WorkflowRunEvent.RESUME: WorkflowRunStatus.RUNNING,
        WorkflowRunEvent.COMPLETE: WorkflowRunStatus.COMPLETED,
        WorkflowRunEvent.FAIL: WorkflowRunStatus.FAILED,
        WorkflowRunEvent.TIMEOUT: WorkflowRunStatus.TIMED_OUT,
        WorkflowRunEvent.CANCEL: WorkflowRunStatus.CANCELED,
    },
}

_terminal_workflow_run_statuses: frozenset[WorkflowRunStatus] = frozenset({
    WorkflowRunStatus.COMPLETED,
    WorkflowRunStatus.FAILED,
    WorkflowRunStatus.TIMED_OUT,
    WorkflowRunStatus.CANCELED,
})


def can_transition_workflow_run(
    from_status: WorkflowRunStatus, event: WorkflowRunEvent,
) -> bool:
    events = _workflow_run_transitions.get(from_status)
    if events is None:
        return False
    return event in events


def transition_workflow_run(
    from_status: WorkflowRunStatus, event: WorkflowRunEvent,
) -> WorkflowRunStatus:
    events = _workflow_run_transitions.get(from_status)
    if events is None:
        raise ValueError(f"invalid workflow run status {from_status!r}")
    next_status = events.get(event)
    if next_status is None:
        raise ValueError(f"invalid transition: {from_status!r} + {event!r}")
    return next_status


def is_terminal_workflow_run_status(status: WorkflowRunStatus) -> bool:
    return status in _terminal_workflow_run_statuses
