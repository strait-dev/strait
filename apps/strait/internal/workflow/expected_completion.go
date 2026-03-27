package workflow

import (
	"time"

	"strait/internal/domain"
)

// CalculateExpectedCompletion computes the expected completion time for a workflow run
// based on the critical path (longest path) through the DAG using step expected durations.
// Returns nil if no steps have expected durations configured.
func CalculateExpectedCompletion(steps []domain.WorkflowStep, startTime time.Time) *time.Time {
	if len(steps) == 0 {
		return nil
	}

	// Check if any step has expected duration.
	hasAny := false
	for _, s := range steps {
		if s.ExpectedDurationSecs > 0 {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return nil
	}

	// Build step map and adjacency list.
	stepMap := make(map[string]int, len(steps))
	for i, s := range steps {
		stepMap[s.StepRef] = i
	}

	// Compute longest path using dynamic programming (topological order).
	// For each step, dist[ref] = max time from any root to complete that step.
	dist := make(map[string]int, len(steps))

	// Process in topological order (Kahn's algorithm).
	inDegree := make(map[string]int, len(steps))
	children := make(map[string][]string, len(steps))
	for _, s := range steps {
		inDegree[s.StepRef] = len(s.DependsOn)
		for _, dep := range s.DependsOn {
			children[dep] = append(children[dep], s.StepRef)
		}
	}

	queue := make([]string, 0, len(steps))
	for _, s := range steps {
		if inDegree[s.StepRef] == 0 {
			queue = append(queue, s.StepRef)
			dist[s.StepRef] = steps[stepMap[s.StepRef]].ExpectedDurationSecs
		}
	}

	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]

		for _, child := range children[ref] {
			childIdx := stepMap[child]
			childDur := steps[childIdx].ExpectedDurationSecs
			newDist := dist[ref] + childDur
			if newDist > dist[child] {
				dist[child] = newDist
			}
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	// Find the maximum distance (critical path length).
	maxDist := 0
	for _, d := range dist {
		if d > maxDist {
			maxDist = d
		}
	}

	if maxDist == 0 {
		return nil
	}

	expected := startTime.Add(time.Duration(maxDist) * time.Second)
	return &expected
}

// RecalculateExpectedCompletion updates the expected completion based on actual progress.
// It uses the remaining steps' expected durations plus the current time.
func RecalculateExpectedCompletion(
	steps []domain.WorkflowStep,
	completedRefs map[string]bool,
	now time.Time,
) *time.Time {
	if len(steps) == 0 {
		return nil
	}

	// Filter to remaining steps only.
	remaining := make([]domain.WorkflowStep, 0, len(steps))
	for _, s := range steps {
		if !completedRefs[s.StepRef] {
			// Remap dependencies to only include non-completed deps.
			filtered := domain.WorkflowStep{
				StepRef:              s.StepRef,
				DependsOn:            make([]string, 0, len(s.DependsOn)),
				ExpectedDurationSecs: s.ExpectedDurationSecs,
			}
			for _, dep := range s.DependsOn {
				if !completedRefs[dep] {
					filtered.DependsOn = append(filtered.DependsOn, dep)
				}
			}
			remaining = append(remaining, filtered)
		}
	}

	return CalculateExpectedCompletion(remaining, now)
}
