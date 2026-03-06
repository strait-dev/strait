package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidateWorkflowTransition_AllValidTransitions(t *testing.T) {
	for from, toStatuses := range validWorkflowTransitions {
		for _, to := range toStatuses {
			t.Run(fmt.Sprintf("%s->%s", from, to), func(t *testing.T) {
				if err := ValidateWorkflowTransition(from, to); err != nil {
					t.Errorf("expected valid transition %s -> %s, got error: %v", from, to, err)
				}
			})
		}
	}
}

func TestValidateWorkflowTransition_InvalidTransitions(t *testing.T) {
	tests := []struct {
		from WorkflowRunStatus
		to   WorkflowRunStatus
	}{
		{WfStatusCompleted, WfStatusRunning},
		{WfStatusCompleted, WfStatusPending},
		{WfStatusFailed, WfStatusRunning},
		{WfStatusFailed, WfStatusCompleted},
		{WfStatusCanceled, WfStatusRunning},
		{WfStatusCanceled, WfStatusPending},
		{WfStatusPending, WfStatusCompleted},
		{WfStatusRunning, WfStatusPending},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s->%s", tc.from, tc.to), func(t *testing.T) {
			if err := ValidateWorkflowTransition(tc.from, tc.to); err == nil {
				t.Errorf("expected invalid transition %s -> %s to fail", tc.from, tc.to)
			}
		})
	}
}

func TestValidateWorkflowTransition_UnknownStatus(t *testing.T) {
	err := ValidateWorkflowTransition(WorkflowRunStatus("unknown"), WfStatusRunning)
	if err == nil {
		t.Fatal("expected error for unknown status")
	}

	var unknownErr *UnknownStatusError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected UnknownStatusError, got %T: %v", err, err)
	}
}

func TestValidateWorkflowTransition_TerminalHaveNoTransitions(t *testing.T) {
	terminalStatuses := []WorkflowRunStatus{WfStatusCompleted, WfStatusFailed, WfStatusCanceled}
	for _, status := range terminalStatuses {
		t.Run(string(status), func(t *testing.T) {
			transitions, ok := validWorkflowTransitions[status]
			if !ok {
				t.Errorf("terminal status %s not found in validWorkflowTransitions", status)
			}
			if len(transitions) != 0 {
				t.Errorf("terminal status %s should not have transitions, got %v", status, transitions)
			}
		})
	}
}

func TestAllWorkflowStatusesCovered(t *testing.T) {
	allStatuses := []WorkflowRunStatus{
		WfStatusPending,
		WfStatusRunning,
		WfStatusCompleted,
		WfStatusFailed,
		WfStatusCanceled,
	}

	for _, status := range allStatuses {
		t.Run(string(status), func(t *testing.T) {
			if _, ok := validWorkflowTransitions[status]; !ok {
				t.Errorf("status %s is missing from validWorkflowTransitions map", status)
			}
		})
	}

	if len(validWorkflowTransitions) != len(allStatuses) {
		t.Fatalf("validWorkflowTransitions has %d statuses, expected %d", len(validWorkflowTransitions), len(allStatuses))
	}
}

func TestValidateStepTransition_AllValidTransitions(t *testing.T) {
	for from, toStatuses := range validStepTransitions {
		for _, to := range toStatuses {
			t.Run(fmt.Sprintf("%s->%s", from, to), func(t *testing.T) {
				if err := ValidateStepTransition(from, to); err != nil {
					t.Errorf("expected valid transition %s -> %s, got error: %v", from, to, err)
				}
			})
		}
	}
}

func TestValidateStepTransition_InvalidTransitions(t *testing.T) {
	tests := []struct {
		from StepRunStatus
		to   StepRunStatus
	}{
		{StepCompleted, StepRunning},
		{StepCompleted, StepPending},
		{StepFailed, StepRunning},
		{StepFailed, StepPending},
		{StepSkipped, StepRunning},
		{StepSkipped, StepPending},
		{StepCanceled, StepRunning},
		{StepCanceled, StepPending},
		{StepPending, StepCompleted},
		{StepPending, StepFailed},
		{StepWaiting, StepCompleted},
		{StepWaiting, StepFailed},
		{StepRunning, StepPending},
		{StepRunning, StepWaiting},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s->%s", tc.from, tc.to), func(t *testing.T) {
			if err := ValidateStepTransition(tc.from, tc.to); err == nil {
				t.Errorf("expected invalid transition %s -> %s to fail", tc.from, tc.to)
			}
		})
	}
}

func TestValidateStepTransition_UnknownStatus(t *testing.T) {
	err := ValidateStepTransition(StepRunStatus("unknown"), StepRunning)
	if err == nil {
		t.Fatal("expected error for unknown status")
	}

	var unknownErr *UnknownStatusError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected UnknownStatusError, got %T: %v", err, err)
	}
}

func TestValidateStepTransition_TerminalHaveNoTransitions(t *testing.T) {
	terminalStatuses := []StepRunStatus{StepCompleted, StepFailed, StepSkipped, StepCanceled}
	for _, status := range terminalStatuses {
		t.Run(string(status), func(t *testing.T) {
			transitions, ok := validStepTransitions[status]
			if !ok {
				t.Errorf("terminal status %s not found in validStepTransitions", status)
			}
			if len(transitions) != 0 {
				t.Errorf("terminal status %s should not have transitions, got %v", status, transitions)
			}
		})
	}
}

func TestAllStepStatusesCovered(t *testing.T) {
	allStatuses := []StepRunStatus{
		StepPending,
		StepWaiting,
		StepRunning,
		StepCompleted,
		StepFailed,
		StepSkipped,
		StepCanceled,
	}

	for _, status := range allStatuses {
		t.Run(string(status), func(t *testing.T) {
			if _, ok := validStepTransitions[status]; !ok {
				t.Errorf("status %s is missing from validStepTransitions map", status)
			}
		})
	}

	if len(validStepTransitions) != len(allStatuses) {
		t.Fatalf("validStepTransitions has %d statuses, expected %d", len(validStepTransitions), len(allStatuses))
	}
}
