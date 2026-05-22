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

	maxDist := calculateExpectedDuration(steps)
	if maxDist == 0 {
		return nil
	}

	expected := startTime.Add(time.Duration(maxDist) * time.Second)
	return &expected
}

func calculateExpectedDuration(steps []domain.WorkflowStep) int {
	stepIndex, needsDepDedup := buildExpectedCompletionStepIndex(steps)
	// Compute longest path using dynamic programming (topological order).
	// For each step, dist[index] = max time from any root to complete that step.
	dist := make([]int, len(steps))

	// Process in topological order (Kahn's algorithm).
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
			depIdx, ok := stepIndex[dep]
			if !ok {
				inDegree[stepIdx]++
				continue
			}
			if depSeen != nil {
				if depSeen[depIdx] == seenMarker {
					continue
				}
				depSeen[depIdx] = seenMarker
			}
			inDegree[stepIdx]++
			childCounts[depIdx]++
			totalEdges++
		}
	}

	if totalEdges == 0 {
		maxDist := 0
		for _, s := range steps {
			if s.ExpectedDurationSecs > maxDist {
				maxDist = s.ExpectedDurationSecs
			}
		}
		return maxDist
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
			depIdx, ok := stepIndex[dep]
			if !ok {
				continue
			}
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
	for i := range steps {
		if inDegree[i] == 0 {
			queue = append(queue, i)
			dist[i] = steps[i].ExpectedDurationSecs
		}
	}

	for i := 0; i < len(queue); i++ {
		stepIdx := queue[i]
		for _, childIdx := range children[stepIdx] {
			childDur := steps[childIdx].ExpectedDurationSecs
			newDist := dist[stepIdx] + childDur
			if newDist > dist[childIdx] {
				dist[childIdx] = newDist
			}
			inDegree[childIdx]--
			if inDegree[childIdx] == 0 {
				queue = append(queue, childIdx)
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

	return maxDist
}

func buildExpectedCompletionStepIndex(steps []domain.WorkflowStep) (map[string]int, bool) {
	stepIndex := make(map[string]int, len(steps))
	needsDepDedup := false
	for i, s := range steps {
		if _, ok := stepIndex[s.StepRef]; !ok {
			stepIndex[s.StepRef] = i
		}
		needsDepDedup = needsDepDedup || len(s.DependsOn) > 1
	}
	return stepIndex, needsDepDedup
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

	remainingStepIndexes := make([]int, 0, len(steps))
	stepIndex := make(map[string]int, len(steps))
	hasAny := false
	for i, s := range steps {
		if completedRefs[s.StepRef] {
			continue
		}
		stepIndex[s.StepRef] = len(remainingStepIndexes)
		remainingStepIndexes = append(remainingStepIndexes, i)
		if s.ExpectedDurationSecs > 0 {
			hasAny = true
		}
	}
	if len(remainingStepIndexes) == 0 || !hasAny {
		return nil
	}

	maxDist := calculateRemainingExpectedDuration(steps, remainingStepIndexes, stepIndex)
	if maxDist == 0 {
		return nil
	}
	expected := now.Add(time.Duration(maxDist) * time.Second)
	return &expected
}

func calculateRemainingExpectedDuration(
	steps []domain.WorkflowStep,
	remainingStepIndexes []int,
	stepIndex map[string]int,
) int {
	inDegree := make([]int, len(remainingStepIndexes))
	childCounts := make([]int, len(remainingStepIndexes))
	needsDepDedup := false
	for _, originalIdx := range remainingStepIndexes {
		needsDepDedup = needsDepDedup || len(steps[originalIdx].DependsOn) > 1
	}
	var depSeen []int
	if needsDepDedup {
		depSeen = make([]int, len(remainingStepIndexes))
	}

	totalEdges := 0
	for compactIdx, originalIdx := range remainingStepIndexes {
		seenMarker := compactIdx + 1
		for _, dep := range steps[originalIdx].DependsOn {
			depIdx, ok := stepIndex[dep]
			if !ok {
				continue
			}
			if depSeen != nil {
				if depSeen[depIdx] == seenMarker {
					continue
				}
				depSeen[depIdx] = seenMarker
			}
			inDegree[compactIdx]++
			childCounts[depIdx]++
			totalEdges++
		}
	}

	dist := make([]int, len(remainingStepIndexes))
	if totalEdges == 0 {
		maxDist := 0
		for compactIdx, originalIdx := range remainingStepIndexes {
			dist[compactIdx] = steps[originalIdx].ExpectedDurationSecs
			if dist[compactIdx] > maxDist {
				maxDist = dist[compactIdx]
			}
		}
		return maxDist
	}

	children := make([][]int, len(remainingStepIndexes))
	edgeStorage := make([]int, totalEdges)
	offset := 0
	for i, count := range childCounts {
		children[i] = edgeStorage[offset : offset : offset+count]
		offset += count
	}
	for compactIdx, originalIdx := range remainingStepIndexes {
		seenMarker := len(remainingStepIndexes) + compactIdx + 1
		for _, dep := range steps[originalIdx].DependsOn {
			depIdx, ok := stepIndex[dep]
			if !ok {
				continue
			}
			if depSeen != nil {
				if depSeen[depIdx] == seenMarker {
					continue
				}
				depSeen[depIdx] = seenMarker
			}
			children[depIdx] = append(children[depIdx], compactIdx)
		}
	}

	queue := childCounts[:0]
	for compactIdx, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, compactIdx)
			dist[compactIdx] = steps[remainingStepIndexes[compactIdx]].ExpectedDurationSecs
		}
	}

	for i := 0; i < len(queue); i++ {
		stepIdx := queue[i]
		for _, childIdx := range children[stepIdx] {
			childDur := steps[remainingStepIndexes[childIdx]].ExpectedDurationSecs
			newDist := dist[stepIdx] + childDur
			if newDist > dist[childIdx] {
				dist[childIdx] = newDist
			}
			inDegree[childIdx]--
			if inDegree[childIdx] == 0 {
				queue = append(queue, childIdx)
			}
		}
	}

	maxDist := 0
	for _, d := range dist {
		if d > maxDist {
			maxDist = d
		}
	}
	return maxDist
}
