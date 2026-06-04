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

func TestValidateWorkflowSteps_RejectsInvalidEventNotifyURL(t *testing.T) {
	globalAllowPrivateEndpoints.Store(false)
	t.Cleanup(func() { globalAllowPrivateEndpoints.Store(false) })

	tests := []struct {
		name      string
		notifyURL string
		want      string
	}{
		{
			name:      "localhost",
			notifyURL: "http://localhost/hook",
			want:      "event_notify_url",
		},
		{
			name:      "private ip",
			notifyURL: "http://192.168.1.10/hook",
			want:      "event_notify_url",
		},
		{
			name:      "non http scheme",
			notifyURL: "file:///etc/passwd",
			want:      "event_notify_url",
		},
		{
			name:      "disallowed port",
			notifyURL: "https://example.com:4444/hook",
			want:      "port 4444 is not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := []workflowStepRequest{{
				StepRef:        "wait",
				StepType:       domain.WorkflowStepTypeWaitForEvent,
				EventKey:       "external.signal",
				EventNotifyURL: tt.notifyURL,
			}}

			err := validateWorkflowSteps(steps)
			if err == nil {
				t.Fatal("expected event_notify_url validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestValidateWorkflowSteps_AllowsValidEventNotifyURL(t *testing.T) {
	t.Parallel()

	steps := []workflowStepRequest{{
		StepRef:        "wait",
		StepType:       domain.WorkflowStepTypeWaitForEvent,
		EventKey:       "external.signal",
		EventNotifyURL: "https://example.com:443/hook",
	}}

	if err := validateWorkflowSteps(steps); err != nil {
		t.Fatalf("validate valid event_notify_url: %v", err)
	}
}

func TestValidateWorkflowStepAcyclic_DuplicateDependencyIsNotCycle(t *testing.T) {
	t.Parallel()

	steps := []workflowStepRequest{
		{StepRef: "a", JobID: "job-a"},
		{StepRef: "b", JobID: "job-b", DependsOn: []string{"a", "a"}},
	}

	if err := validateWorkflowSteps(steps); err != nil {
		t.Fatalf("duplicate dependency should not be reported as a cycle: %v", err)
	}
}
