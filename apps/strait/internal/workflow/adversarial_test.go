package workflow

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestValidateDAG_CycleDetection(t *testing.T) {
	t.Parallel()

	steps := []domain.WorkflowStep{
		{StepRef: "A", DependsOn: []string{"C"}},
		{StepRef: "B", DependsOn: []string{"A"}},
		{StepRef: "C", DependsOn: []string{"B"}},
	}
	err := ValidateDAG(steps)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cycle")
}

func TestValidateDAG_SelfLoop(t *testing.T) {
	t.Parallel()

	steps := []domain.WorkflowStep{
		{StepRef: "A", DependsOn: []string{"A"}},
	}
	err := ValidateDAG(steps)
	require.Error(t, err)
	require.Contains(t, err.Error(), "depends on itself")
}

func TestValidateDAG_DisconnectedNodes(t *testing.T) {
	t.Parallel()

	// Two independent subgraphs are valid DAGs; both are roots.
	steps := []domain.WorkflowStep{
		{StepRef: "A"},
		{StepRef: "B"},
		{StepRef: "C", DependsOn: []string{"A"}},
	}
	err := ValidateDAG(steps)
	require.NoError(t, err)
}

func TestValidateDAG_DuplicateStepRefs(t *testing.T) {
	t.Parallel()

	steps := []domain.WorkflowStep{
		{StepRef: "A"},
		{StepRef: "A"},
	}
	err := ValidateDAG(steps)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate step_ref")
}

func TestValidateDAG_EmptySteps(t *testing.T) {
	t.Parallel()

	err := ValidateDAG(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one step")

	err = ValidateDAG([]domain.WorkflowStep{})
	require.Error(t, err)
}

func TestValidateDAG_LargeGraph(t *testing.T) {
	t.Parallel()

	// Build a linear chain of 100 steps.
	steps := make([]domain.WorkflowStep, 100)
	for i := range 100 {
		ref := fmt.Sprintf("step_%d", i)
		var deps []string
		if i > 0 {
			deps = []string{fmt.Sprintf("step_%d", i-1)}
		}
		steps[i] = domain.WorkflowStep{StepRef: ref, DependsOn: deps}
	}
	err := ValidateDAG(steps)
	require.NoError(t, err)
}

func TestEvaluateCondition_TypeCoercion(t *testing.T) {
	t.Parallel()

	// String "42" compared with eq against number 42 via fmt.Sprint coercion.
	cond := json.RawMessage(`{"type":"eq","left":{"value":"42"},"right":{"value":42}}`)
	statuses := map[string]domain.StepRunStatus{}
	ok, err := EvaluateCondition(cond, statuses)
	require.NoError(t, err)
	require.True(t, ok)

	// String "42" with gt should fail because gt requires numeric operands.
	cond = json.RawMessage(`{"type":"gt","left":{"value":"42"},"right":{"value":10}}`)
	_, err = EvaluateCondition(cond, statuses)
	require.Error(t, err)
}

func TestEvaluateCondition_DeeplyNested(t *testing.T) {
	t.Parallel()

	// Build a 50-level deep all_of nesting, each wrapping a step_status check.
	statuses := map[string]domain.StepRunStatus{
		"root": domain.StepCompleted,
	}

	inner := json.RawMessage(`{"type":"step_status","step_ref":"root","status":"completed"}`)
	for range 50 {
		wrapped, err := json.Marshal(map[string]any{
			"type":       "all_of",
			"conditions": []json.RawMessage{inner},
		})
		require.NoError(t, err)

		inner = wrapped
	}

	ok, err := EvaluateCondition(inner, statuses)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestEvaluateCondition_UnknownType(t *testing.T) {
	t.Parallel()

	cond := json.RawMessage(`{"type":"xor","left":true,"right":false}`)
	statuses := map[string]domain.StepRunStatus{}
	_, err := EvaluateCondition(cond, statuses)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown condition type")
}

func TestRenderTemplateVars_DeeplyNested(t *testing.T) {
	t.Parallel()

	// Build a 100-level nested variable path: a.b.c.d...
	parts := make([]string, 100)
	for i := range parts {
		parts[i] = fmt.Sprintf("k%d", i)
	}
	varPath := strings.Join(parts, ".")

	// Build nested vars object.
	var current any = "leaf_value"
	for _, part := range slices.Backward(parts) {
		current = map[string]any{part: current}
	}
	varsBytes, err := json.Marshal(current)
	require.NoError(t, err)

	payload := fmt.Sprintf(`{"val":"{{%s}}"}`, varPath)
	result := renderTemplateVars(json.RawMessage(payload), varsBytes)

	var out map[string]any
	require.NoError(t, json.
		Unmarshal(result,
			&out))
	require.Equal(t, "leaf_value",

		out["val"],
	)
}

func TestRenderTemplateVars_HugePayload(t *testing.T) {
	t.Parallel()

	// Build a ~5MB payload string with a template var at the end.
	filler := strings.Repeat("x", 5*1024*1024)
	payload := fmt.Sprintf(`{"data":"%s {{name}}"}`, filler)
	vars := `{"name":"world"}`

	result := renderTemplateVars(json.RawMessage(payload), json.RawMessage(vars))

	var out map[string]any
	require.NoError(t, json.
		Unmarshal(result,
			&out))

	data, ok := out["data"].(string)
	require.True(t, ok)
	require.True(t, strings.HasSuffix(data, " world"))
}

func TestRenderTemplateVars_SpecialCharsInVarNames(t *testing.T) {
	t.Parallel()

	// The regex only matches word chars and dots, so {{a.b[0].c}} should not resolve.
	payload := `{"val":"{{a.b[0].c}}"}`
	vars := `{"a":{"b":{"0":{"c":"found"}}}}`

	result := renderTemplateVars(json.RawMessage(payload), json.RawMessage(vars))

	var out map[string]any
	require.NoError(t, json.
		Unmarshal(result,
			&out))

	// The bracket syntax should not be matched by the template regex.
	val, ok := out["val"].(string)
	require.True(t, ok)
	require.Contains(t, val, "{{")
}

func FuzzEvaluateConditionAdversarial(f *testing.F) {
	f.Add(`{"type":"step_status","step_ref":"a","status":"completed"}`)
	f.Add(`{"type":"eq","left":{"value":1},"right":{"value":1}}`)
	f.Add(`{"type":"unknown"}`)
	f.Add(`{}`)
	f.Add(`""`)
	f.Add(`null`)
	f.Add(`{"type":"not","condition":{"type":"step_status","step_ref":"x","status":"failed"}}`)
	f.Add(`{"type":"all_of","conditions":[]}`)

	f.Fuzz(func(t *testing.T, condJSON string) {
		statuses := map[string]domain.StepRunStatus{
			"a": domain.StepCompleted,
			"b": domain.StepFailed,
		}
		// Must not panic.
		_, _ = EvaluateCondition(json.RawMessage(condJSON), statuses)
	})
}

func FuzzRenderTemplateVarsAdversarial(f *testing.F) {
	f.Add(`{"key":"{{name}}"}`, `{"name":"val"}`)
	f.Add(`{"key":"hello"}`, `{}`)
	f.Add(`{}`, `{}`)
	f.Add(`{"a":"{{x.y.z}}"}`, `{"x":{"y":{"z":42}}}`)
	f.Add(`""`, `""`)
	f.Add(`null`, `null`)
	f.Add(`{"k":"{{broken"}`, `{"broken":1}`)

	f.Fuzz(func(t *testing.T, payload, vars string) {
		// Must not panic.
		_ = renderTemplateVars(json.RawMessage(payload), json.RawMessage(vars))
	})
}
