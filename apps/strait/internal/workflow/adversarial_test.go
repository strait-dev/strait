package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestValidateDAG_CycleDetection(t *testing.T) {
	t.Parallel()

	steps := []domain.WorkflowStep{
		{StepRef: "A", DependsOn: []string{"C"}},
		{StepRef: "B", DependsOn: []string{"A"}},
		{StepRef: "C", DependsOn: []string{"B"}},
	}
	err := ValidateDAG(steps)
	if err == nil {
		t.Fatal("expected cycle detection error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected error mentioning cycle, got: %v", err)
	}
}

func TestValidateDAG_SelfLoop(t *testing.T) {
	t.Parallel()

	steps := []domain.WorkflowStep{
		{StepRef: "A", DependsOn: []string{"A"}},
	}
	err := ValidateDAG(steps)
	if err == nil {
		t.Fatal("expected self-loop error, got nil")
	}
	if !strings.Contains(err.Error(), "depends on itself") {
		t.Fatalf("expected 'depends on itself' error, got: %v", err)
	}
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
	if err != nil {
		t.Fatalf("expected disconnected but acyclic DAG to be valid, got: %v", err)
	}
}

func TestValidateDAG_DuplicateStepRefs(t *testing.T) {
	t.Parallel()

	steps := []domain.WorkflowStep{
		{StepRef: "A"},
		{StepRef: "A"},
	}
	err := ValidateDAG(steps)
	if err == nil {
		t.Fatal("expected duplicate step_ref error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate step_ref") {
		t.Fatalf("expected 'duplicate step_ref' error, got: %v", err)
	}
}

func TestValidateDAG_EmptySteps(t *testing.T) {
	t.Parallel()

	err := ValidateDAG(nil)
	if err == nil {
		t.Fatal("expected error for nil steps, got nil")
	}
	if !strings.Contains(err.Error(), "at least one step") {
		t.Fatalf("expected 'at least one step' error, got: %v", err)
	}

	err = ValidateDAG([]domain.WorkflowStep{})
	if err == nil {
		t.Fatal("expected error for empty steps, got nil")
	}
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
	if err != nil {
		t.Fatalf("expected valid large DAG, got: %v", err)
	}
}

func TestEvaluateCondition_TypeCoercion(t *testing.T) {
	t.Parallel()

	// String "42" compared with eq against number 42 via fmt.Sprint coercion.
	cond := json.RawMessage(`{"type":"eq","left":{"value":"42"},"right":{"value":42}}`)
	statuses := map[string]domain.StepRunStatus{}
	ok, err := EvaluateCondition(cond, statuses)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected eq to coerce string '42' == number 42 via Sprint, got false")
	}

	// String "42" with gt should fail because gt requires numeric operands.
	cond = json.RawMessage(`{"type":"gt","left":{"value":"42"},"right":{"value":10}}`)
	_, err = EvaluateCondition(cond, statuses)
	if err == nil {
		t.Fatal("expected error for gt with string left operand, got nil")
	}
}

func TestEvaluateCondition_DeeplyNested(t *testing.T) {
	t.Parallel()

	// Build a 50-level deep all_of nesting, each wrapping a step_status check.
	statuses := map[string]domain.StepRunStatus{
		"root": domain.StepCompleted,
	}

	inner := json.RawMessage(`{"type":"step_status","step_ref":"root","status":"completed"}`)
	for i := range 50 {
		wrapped, err := json.Marshal(map[string]any{
			"type":       "all_of",
			"conditions": []json.RawMessage{inner},
		})
		if err != nil {
			t.Fatalf("failed to marshal nested condition at level %d: %v", i, err)
		}
		inner = wrapped
	}

	ok, err := EvaluateCondition(inner, statuses)
	if err != nil {
		t.Fatalf("unexpected error evaluating deeply nested condition: %v", err)
	}
	if !ok {
		t.Fatal("expected deeply nested condition to evaluate to true")
	}
}

func TestEvaluateCondition_UnknownType(t *testing.T) {
	t.Parallel()

	cond := json.RawMessage(`{"type":"xor","left":true,"right":false}`)
	statuses := map[string]domain.StepRunStatus{}
	_, err := EvaluateCondition(cond, statuses)
	if err == nil {
		t.Fatal("expected error for unknown condition type, got nil")
	}
	if !strings.Contains(err.Error(), "unknown condition type") {
		t.Fatalf("expected 'unknown condition type' error, got: %v", err)
	}
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
	for i := len(parts) - 1; i >= 0; i-- {
		current = map[string]any{parts[i]: current}
	}
	varsBytes, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("failed to marshal vars: %v", err)
	}

	payload := fmt.Sprintf(`{"val":"{{%s}}"}`, varPath)
	result := renderTemplateVars(json.RawMessage(payload), varsBytes)

	var out map[string]any
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if out["val"] != "leaf_value" {
		t.Fatalf("expected leaf_value, got %v", out["val"])
	}
}

func TestRenderTemplateVars_HugePayload(t *testing.T) {
	t.Parallel()

	// Build a ~5MB payload string with a template var at the end.
	filler := strings.Repeat("x", 5*1024*1024)
	payload := fmt.Sprintf(`{"data":"%s {{name}}"}`, filler)
	vars := `{"name":"world"}`

	result := renderTemplateVars(json.RawMessage(payload), json.RawMessage(vars))

	var out map[string]any
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatalf("failed to unmarshal huge result: %v", err)
	}
	data, ok := out["data"].(string)
	if !ok {
		t.Fatal("expected data to be string")
	}
	if !strings.HasSuffix(data, " world") {
		t.Fatalf("expected data to end with ' world', got suffix: %q", data[len(data)-20:])
	}
}

func TestRenderTemplateVars_SpecialCharsInVarNames(t *testing.T) {
	t.Parallel()

	// The regex only matches word chars and dots, so {{a.b[0].c}} should not resolve.
	payload := `{"val":"{{a.b[0].c}}"}`
	vars := `{"a":{"b":{"0":{"c":"found"}}}}`

	result := renderTemplateVars(json.RawMessage(payload), json.RawMessage(vars))

	var out map[string]any
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	// The bracket syntax should not be matched by the template regex.
	val, ok := out["val"].(string)
	if !ok {
		t.Fatal("expected val to be string")
	}
	if !strings.Contains(val, "{{") {
		t.Fatalf("expected unresolved template with brackets, got: %s", val)
	}
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
