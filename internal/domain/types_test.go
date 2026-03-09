package domain

import (
	"testing"

	"strait/internal/testutil"
)

func TestWorkflowRunStatus_IsTerminal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   WorkflowRunStatus
		expected bool
	}{
		{WfStatusPending, false},
		{WfStatusRunning, false},
		{WfStatusCompleted, true},
		{WfStatusFailed, true},
		{WfStatusCanceled, true},
	}

	got := make(map[WorkflowRunStatus]bool, len(tests))
	want := make(map[WorkflowRunStatus]bool, len(tests))
	for _, tc := range tests {
		got[tc.status] = tc.status.IsTerminal()
		want[tc.status] = tc.expected
	}
	testutil.AssertEqual(t, got, want)
}

func TestStepRunStatus_IsTerminal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   StepRunStatus
		expected bool
	}{
		{StepPending, false},
		{StepWaiting, false},
		{StepRunning, false},
		{StepCompleted, true},
		{StepFailed, true},
		{StepSkipped, true},
		{StepCanceled, true},
	}

	got := make(map[StepRunStatus]bool, len(tests))
	want := make(map[StepRunStatus]bool, len(tests))
	for _, tc := range tests {
		got[tc.status] = tc.status.IsTerminal()
		want[tc.status] = tc.expected
	}
	testutil.AssertEqual(t, got, want)
}

func TestEventTriggerStatusConstants(t *testing.T) {
	t.Parallel()

	testutil.AssertEqual(t, EventTriggerStatusWaiting, "waiting")
	testutil.AssertEqual(t, EventTriggerStatusReceived, "received")
	testutil.AssertEqual(t, EventTriggerStatusTimedOut, "timed_out")
	testutil.AssertEqual(t, EventTriggerStatusCanceled, "canceled")
}

func TestWorkflowStepTypeWaitForEvent(t *testing.T) {
	t.Parallel()

	testutil.AssertEqual(t, string(WorkflowStepTypeWaitForEvent), "wait_for_event")

	// Verify it is distinct from existing step types.
	types := []WorkflowStepType{
		WorkflowStepTypeJob,
		WorkflowStepTypeApproval,
		WorkflowStepTypeSubWorkflow,
		WorkflowStepTypeWaitForEvent,
	}
	seen := make(map[WorkflowStepType]struct{}, len(types))
	for _, st := range types {
		if _, dup := seen[st]; dup {
			t.Fatalf("duplicate step type: %s", st)
		}
		seen[st] = struct{}{}
	}
}

func TestDefaultEventTimeoutSecs(t *testing.T) {
	t.Parallel()

	if DefaultEventTimeoutSecs != 3600 {
		t.Fatalf("DefaultEventTimeoutSecs = %d, want 3600", DefaultEventTimeoutSecs)
	}
}
