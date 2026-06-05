package api

import (
	"slices"
	"time"

	"github.com/samber/lo"

	"strait/internal/domain"
)

type workflowStepRefHeap []string

func (h *workflowStepRefHeap) init() {
	for i := len(*h)/2 - 1; i >= 0; i-- {
		h.siftDown(i)
	}
}

func (h *workflowStepRefHeap) push(ref string) {
	*h = append(*h, ref)
	h.siftUp(len(*h) - 1)
}

func (h *workflowStepRefHeap) pop() string {
	old := *h
	n := len(old)
	ref := old[0]
	old[0] = old[n-1]
	old[n-1] = ""
	*h = old[:n-1]
	h.siftDown(0)
	return ref
}

func (h *workflowStepRefHeap) siftUp(i int) {
	refs := *h
	for i > 0 {
		parent := (i - 1) / 2
		if refs[parent] <= refs[i] {
			return
		}
		refs[parent], refs[i] = refs[i], refs[parent]
		i = parent
	}
}

func (h *workflowStepRefHeap) siftDown(i int) {
	refs := *h
	for {
		left := 2*i + 1
		if left >= len(refs) {
			return
		}
		child := left
		if right := left + 1; right < len(refs) && refs[right] < refs[left] {
			child = right
		}
		if refs[i] <= refs[child] {
			return
		}
		refs[i], refs[child] = refs[child], refs[i]
		i = child
	}
}

func estimateWorkflowCriticalPath(steps []domain.WorkflowStep, runByRef map[string]domain.WorkflowStepRun, now time.Time) ([]string, int64, int64) {
	if len(steps) == 0 {
		return nil, 0, 0
	}

	stepByRef := lo.KeyBy(steps, func(step domain.WorkflowStep) string { return step.StepRef })
	indegree := make(map[string]int, len(steps))
	children := make(map[string][]string, len(steps))
	for _, step := range steps {
		indegree[step.StepRef] = 0
		children[step.StepRef] = []string{}
	}
	for _, step := range steps {
		for _, dep := range step.DependsOn {
			if _, ok := indegree[dep]; !ok {
				continue
			}
			children[dep] = append(children[dep], step.StepRef)
			indegree[step.StepRef]++
		}
	}

	queue := make(workflowStepRefHeap, 0, len(steps))
	for ref, degree := range indegree {
		if degree == 0 {
			queue = append(queue, ref)
		}
	}
	queue.init()

	prev := make(map[string]string, len(steps))
	longestByRef := make(map[string]int64, len(steps))
	totalEstimateByRef := make(map[string]int64, len(steps))
	remainingByRef := make(map[string]int64, len(steps))
	for len(queue) > 0 {
		ref := queue.pop()

		step := stepByRef[ref]
		stepRun := runByRef[ref]
		totalEstimateMS, remainingMS := estimateStepTiming(step, stepRun, now)
		totalEstimateByRef[ref] = totalEstimateMS
		remainingByRef[ref] = remainingMS

		bestParentRef := ""
		bestParentDistance := int64(0)
		for _, dep := range step.DependsOn {
			distance, ok := longestByRef[dep]
			if !ok {
				continue
			}
			if distance > bestParentDistance {
				bestParentDistance = distance
				bestParentRef = dep
			}
		}
		prev[ref] = bestParentRef
		longestByRef[ref] = bestParentDistance + totalEstimateMS

		for _, child := range children[ref] {
			indegree[child]--
			if indegree[child] == 0 {
				queue.push(child)
			}
		}
	}

	pathEnd := ""
	pathDistance := int64(0)
	for ref, distance := range longestByRef {
		if distance > pathDistance || (distance == pathDistance && (pathEnd == "" || ref < pathEnd)) {
			pathEnd = ref
			pathDistance = distance
		}
	}

	path := make([]string, 0, len(steps))
	for ref := pathEnd; ref != ""; ref = prev[ref] {
		path = append(path, ref)
	}
	slices.Reverse(path)

	remainingMS := int64(0)
	for _, ref := range path {
		remainingMS += remainingByRef[ref]
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
