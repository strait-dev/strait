"""Tests for workflow run FSM."""

import pytest

from strait.fsm._workflow import (
    WorkflowRunEvent,
    WorkflowRunStatus,
    can_transition_workflow_run,
    is_terminal_workflow_run_status,
    transition_workflow_run,
)

VALID_TRANSITIONS = [
    (WorkflowRunStatus.PENDING, WorkflowRunEvent.START, WorkflowRunStatus.RUNNING),
    (WorkflowRunStatus.PENDING, WorkflowRunEvent.CANCEL, WorkflowRunStatus.CANCELED),
    (WorkflowRunStatus.RUNNING, WorkflowRunEvent.PAUSE, WorkflowRunStatus.PAUSED),
    (WorkflowRunStatus.RUNNING, WorkflowRunEvent.COMPLETE, WorkflowRunStatus.COMPLETED),
    (WorkflowRunStatus.RUNNING, WorkflowRunEvent.FAIL, WorkflowRunStatus.FAILED),
    (WorkflowRunStatus.RUNNING, WorkflowRunEvent.TIMEOUT, WorkflowRunStatus.TIMED_OUT),
    (WorkflowRunStatus.RUNNING, WorkflowRunEvent.CANCEL, WorkflowRunStatus.CANCELED),
    (WorkflowRunStatus.PAUSED, WorkflowRunEvent.RESUME, WorkflowRunStatus.RUNNING),
    (WorkflowRunStatus.PAUSED, WorkflowRunEvent.COMPLETE, WorkflowRunStatus.COMPLETED),
    (WorkflowRunStatus.PAUSED, WorkflowRunEvent.FAIL, WorkflowRunStatus.FAILED),
    (WorkflowRunStatus.PAUSED, WorkflowRunEvent.TIMEOUT, WorkflowRunStatus.TIMED_OUT),
    (WorkflowRunStatus.PAUSED, WorkflowRunEvent.CANCEL, WorkflowRunStatus.CANCELED),
]


@pytest.mark.parametrize("from_status,event,expected", VALID_TRANSITIONS)
def test_valid_transition(from_status, event, expected):
    assert can_transition_workflow_run(from_status, event)
    assert transition_workflow_run(from_status, event) == expected


INVALID_TRANSITIONS = [
    (WorkflowRunStatus.COMPLETED, WorkflowRunEvent.START),
    (WorkflowRunStatus.FAILED, WorkflowRunEvent.RESUME),
    (WorkflowRunStatus.CANCELED, WorkflowRunEvent.FAIL),
    (WorkflowRunStatus.TIMED_OUT, WorkflowRunEvent.CANCEL),
    (WorkflowRunStatus.PENDING, WorkflowRunEvent.PAUSE),
    (WorkflowRunStatus.PENDING, WorkflowRunEvent.COMPLETE),
]


@pytest.mark.parametrize("from_status,event", INVALID_TRANSITIONS)
def test_invalid_transition_rejected(from_status, event):
    assert not can_transition_workflow_run(from_status, event)
    with pytest.raises(ValueError):
        transition_workflow_run(from_status, event)


TERMINAL = [
    WorkflowRunStatus.COMPLETED, WorkflowRunStatus.FAILED,
    WorkflowRunStatus.TIMED_OUT, WorkflowRunStatus.CANCELED,
]

NON_TERMINAL = [
    WorkflowRunStatus.PENDING, WorkflowRunStatus.RUNNING, WorkflowRunStatus.PAUSED,
]


@pytest.mark.parametrize("status", TERMINAL)
def test_terminal_status(status):
    assert is_terminal_workflow_run_status(status)


@pytest.mark.parametrize("status", NON_TERMINAL)
def test_non_terminal_status(status):
    assert not is_terminal_workflow_run_status(status)
