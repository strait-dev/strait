package workflow

import (
	"encoding/json"
	"fmt"
	"sort"

	"strait/internal/domain"
)

// CompensationPlan describes the compensation actions needed for a failed workflow run.
type CompensationPlan struct {
	WorkflowRunID string
	// Steps to compensate in order (reverse topological by default).
	Steps []CompensationStep
}

// CompensationStep describes a single step that needs compensation.
type CompensationStep struct {
	StepRef           string
	StepRunID         string
	CompensationJobID string
	TimeoutSecs       int
	OriginalOutput    json.RawMessage
}

// BuildCompensationPlan creates a plan for compensating completed steps in reverse
// topological order. Only steps that completed successfully AND have a compensation
// job configured are included.
func BuildCompensationPlan(
	workflowRunID string,
	steps []domain.WorkflowStep,
	stepRuns []domain.WorkflowStepRun,
) (*CompensationPlan, error) {
	if len(steps) == 0 || len(stepRuns) == 0 {
		return nil, nil
	}

	// Index step runs by ref.
	runByRef := make(map[string]*domain.WorkflowStepRun, len(stepRuns))
	for i := range stepRuns {
		runByRef[stepRuns[i].StepRef] = &stepRuns[i]
	}

	// Collect completed steps with compensation jobs by definition index so the
	// final plan can be produced by one reverse topological pass.
	compensableRuns := make([]*domain.WorkflowStepRun, len(steps))
	compensableCount := 0
	for i := range steps {
		step := &steps[i]
		if step.CompensationJobID == "" {
			continue
		}
		sr, ok := runByRef[step.StepRef]
		if !ok || sr.Status != domain.StepCompleted {
			continue
		}
		compensableRuns[i] = sr
		compensableCount++
	}

	if compensableCount == 0 {
		return nil, nil
	}

	topoOrder := buildTopologicalOrderIndexes(steps)

	plan := &CompensationPlan{
		WorkflowRunID: workflowRunID,
		Steps:         make([]CompensationStep, 0, compensableCount),
	}
	for i := len(topoOrder) - 1; i >= 0; i-- {
		stepIdx := topoOrder[i]
		stepRun := compensableRuns[stepIdx]
		if stepRun == nil {
			continue
		}
		step := &steps[stepIdx]
		plan.Steps = append(plan.Steps, CompensationStep{
			StepRef:           step.StepRef,
			StepRunID:         stepRun.ID,
			CompensationJobID: step.CompensationJobID,
			TimeoutSecs:       step.CompensationTimeoutSecs,
			OriginalOutput:    stepRun.Output,
		})
	}

	return plan, nil
}

// buildTopologicalOrder returns step refs in topological order using Kahn's algorithm.
func buildTopologicalOrder(steps []domain.WorkflowStep) []string {
	indexes := buildTopologicalOrderIndexes(steps)
	order := make([]string, len(indexes))
	for i, stepIdx := range indexes {
		order[i] = steps[stepIdx].StepRef
	}
	return order
}

func buildTopologicalOrderIndexes(steps []domain.WorkflowStep) []int {
	stepIndex := make(map[string]int, len(steps))
	for i, s := range steps {
		if _, ok := stepIndex[s.StepRef]; !ok {
			stepIndex[s.StepRef] = i
		}
	}
	return buildTopologicalOrderIndexesWithStepIndex(steps, stepIndex)
}

func buildStepIndex(steps []domain.WorkflowStep) map[string]int {
	stepIndex := make(map[string]int, len(steps))
	for i, s := range steps {
		if _, ok := stepIndex[s.StepRef]; !ok {
			stepIndex[s.StepRef] = i
		}
	}
	return stepIndex
}

func buildTopologicalOrderIndexesWithStepIndex(steps []domain.WorkflowStep, stepIndex map[string]int) []int {
	needsDepDedup := false
	for _, s := range steps {
		needsDepDedup = needsDepDedup || len(s.DependsOn) > 1
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
	for idx, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, idx)
		}
	}
	// Sort roots for deterministic order.
	if len(queue) > 1 {
		sort.Slice(queue, func(i, j int) bool {
			return steps[queue[i]].StepRef < steps[queue[j]].StepRef
		})
	}

	order := make([]int, 0, len(steps))
	for i := 0; i < len(queue); i++ {
		stepIdx := queue[i]
		order = append(order, stepIdx)

		kids := children[stepIdx]
		if len(kids) > 1 {
			sort.Slice(kids, func(i, j int) bool {
				return steps[kids[i]].StepRef < steps[kids[j]].StepRef
			})
		}
		for _, child := range kids {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	return order
}

// ValidateCompensationRequest checks that a workflow run is eligible for compensation.
func ValidateCompensationRequest(run *domain.WorkflowRun) error {
	if run == nil {
		return fmt.Errorf("workflow run is nil")
	}

	switch run.Status {
	case domain.WfStatusFailed, domain.WfStatusTimedOut:
		return nil // eligible for compensation
	case domain.WfStatusCompensating:
		return fmt.Errorf("workflow run %s is already compensating", run.ID)
	case domain.WfStatusCompensated:
		return fmt.Errorf("workflow run %s is already compensated", run.ID)
	case domain.WfStatusCompensationFailed:
		return fmt.Errorf("workflow run %s compensation already failed", run.ID)
	default:
		return fmt.Errorf("workflow run %s has status %s, only failed or timed_out runs can be compensated", run.ID, run.Status)
	}
}
