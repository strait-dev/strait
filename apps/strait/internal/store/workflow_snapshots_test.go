package store

import (
	"encoding/json"
	"testing"

	"strait/internal/domain"
)

func TestParseSnapshotDefinition_RoundTrip(t *testing.T) {
	steps := []domain.WorkflowStep{
		{
			ID:                    "step-1",
			WorkflowID:            "wf-1",
			JobID:                 "job-1",
			StepRef:               "build",
			DependsOn:             []string{},
			Condition:             json.RawMessage(`{"all_of":["deploy"]}`),
			OnFailure:             domain.FailWorkflow,
			Payload:               json.RawMessage(`{"key":"value"}`),
			StepType:              domain.WorkflowStepTypeJob,
			ApprovalTimeoutSecs:   600,
			ApprovalApprovers:     []string{"alice", "bob"},
			RetryMaxAttempts:      3,
			RetryBackoff:          domain.RetryBackoffExponential,
			RetryInitialDelaySecs: 5,
			RetryMaxDelaySecs:     60,
			TimeoutSecsOverride:   120,
			OutputTransform:       "$.result",
			SubWorkflowID:         "",
			MaxNestingDepth:       5,
			EventKey:              "my-event",
			EventTimeoutSecs:      3600,
			EventNotifyURL:        "https://example.com/notify",
			SleepDurationSecs:     30,
			EventEmitKey:          "emit-key",
			ConcurrencyKey:        "ck-1",
			ResourceClass:         "medium",
		},
		{
			ID:         "step-2",
			WorkflowID: "wf-1",
			JobID:      "job-2",
			StepRef:    "deploy",
			DependsOn:  []string{"build"},
			OnFailure:  domain.Continue,
			StepType:   domain.WorkflowStepTypeApproval,
		},
		{
			ID:            "step-3",
			WorkflowID:    "wf-1",
			StepRef:       "wait",
			StepType:      domain.WorkflowStepTypeWaitForEvent,
			EventKey:      "deploy-done",
			DependsOn:     []string{"deploy"},
			OnFailure:     domain.SkipDependents,
			ResourceClass: "small",
		},
		{
			ID:                "step-4",
			WorkflowID:        "wf-1",
			StepRef:           "sleep-step",
			StepType:          domain.WorkflowStepTypeSleep,
			SleepDurationSecs: 60,
			DependsOn:         []string{"wait"},
			OnFailure:         domain.FailWorkflow,
		},
		{
			ID:            "step-5",
			WorkflowID:    "wf-1",
			StepRef:       "sub-wf",
			StepType:      domain.WorkflowStepTypeSubWorkflow,
			SubWorkflowID: "child-wf-1",
			DependsOn:     []string{"sleep-step"},
			OnFailure:     domain.FailWorkflow,
		},
	}

	def := domain.WorkflowSnapshotDefinition{
		Workflow: domain.WorkflowSnapshotMeta{
			ID:                "wf-1",
			ProjectID:         "proj-1",
			Name:              "My Workflow",
			Slug:              "my-workflow",
			Description:       "A test workflow",
			Tags:              map[string]string{"team": "platform"},
			Version:           3,
			VersionID:         "vid-abc",
			TimeoutSecs:       3600,
			MaxConcurrentRuns: 5,
			MaxParallelSteps:  3,
		},
		Steps: steps,
	}

	// Serialize
	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Deserialize
	parsed, err := ParseSnapshotDefinition(json.RawMessage(data))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Verify workflow metadata
	if parsed.Workflow.ID != "wf-1" {
		t.Errorf("workflow ID = %q, want wf-1", parsed.Workflow.ID)
	}
	if parsed.Workflow.Name != "My Workflow" {
		t.Errorf("workflow Name = %q, want My Workflow", parsed.Workflow.Name)
	}
	if parsed.Workflow.Version != 3 {
		t.Errorf("workflow Version = %d, want 3", parsed.Workflow.Version)
	}
	if parsed.Workflow.Tags["team"] != "platform" {
		t.Errorf("workflow Tags[team] = %q, want platform", parsed.Workflow.Tags["team"])
	}

	// Verify all steps came back
	if len(parsed.Steps) != 5 {
		t.Fatalf("steps count = %d, want 5", len(parsed.Steps))
	}

	// Verify every step type
	stepTypes := map[string]domain.WorkflowStepType{
		"build":      domain.WorkflowStepTypeJob,
		"deploy":     domain.WorkflowStepTypeApproval,
		"wait":       domain.WorkflowStepTypeWaitForEvent,
		"sleep-step": domain.WorkflowStepTypeSleep,
		"sub-wf":     domain.WorkflowStepTypeSubWorkflow,
	}
	for _, step := range parsed.Steps {
		expected, ok := stepTypes[step.StepRef]
		if !ok {
			t.Errorf("unexpected step ref %q", step.StepRef)
			continue
		}
		if step.StepType != expected {
			t.Errorf("step %q type = %q, want %q", step.StepRef, step.StepType, expected)
		}
	}

	// Verify all fields of step 1 (the most populated)
	s := parsed.Steps[0]
	if s.JobID != "job-1" {
		t.Errorf("step[0].JobID = %q, want job-1", s.JobID)
	}
	if s.RetryMaxAttempts != 3 {
		t.Errorf("step[0].RetryMaxAttempts = %d, want 3", s.RetryMaxAttempts)
	}
	if s.RetryBackoff != domain.RetryBackoffExponential {
		t.Errorf("step[0].RetryBackoff = %q, want exponential", s.RetryBackoff)
	}
	if s.RetryInitialDelaySecs != 5 {
		t.Errorf("step[0].RetryInitialDelaySecs = %d, want 5", s.RetryInitialDelaySecs)
	}
	if s.TimeoutSecsOverride != 120 {
		t.Errorf("step[0].TimeoutSecsOverride = %d, want 120", s.TimeoutSecsOverride)
	}
	if s.OutputTransform != "$.result" {
		t.Errorf("step[0].OutputTransform = %q, want $.result", s.OutputTransform)
	}
	if s.EventKey != "my-event" {
		t.Errorf("step[0].EventKey = %q, want my-event", s.EventKey)
	}
	if s.EventNotifyURL != "https://example.com/notify" {
		t.Errorf("step[0].EventNotifyURL = %q", s.EventNotifyURL)
	}
	if s.ConcurrencyKey != "ck-1" {
		t.Errorf("step[0].ConcurrencyKey = %q, want ck-1", s.ConcurrencyKey)
	}
	if s.ResourceClass != "medium" {
		t.Errorf("step[0].ResourceClass = %q, want medium", s.ResourceClass)
	}
	if s.OnFailure != domain.FailWorkflow {
		t.Errorf("step[0].OnFailure = %q, want fail_workflow", s.OnFailure)
	}
	if string(s.Condition) != `{"all_of":["deploy"]}` {
		t.Errorf("step[0].Condition = %s", s.Condition)
	}
	if string(s.Payload) != `{"key":"value"}` {
		t.Errorf("step[0].Payload = %s", s.Payload)
	}
	if s.ApprovalTimeoutSecs != 600 {
		t.Errorf("step[0].ApprovalTimeoutSecs = %d, want 600", s.ApprovalTimeoutSecs)
	}
	if len(s.ApprovalApprovers) != 2 || s.ApprovalApprovers[0] != "alice" {
		t.Errorf("step[0].ApprovalApprovers = %v", s.ApprovalApprovers)
	}
	if s.SleepDurationSecs != 30 {
		t.Errorf("step[0].SleepDurationSecs = %d, want 30", s.SleepDurationSecs)
	}
	if s.EventEmitKey != "emit-key" {
		t.Errorf("step[0].EventEmitKey = %q, want emit-key", s.EventEmitKey)
	}
}

