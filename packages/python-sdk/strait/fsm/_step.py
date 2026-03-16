"""Step run lifecycle finite state machine."""

from __future__ import annotations

from enum import StrEnum


class StepRunStatus(StrEnum):
    PENDING = "pending"
    WAITING = "waiting"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    SKIPPED = "skipped"
    CANCELED = "canceled"


class StepRunEvent(StrEnum):
    WAIT = "WAIT"
    START = "START"
    COMPLETE = "COMPLETE"
    FAIL = "FAIL"
    SKIP = "SKIP"
    CANCEL = "CANCEL"


_step_run_transitions: dict[StepRunStatus, dict[StepRunEvent, StepRunStatus]] = {
    StepRunStatus.PENDING: {
        StepRunEvent.WAIT: StepRunStatus.WAITING,
        StepRunEvent.START: StepRunStatus.RUNNING,
        StepRunEvent.SKIP: StepRunStatus.SKIPPED,
        StepRunEvent.CANCEL: StepRunStatus.CANCELED,
        StepRunEvent.COMPLETE: StepRunStatus.COMPLETED,
    },
    StepRunStatus.WAITING: {
        StepRunEvent.START: StepRunStatus.RUNNING,
        StepRunEvent.SKIP: StepRunStatus.SKIPPED,
        StepRunEvent.CANCEL: StepRunStatus.CANCELED,
        StepRunEvent.COMPLETE: StepRunStatus.COMPLETED,
    },
    StepRunStatus.RUNNING: {
        StepRunEvent.COMPLETE: StepRunStatus.COMPLETED,
        StepRunEvent.FAIL: StepRunStatus.FAILED,
        StepRunEvent.CANCEL: StepRunStatus.CANCELED,
    },
}

_terminal_step_run_statuses: frozenset[StepRunStatus] = frozenset({
    StepRunStatus.COMPLETED,
    StepRunStatus.FAILED,
    StepRunStatus.SKIPPED,
    StepRunStatus.CANCELED,
})


def can_transition_step_run(from_status: StepRunStatus, event: StepRunEvent) -> bool:
    events = _step_run_transitions.get(from_status)
    if events is None:
        return False
    return event in events


def transition_step_run(from_status: StepRunStatus, event: StepRunEvent) -> StepRunStatus:
    events = _step_run_transitions.get(from_status)
    if events is None:
        raise ValueError(f"invalid step run status {from_status!r}")
    next_status = events.get(event)
    if next_status is None:
        raise ValueError(f"invalid transition: {from_status!r} + {event!r}")
    return next_status


def is_terminal_step_run_status(status: StepRunStatus) -> bool:
    return status in _terminal_step_run_statuses
