package authoring

import (
	"context"
	"errors"
	"testing"
)

type fakeWorkflowClient struct {
	createFn  func(ctx context.Context, body map[string]any) (map[string]any, error)
	triggerFn func(ctx context.Context, wfID string, body map[string]any) (map[string]any, error)
	getRunFn  func(ctx context.Context, runID string) (map[string]any, error)
}

func (f *fakeWorkflowClient) CreateWorkflow(ctx context.Context, body map[string]any) (map[string]any, error) {
	return f.createFn(ctx, body)
}
func (f *fakeWorkflowClient) TriggerWorkflow(ctx context.Context, wfID string, body map[string]any) (map[string]any, error) {
	return f.triggerFn(ctx, wfID, body)
}
func (f *fakeWorkflowClient) GetRun(ctx context.Context, runID string) (map[string]any, error) {
	return f.getRunFn(ctx, runID)
}

func TestDefineWorkflow_Kind(t *testing.T) {
	wf := DefineWorkflow(WorkflowOptions[testPayload]{
		Name:  "Test Workflow",
		Slug:  "test-wf",
		Steps: []Step{Job("a", "job_1")},
	})

	if wf.Kind != "workflow" {
		t.Errorf("expected kind 'workflow', got %q", wf.Kind)
	}
}

func TestDefineWorkflow_ToRegistrationBody(t *testing.T) {
	maxConc := 10
	maxPar := 3

	wf := DefineWorkflow(WorkflowOptions[testPayload]{
		Name:              "Order Pipeline",
		Slug:              "order-pipeline",
		ProjectID:         "proj_1",
		MaxConcurrentRuns: &maxConc,
		MaxParallelSteps:  &maxPar,
		Steps: []Step{
			Job("validate", "job_validate"),
			Job("charge", "job_charge", DependsOn("validate")),
		},
	})

	body, err := wf.ToRegistrationBody("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if body["name"] != "Order Pipeline" {
		t.Error("expected name")
	}
	if body["slug"] != "order-pipeline" {
		t.Error("expected slug")
	}
	steps, ok := body["steps"].([]map[string]any)
	if !ok || len(steps) != 2 {
		t.Error("expected 2 steps")
	}
	if body["max_concurrent_runs"] != 10 {
		t.Error("expected max_concurrent_runs")
	}
}

func TestDefineWorkflow_ToRegistrationBody_InvalidDAG(t *testing.T) {
	wf := DefineWorkflow(WorkflowOptions[testPayload]{
		Name:      "Bad",
		Slug:      "bad-wf",
		ProjectID: "proj_1",
		Steps: []Step{
			Job("a", "job_1", DependsOn("b")),
			Job("b", "job_2", DependsOn("a")),
		},
	})

	_, err := wf.ToRegistrationBody("")
	if err == nil {
		t.Fatal("expected error for cyclic DAG")
	}
}

func TestDefineWorkflow_ToRegistrationBody_MissingProjectID(t *testing.T) {
	wf := DefineWorkflow(WorkflowOptions[testPayload]{
		Name:  "Test",
		Slug:  "test",
		Steps: []Step{Job("a", "job_1")},
	})

	_, err := wf.ToRegistrationBody("")
	if err == nil {
		t.Fatal("expected error for missing projectId")
	}
}

func TestDefineWorkflow_Register(t *testing.T) {
	wf := DefineWorkflow(WorkflowOptions[testPayload]{
		Name:      "Test",
		Slug:      "test",
		ProjectID: "proj_1",
		Steps:     []Step{Job("a", "job_1")},
	})

	client := &fakeWorkflowClient{
		createFn: func(_ context.Context, body map[string]any) (map[string]any, error) {
			return map[string]any{"id": "wf_123"}, nil
		},
	}

	result, err := wf.Register(context.Background(), client, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "wf_123" {
		t.Error("expected wf_123")
	}
}

func TestDefineWorkflow_Trigger(t *testing.T) {
	wf := DefineWorkflow(WorkflowOptions[testPayload]{
		Name:      "Test",
		Slug:      "test",
		ProjectID: "proj_1",
		Steps:     []Step{Job("a", "job_1")},
	})

	client := &fakeWorkflowClient{
		createFn: func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{"id": "wf_123"}, nil
		},
		triggerFn: func(_ context.Context, wfID string, body map[string]any) (map[string]any, error) {
			if wfID != "wf_123" {
				t.Errorf("expected wf_123, got %q", wfID)
			}
			return map[string]any{"id": "wfrun_1", "status": "pending"}, nil
		},
	}

	_, _ = wf.Register(context.Background(), client, "")

	result, err := wf.Trigger(context.Background(), client, TriggerWorkflowInput[testPayload]{
		Payload: testPayload{SKU: "test"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "wfrun_1" {
		t.Error("expected wfrun_1")
	}
}

func TestDefineWorkflow_Trigger_NoID(t *testing.T) {
	wf := DefineWorkflow(WorkflowOptions[testPayload]{
		Name:  "Test",
		Slug:  "test",
		Steps: []Step{Job("a", "job_1")},
	})

	_, err := wf.Trigger(context.Background(), &fakeWorkflowClient{}, TriggerWorkflowInput[testPayload]{
		Payload: testPayload{SKU: "test"},
	})

	if err == nil {
		t.Fatal("expected error for missing workflowID")
	}
}

func TestDefineWorkflow_Register_Error(t *testing.T) {
	wf := DefineWorkflow(WorkflowOptions[testPayload]{
		Name:      "Test",
		Slug:      "test",
		ProjectID: "proj_1",
		Steps:     []Step{Job("a", "job_1")},
	})

	client := &fakeWorkflowClient{
		createFn: func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return nil, errors.New("api error")
		},
	}

	_, err := wf.Register(context.Background(), client, "")
	if err == nil || err.Error() != "api error" {
		t.Errorf("expected 'api error', got %v", err)
	}
}

func TestDefineDag_Kind(t *testing.T) {
	dag := DefineDag(WorkflowOptions[testPayload]{
		Name:  "Test DAG",
		Slug:  "test-dag",
		Steps: []Step{Job("a", "job_1")},
	})

	if dag.Kind != "dag" {
		t.Errorf("expected kind 'dag', got %q", dag.Kind)
	}
}

func TestDefineWorkflow_Trigger_WithStepOverrides(t *testing.T) {
	wf := DefineWorkflow(WorkflowOptions[testPayload]{
		Name:  "Test",
		Slug:  "test",
		Steps: []Step{Job("a", "job_1")},
	})

	client := &fakeWorkflowClient{
		triggerFn: func(_ context.Context, _ string, body map[string]any) (map[string]any, error) {
			if body["step_overrides"] == nil {
				t.Error("expected step_overrides")
			}
			return map[string]any{"id": "wfrun_1"}, nil
		},
	}

	_, err := wf.Trigger(context.Background(), client, TriggerWorkflowInput[testPayload]{
		WorkflowID:    "wf_1",
		Payload:       testPayload{SKU: "test"},
		StepOverrides: map[string]any{"validate": map[string]any{"skip": true}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
