package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/sourcegraph/conc"

	"strait/internal/domain"
)

// Payload mapping edge cases.

func TestApplyPayloadMapping_DeeplyNestedPaths(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{
		"level1": {
			"level2": {
				"level3": {
					"level4": {
						"value": "deep"
					}
				}
			}
		}
	}`)
	mapping := json.RawMessage(`{"deep_val": "level1.level2.level3.level4.value"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(mapped, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["deep_val"] != "deep" {
		t.Errorf("deep_val = %v, want %q", out["deep_val"], "deep")
	}
}

func TestApplyPayloadMapping_MissingPaths(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{"existing": "value"}`)
	mapping := json.RawMessage(`{"out": "nonexistent.path"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(mapped, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Missing paths should produce no output key.
	if _, ok := out["out"]; ok {
		t.Error("missing path should not produce output key")
	}
}

func TestApplyPayloadMapping_MixedExistingAndMissing(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{"user": {"id": "u1", "name": "Alice"}, "meta": {"version": 2}}`)
	mapping := json.RawMessage(`{
		"user_id": "user.id",
		"missing_field": "user.nonexistent",
		"version": "meta.version"
	}`)

	mapped, err := applyPayloadMapping(result, mapping)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(mapped, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["user_id"] != "u1" {
		t.Errorf("user_id = %v, want u1", out["user_id"])
	}
	if _, ok := out["missing_field"]; ok {
		t.Error("missing_field should not be present")
	}
	if out["version"] != float64(2) {
		t.Errorf("version = %v, want 2", out["version"])
	}
}

func TestApplyPayloadMapping_TopLevelKeys(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{"status": "ok", "count": 42}`)
	mapping := json.RawMessage(`{"s": "status", "c": "count"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(mapped, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["s"] != "ok" {
		t.Errorf("s = %v, want ok", out["s"])
	}
}

func TestApplyPayloadMapping_InvalidMapping(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{"a": 1}`)
	mapping := json.RawMessage(`not valid json`)

	_, err := applyPayloadMapping(result, mapping)
	if err == nil {
		t.Fatal("expected error for invalid mapping JSON")
	}
}

func TestApplyPayloadMapping_EmptyMapping(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{"a": 1}`)
	mapping := json.RawMessage(`{}`)

	mapped, err := applyPayloadMapping(result, mapping)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty mapping -> empty output.
	var out map[string]any
	if err := json.Unmarshal(mapped, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("empty mapping should produce empty output, got %v", out)
	}
}

func TestApplyPayloadMapping_ArrayResult(t *testing.T) {
	t.Parallel()
	// Arrays cannot be navigated with dot-notation; result returned as-is.
	result := json.RawMessage(`[1, 2, 3]`)
	mapping := json.RawMessage(`{"first": "0"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(mapped) != string(result) {
		t.Errorf("array result should pass through, got %s", mapped)
	}
}

func TestApplyPayloadMapping_ScalarResult(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`"just a string"`)
	mapping := json.RawMessage(`{"val": "key"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(mapped) != string(result) {
		t.Errorf("scalar result should pass through, got %s", mapped)
	}
}

// extractPath edge cases.

func TestExtractPath_EmptyPath(t *testing.T) {
	t.Parallel()
	data := map[string]any{"key": "value", "": "empty_key"}
	got := extractPath(data, "")
	// Empty path looks up empty string key.
	if got != "empty_key" {
		t.Errorf("extractPath('') = %v, want 'empty_key'", got)
	}
}

func TestExtractPath_SingleLevel(t *testing.T) {
	t.Parallel()
	data := map[string]any{"key": "value"}
	got := extractPath(data, "key")
	if got != "value" {
		t.Errorf("extractPath('key') = %v, want 'value'", got)
	}
}

func TestExtractPath_NestedArray(t *testing.T) {
	t.Parallel()
	data := map[string]any{"items": []any{1, 2, 3}}
	got := extractPath(data, "items")
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", got)
	}
	if len(arr) != 3 {
		t.Errorf("len = %d, want 3", len(arr))
	}
}

func TestExtractPath_IntermediateNonMap(t *testing.T) {
	t.Parallel()
	data := map[string]any{"a": "string_value"}
	got := extractPath(data, "a.b.c")
	if got != nil {
		t.Errorf("expected nil for path through non-map, got %v", got)
	}
}

// OnCompleteTrigger advanced scenarios.

func TestOnCompleteTrigger_EmptyResult(t *testing.T) {
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
		ID: "job-1", ProjectID: "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
	}

	// Empty result should still trigger with nil payload.
	oct.MaybeTrigger(context.Background(), run, job, nil)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if len(trigger.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(trigger.calls))
	}
}

func TestOnCompleteTrigger_LargePayload(t *testing.T) {
	t.Parallel()
	lookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/process": {ID: "wf-2"},
		},
	}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	// Build a large result.
	items := make([]map[string]string, 1000)
	for i := range items {
		items[i] = map[string]string{"id": fmt.Sprintf("item-%d", i)}
	}
	result, _ := json.Marshal(map[string]any{"items": items})

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID: "job-1", ProjectID: "proj-1",
		OnCompleteTriggerWorkflow: "process",
	}

	oct.MaybeTrigger(context.Background(), run, job, result)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if len(trigger.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(trigger.calls))
	}
	if len(trigger.calls[0].payload) < 1000 {
		t.Error("large payload should be passed through")
	}
}

func TestOnCompleteTrigger_PayloadMappingError(t *testing.T) {
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
		ID: "job-1", ProjectID: "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
		OnCompletePayloadMapping:  json.RawMessage(`invalid json`),
	}
	result := json.RawMessage(`{"data": "value"}`)

	// Should fall back to full result when mapping fails.
	oct.MaybeTrigger(context.Background(), run, job, result)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if len(trigger.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(trigger.calls))
	}
	// Should have received the full result as fallback.
	if string(trigger.calls[0].payload) != string(result) {
		t.Errorf("expected full result as fallback, got %s", trigger.calls[0].payload)
	}
}

func TestOnCompleteTrigger_ConcurrentTriggers(t *testing.T) {
	t.Parallel()
	lookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-1"},
		},
	}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	var wg conc.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			run := &domain.JobRun{
				ID:     fmt.Sprintf("run-%d", i),
				Status: domain.StatusCompleted,
			}
			job := &domain.Job{
				ID: fmt.Sprintf("job-%d", i), ProjectID: "proj-1",
				OnCompleteTriggerWorkflow: "deploy",
			}
			oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))
		})
	}
	wg.Wait()

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if len(trigger.calls) != 20 {
		t.Errorf("expected 20 concurrent triggers, got %d", len(trigger.calls))
	}
}

func TestOnCompleteTrigger_AllNonCompletedStatuses(t *testing.T) {
	t.Parallel()
	statuses := []domain.RunStatus{
		domain.StatusQueued,
		domain.StatusDequeued,
		domain.StatusExecuting,
		domain.StatusFailed,
		domain.StatusTimedOut,
		domain.StatusCrashed,
		domain.StatusCanceled,
		domain.StatusExpired,
		domain.StatusDeadLetter,
		domain.StatusSystemFailed,
		domain.StatusWaiting,
		domain.StatusPaused,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			trigger := &mockWorkflowTriggerer{}
			oct := NewOnCompleteTrigger(
				&mockWorkflowLookup{workflows: map[string]*domain.Workflow{"p/w": {ID: "wf"}}},
				trigger, nil, nil, nil,
			)
			run := &domain.JobRun{ID: "r", Status: status}
			job := &domain.Job{ID: "j", ProjectID: "p", OnCompleteTriggerWorkflow: "w"}
			oct.MaybeTrigger(context.Background(), run, job, nil)

			trigger.mu.Lock()
			defer trigger.mu.Unlock()
			if len(trigger.calls) != 0 {
				t.Errorf("status %s should not trigger workflow", status)
			}
		})
	}
}
