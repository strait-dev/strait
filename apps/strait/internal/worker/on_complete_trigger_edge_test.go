package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.NoError(
		t, err)

	var out map[string]any
	require.NoError(
		t, json.Unmarshal(mapped,
			&out))
	assert.Equal(t,
		"deep", out["deep_val"])
}

func TestApplyPayloadMapping_MissingPaths(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{"existing": "value"}`)
	mapping := json.RawMessage(`{"out": "nonexistent.path"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	require.NoError(
		t, err)

	var out map[string]any
	require.NoError(
		t, json.Unmarshal(mapped,
			&out))

	// Missing paths should produce no output key.
	if _, ok := out["out"]; ok {
		assert.Fail(t,

			"missing path should not produce output key")
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
	require.NoError(
		t, err)

	var out map[string]any
	require.NoError(
		t, json.Unmarshal(mapped,
			&out))
	assert.Equal(t,
		"u1", out["user_id"])

	if _, ok := out["missing_field"]; ok {
		assert.Fail(t,

			"missing_field should not be present")
	}
	assert.InDelta(t,
		float64(2), out["version"], 1e-9)
}

func TestApplyPayloadMapping_DirectOutputSortedAndSkipsMissing(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{"user":{"id":"u1"},"order":{"id":"o1"}}`)
	mapping := json.RawMessage(`{"z_user":"user.id","a_order":"order.id","m_missing":"missing.path"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	require.NoError(t, err)
	assert.Equal(t, `{"a_order":"o1","z_user":"u1"}`, string(mapped))
}

func TestApplyPayloadMapping_TopLevelKeys(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{"status": "ok", "count": 42}`)
	mapping := json.RawMessage(`{"s": "status", "c": "count"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	require.NoError(
		t, err)

	var out map[string]any
	require.NoError(
		t, json.Unmarshal(mapped,
			&out))
	assert.Equal(t,
		"ok", out["s"])
}

func TestApplyPayloadMapping_InvalidMapping(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{"a": 1}`)
	mapping := json.RawMessage(`not valid json`)

	_, err := applyPayloadMapping(result, mapping)
	require.Error(t,
		err)
}

func TestApplyPayloadMapping_EmptyMapping(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{"a": 1}`)
	mapping := json.RawMessage(`{}`)

	mapped, err := applyPayloadMapping(result, mapping)
	require.NoError(
		t, err)

	// Empty mapping -> empty output.
	var out map[string]any
	require.NoError(
		t, json.Unmarshal(mapped,
			&out))
	assert.Empty(t, out)
}

func TestApplyPayloadMapping_ArrayResult(t *testing.T) {
	t.Parallel()
	// Arrays cannot be navigated with dot-notation; result returned as-is.
	result := json.RawMessage(`[1, 2, 3]`)
	mapping := json.RawMessage(`{"first": "0"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	require.NoError(
		t, err)
	assert.Equal(t,
		string(result), string(mapped))
}

func TestApplyPayloadMapping_ScalarResult(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`"just a string"`)
	mapping := json.RawMessage(`{"val": "key"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	require.NoError(
		t, err)
	assert.Equal(t,
		string(result), string(mapped))
}

func BenchmarkApplyPayloadMapping(b *testing.B) {
	result := json.RawMessage(`{"user":{"id":"usr-123","name":"Ada Lovelace","plan":"pro"},"order":{"id":"ord-456","total":129.95,"items":[{"sku":"sku-1","qty":2},{"sku":"sku-2","qty":1}]},"flags":{"trial":false,"priority":true},"metadata":{"region":"us-east-1","source":"api"}}`)
	mapping := json.RawMessage(`{"user_id":"user.id","user_name":"user.name","order_id":"order.id","order_total":"order.total","first_sku":"order.items.0.sku","priority":"flags.priority","region":"metadata.region"}`)

	b.ReportAllocs()
	for b.Loop() {
		mapped, err := applyPayloadMapping(result, mapping)
		if err != nil {
			b.Fatal(err)
		}
		if len(mapped) == 0 {
			b.Fatal("applyPayloadMapping() returned empty payload")
		}
	}
}

// extractPath edge cases.

func TestExtractPath_EmptyPath(t *testing.T) {
	t.Parallel()
	data := map[string]any{"key": "value", "": "empty_key"}
	got := extractPath(data, "")
	assert.Equal(t,
		"empty_key", got)

	// Empty path looks up empty string key.
}

func TestExtractPath_SingleLevel(t *testing.T) {
	t.Parallel()
	data := map[string]any{"key": "value"}
	got := extractPath(data, "key")
	assert.Equal(t,
		"value", got)
}

func TestExtractPath_NestedArray(t *testing.T) {
	t.Parallel()
	data := map[string]any{"items": []any{1, 2, 3}}
	got := extractPath(data, "items")
	arr, ok := got.([]any)
	require.True(t,
		ok)
	assert.Len(t, arr,
		3)
}

func TestExtractPath_IntermediateNonMap(t *testing.T) {
	t.Parallel()
	data := map[string]any{"a": "string_value"}
	got := extractPath(data, "a.b.c")
	assert.Nil(t, got)
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
	require.Len(t, trigger.
		calls, 1)
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
	require.Len(t, trigger.
		calls, 1)
	assert.GreaterOrEqual(t, len(trigger.
		calls[0].payload,
	), 1000)
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
	require.Len(t, trigger.
		calls, 1)
	assert.Equal(t,
		string(result), string(trigger.
			calls[0].payload))

	// Should have received the full result as fallback.
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
	assert.Len(t, trigger.
		calls, 20)
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
			assert.Empty(t, trigger.
				calls)
		})
	}
}
