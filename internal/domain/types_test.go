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
