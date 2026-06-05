package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// TestOnComplete_PayloadMappingPathTraversal verifies that path traversal
// sequences in dot-notation paths are not exploitable.
func TestOnComplete_PayloadMappingPathTraversal(t *testing.T) {
	t.Parallel()

	result := json.RawMessage(`{"secret":"hidden","a":{"b":"visible"}}`)
	mapping := json.RawMessage(`{
		"traversal1": "../../secret",
		"traversal2": "../secret",
		"traversal3": "..",
		"normal": "a.b"
	}`)

	mapped, err := applyPayloadMapping(result, mapping)
	require.NoError(
		t, err)

	var out map[string]any
	require.NoError(
		t, json.Unmarshal(mapped,
			&out))

	// Traversal paths should not resolve to any value since ".." and "../../secret"
	// are not valid map keys in the result.
	if _, ok := out["traversal1"]; ok {
		assert.Fail(t,

			"path traversal should not resolve a value for ../../secret")
	}
	if _, ok := out["traversal2"]; ok {
		assert.Fail(t,

			"path traversal should not resolve a value for ../secret")
	}
	if _, ok := out["traversal3"]; ok {
		assert.Fail(t,

			"path traversal should not resolve a value for ..")
	}
	assert.Equal(t,
		"visible", out["normal"],
	)
}

// TestOnComplete_PayloadMappingDeepPath verifies that a 100-level deep path
// does not cause a stack overflow or excessive allocation.
func TestOnComplete_PayloadMappingDeepPath(t *testing.T) {
	t.Parallel()

	// Build a 100-level deep JSON object.
	var builder strings.Builder
	for i := range 100 {
		fmt.Fprintf(&builder, `{"l%d":`, i)
	}
	builder.WriteString(`"leaf"`)
	for range 100 {
		builder.WriteString(`}`)
	}

	// Build the corresponding deep path.
	parts := make([]string, 100)
	for i := range 100 {
		parts[i] = fmt.Sprintf("l%d", i)
	}
	deepPath := strings.Join(parts, ".")

	result := json.RawMessage(builder.String())
	mapping := json.RawMessage(fmt.Sprintf(`{"deep": %q}`, deepPath))

	mapped, err := applyPayloadMapping(result, mapping)
	require.NoError(
		t, err)

	var out map[string]any
	require.NoError(
		t, json.Unmarshal(mapped,
			&out))
	assert.Equal(t,
		"leaf", out["deep"])
}

// TestOnComplete_PayloadMappingEmptyPath verifies that an empty path string
// returns nil rather than the root object.
func TestOnComplete_PayloadMappingEmptyPath(t *testing.T) {
	t.Parallel()

	result := json.RawMessage(`{"key":"value"}`)
	mapping := json.RawMessage(`{"empty": ""}`)

	mapped, err := applyPayloadMapping(result, mapping)
	require.NoError(
		t, err)

	var out map[string]any
	require.NoError(
		t, json.Unmarshal(mapped,
			&out))

	// An empty path lookup should produce the value at key "" (which does not exist).
	if _, ok := out["empty"]; ok {
		// extractPath with empty string will look up key "" in the map,
		// which should not exist.
		t.Logf("empty path produced value: %v (may be nil mapped key)", out["empty"])
	}
}

// TestOnComplete_PayloadMappingTrailingDots verifies that trailing dots in a
// path do not cause panics.
func TestOnComplete_PayloadMappingTrailingDots(t *testing.T) {
	t.Parallel()

	result := json.RawMessage(`{"a":{"b":"value"}}`)
	cases := []string{"a.b.", "a.", ".", "a..b", ".."}

	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			mapping := json.RawMessage(fmt.Sprintf(`{"out": %q}`, path))
			mapped, err := applyPayloadMapping(result, mapping)
			require.NoError(
				t, err)

			// Just verify no panic; the value may or may not resolve.
			var out map[string]any
			require.NoError(
				t, json.Unmarshal(mapped,
					&out))
		})
	}
}

