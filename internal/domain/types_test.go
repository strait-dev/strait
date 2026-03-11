package domain

import (
	"reflect"
	"testing"
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
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IsTerminal map mismatch: got=%v want=%v", got, want)
	}
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
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IsTerminal map mismatch: got=%v want=%v", got, want)
	}
}

func TestEventTriggerStatusConstants(t *testing.T) {
	t.Parallel()

	if EventTriggerStatusWaiting != "waiting" || EventTriggerStatusReceived != "received" || EventTriggerStatusTimedOut != "timed_out" || EventTriggerStatusCanceled != "canceled" {
		t.Fatalf("unexpected event trigger status constants: %q %q %q %q", EventTriggerStatusWaiting, EventTriggerStatusReceived, EventTriggerStatusTimedOut, EventTriggerStatusCanceled)
	}
}

func TestWorkflowStepTypeWaitForEvent(t *testing.T) {
	t.Parallel()

	if string(WorkflowStepTypeWaitForEvent) != "wait_for_event" {
		t.Fatalf("WorkflowStepTypeWaitForEvent = %q, want wait_for_event", WorkflowStepTypeWaitForEvent)
	}

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
