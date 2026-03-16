"""Tests for step run FSM."""

import pytest

from strait.fsm._step import (
    StepRunEvent,
    StepRunStatus,
    can_transition_step_run,
    is_terminal_step_run_status,
    transition_step_run,
)

VALID_TRANSITIONS = [
    (StepRunStatus.PENDING, StepRunEvent.WAIT, StepRunStatus.WAITING),
    (StepRunStatus.PENDING, StepRunEvent.START, StepRunStatus.RUNNING),
    (StepRunStatus.PENDING, StepRunEvent.SKIP, StepRunStatus.SKIPPED),
    (StepRunStatus.PENDING, StepRunEvent.CANCEL, StepRunStatus.CANCELED),
    (StepRunStatus.PENDING, StepRunEvent.COMPLETE, StepRunStatus.COMPLETED),
    (StepRunStatus.WAITING, StepRunEvent.START, StepRunStatus.RUNNING),
    (StepRunStatus.WAITING, StepRunEvent.SKIP, StepRunStatus.SKIPPED),
    (StepRunStatus.WAITING, StepRunEvent.CANCEL, StepRunStatus.CANCELED),
    (StepRunStatus.WAITING, StepRunEvent.COMPLETE, StepRunStatus.COMPLETED),
    (StepRunStatus.RUNNING, StepRunEvent.COMPLETE, StepRunStatus.COMPLETED),
    (StepRunStatus.RUNNING, StepRunEvent.FAIL, StepRunStatus.FAILED),
    (StepRunStatus.RUNNING, StepRunEvent.CANCEL, StepRunStatus.CANCELED),
]


@pytest.mark.parametrize("from_status,event,expected", VALID_TRANSITIONS)
def test_valid_transition(from_status, event, expected):
    assert can_transition_step_run(from_status, event)
    assert transition_step_run(from_status, event) == expected


INVALID_TRANSITIONS = [
    (StepRunStatus.COMPLETED, StepRunEvent.FAIL),
    (StepRunStatus.FAILED, StepRunEvent.COMPLETE),
    (StepRunStatus.SKIPPED, StepRunEvent.START),
    (StepRunStatus.CANCELED, StepRunEvent.START),
    (StepRunStatus.RUNNING, StepRunEvent.WAIT),
    (StepRunStatus.RUNNING, StepRunEvent.SKIP),
]


@pytest.mark.parametrize("from_status,event", INVALID_TRANSITIONS)
def test_invalid_transition_rejected(from_status, event):
    assert not can_transition_step_run(from_status, event)
    with pytest.raises(ValueError):
        transition_step_run(from_status, event)


TERMINAL = [
    StepRunStatus.COMPLETED, StepRunStatus.FAILED,
    StepRunStatus.SKIPPED, StepRunStatus.CANCELED,
]

NON_TERMINAL = [StepRunStatus.PENDING, StepRunStatus.WAITING, StepRunStatus.RUNNING]


@pytest.mark.parametrize("status", TERMINAL)
def test_terminal_status(status):
    assert is_terminal_step_run_status(status)


@pytest.mark.parametrize("status", NON_TERMINAL)
def test_non_terminal_status(status):
    assert not is_terminal_step_run_status(status)
