package authoring

import "testing"

func TestJobStep(t *testing.T) {
	s := Job("validate", "job_validate")
	if s.StepRef() != "validate" {
		t.Errorf("expected ref 'validate', got %q", s.StepRef())
	}
	if s.Type() != StepTypeJob {
		t.Errorf("expected type 'job', got %q", s.Type())
	}
	if s.JobID != "job_validate" {
		t.Errorf("expected jobID 'job_validate', got %q", s.JobID)
	}
}

func TestJobStep_WithDependsOn(t *testing.T) {
	s := Job("charge", "job_charge", DependsOn("validate"))
	deps := s.BaseOptions().DependsOn
	if len(deps) != 1 || deps[0] != "validate" {
		t.Errorf("expected dependsOn [validate], got %v", deps)
	}
}

func TestApprovalStep(t *testing.T) {
	timeout := 3600
	s := Approval("review", func(a *ApprovalStep) {
		a.ApprovalTimeoutSecs = &timeout
		a.Approvers = []string{"admin"}
		a.DependsOn = []string{"charge"}
	})

	if s.Type() != StepTypeApproval {
		t.Error("expected approval type")
	}
	if *s.ApprovalTimeoutSecs != 3600 {
		t.Error("expected timeout 3600")
	}
	if len(s.Approvers) != 1 || s.Approvers[0] != "admin" {
		t.Error("expected approvers [admin]")
	}
}

func TestSubWorkflowStep(t *testing.T) {
	depth := 3
	s := SubWorkflow("notify", "wf_notifications", func(sw *SubWorkflowStep) {
		sw.MaxNestingDepth = &depth
	})

	if s.Type() != StepTypeSubWorkflow {
		t.Error("expected sub_workflow type")
	}
	if s.SubWorkflowID != "wf_notifications" {
		t.Error("expected sub workflow ID")
	}
	if *s.MaxNestingDepth != 3 {
		t.Error("expected max nesting depth 3")
	}
}

func TestWaitForEventStep(t *testing.T) {
	timeout := 86400
	s := WaitForEvent("confirm", "shipping.confirmed", func(w *WaitForEventStep) {
		w.EventTimeoutSecs = &timeout
		w.EventNotifyURL = "https://hooks.example.com"
	})

	if s.Type() != StepTypeWaitForEvent {
		t.Error("expected wait_for_event type")
	}
	if s.EventKey != "shipping.confirmed" {
		t.Error("expected event key")
	}
}

func TestSleepStep(t *testing.T) {
	s := Sleep("cooldown", 60)
	if s.Type() != StepTypeSleep {
		t.Error("expected sleep type")
	}
	if s.SleepDurationSecs != 60 {
		t.Error("expected 60 seconds")
	}
}

func TestStepToAPI_Job(t *testing.T) {
	s := Job("validate", "job_validate", DependsOn("init"))
	api := StepToAPI(s)

	if api["step_ref"] != "validate" {
		t.Error("expected step_ref")
	}
	if api["type"] != "job" {
		t.Error("expected type job")
	}
	if api["job_id"] != "job_validate" {
		t.Error("expected job_id")
	}
	deps, ok := api["depends_on"].([]string)
	if !ok || len(deps) != 1 || deps[0] != "init" {
		t.Error("expected depends_on [init]")
	}
}

func TestStepToAPI_Approval(t *testing.T) {
	timeout := 3600
	s := Approval("review", func(a *ApprovalStep) {
		a.ApprovalTimeoutSecs = &timeout
		a.Approvers = []string{"admin"}
	})
	api := StepToAPI(s)

	if api["approval_timeout_secs"] != 3600 {
		t.Error("expected approval_timeout_secs")
	}
	approvers, ok := api["approvers"].([]string)
	if !ok || len(approvers) != 1 {
		t.Error("expected approvers")
	}
}

func TestStepToAPI_SubWorkflow(t *testing.T) {
	depth := 2
	s := SubWorkflow("sub", "wf_1", func(sw *SubWorkflowStep) {
		sw.MaxNestingDepth = &depth
	})
	api := StepToAPI(s)

	if api["sub_workflow_id"] != "wf_1" {
		t.Error("expected sub_workflow_id")
	}
	if api["max_nesting_depth"] != 2 {
		t.Error("expected max_nesting_depth")
	}
}

func TestStepToAPI_WaitForEvent(t *testing.T) {
	s := WaitForEvent("wait", "event.key")
	api := StepToAPI(s)

	if api["event_key"] != "event.key" {
		t.Error("expected event_key")
	}
}

func TestStepToAPI_Sleep(t *testing.T) {
	s := Sleep("pause", 120)
	api := StepToAPI(s)

	if api["sleep_duration_secs"] != 120 {
		t.Error("expected sleep_duration_secs")
	}
}

func TestStepToAPI_BaseOptions(t *testing.T) {
	retryAttempts := 3
	retryDelay := 5
	retryMax := 60
	timeout := 300

	s := Job("process", "job_process", func(o *BaseStepOptions) {
		o.DependsOn = []string{"validate"}
		o.OnFailure = OnFailureFailWorkflow
		o.RetryMaxAttempts = &retryAttempts
		o.RetryBackoff = RetryBackoffExponential
		o.RetryInitialDelaySecs = &retryDelay
		o.RetryMaxDelaySecs = &retryMax
		o.TimeoutSecsOverride = &timeout
		o.OutputTransform = "$.result"
		o.ConcurrencyKey = "order_id"
		o.ResourceClass = ResourceClassLarge
	})

	api := StepToAPI(s)

	if api["on_failure"] != "fail_workflow" {
		t.Error("expected on_failure")
	}
	if api["retry_max_attempts"] != 3 {
		t.Error("expected retry_max_attempts")
	}
	if api["retry_backoff"] != "exponential" {
		t.Error("expected retry_backoff")
	}
	if api["retry_initial_delay_secs"] != 5 {
		t.Error("expected retry_initial_delay_secs")
	}
	if api["retry_max_delay_secs"] != 60 {
		t.Error("expected retry_max_delay_secs")
	}
	if api["timeout_secs_override"] != 300 {
		t.Error("expected timeout_secs_override")
	}
	if api["output_transform"] != "$.result" {
		t.Error("expected output_transform")
	}
	if api["concurrency_key"] != "order_id" {
		t.Error("expected concurrency_key")
	}
	if api["resource_class"] != "large" {
		t.Error("expected resource_class")
	}
}
