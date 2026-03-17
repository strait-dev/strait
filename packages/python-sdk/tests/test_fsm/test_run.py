"""Tests for run FSM — exhaustive table-driven transitions."""

import pytest

from strait.fsm._run import (
    RunEvent,
    RunStatus,
    can_transition_run,
    is_terminal_run_status,
    transition_run,
)

VALID_TRANSITIONS = [
    (RunStatus.DELAYED, RunEvent.ENQUEUE, RunStatus.QUEUED),
    (RunStatus.DELAYED, RunEvent.CANCEL, RunStatus.CANCELED),
    (RunStatus.DELAYED, RunEvent.EXPIRE, RunStatus.EXPIRED),
    (RunStatus.QUEUED, RunEvent.DEQUEUE, RunStatus.DEQUEUED),
    (RunStatus.QUEUED, RunEvent.CANCEL, RunStatus.CANCELED),
    (RunStatus.QUEUED, RunEvent.EXPIRE, RunStatus.EXPIRED),
    (RunStatus.DEQUEUED, RunEvent.EXECUTE, RunStatus.EXECUTING),
    (RunStatus.DEQUEUED, RunEvent.REQUEUE, RunStatus.QUEUED),
    (RunStatus.DEQUEUED, RunEvent.CANCEL, RunStatus.CANCELED),
    (RunStatus.DEQUEUED, RunEvent.SYSTEM_FAIL, RunStatus.SYSTEM_FAILED),
    (RunStatus.EXECUTING, RunEvent.COMPLETE, RunStatus.COMPLETED),
    (RunStatus.EXECUTING, RunEvent.FAIL, RunStatus.FAILED),
    (RunStatus.EXECUTING, RunEvent.TIMEOUT, RunStatus.TIMED_OUT),
    (RunStatus.EXECUTING, RunEvent.CRASH, RunStatus.CRASHED),
    (RunStatus.EXECUTING, RunEvent.CANCEL, RunStatus.CANCELED),
    (RunStatus.EXECUTING, RunEvent.WAIT, RunStatus.WAITING),
    (RunStatus.EXECUTING, RunEvent.REQUEUE, RunStatus.QUEUED),
    (RunStatus.EXECUTING, RunEvent.SYSTEM_FAIL, RunStatus.SYSTEM_FAILED),
    (RunStatus.EXECUTING, RunEvent.DEAD_LETTER, RunStatus.DEAD_LETTER),
    (RunStatus.WAITING, RunEvent.EXECUTE, RunStatus.EXECUTING),
    (RunStatus.WAITING, RunEvent.COMPLETE, RunStatus.COMPLETED),
    (RunStatus.WAITING, RunEvent.FAIL, RunStatus.FAILED),
    (RunStatus.WAITING, RunEvent.CANCEL, RunStatus.CANCELED),
    (RunStatus.WAITING, RunEvent.TIMEOUT, RunStatus.TIMED_OUT),
    (RunStatus.DEAD_LETTER, RunEvent.REQUEUE, RunStatus.QUEUED),
    (RunStatus.DEAD_LETTER, RunEvent.REPLAY, RunStatus.REPLAY_STAGED),
    (RunStatus.REPLAY_STAGED, RunEvent.ENQUEUE, RunStatus.QUEUED),
    (RunStatus.REPLAY_STAGED, RunEvent.CANCEL, RunStatus.CANCELED),
]


@pytest.mark.parametrize("from_status,event,expected", VALID_TRANSITIONS)
def test_valid_transition(from_status, event, expected):
    assert can_transition_run(from_status, event)
    assert transition_run(from_status, event) == expected


INVALID_TRANSITIONS = [
    (RunStatus.COMPLETED, RunEvent.EXECUTE),
    (RunStatus.FAILED, RunEvent.COMPLETE),
    (RunStatus.CANCELED, RunEvent.ENQUEUE),
    (RunStatus.EXPIRED, RunEvent.DEQUEUE),
    (RunStatus.TIMED_OUT, RunEvent.FAIL),
    (RunStatus.CRASHED, RunEvent.CANCEL),
    (RunStatus.SYSTEM_FAILED, RunEvent.REQUEUE),
    (RunStatus.DELAYED, RunEvent.EXECUTE),
    (RunStatus.QUEUED, RunEvent.COMPLETE),
]


@pytest.mark.parametrize("from_status,event", INVALID_TRANSITIONS)
def test_invalid_transition_rejected(from_status, event):
    assert not can_transition_run(from_status, event)
    with pytest.raises(ValueError):
        transition_run(from_status, event)


TERMINAL = [
    RunStatus.COMPLETED, RunStatus.FAILED, RunStatus.TIMED_OUT,
    RunStatus.CRASHED, RunStatus.SYSTEM_FAILED, RunStatus.CANCELED, RunStatus.EXPIRED,
]

NON_TERMINAL = [
    RunStatus.DELAYED, RunStatus.QUEUED, RunStatus.DEQUEUED,
    RunStatus.EXECUTING, RunStatus.WAITING, RunStatus.DEAD_LETTER, RunStatus.REPLAY_STAGED,
]


@pytest.mark.parametrize("status", TERMINAL)
def test_terminal_status(status):
    assert is_terminal_run_status(status)


@pytest.mark.parametrize("status", NON_TERMINAL)
def test_non_terminal_status(status):
    assert not is_terminal_run_status(status)
