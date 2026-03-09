package domain

import (
	"testing"
)

func TestStatusCanceling_Transitions(t *testing.T) {
	t.Parallel()

	// Canceling should be reachable from executing and waiting
	validFrom := []RunStatus{StatusExecuting, StatusWaiting}
	for _, from := range validFrom {
		if err := ValidateTransition(from, StatusCanceling); err != nil {
			t.Errorf("expected valid transition %s -> canceling, got: %v", from, err)
		}
	}

	// Canceling should only transition to canceled
	if err := ValidateTransition(StatusCanceling, StatusCanceled); err != nil {
		t.Errorf("expected valid transition canceling -> canceled, got: %v", err)
	}

	// Canceling should NOT transition to anything else
	invalidTo := []RunStatus{StatusExecuting, StatusCompleted, StatusFailed, StatusQueued}
	for _, to := range invalidTo {
		if err := ValidateTransition(StatusCanceling, to); err == nil {
			t.Errorf("expected invalid transition canceling -> %s", to)
		}
	}
}

func TestStatusCanceling_IsValid(t *testing.T) {
	if !StatusCanceling.IsValid() {
		t.Error("expected canceling to be valid status")
	}
}

func TestStatusCanceling_IsNotTerminal(t *testing.T) {
	if StatusCanceling.IsTerminal() {
		t.Error("expected canceling to NOT be terminal (it's transitional)")
	}
}

func TestJobCancelEndpointURL(t *testing.T) {
	job := Job{
		ID:                "job-1",
		CancelEndpointURL: "https://example.com/cancel",
		ExecutionMode:     "http",
	}
	if job.CancelEndpointURL != "https://example.com/cancel" {
		t.Errorf("expected cancel URL, got %s", job.CancelEndpointURL)
	}

	// Empty by default
	job2 := Job{ID: "job-2"}
	if job2.CancelEndpointURL != "" {
		t.Errorf("expected empty cancel URL for default job")
	}
}

func TestWorkflowStepCompensateRef(t *testing.T) {
	step := WorkflowStep{
		ID:                "ws-1",
		StepRef:           "allocate",
		CompensateStepRef: "deallocate",
	}
	if step.CompensateStepRef != "deallocate" {
		t.Errorf("expected deallocate, got %s", step.CompensateStepRef)
	}

	// Empty by default
	step2 := WorkflowStep{ID: "ws-2", StepRef: "process"}
	if step2.CompensateStepRef != "" {
		t.Errorf("expected empty compensate ref for default step")
	}
}
