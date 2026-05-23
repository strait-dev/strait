package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidateWorkflowTransition_AllValidTransitions(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	tests := []struct {
		from WorkflowRunStatus
		to   WorkflowRunStatus
	}{
		{WfStatusCompleted, WfStatusRunning},
		{WfStatusCompleted, WfStatusPending},
		{WfStatusFailed, WfStatusRunning},
		{WfStatusFailed, WfStatusCompleted},
		{WfStatusTimedOut, WfStatusRunning},
		{WfStatusCompensated, WfStatusRunning},
		{WfStatusCompensationFailed, WfStatusRunning},
		{WfStatusPending, WfStatusCompleted},
		{WfStatusRunning, WfStatusPending},
		{WfStatusPaused, WfStatusPending},
		{WfStatusCanceled, WfStatusRunning},
		{WfStatusCanceled, WfStatusPending},
		{WorkflowRunStatus("unknown"), WfStatusRunning},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s->%s", tc.from, tc.to), func(t *testing.T) {
			if err := ValidateWorkflowTransition(tc.from, tc.to); err == nil {
				t.Errorf("expected invalid transition %s -> %s to fail", tc.from, tc.to)
			}
		})
	}
}

func TestValidateWorkflowTransition_QueuedParkedState(t *testing.T) {
	t.Parallel()
	// A singleton-parked workflow run promotes to running or is canceled by the
	// replace policy; it can never jump straight to a terminal completion.
	for _, to := range []WorkflowRunStatus{WfStatusRunning, WfStatusCanceled} {
		if err := ValidateWorkflowTransition(WfStatusQueued, to); err != nil {
			t.Errorf("queued -> %s should be valid, got %v", to, err)
		}
	}
	for _, to := range []WorkflowRunStatus{WfStatusPending, WfStatusCompleted, WfStatusFailed, WfStatusPaused} {
		if err := ValidateWorkflowTransition(WfStatusQueued, to); err == nil {
			t.Errorf("queued -> %s should be invalid", to)
		}
	}
	if WfStatusQueued.IsTerminal() {
		t.Error("queued must not be terminal")
	}
	if !WfStatusQueued.IsValid() {
		t.Error("queued must be a valid status")
	}
}

func TestValidateWorkflowTransition_UnknownStatus(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	// These statuses have zero outbound transitions.
	fullyTerminal := []WorkflowRunStatus{WfStatusCompleted, WfStatusCanceled, WfStatusCompensated, WfStatusCompensationFailed}
	for _, status := range fullyTerminal {
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
	// failed and timed_out can transition to compensating.
	for _, status := range []WorkflowRunStatus{WfStatusFailed, WfStatusTimedOut} {
		t.Run(string(status)+"_can_compensate", func(t *testing.T) {
			transitions := validWorkflowTransitions[status]
			if len(transitions) != 1 || transitions[0] != WfStatusCompensating {
				t.Errorf("status %s should only transition to compensating, got %v", status, transitions)
			}
		})
	}
}

func TestAllWorkflowStatusesCovered(t *testing.T) {
	t.Parallel()
	allStatuses := []WorkflowRunStatus{
		WfStatusPending,
		WfStatusQueued,
		WfStatusRunning,
		WfStatusPaused,
		WfStatusCompleted,
		WfStatusFailed,
		WfStatusTimedOut,
		WfStatusCanceled,
		WfStatusCompensating,
		WfStatusCompensated,
		WfStatusCompensationFailed,
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

func TestWorkflowRunStatusIsTerminal_AllStatuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   WorkflowRunStatus
		expected bool
	}{
		{WfStatusPending, false},
		{WfStatusQueued, false},
		{WfStatusRunning, false},
		{WfStatusPaused, false},
		{WfStatusCompleted, true},
		{WfStatusFailed, true},
		{WfStatusTimedOut, true},
		{WfStatusCanceled, true},
		{WfStatusCompensating, false},
		{WfStatusCompensated, true},
		{WfStatusCompensationFailed, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.IsTerminal(); got != tc.expected {
				t.Errorf("status %s IsTerminal() = %v, expected %v", tc.status, got, tc.expected)
			}
		})
	}
}

func TestWorkflowRunStatusIsValid(t *testing.T) {
	t.Parallel()
	valid := []WorkflowRunStatus{
		WfStatusQueued,
		WfStatusRunning,
		WfStatusCompensating,
		WfStatusCompensated,
		WfStatusCompensationFailed,
	}
	for _, status := range valid {
		if !status.IsValid() {
			t.Fatalf("expected %s to be valid", status)
		}
	}
	if WorkflowRunStatus("not-valid").IsValid() {
		t.Fatal("expected arbitrary workflow status to be invalid")
	}
}

