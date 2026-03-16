"""Run lifecycle finite state machine."""

from __future__ import annotations

from enum import StrEnum


class RunStatus(StrEnum):
    DELAYED = "delayed"
    QUEUED = "queued"
    DEQUEUED = "dequeued"
    EXECUTING = "executing"
    WAITING = "waiting"
    COMPLETED = "completed"
    FAILED = "failed"
    TIMED_OUT = "timed_out"
    CRASHED = "crashed"
    SYSTEM_FAILED = "system_failed"
    CANCELED = "canceled"
    EXPIRED = "expired"
    DEAD_LETTER = "dead_letter"
    REPLAY_STAGED = "replay_staged"


class RunEvent(StrEnum):
    ENQUEUE = "ENQUEUE"
    DEQUEUE = "DEQUEUE"
    EXECUTE = "EXECUTE"
    COMPLETE = "COMPLETE"
    FAIL = "FAIL"
    TIMEOUT = "TIMEOUT"
    CRASH = "CRASH"
    SYSTEM_FAIL = "SYSTEM_FAIL"
    CANCEL = "CANCEL"
    EXPIRE = "EXPIRE"
    WAIT = "WAIT"
    REQUEUE = "REQUEUE"
    DEAD_LETTER = "DEAD_LETTER"
    REPLAY = "REPLAY"


_run_transitions: dict[RunStatus, dict[RunEvent, RunStatus]] = {
    RunStatus.DELAYED: {
        RunEvent.ENQUEUE: RunStatus.QUEUED,
        RunEvent.CANCEL: RunStatus.CANCELED,
        RunEvent.EXPIRE: RunStatus.EXPIRED,
    },
    RunStatus.QUEUED: {
        RunEvent.DEQUEUE: RunStatus.DEQUEUED,
        RunEvent.CANCEL: RunStatus.CANCELED,
        RunEvent.EXPIRE: RunStatus.EXPIRED,
    },
    RunStatus.DEQUEUED: {
        RunEvent.EXECUTE: RunStatus.EXECUTING,
        RunEvent.REQUEUE: RunStatus.QUEUED,
        RunEvent.CANCEL: RunStatus.CANCELED,
        RunEvent.SYSTEM_FAIL: RunStatus.SYSTEM_FAILED,
    },
    RunStatus.EXECUTING: {
        RunEvent.COMPLETE: RunStatus.COMPLETED,
        RunEvent.FAIL: RunStatus.FAILED,
        RunEvent.TIMEOUT: RunStatus.TIMED_OUT,
        RunEvent.CRASH: RunStatus.CRASHED,
        RunEvent.CANCEL: RunStatus.CANCELED,
        RunEvent.WAIT: RunStatus.WAITING,
        RunEvent.REQUEUE: RunStatus.QUEUED,
        RunEvent.SYSTEM_FAIL: RunStatus.SYSTEM_FAILED,
        RunEvent.DEAD_LETTER: RunStatus.DEAD_LETTER,
    },
    RunStatus.WAITING: {
        RunEvent.EXECUTE: RunStatus.EXECUTING,
        RunEvent.COMPLETE: RunStatus.COMPLETED,
        RunEvent.FAIL: RunStatus.FAILED,
        RunEvent.CANCEL: RunStatus.CANCELED,
        RunEvent.TIMEOUT: RunStatus.TIMED_OUT,
    },
    RunStatus.DEAD_LETTER: {
        RunEvent.REQUEUE: RunStatus.QUEUED,
        RunEvent.REPLAY: RunStatus.REPLAY_STAGED,
    },
    RunStatus.REPLAY_STAGED: {
        RunEvent.ENQUEUE: RunStatus.QUEUED,
        RunEvent.CANCEL: RunStatus.CANCELED,
    },
}

_terminal_run_statuses: frozenset[RunStatus] = frozenset({
    RunStatus.COMPLETED,
    RunStatus.FAILED,
    RunStatus.TIMED_OUT,
    RunStatus.CRASHED,
    RunStatus.SYSTEM_FAILED,
    RunStatus.CANCELED,
    RunStatus.EXPIRED,
})


def can_transition_run(from_status: RunStatus, event: RunEvent) -> bool:
    events = _run_transitions.get(from_status)
    if events is None:
        return False
    return event in events


def transition_run(from_status: RunStatus, event: RunEvent) -> RunStatus:
    events = _run_transitions.get(from_status)
    if events is None:
        raise ValueError(f"invalid run status {from_status!r}")
    next_status = events.get(event)
    if next_status is None:
        raise ValueError(f"invalid transition: {from_status!r} + {event!r}")
    return next_status


def is_terminal_run_status(status: RunStatus) -> bool:
    return status in _terminal_run_statuses
