package api

import (
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestValidateWorkflowSteps_MaxStepLimit(t *testing.T) {
	t.Parallel()

	steps := make([]workflowStepRequest, 1001)
	for i := range steps {
		steps[i] = workflowStepRequest{StepRef: "s", JobID: "job-1"}
	}

	err := validateWorkflowSteps(steps)
	if err == nil {
		t.Fatal("expected max step limit error")
	}
}

func TestValidateWorkflowSteps_InvalidResourceClass(t *testing.T) {
	t.Parallel()

	steps := []workflowStepRequest{{
		StepRef:       "s1",
		JobID:         "job-1",
		ResourceClass: "xlarge",
	}}

	err := validateWorkflowSteps(steps)
	if err == nil {
		t.Fatal("expected resource_class validation error")
	}
}

func TestValidateWorkflowSteps_RejectsUnknownStepType(t *testing.T) {
	t.Parallel()

	steps := []workflowStepRequest{{
		StepRef:  "s1",
		StepType: "approval_bypass",
		JobID:    "job-1",
	}}

	err := validateWorkflowSteps(steps)
	if err == nil {
		t.Fatal("expected unknown step_type validation error")
	}
	if !strings.Contains(err.Error(), "invalid step_type") {
		t.Fatalf("expected invalid step_type error, got %v", err)
	}
}

func TestValidateWorkflowSteps_RejectsOversizedSleepDuration(t *testing.T) {
	t.Parallel()

	steps := []workflowStepRequest{{
		StepRef:           "sleep-too-long",
		StepType:          domain.WorkflowStepTypeSleep,
		SleepDurationSecs: domain.MaxSleepDurationSecs + 1,
	}}

	err := validateWorkflowSteps(steps)
	if err == nil {
		t.Fatal("expected oversized sleep duration validation error")
	}
	if !strings.Contains(err.Error(), "sleep_duration_secs must be <=") {
		t.Fatalf("expected sleep duration cap error, got %v", err)
	}
}

func TestValidateWorkflowSteps_AllowsMaxSleepDuration(t *testing.T) {
	t.Parallel()

	steps := []workflowStepRequest{{
		StepRef:           "sleep-max",
		StepType:          domain.WorkflowStepTypeSleep,
		SleepDurationSecs: domain.MaxSleepDurationSecs,
	}}

	if err := validateWorkflowSteps(steps); err != nil {
		t.Fatalf("validate max sleep duration: %v", err)
	}
}
