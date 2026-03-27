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

	// Collect completed steps with compensation jobs.
	var compensable []compensableEntry
	for _, step := range steps {
		if step.CompensationJobID == "" {
			continue
		}
		sr, ok := runByRef[step.StepRef]
		if !ok || sr.Status != domain.StepCompleted {
			continue
		}
		compensable = append(compensable, compensableEntry{
			step:    step,
			stepRun: sr,
		})
	}

	if len(compensable) == 0 {
		return nil, nil
	}

	// Sort in reverse topological order.
	topoOrder := buildTopologicalOrder(steps)
	orderIndex := make(map[string]int, len(topoOrder))
	for i, ref := range topoOrder {
		orderIndex[ref] = i
	}

	sort.Slice(compensable, func(i, j int) bool {
		// Higher topological index = later in execution = first to compensate.
		return orderIndex[compensable[i].step.StepRef] > orderIndex[compensable[j].step.StepRef]
	})

	plan := &CompensationPlan{
		WorkflowRunID: workflowRunID,
		Steps:         make([]CompensationStep, len(compensable)),
	}
	for i, entry := range compensable {
		plan.Steps[i] = CompensationStep{
			StepRef:           entry.step.StepRef,
			StepRunID:         entry.stepRun.ID,
			CompensationJobID: entry.step.CompensationJobID,
			TimeoutSecs:       entry.step.CompensationTimeoutSecs,
			OriginalOutput:    entry.stepRun.Output,
		}
	}

	return plan, nil
}

type compensableEntry struct {
	step    domain.WorkflowStep
	stepRun *domain.WorkflowStepRun
}

// buildTopologicalOrder returns step refs in topological order using Kahn's algorithm.
func buildTopologicalOrder(steps []domain.WorkflowStep) []string {
	inDegree := make(map[string]int, len(steps))
	children := make(map[string][]string, len(steps))

	for _, s := range steps {
		if _, ok := inDegree[s.StepRef]; !ok {
			inDegree[s.StepRef] = 0
		}
		for _, dep := range s.DependsOn {
			children[dep] = append(children[dep], s.StepRef)
			inDegree[s.StepRef]++
		}
	}

	queue := make([]string, 0)
	for ref, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, ref)
		}
	}
	// Sort roots for deterministic order.
	sort.Strings(queue)

	var order []string
	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]
		order = append(order, ref)

		kids := children[ref]
		sort.Strings(kids)
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
