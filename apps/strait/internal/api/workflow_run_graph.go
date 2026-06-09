package api

import (
	"slices"
	"time"

	"strait/internal/domain"
)

type workflowStepIndexHeap struct {
	indexes []int
	steps   []domain.WorkflowStep
}

func (h *workflowStepIndexHeap) init() {
	for i := len(h.indexes)/2 - 1; i >= 0; i-- {
		h.siftDown(i)
	}
}

func (h *workflowStepIndexHeap) push(stepIdx int) {
	h.indexes = append(h.indexes, stepIdx)
	h.siftUp(len(h.indexes) - 1)
}

func (h *workflowStepIndexHeap) pop() int {
	old := h.indexes
	n := len(old)
	stepIdx := old[0]
	old[0] = old[n-1]
	old[n-1] = 0
	h.indexes = old[:n-1]
	h.siftDown(0)
	return stepIdx
}

func (h *workflowStepIndexHeap) len() int {
	return len(h.indexes)
}

func (h *workflowStepIndexHeap) less(i, j int) bool {
	return h.steps[h.indexes[i]].StepRef < h.steps[h.indexes[j]].StepRef
}

func (h *workflowStepIndexHeap) siftUp(i int) {
	indexes := h.indexes
	for i > 0 {
		parent := (i - 1) / 2
		if !h.less(i, parent) {
			return
		}
		indexes[parent], indexes[i] = indexes[i], indexes[parent]
		i = parent
	}
}

func (h *workflowStepIndexHeap) siftDown(i int) {
	indexes := h.indexes
	for {
		left := 2*i + 1
		if left >= len(indexes) {
			return
		}
		child := left
		if right := left + 1; right < len(indexes) && h.less(right, left) {
			child = right
		}
		if !h.less(child, i) {
			return
		}
		indexes[i], indexes[child] = indexes[child], indexes[i]
		i = child
	}
}

func estimateWorkflowCriticalPath(steps []domain.WorkflowStep, runByRef map[string]domain.WorkflowStepRun, now time.Time) ([]string, int64, int64) {
	if len(steps) == 0 {
		return nil, 0, 0
	}

	stepIndex := make(map[string]int, len(steps))
	for i, step := range steps {
		stepIndex[step.StepRef] = i
	}

	indegree := make([]int, len(steps))
	childCounts := make([]int, len(steps))
	totalEdges := 0
	for stepIdx, step := range steps {
		for _, dep := range step.DependsOn {
			depIdx, ok := stepIndex[dep]
			if !ok {
				continue
			}
			childCounts[depIdx]++
			indegree[stepIdx]++
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
	for stepIdx, step := range steps {
		for _, dep := range step.DependsOn {
			depIdx, ok := stepIndex[dep]
			if !ok {
				continue
			}
			children[depIdx] = append(children[depIdx], stepIdx)
		}
	}

	queue := workflowStepIndexHeap{
		indexes: make([]int, 0, len(steps)),
		steps:   steps,
	}
	for stepIdx, degree := range indegree {
		if degree == 0 {
			queue.indexes = append(queue.indexes, stepIdx)
		}
	}
	queue.init()

	prev := make([]int, len(steps))
	for i := range prev {
		prev[i] = -1
	}
	longestByStep := make([]int64, len(steps))
	remainingByStep := make([]int64, len(steps))
	for queue.len() > 0 {
		stepIdx := queue.pop()

		step := steps[stepIdx]
		stepRun := runByRef[step.StepRef]
		totalEstimateMS, remainingMS := estimateStepTiming(step, stepRun, now)
		remainingByStep[stepIdx] = remainingMS

		bestParentIdx := -1
		bestParentDistance := int64(0)
		for _, dep := range step.DependsOn {
			depIdx, ok := stepIndex[dep]
			if !ok {
				continue
			}
			distance := longestByStep[depIdx]
			if distance > bestParentDistance {
				bestParentDistance = distance
				bestParentIdx = depIdx
			}
		}
		prev[stepIdx] = bestParentIdx
		longestByStep[stepIdx] = bestParentDistance + totalEstimateMS

		for _, child := range children[stepIdx] {
			indegree[child]--
			if indegree[child] == 0 {
				queue.push(child)
			}
		}
	}

	pathEnd := -1
	pathDistance := int64(0)
	for stepIdx, distance := range longestByStep {
		if distance > pathDistance || (distance == pathDistance && (pathEnd < 0 || steps[stepIdx].StepRef < steps[pathEnd].StepRef)) {
			pathEnd = stepIdx
			pathDistance = distance
		}
	}

	path := make([]string, 0, len(steps))
	for stepIdx := pathEnd; stepIdx >= 0; stepIdx = prev[stepIdx] {
		path = append(path, steps[stepIdx].StepRef)
	}
	slices.Reverse(path)

	remainingMS := int64(0)
	for _, ref := range path {
		remainingMS += remainingByStep[stepIndex[ref]]
	}
	return path, pathDistance, remainingMS
}

func estimateStepTiming(step domain.WorkflowStep, stepRun domain.WorkflowStepRun, now time.Time) (int64, int64) {
	totalEstimateMS := int64(0)
	if step.TimeoutSecsOverride > 0 {
		totalEstimateMS = int64(step.TimeoutSecsOverride) * 1000
	}

	spentMS := int64(0)
	if stepRun.StartedAt != nil {
		spentMS = now.Sub(*stepRun.StartedAt).Milliseconds()
		spentMS = max(spentMS, 0)
	}
	if stepRun.StartedAt != nil && stepRun.FinishedAt != nil {
		actualMS := stepRun.FinishedAt.Sub(*stepRun.StartedAt).Milliseconds()
		actualMS = max(actualMS, 0)
		spentMS = actualMS
		totalEstimateMS = actualMS
	}
	if totalEstimateMS == 0 {
		totalEstimateMS = spentMS
	}
	if stepRun.Status.IsTerminal() {
		return totalEstimateMS, totalEstimateMS
	}
	if spentMS > totalEstimateMS {
		spentMS = totalEstimateMS
	}
	if totalEstimateMS == 0 {
		return 0, 0
	}
	return totalEstimateMS, totalEstimateMS - spentMS
}
