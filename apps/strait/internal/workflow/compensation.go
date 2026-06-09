package workflow

import (
	"encoding/json"
	"fmt"
	"slices"
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

	// Collect completed steps with compensation jobs by definition index so the
	// final plan can be produced by one reverse topological pass.
	runLookup := stepRunLookup{runs: stepRuns}
	if stepsFormOrderedLinearChainByRef(steps) {
		return buildLinearCompensationPlan(workflowRunID, steps, &runLookup)
	}

	compensableRuns := make([]*domain.WorkflowStepRun, len(steps))
	compensableCount := 0
	for i := range steps {
		step := &steps[i]
		if step.CompensationJobID == "" {
			continue
		}
		sr, ok := runLookup.get(i, step.StepRef)
		if !ok || sr.Status != domain.StepCompleted {
			continue
		}
		compensableRuns[i] = sr
		compensableCount++
	}

	if compensableCount == 0 {
		return nil, nil
	}

	topoOrder := []int(nil)
	if !stepsFormOrderedLinearChainByRef(steps) {
		stepIndex := buildStepIndex(steps)
		if stepsAreTopologicallyOrdered(steps, stepIndex) {
			stepIndex = nil
		}
		if stepIndex != nil {
			topoOrder = buildTopologicalOrderIndexesWithStepIndex(steps, stepIndex)
		}
	}

	plan := &CompensationPlan{
		WorkflowRunID: workflowRunID,
		Steps:         make([]CompensationStep, 0, compensableCount),
	}
	if topoOrder == nil {
		for stepIdx := range slices.Backward(steps) {
			appendCompensationStep(plan, steps, compensableRuns, stepIdx)
		}
	} else {
		for _, stepIdx := range slices.Backward(topoOrder) {
			appendCompensationStep(plan, steps, compensableRuns, stepIdx)
		}
	}

	return plan, nil
}

func buildLinearCompensationPlan(
	workflowRunID string,
	steps []domain.WorkflowStep,
	runLookup *stepRunLookup,
) (*CompensationPlan, error) {
	compensableCount := 0
	for i := range steps {
		step := &steps[i]
		if step.CompensationJobID == "" {
			continue
		}
		sr, ok := runLookup.get(i, step.StepRef)
		if ok && sr.Status == domain.StepCompleted {
			compensableCount++
		}
	}
	if compensableCount == 0 {
		return nil, nil
	}

	plan := &CompensationPlan{
		WorkflowRunID: workflowRunID,
		Steps:         make([]CompensationStep, 0, compensableCount),
	}
	for stepIdx := range slices.Backward(steps) {
		step := &steps[stepIdx]
		if step.CompensationJobID == "" {
			continue
		}
		stepRun, ok := runLookup.get(stepIdx, step.StepRef)
		if !ok || stepRun.Status != domain.StepCompleted {
			continue
		}
		appendCompensationStepFromRun(plan, step, stepRun)
	}
	return plan, nil
}

type stepRunLookup struct {
	runs  []domain.WorkflowStepRun
	byRef map[string]*domain.WorkflowStepRun
}

func (l *stepRunLookup) get(definitionIndex int, stepRef string) (*domain.WorkflowStepRun, bool) {
	if l.byRef == nil && definitionIndex < len(l.runs) && l.runs[definitionIndex].StepRef == stepRef {
		return &l.runs[definitionIndex], true
	}
	if l.byRef == nil {
		l.byRef = make(map[string]*domain.WorkflowStepRun, len(l.runs))
		for i := range l.runs {
			l.byRef[l.runs[i].StepRef] = &l.runs[i]
		}
	}
	run, ok := l.byRef[stepRef]
	return run, ok
}

func appendCompensationStep(
	plan *CompensationPlan,
	steps []domain.WorkflowStep,
	compensableRuns []*domain.WorkflowStepRun,
	stepIdx int,
) {
	stepRun := compensableRuns[stepIdx]
	if stepRun == nil {
		return
	}
	step := &steps[stepIdx]
	appendCompensationStepFromRun(plan, step, stepRun)
}

func appendCompensationStepFromRun(plan *CompensationPlan, step *domain.WorkflowStep, stepRun *domain.WorkflowStepRun) {
	plan.Steps = append(plan.Steps, CompensationStep{
		StepRef:           step.StepRef,
		StepRunID:         stepRun.ID,
		CompensationJobID: step.CompensationJobID,
		TimeoutSecs:       step.CompensationTimeoutSecs,
		OriginalOutput:    stepRun.Output,
	})
}

func stepsAreTopologicallyOrdered(steps []domain.WorkflowStep, stepIndex map[string]int) bool {
	for stepIdx := range steps {
		for _, dep := range steps[stepIdx].DependsOn {
			depIdx, ok := stepIndex[dep]
			if !ok || depIdx >= stepIdx {
				return false
			}
		}
	}
	return true
}

func stepsFormOrderedLinearChainByRef(steps []domain.WorkflowStep) bool {
	if len(steps) <= 1 {
		return true
	}
	if len(steps[0].DependsOn) != 0 {
		return false
	}
	for stepIdx := 1; stepIdx < len(steps); stepIdx++ {
		deps := steps[stepIdx].DependsOn
		if len(deps) != 1 || deps[0] != steps[stepIdx-1].StepRef {
			return false
		}
	}
	return true
}

// buildTopologicalOrder returns step refs in topological order using Kahn's algorithm.
func buildTopologicalOrder(steps []domain.WorkflowStep) []string {
	if stepsFormOrderedLinearChainByRef(steps) {
		order := make([]string, len(steps))
		for i := range steps {
			order[i] = steps[i].StepRef
		}
		return order
	}
	indexes := buildTopologicalOrderIndexes(steps)
	order := make([]string, len(indexes))
	for i, stepIdx := range indexes {
		order[i] = steps[stepIdx].StepRef
	}
	return order
}

func buildTopologicalOrderIndexes(steps []domain.WorkflowStep) []int {
	if stepsFormOrderedLinearChainByRef(steps) {
		return orderedStepIndexes(len(steps))
	}
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
	if stepsFormOrderedLinearChain(steps, stepIndex) {
		return orderedStepIndexes(len(steps))
	}

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

func stepsFormOrderedLinearChain(steps []domain.WorkflowStep, stepIndex map[string]int) bool {
	if len(steps) <= 1 {
		return true
	}
	if len(steps[0].DependsOn) != 0 {
		return false
	}
	for stepIdx := 1; stepIdx < len(steps); stepIdx++ {
		deps := steps[stepIdx].DependsOn
		if len(deps) != 1 {
			return false
		}
		if depIdx, ok := stepIndex[deps[0]]; !ok || depIdx != stepIdx-1 {
			return false
		}
	}
	return true
}

func orderedStepIndexes(size int) []int {
	indexes := make([]int, size)
	for i := range indexes {
		indexes[i] = i
	}
	return indexes
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
