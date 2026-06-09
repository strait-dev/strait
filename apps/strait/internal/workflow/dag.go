package workflow

import (
	"fmt"
	"sort"

	"strait/internal/domain"
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

	stepIndex := make(map[string]int, len(steps))
	needsDepDedup := false
	orderedDependencies := true

	for i, s := range steps {
		if _, exists := stepIndex[s.StepRef]; exists {
			return fmt.Errorf("duplicate step_ref: %q", s.StepRef)
		}
		needsDepDedup = needsDepDedup || len(s.DependsOn) > 1

		for _, dep := range s.DependsOn {
			if dep == s.StepRef {
				return fmt.Errorf("step %q depends on itself", s.StepRef)
			}
			if _, exists := stepIndex[dep]; !exists {
				orderedDependencies = false
			}
		}

		stepIndex[s.StepRef] = i
	}

	if orderedDependencies {
		return nil
	}

	alreadyOrdered, err := validateDAGDependenciesAreOrdered(steps, stepIndex)
	if err != nil {
		return err
	}
	if alreadyOrdered {
		return nil
	}

	inDegree := make([]int, len(steps))
	childCounts := make([]int, len(steps))
	var depSeen []int
	if needsDepDedup {
		depSeen = make([]int, len(steps))
	}
	totalEdges := 0

	for stepIdx, s := range steps {
		seenMarker := stepIdx + 1
		for _, dep := range s.DependsOn {
			depIdx, exists := stepIndex[dep]
			if !exists {
				return fmt.Errorf("step %q depends on unknown step %q", s.StepRef, dep)
			}
			if depIdx == stepIdx {
				return fmt.Errorf("step %q depends on itself", s.StepRef)
			}
			if depSeen != nil {
				if depSeen[depIdx] == seenMarker {
					continue
				}
				depSeen[depIdx] = seenMarker
			}

			childCounts[depIdx]++
			inDegree[stepIdx]++
			totalEdges++
		}
	}

	if totalEdges == 0 {
		return nil
	}

	children := make([][]int, len(steps))
	edgeStorage := make([]int, totalEdges)
	offset := 0
	for i, count := range childCounts {
		children[i] = edgeStorage[offset : offset : offset+count]
		offset += count
	}
	for stepIdx, s := range steps {
		seenMarker := len(steps) + stepIdx + 1
		for _, dep := range s.DependsOn {
			depIdx := stepIndex[dep]
			if depSeen != nil {
				if depSeen[depIdx] == seenMarker {
					continue
				}
				depSeen[depIdx] = seenMarker
			}
			children[depIdx] = append(children[depIdx], stepIdx)
		}
	}

	queue := childCounts[:0]
	for i, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, i)
		}
	}
	initialRoots := len(queue)

	visitedCount := 0
	for i := 0; i < len(queue); i++ {
		stepIdx := queue[i]
		visitedCount++

		for _, dependent := range children[stepIdx] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if visitedCount != len(steps) {
		unvisited := make([]string, 0, len(steps)-visitedCount)
		for i := range steps {
			if inDegree[i] > 0 {
				unvisited = append(unvisited, steps[i].StepRef)
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

func validateDAGDependenciesAreOrdered(steps []domain.WorkflowStep, stepIndex map[string]int) (bool, error) {
	alreadyOrdered := true
	for stepIdx, s := range steps {
		for _, dep := range s.DependsOn {
			depIdx, exists := stepIndex[dep]
			if !exists {
				return false, fmt.Errorf("step %q depends on unknown step %q", s.StepRef, dep)
			}
			if depIdx == stepIdx {
				return false, fmt.Errorf("step %q depends on itself", s.StepRef)
			}
			if depIdx > stepIdx {
				alreadyOrdered = false
			}
		}
	}
	return alreadyOrdered, nil
}