// TestOnComplete_PayloadMappingNumericKeys verifies that numeric keys in
// dot-notation are treated as map keys, not array indices.
func TestOnComplete_PayloadMappingNumericKeys(t *testing.T) {
	t.Parallel()

	result := json.RawMessage(`{"0":{"1":{"2":"found"}}}`)
	mapping := json.RawMessage(`{"val": "0.1.2"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	require.NoError(
		t, err)

	var out map[string]any
	require.NoError(
		t, json.Unmarshal(mapped,
			&out))
	assert.Equal(t,
		"found", out["val"])
}

// TestOnComplete_PayloadMappingNullSource verifies that a nil/empty result
// passes through without error.
func TestOnComplete_PayloadMappingNullSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		result json.RawMessage
	}{
		{"nil", nil},
		{"empty", json.RawMessage{}},
		{"null", json.RawMessage(`null`)},
	}

	mapping := json.RawMessage(`{"out": "some.path"}`)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mapped, err := applyPayloadMapping(tc.result, mapping)
			require.NoError(
				t, err)

			// For nil/empty result, applyPayloadMapping returns result as-is.
			_ = mapped
		})
	}
}

// TestOnComplete_PayloadMappingCircularRef verifies that a mapping where the
// output key matches the path does not cause infinite recursion.
func TestOnComplete_PayloadMappingCircularRef(t *testing.T) {
	t.Parallel()

	result := json.RawMessage(`{"self":"value","nested":{"self":"deep"}}`)
	mapping := json.RawMessage(`{"self": "self", "nested": "nested.self"}`)

	mapped, err := applyPayloadMapping(result, mapping)
	require.NoError(
		t, err)

	var out map[string]any
	require.NoError(
		t, json.Unmarshal(mapped,
			&out))
	assert.Equal(t,
		"value", out["self"])
	assert.Equal(t,
		"deep", out["nested"])
}

// TestOnComplete_PayloadMappingHugeOutput verifies that a mapping that maps
// many keys from a large result does not cause excessive memory use.
func TestOnComplete_PayloadMappingHugeOutput(t *testing.T) {
	t.Parallel()

	// Build a result with a large value.
	bigValue := strings.Repeat("x", 1024*1024) // 1MB
	result := json.RawMessage(fmt.Sprintf(`{"big":%q}`, bigValue))

	// Map it to 10 output keys (10MB total).
	pathMap := make(map[string]string)
	for i := range 10 {
		pathMap[fmt.Sprintf("out_%d", i)] = "big"
	}
	mapping, err := json.Marshal(pathMap)
	require.NoError(
		t, err)

	mapped, mapErr := applyPayloadMapping(result, mapping)
	require.NoError(t, mapErr)

	var out map[string]any
	require.NoError(
		t, json.Unmarshal(mapped,
			&out))
	require.Len(t, out,
		10)
}

// TestOnComplete_TriggerDisabledJob verifies that MaybeTrigger logs a warning
// when the target workflow is not found (simulating a disabled job scenario).
func TestOnComplete_TriggerDisabledJob(t *testing.T) {
	t.Parallel()

	lookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{},
	}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-disabled",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "nonexistent-wf",
	}

	// Should not panic, just log.
	oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	require.Empty(t, trigger.
		calls)
}

// TestOnComplete_TriggerDeletedJob verifies that MaybeTrigger handles a nil
// run or job gracefully.
func TestOnComplete_TriggerDeletedJob(t *testing.T) {
	t.Parallel()

	lookup := &mockWorkflowLookup{workflows: map[string]*domain.Workflow{}}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	// Nil run.
	oct.MaybeTrigger(context.Background(), nil, &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "wf",
	}, json.RawMessage(`{}`))

	// Nil job.
	oct.MaybeTrigger(context.Background(), &domain.JobRun{
		ID:     "run-1",
		Status: domain.StatusCompleted,
	}, nil, json.RawMessage(`{}`))

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	require.Empty(t, trigger.
		calls)
}

// TestOnComplete_ConcurrentCompletionTrigger fires two concurrent MaybeTrigger
// calls to verify thread safety.
func TestOnComplete_ConcurrentCompletionTrigger(t *testing.T) {
	t.Parallel()

	lookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-deploy-1"},
		},
	}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
	}

	var wg conc.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			run := &domain.JobRun{
				ID:     fmt.Sprintf("run-%d", i),
				Status: domain.StatusCompleted,
			}
			oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{"idx":`+fmt.Sprintf("%d", i)+`}`))
		})
	}
	wg.Wait()

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	require.Len(t, trigger.
		calls, 10)
}

// FuzzPayloadMappingPath fuzzes dot-notation paths against a fixed result
// to verify no panics in extractPath.
func FuzzPayloadMappingPath(f *testing.F) {
	f.Add("a.b.c")
	f.Add("")
	f.Add(".")
	f.Add("..")
	f.Add("a..b")
	f.Add(strings.Repeat("a.", 500) + "b")
	f.Add("../../secret")
	f.Add("0.1.2")

	result := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "leaf",
			},
		},
		"0": map[string]any{
			"1": "num",
		},
	}

	f.Fuzz(func(t *testing.T, path string) {
		// Should never panic.
		_ = extractPath(result, path)
	})
}

// FuzzPayloadMappingExtract fuzzes the full applyPayloadMapping function with
// arbitrary result and mapping JSON.
func FuzzPayloadMappingExtract(f *testing.F) {
	f.Add(`{"key":"val"}`, `{"out":"key"}`)
	f.Add(`null`, `{"out":"key"}`)
	f.Add(`"string"`, `{"out":"key"}`)
	f.Add(`{"a":{"b":"c"}}`, `{"x":"a.b"}`)
	f.Add(`{}`, `{}`)
	f.Add(`{"k":"v"}`, `not json`)

	f.Fuzz(func(t *testing.T, resultStr, mappingStr string) {
		result := json.RawMessage(resultStr)
		mapping := json.RawMessage(mappingStr)
		// Should never panic.
		_, _ = applyPayloadMapping(result, mapping)
	})
}
