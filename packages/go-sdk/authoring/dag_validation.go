package authoring

import (
	"fmt"
	"strings"

	strait "github.com/strait-dev/go-sdk"
)

// ValidateDag validates a DAG of workflow steps using Kahn's algorithm.
// It returns the topologically sorted step refs or an error if the DAG is invalid.
func ValidateDag(steps []Step) ([]string, error) {
	if len(steps) == 0 {
		return nil, nil
	}

	refs := make([]string, len(steps))
	for i, s := range steps {
		refs[i] = s.StepRef()
	}

	if err := checkDuplicateRefs(refs); err != nil {
		return nil, err
	}

	refSet := make(map[string]bool, len(refs))
	for _, ref := range refs {
		refSet[ref] = true
	}

	if err := checkMissingRefs(steps, refSet); err != nil {
		return nil, err
	}

	return topologicalSort(steps, refs)
}

func checkDuplicateRefs(refs []string) error {
	seen := make(map[string]bool, len(refs))
	var duplicates []string

	for _, ref := range refs {
		if seen[ref] {
			duplicates = append(duplicates, ref)
		}
		seen[ref] = true
	}

	if len(duplicates) > 0 {
		return &strait.DagValidationError{
			Message:       fmt.Sprintf("Duplicate step refs: %s", strings.Join(duplicates, ", ")),
			DuplicateRefs: duplicates,
		}
	}
	return nil
}

func checkMissingRefs(steps []Step, allRefs map[string]bool) error {
	var missing []string

	for _, s := range steps {
		for _, dep := range s.BaseOptions().DependsOn {
			if !allRefs[dep] {
				missing = append(missing, dep)
			}
		}
	}

	if len(missing) > 0 {
		return &strait.DagValidationError{
			Message:     fmt.Sprintf("References to non-existent steps: %s", strings.Join(missing, ", ")),
			MissingRefs: missing,
		}
	}
	return nil
}

func topologicalSort(steps []Step, refs []string) ([]string, error) {
	inDegree := make(map[string]int, len(refs))
	adjacency := make(map[string][]string, len(refs))

	for _, ref := range refs {
		inDegree[ref] = 0
		adjacency[ref] = nil
	}

	for _, s := range steps {
		for _, dep := range s.BaseOptions().DependsOn {
			adjacency[dep] = append(adjacency[dep], s.StepRef())
			inDegree[s.StepRef()]++
		}
	}

	var queue []string
	for _, ref := range refs {
		if inDegree[ref] == 0 {
			queue = append(queue, ref)
		}
	}

	var sorted []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		sorted = append(sorted, node)

		for _, neighbor := range adjacency[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(sorted) != len(steps) {
		var inCycle []string
		sortedSet := make(map[string]bool, len(sorted))
		for _, ref := range sorted {
			sortedSet[ref] = true
		}
		for _, ref := range refs {
			if !sortedSet[ref] {
				inCycle = append(inCycle, ref)
			}
		}
		return nil, &strait.DagValidationError{
			Message: fmt.Sprintf("Circular dependency detected involving steps: %s", strings.Join(inCycle, ", ")),
			Cycles:  inCycle,
		}
	}

	return sorted, nil
}
