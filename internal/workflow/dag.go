package workflow

import (
	"fmt"
	"sort"

	"orchestrator/internal/domain"
)

// ValidateDAG validates that workflow steps form a valid directed acyclic graph.
// It checks for:
// - Cycle detection using Kahn's algorithm (topological sort)
// - All depends_on references exist in the step set
// - No self-dependencies
// - No duplicate step_refs
// - At least one root step (step with no dependencies)
// - At least one step total.
func ValidateDAG(steps []domain.WorkflowStep) error {
	if len(steps) == 0 {
		return fmt.Errorf("workflow must have at least one step")
	}

	adjacency := make(map[string]map[string]struct{}, len(steps))
	inDegree := make(map[string]int, len(steps))

	for _, s := range steps {
		if _, exists := inDegree[s.StepRef]; exists {
			return fmt.Errorf("duplicate step_ref: %q", s.StepRef)
		}
		inDegree[s.StepRef] = 0
		adjacency[s.StepRef] = make(map[string]struct{})
	}

	for _, s := range steps {
		for _, dep := range s.DependsOn {
			if _, exists := inDegree[dep]; !exists {
				return fmt.Errorf("step %q depends on unknown step %q", s.StepRef, dep)
			}
			if dep == s.StepRef {
				return fmt.Errorf("step %q depends on itself", s.StepRef)
			}

			if _, exists := adjacency[dep][s.StepRef]; !exists {
				adjacency[dep][s.StepRef] = struct{}{}
				inDegree[s.StepRef]++
			}
		}
	}

	queue := make([]string, 0, len(steps))
	for ref, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, ref)
		}
	}
	initialRoots := len(queue)

	visitedCount := 0
	visited := make(map[string]struct{}, len(steps))
	for i := 0; i < len(queue); i++ {
		ref := queue[i]
		visitedCount++
		visited[ref] = struct{}{}

		for dependent := range adjacency[ref] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if visitedCount != len(steps) {
		unvisited := make([]string, 0, len(steps)-visitedCount)
		for ref := range inDegree {
			if _, ok := visited[ref]; !ok {
				unvisited = append(unvisited, ref)
			}
		}
		sort.Strings(unvisited)
		return fmt.Errorf("cycle detected involving steps: %v", unvisited)
	}

	if initialRoots == 0 {
		return fmt.Errorf("workflow must have at least one root step")
	}

	return nil
}