func TestParseSnapshotDefinition_ComplexConditions(t *testing.T) {
	def := domain.WorkflowSnapshotDefinition{
		Steps: []domain.WorkflowStep{
			{
				StepRef:   "step-nested",
				Condition: json.RawMessage(`{"any_of":[{"all_of":["a","b"]},{"none_of":["c"]}]}`),
			},
		},
	}

	data, _ := json.Marshal(def)
	parsed, err := ParseSnapshotDefinition(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if string(parsed.Steps[0].Condition) != `{"any_of":[{"all_of":["a","b"]},{"none_of":["c"]}]}` {
		t.Errorf("nested condition not preserved: %s", parsed.Steps[0].Condition)
	}
}

func TestParseSnapshotDefinition_EmptyOptionalFields(t *testing.T) {
	def := domain.WorkflowSnapshotDefinition{
		Steps: []domain.WorkflowStep{
			{
				StepRef:   "minimal",
				StepType:  domain.WorkflowStepTypeJob,
				OnFailure: domain.FailWorkflow,
			},
		},
	}

	data, _ := json.Marshal(def)
	parsed, err := ParseSnapshotDefinition(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := parsed.Steps[0]
	if s.OutputTransform != "" {
		t.Errorf("OutputTransform = %q, want empty", s.OutputTransform)
	}
	if s.EventKey != "" {
		t.Errorf("EventKey = %q, want empty", s.EventKey)
	}
	if s.SubWorkflowID != "" {
		t.Errorf("SubWorkflowID = %q, want empty", s.SubWorkflowID)
	}
	if s.Condition != nil {
		t.Errorf("Condition = %s, want nil", s.Condition)
	}
	if s.Payload != nil {
		t.Errorf("Payload = %s, want nil", s.Payload)
	}
}

func TestParseSnapshotDefinition_AllRetryFields(t *testing.T) {
	def := domain.WorkflowSnapshotDefinition{
		Steps: []domain.WorkflowStep{
			{
				StepRef:               "retry-step",
				RetryMaxAttempts:      5,
				RetryBackoff:          domain.RetryBackoffFixed,
				RetryInitialDelaySecs: 10,
				RetryMaxDelaySecs:     120,
			},
		},
	}

	data, _ := json.Marshal(def)
	parsed, err := ParseSnapshotDefinition(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s := parsed.Steps[0]
	if s.RetryMaxAttempts != 5 {
		t.Errorf("RetryMaxAttempts = %d, want 5", s.RetryMaxAttempts)
	}
	if s.RetryBackoff != domain.RetryBackoffFixed {
		t.Errorf("RetryBackoff = %q, want fixed", s.RetryBackoff)
	}
	if s.RetryInitialDelaySecs != 10 {
		t.Errorf("RetryInitialDelaySecs = %d, want 10", s.RetryInitialDelaySecs)
	}
	if s.RetryMaxDelaySecs != 120 {
		t.Errorf("RetryMaxDelaySecs = %d, want 120", s.RetryMaxDelaySecs)
	}
}

func TestParseSnapshotDefinition_ExhaustiveFieldCheck(t *testing.T) {
	// Create a step with ALL fields populated to ensure nothing is lost.
	step := domain.WorkflowStep{
		ID:                    "ws-id-1",
		WorkflowID:            "wf-id-1",
		JobID:                 "job-id-1",
		StepRef:               "exhaustive",
		DependsOn:             []string{"a", "b", "c"},
		Condition:             json.RawMessage(`{"op":"eq","field":"status","value":"ok"}`),
		OnFailure:             domain.SkipDependents,
		Payload:               json.RawMessage(`{"x":1,"nested":{"y":2}}`),
		StepType:              domain.WorkflowStepTypeJob,
		ApprovalTimeoutSecs:   900,
		ApprovalApprovers:     []string{"admin"},
		RetryMaxAttempts:      7,
		RetryBackoff:          domain.RetryBackoffExponential,
		RetryInitialDelaySecs: 2,
		RetryMaxDelaySecs:     300,
		TimeoutSecsOverride:   600,
		OutputTransform:       "$.data.result",
		SubWorkflowID:         "sub-wf-id",
		MaxNestingDepth:       3,
		EventKey:              "evt-key-123",
		EventTimeoutSecs:      7200,
		EventNotifyURL:        "https://hooks.example.com",
		SleepDurationSecs:     45,
		EventEmitKey:          "emit-done",
		ConcurrencyKey:        "project:deploy",
		ResourceClass:         "large",
	}

	def := domain.WorkflowSnapshotDefinition{
		Workflow: domain.WorkflowSnapshotMeta{ID: "wf-id-1"},
		Steps:    []domain.WorkflowStep{step},
	}

	data, _ := json.Marshal(def)
	parsed, err := ParseSnapshotDefinition(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	got := parsed.Steps[0]

	// Check every field
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"ID", got.ID, step.ID},
		{"WorkflowID", got.WorkflowID, step.WorkflowID},
		{"JobID", got.JobID, step.JobID},
		{"StepRef", got.StepRef, step.StepRef},
		{"DependsOn_len", len(got.DependsOn), 3},
		{"OnFailure", string(got.OnFailure), string(step.OnFailure)},
		{"StepType", string(got.StepType), string(step.StepType)},
		{"ApprovalTimeoutSecs", got.ApprovalTimeoutSecs, step.ApprovalTimeoutSecs},
		{"ApprovalApprovers_len", len(got.ApprovalApprovers), 1},
		{"RetryMaxAttempts", got.RetryMaxAttempts, step.RetryMaxAttempts},
		{"RetryBackoff", string(got.RetryBackoff), string(step.RetryBackoff)},
		{"RetryInitialDelaySecs", got.RetryInitialDelaySecs, step.RetryInitialDelaySecs},
		{"RetryMaxDelaySecs", got.RetryMaxDelaySecs, step.RetryMaxDelaySecs},
		{"TimeoutSecsOverride", got.TimeoutSecsOverride, step.TimeoutSecsOverride},
		{"OutputTransform", got.OutputTransform, step.OutputTransform},
		{"SubWorkflowID", got.SubWorkflowID, step.SubWorkflowID},
		{"MaxNestingDepth", got.MaxNestingDepth, step.MaxNestingDepth},
		{"EventKey", got.EventKey, step.EventKey},
		{"EventTimeoutSecs", got.EventTimeoutSecs, step.EventTimeoutSecs},
		{"EventNotifyURL", got.EventNotifyURL, step.EventNotifyURL},
		{"SleepDurationSecs", got.SleepDurationSecs, step.SleepDurationSecs},
		{"EventEmitKey", got.EventEmitKey, step.EventEmitKey},
		{"ConcurrencyKey", got.ConcurrencyKey, step.ConcurrencyKey},
		{"ResourceClass", got.ResourceClass, step.ResourceClass},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestParseSnapshotDefinition_EmptyDefinition(t *testing.T) {
	t.Parallel()
	_, err := ParseSnapshotDefinition(nil)
	if err == nil {
		t.Error("expected error for nil definition")
	}
	_, err = ParseSnapshotDefinition(json.RawMessage(``))
	if err == nil {
		t.Error("expected error for empty definition")
	}
}

func TestParseSnapshotDefinition_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := ParseSnapshotDefinition(json.RawMessage(`{broken`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseSnapshotDefinition_DuplicateStepRefs(t *testing.T) {
	t.Parallel()
	def := domain.WorkflowSnapshotDefinition{
		Steps: []domain.WorkflowStep{
			{StepRef: "build", StepType: domain.WorkflowStepTypeJob},
			{StepRef: "build", StepType: domain.WorkflowStepTypeJob}, // duplicate
		},
	}
	data, _ := json.Marshal(def)
	_, err := ParseSnapshotDefinition(data)
	if err == nil {
		t.Error("expected error for duplicate step_ref")
	}
}

func TestParseSnapshotDefinition_ZeroSteps(t *testing.T) {
	t.Parallel()
	def := domain.WorkflowSnapshotDefinition{
		Workflow: domain.WorkflowSnapshotMeta{ID: "wf-1"},
		Steps:    []domain.WorkflowStep{},
	}
	data, _ := json.Marshal(def)
	// Zero steps is valid — a workflow can have no steps at trigger time
	// (e.g., all steps disabled via overrides).
	parsed, err := ParseSnapshotDefinition(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parsed.Steps) != 0 {
		t.Errorf("steps = %d, want 0", len(parsed.Steps))
	}
}

func TestParseSnapshotDefinition_UniqueStepRefs_Pass(t *testing.T) {
	t.Parallel()
	def := domain.WorkflowSnapshotDefinition{
		Steps: []domain.WorkflowStep{
			{StepRef: "a"},
			{StepRef: "b"},
			{StepRef: "c"},
		},
	}
	data, _ := json.Marshal(def)
	parsed, err := ParseSnapshotDefinition(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parsed.Steps) != 3 {
		t.Errorf("steps = %d, want 3", len(parsed.Steps))
	}
}
