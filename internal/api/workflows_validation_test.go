package api

import (
	"testing"
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
