package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"strait/internal/domain"
)

type mockWorkflowLookup struct {
	mu        sync.Mutex
	workflows map[string]*domain.Workflow
}

func (m *mockWorkflowLookup) GetWorkflowBySlug(_ context.Context, projectID, slug string) (*domain.Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := projectID + "/" + slug
	if wf, ok := m.workflows[key]; ok {
		return wf, nil
	}
	return nil, fmt.Errorf("workflow %q not found in project %q", slug, projectID)
}

type triggerCall struct {
	workflowID  string
	projectID   string
	payload     json.RawMessage
	triggeredBy string
	extraTags   map[string]string
}

type mockWorkflowTriggerer struct {
	mu    sync.Mutex
	calls []triggerCall
	err   error
}

func (m *mockWorkflowTriggerer) TriggerWorkflow(_ context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, _ []domain.StepOverride, extraTags map[string]string) (*domain.WorkflowRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, triggerCall{
		workflowID:  workflowID,
		projectID:   projectID,
		payload:     payload,
		triggeredBy: triggeredBy,
		extraTags:   extraTags,
	})
	if m.err != nil {
		return nil, m.err
	}
	return &domain.WorkflowRun{ID: "wfr-test-1"}, nil
}

func TestOnCompleteTrigger_HappyPath(t *testing.T) {
	t.Parallel()
	lookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-deploy-1"},
		},
	}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
	}
	result := json.RawMessage(`{"output":"done"}`)

	oct.MaybeTrigger(context.Background(), run, job, result)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if len(trigger.calls) != 1 {
		t.Fatalf("expected 1 trigger call, got %d", len(trigger.calls))
	}
	call := trigger.calls[0]
	if call.workflowID != "wf-deploy-1" {
		t.Errorf("workflowID = %q, want %q", call.workflowID, "wf-deploy-1")
	}
	if call.triggeredBy != domain.TriggerJobCompletion {
		t.Errorf("triggeredBy = %q, want %q", call.triggeredBy, domain.TriggerJobCompletion)
	}
	if call.extraTags["source_job_id"] != "job-1" {
		t.Errorf("source_job_id = %q, want %q", call.extraTags["source_job_id"], "job-1")
	}
	if call.extraTags["source_run_id"] != "run-1" {
		t.Errorf("source_run_id = %q, want %q", call.extraTags["source_run_id"], "run-1")
	}
}

func TestOnCompleteTrigger_NoWorkflowConfigured(t *testing.T) {
	t.Parallel()
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(nil, trigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{ID: "job-1", ProjectID: "proj-1"}

	oct.MaybeTrigger(context.Background(), run, job, nil)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if len(trigger.calls) != 0 {
		t.Fatalf("expected 0 trigger calls for no workflow configured, got %d", len(trigger.calls))
	}
}

func TestOnCompleteTrigger_NonCompletedStatus(t *testing.T) {
	t.Parallel()
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(&mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-1"},
		},
	}, trigger, nil, nil, nil)

	// Failed runs should NOT trigger.
	for _, status := range []domain.RunStatus{
		domain.StatusFailed,
		domain.StatusTimedOut,
		domain.StatusCanceled,
		domain.StatusDeadLetter,
	} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			run := &domain.JobRun{ID: "run-1", Status: status}
			job := &domain.Job{
				ID:                        "job-1",
				ProjectID:                 "proj-1",
				OnCompleteTriggerWorkflow: "deploy",
			}

			oct.MaybeTrigger(context.Background(), run, job, nil)
		})
	}

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if len(trigger.calls) != 0 {
		t.Fatalf("expected 0 trigger calls for non-completed statuses, got %d", len(trigger.calls))
	}
}

func TestOnCompleteTrigger_WorkflowNotFound(t *testing.T) {
	t.Parallel()
	lookup := &mockWorkflowLookup{workflows: map[string]*domain.Workflow{}}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "nonexistent",
	}

	// Should not panic, should not trigger.
	oct.MaybeTrigger(context.Background(), run, job, nil)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if len(trigger.calls) != 0 {
		t.Fatalf("expected 0 trigger calls for missing workflow, got %d", len(trigger.calls))
	}
}

func TestOnCompleteTrigger_TriggerError(t *testing.T) {
	t.Parallel()
	lookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-1"},
		},
	}
	trigger := &mockWorkflowTriggerer{err: fmt.Errorf("trigger failed")}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
	}

	// Should not panic even if trigger fails.
	oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if len(trigger.calls) != 1 {
		t.Fatalf("expected 1 trigger call (even though it failed), got %d", len(trigger.calls))
	}
}

func TestOnCompleteTrigger_PayloadMapping(t *testing.T) {
	t.Parallel()
	lookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-1"},
		},
	}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
		OnCompletePayloadMapping:  json.RawMessage(`{"user_id":"user.id","name":"user.name"}`),
	}
	result := json.RawMessage(`{"user":{"id":"u-123","name":"Alice"},"extra":"ignored"}`)

	oct.MaybeTrigger(context.Background(), run, job, result)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if len(trigger.calls) != 1 {
		t.Fatalf("expected 1 trigger call, got %d", len(trigger.calls))
	}

	var mapped map[string]any
	if err := json.Unmarshal(trigger.calls[0].payload, &mapped); err != nil {
		t.Fatalf("unmarshal mapped payload: %v", err)
	}
	if mapped["user_id"] != "u-123" {
		t.Errorf("user_id = %v, want %q", mapped["user_id"], "u-123")
	}
	if mapped["name"] != "Alice" {
		t.Errorf("name = %v, want %q", mapped["name"], "Alice")
	}
	if _, ok := mapped["extra"]; ok {
		t.Error("mapped payload should not contain 'extra'")
	}
}

func TestExtractPath(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"user": map[string]any{
			"id":   "u-123",
			"name": "Alice",
			"address": map[string]any{
				"city": "NYC",
			},
		},
		"status": "active",
	}

	tests := []struct {
		path     string
		expected any
	}{
		{"status", "active"},
		{"user.id", "u-123"},
		{"user.name", "Alice"},
		{"user.address.city", "NYC"},
		{"nonexistent", nil},
		{"user.nonexistent", nil},
		{"user.address.nonexistent", nil},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			got := extractPath(data, tt.path)
			if got != tt.expected {
				t.Errorf("extractPath(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestApplyPayloadMapping_EmptyInputs(t *testing.T) {
	t.Parallel()

	// Empty result returns as-is.
	result, err := applyPayloadMapping(nil, json.RawMessage(`{"a":"b"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %s", result)
	}

	// Empty mapping returns result as-is.
	input := json.RawMessage(`{"key":"val"}`)
	result, err = applyPayloadMapping(input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != string(input) {
		t.Errorf("expected %s, got %s", input, result)
	}
}

func TestApplyPayloadMapping_NonObjectResult(t *testing.T) {
	t.Parallel()
	// Non-object result (array) should return as-is.
	input := json.RawMessage(`[1,2,3]`)
	result, err := applyPayloadMapping(input, json.RawMessage(`{"a":"0"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != string(input) {
		t.Errorf("expected %s, got %s", input, result)
	}
}

func TestOnCompleteTrigger_NilLookupAndTriggerer(t *testing.T) {
	t.Parallel()
	oct := NewOnCompleteTrigger(nil, nil, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
	}

	// Should not panic.
	oct.MaybeTrigger(context.Background(), run, job, nil)
}
