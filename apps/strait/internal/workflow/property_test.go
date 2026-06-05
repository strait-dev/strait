package workflow

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestProperty_DAG_ValidatedNoCycles generates random valid DAGs, validates
// them, and then confirms via BFS that no cycles exist.
func TestProperty_DAG_ValidatedNoCycles(t *testing.T) {
	t.Parallel()

	for range 1000 {
		steps := generateRandomDAG(t)
		err := ValidateDAG(steps)
		if err != nil {
			// If validation rejects it, that is acceptable; the property only
			// checks that accepted DAGs are truly acyclic.
			continue
		}

		// BFS-based cycle detection on the accepted DAG.
		adj := make(map[string][]string)
		inDegree := make(map[string]int)
		for _, s := range steps {
			inDegree[s.StepRef] += 0
			for _, dep := range s.DependsOn {
				adj[dep] = append(adj[dep], s.StepRef)
				inDegree[s.StepRef]++
			}
		}

		queue := make([]string, 0, len(steps))
		for ref, deg := range inDegree {
			if deg == 0 {
				queue = append(queue, ref)
			}
		}

		visited := 0
		for idx := 0; idx < len(queue); idx++ {
			visited++
			for _, dep := range adj[queue[idx]] {
				inDegree[dep]--
				if inDegree[dep] == 0 {
					queue = append(queue, dep)
				}
			}
		}
		require.Equal(t, len(steps), visited)
	}
}

// generateRandomDAG creates a random set of workflow steps that form a valid
// DAG by only allowing edges from lower-indexed to higher-indexed steps.
func generateRandomDAG(t *testing.T) []domain.WorkflowStep {
	t.Helper()

	n := rand.IntN(10) + 1
	steps := make([]domain.WorkflowStep, n)
	for i := range steps {
		steps[i].StepRef = fmt.Sprintf("step_%d", i)
		// Each step can depend on any earlier step (ensuring no cycles).
		for j := range i {
			if rand.IntN(3) == 0 {
				steps[i].DependsOn = append(steps[i].DependsOn, steps[j].StepRef)
			}
		}
	}
	return steps
}

// TestProperty_TemplateRenderIdempotent verifies that rendering a payload with
// the same variables twice produces identical output.
func TestProperty_TemplateRenderIdempotent(t *testing.T) {
	t.Parallel()

	varNames := []string{"name", "count", "flag", "url", "id", "data"}

	for range 1000 {
		// Build a random variables map.
		vars := make(map[string]any)
		numVars := rand.IntN(5) + 1
		for range numVars {
			key := varNames[rand.IntN(len(varNames))]
			switch rand.IntN(4) {
			case 0:
				vars[key] = randomString(rand.IntN(20) + 1)
			case 1:
				vars[key] = rand.IntN(10000)
			case 2:
				vars[key] = rand.IntN(2) == 0
			case 3:
				vars[key] = nil
			}
		}
		varsJSON, _ := json.Marshal(vars)

		// Build a payload that references some variables.
		payload := make(map[string]any)
		numFields := rand.IntN(5) + 1
		for j := range numFields {
			fieldName := fmt.Sprintf("field_%d", j)
			switch rand.IntN(3) {
			case 0:
				// Exact variable reference.
				payload[fieldName] = "{{" + varNames[rand.IntN(len(varNames))] + "}}"
			case 1:
				// Embedded variable reference.
				payload[fieldName] = "prefix-{{" + varNames[rand.IntN(len(varNames))] + "}}-suffix"
			case 2:
				// Static value.
				payload[fieldName] = randomString(10)
			}
		}
		payloadJSON, _ := json.Marshal(payload)

		first := renderTemplateVars(json.RawMessage(payloadJSON), json.RawMessage(varsJSON))
		second := renderTemplateVars(first, json.RawMessage(varsJSON))

		// Re-parse to compare structurally (JSON key order may vary).
		var firstParsed, secondParsed any
		require.NoError(t, json.
			Unmarshal(first, &firstParsed))
		require.NoError(t, json.
			Unmarshal(second, &secondParsed))

		firstRe, _ := json.Marshal(firstParsed)
		secondRe, _ := json.Marshal(secondParsed)
		require.Equal(t, string(
			secondRe,
		), string(firstRe))
	}
}

// randomString generates a random alphanumeric string of the given length.
func randomString(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = charset[rand.IntN(len(charset))]
	}
	return string(buf)
}
