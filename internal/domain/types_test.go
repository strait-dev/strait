package domain

import "testing"

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

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.IsTerminal(); got != tc.expected {
				t.Errorf("WorkflowRunStatus(%s).IsTerminal() = %v, expected %v", tc.status, got, tc.expected)
			}
		})
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

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.IsTerminal(); got != tc.expected {
				t.Errorf("StepRunStatus(%s).IsTerminal() = %v, expected %v", tc.status, got, tc.expected)
			}
		})
	}
}
