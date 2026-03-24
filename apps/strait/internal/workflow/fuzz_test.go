package workflow

import (
	"encoding/json"
	"fmt"
	"testing"

	"strait/internal/domain"
)

// FuzzValidateDAG generates random workflow steps and feeds them to ValidateDAG.
// It verifies the function never panics regardless of input.
func FuzzValidateDAG(f *testing.F) {
	// Seed: valid 3-step linear DAG.
	f.Add(
		uint8(3),
		"step-a,step-b,step-c",
		",step-a,step-b",
		"job,job,job",
	)
	// Seed: invalid cyclic DAG (A->B->C->A).
	f.Add(
		uint8(3),
		"step-a,step-b,step-c",
		"step-c,step-a,step-b",
		"job,job,job",
	)
	// Seed: single step.
	f.Add(
		uint8(1),
		"only",
		"",
		"job",
	)
	// Seed: self-dependency.
	f.Add(
		uint8(1),
		"loop",
		"loop",
		"job",
	)

	f.Fuzz(func(t *testing.T, numSteps uint8, refsCSV, depsCSV, typesCSV string) {
		if numSteps == 0 || numSteps > 20 {
			return
		}

		refs := splitCSV(refsCSV, int(numSteps))
		deps := splitCSV(depsCSV, int(numSteps))
		types := splitCSV(typesCSV, int(numSteps))

		steps := make([]domain.WorkflowStep, int(numSteps))
		for i := range steps {
			var depList []string
			if i < len(deps) && deps[i] != "" {
				depList = splitCSV(deps[i], -1)
			}
			ref := fmt.Sprintf("s%d", i)
			if i < len(refs) && refs[i] != "" {
				ref = refs[i]
			}
			var stepType domain.WorkflowStepType
			if i < len(types) {
				stepType = domain.WorkflowStepType(types[i])
			}
			steps[i] = domain.WorkflowStep{
				StepRef:   ref,
				DependsOn: depList,
				StepType:  stepType,
			}
		}

		// Must not panic. Errors are expected for random inputs.
		_ = ValidateDAG(steps)
	})
}

// FuzzEvaluateCondition fuzzes condition evaluation with random JSON conditions
// and step status maps. It verifies the function never panics.
func FuzzEvaluateCondition(f *testing.F) {
	// Seed: step_status condition.
	f.Add(
		[]byte(`{"type":"step_status","step_ref":"step-1","status":"completed"}`),
		"step-1",
		"completed",
	)
	// Seed: not condition wrapping step_status.
	f.Add(
		[]byte(`{"type":"not","condition":{"type":"step_status","step_ref":"step-1","status":"failed"}}`),
		"step-1",
		"completed",
	)
	// Seed: all_of composite.
	f.Add(
		[]byte(`{"type":"all_of","conditions":[{"type":"step_status","step_ref":"a","status":"completed"}]}`),
		"a",
		"completed",
	)
	// Seed: empty condition.
	f.Add(
		[]byte(``),
		"step-1",
		"pending",
	)
	// Seed: eq condition.
	f.Add(
		[]byte(`{"type":"eq","left":"hello","right":"hello"}`),
		"x",
		"running",
	)
	// Seed: random garbage.
	f.Add(
		[]byte(`{{{not json`),
		"step-1",
		"failed",
	)

	f.Fuzz(func(t *testing.T, condJSON []byte, stepRef, status string) {
		statuses := map[string]domain.StepRunStatus{
			stepRef: domain.StepRunStatus(status),
		}

		// Must not panic. Errors are expected for random inputs.
		_, _ = EvaluateCondition(json.RawMessage(condJSON), statuses)
	})
}

// FuzzRenderTemplateVars fuzzes template variable rendering with random payload
// and variable JSON. It verifies the function never panics.
func FuzzRenderTemplateVars(f *testing.F) {
	// Seed: simple variable substitution.
	f.Add(
		[]byte(`{"greeting":"hello {{name}}"}`),
		[]byte(`{"name":"test"}`),
	)
	// Seed: exact match (type preservation).
	f.Add(
		[]byte(`{"count":"{{num}}"}`),
		[]byte(`{"num":42}`),
	)
	// Seed: nested variable path.
	f.Add(
		[]byte(`{"msg":"{{user.email}}"}`),
		[]byte(`{"user":{"email":"a@b.com"}}`),
	)
	// Seed: no variables.
	f.Add(
		[]byte(`{"plain":"text"}`),
		[]byte(`{}`),
	)
	// Seed: empty payload.
	f.Add(
		[]byte(``),
		[]byte(`{"x":1}`),
	)
	// Seed: malformed JSON.
	f.Add(
		[]byte(`not json`),
		[]byte(`also not json`),
	)

	f.Fuzz(func(t *testing.T, payload, variables []byte) {
		// Must not panic. Errors are expected for random inputs.
		_ = renderTemplateVars(json.RawMessage(payload), json.RawMessage(variables))
	})
}

// FuzzResolveVar fuzzes the resolveVar function with random dot-separated paths
// against nested maps. It verifies the function never panics.
func FuzzResolveVar(f *testing.F) {
	// Seed: simple key.
	f.Add("name", `{"name":"alice"}`)
	// Seed: nested path.
	f.Add("user.email", `{"user":{"email":"a@b.com"}}`)
	// Seed: deeply nested.
	f.Add("a.b.c.d", `{"a":{"b":{"c":{"d":"deep"}}}}`)
	// Seed: missing key.
	f.Add("missing", `{"other":"val"}`)
	// Seed: empty path.
	f.Add("", `{"x":1}`)
	// Seed: path hitting non-map.
	f.Add("x.y", `{"x":42}`)

	f.Fuzz(func(t *testing.T, path, varsJSON string) {
		var vars map[string]any
		if err := json.Unmarshal([]byte(varsJSON), &vars); err != nil {
			// If we can't parse vars, build a simple nested map from the path.
			vars = map[string]any{"key": "value"}
		}

		// Must not panic. Errors are expected for random inputs.
		_, _ = resolveVar(vars, path)
	})
}

// splitCSV splits a string by commas, returning up to max elements.
// If max is negative, no limit is applied.
func splitCSV(s string, max int) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
			if max > 0 && len(parts) >= max {
				break
			}
		}
	}
	return parts
}