func TestValidateWorkflowTransition_ErrorTypes(t *testing.T) {
	t.Parallel()
	t.Run("transition_error_from_terminal", func(t *testing.T) {
		t.Parallel()
		err := ValidateWorkflowTransition(WfStatusCompleted, WfStatusRunning)
		var te *TransitionError
		if !errors.As(err, &te) {
			t.Fatalf("expected *TransitionError, got %T: %v", err, err)
		}
		if te.From != RunStatus(WfStatusCompleted) || te.To != RunStatus(WfStatusRunning) {
			t.Fatalf("TransitionError From=%s To=%s, want completed->running", te.From, te.To)
		}
	})

	t.Run("unknown_status_error", func(t *testing.T) {
		t.Parallel()
		err := ValidateWorkflowTransition(WorkflowRunStatus("bogus"), WfStatusRunning)
		var ue *UnknownStatusError
		if !errors.As(err, &ue) {
			t.Fatalf("expected *UnknownStatusError, got %T: %v", err, err)
		}
		if ue.Status != RunStatus("bogus") {
			t.Fatalf("UnknownStatusError.Status = %s, want bogus", ue.Status)
		}
	})

	t.Run("self_transition_running", func(t *testing.T) {
		t.Parallel()
		err := ValidateWorkflowTransition(WfStatusRunning, WfStatusRunning)
		var te *TransitionError
		if !errors.As(err, &te) {
			t.Fatalf("expected *TransitionError for self-transition, got %T: %v", err, err)
		}
	})
}

func TestValidateStepTransition_AllValidTransitions(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	tests := []struct {
		name string
		from StepRunStatus
		to   StepRunStatus
	}{
		{"completed_to_running", StepCompleted, StepRunning},
		{"completed_to_pending", StepCompleted, StepPending},
		{"failed_to_running", StepFailed, StepRunning},
		{"failed_to_pending", StepFailed, StepPending},
		{"skipped_to_running", StepSkipped, StepRunning},
		{"skipped_to_pending", StepSkipped, StepPending},
		{"canceled_to_running", StepCanceled, StepRunning},
		{"canceled_to_pending", StepCanceled, StepPending},
		{"pending_to_failed", StepPending, StepFailed},
		{"waiting_to_failed", StepWaiting, StepFailed},
		{"running_to_running", StepRunning, StepRunning},
		{"running_to_pending", StepRunning, StepPending},
		{"running_to_waiting", StepRunning, StepWaiting},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateStepTransition(tc.from, tc.to)
			if err == nil {
				t.Fatalf("expected error for %s -> %s, got nil", tc.from, tc.to)
			}
		})
	}
}

func TestValidateStepTransition_UnknownStatus(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestValidateStepTransition_ErrorTypes(t *testing.T) {
	t.Parallel()
	t.Run("transition_error_from_terminal", func(t *testing.T) {
		t.Parallel()
		err := ValidateStepTransition(StepCompleted, StepRunning)
		var te *TransitionError
		if !errors.As(err, &te) {
			t.Fatalf("expected *TransitionError, got %T: %v", err, err)
		}
		if te.From != RunStatus(StepCompleted) || te.To != RunStatus(StepRunning) {
			t.Fatalf("TransitionError From=%s To=%s, want completed->running", te.From, te.To)
		}
	})

	t.Run("unknown_status_error", func(t *testing.T) {
		t.Parallel()
		err := ValidateStepTransition(StepRunStatus("bogus"), StepRunning)
		var ue *UnknownStatusError
		if !errors.As(err, &ue) {
			t.Fatalf("expected *UnknownStatusError, got %T: %v", err, err)
		}
		if ue.Status != RunStatus("bogus") {
			t.Fatalf("UnknownStatusError.Status = %s, want bogus", ue.Status)
		}
	})

	t.Run("self_transition_running", func(t *testing.T) {
		t.Parallel()
		err := ValidateStepTransition(StepRunning, StepRunning)
		var te *TransitionError
		if !errors.As(err, &te) {
			t.Fatalf("expected *TransitionError for self-transition, got %T: %v", err, err)
		}
	})
}

func TestStepRunStatusIsTerminal_AllStatuses(t *testing.T) {
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
				t.Errorf("status %s IsTerminal() = %v, expected %v", tc.status, got, tc.expected)
			}
		})
	}
}
