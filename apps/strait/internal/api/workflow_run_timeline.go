package api

import (
	"sort"
	"time"

	"strait/internal/domain"
)

type workflowTimelineWindow struct {
	start time.Time
	end   time.Time
	ref   string
}

func buildWorkflowRunTimeline(run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, now time.Time) domain.TimelineResponse {
	// Sort by started_at ASC; steps without started_at go to the end.
	sort.Slice(stepRuns, func(i, j int) bool {
		if stepRuns[i].StartedAt == nil && stepRuns[j].StartedAt == nil {
			return stepRuns[i].StepRef < stepRuns[j].StepRef
		}
		if stepRuns[i].StartedAt == nil {
			return false
		}
		if stepRuns[j].StartedAt == nil {
			return true
		}
		return stepRuns[i].StartedAt.Before(*stepRuns[j].StartedAt)
	})

	windows := make([]workflowTimelineWindow, 0, len(stepRuns))
	for _, sr := range stepRuns {
		if sr.StartedAt == nil {
			continue
		}
		end := now
		if sr.FinishedAt != nil {
			end = *sr.FinishedAt
		}
		windows = append(windows, workflowTimelineWindow{start: *sr.StartedAt, end: end, ref: sr.StepRef})
	}
	parallelMap, criticalRefs := buildWorkflowTimelineRelationships(windows)
	waitTracker := newWorkflowTimelineWaitTracker(stepRuns)

	timelineSteps := make([]domain.TimelineStep, 0, len(stepRuns))
	for _, sr := range stepRuns {
		var durationMs int64
		if sr.StartedAt != nil {
			if sr.FinishedAt != nil {
				durationMs = sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
			} else {
				durationMs = now.Sub(*sr.StartedAt).Milliseconds()
			}
		}

		ts := domain.TimelineStep{
			StepRunID:      sr.ID,
			StepRef:        sr.StepRef,
			Status:         string(sr.Status),
			StartedAt:      sr.StartedAt,
			FinishedAt:     sr.FinishedAt,
			DurationMs:     durationMs,
			ParallelWith:   parallelMap[sr.StepRef],
			OnCriticalPath: criticalRefs[sr.StepRef],
			WaitMs:         waitTracker.waitBefore(sr.StartedAt),
		}
		timelineSteps = append(timelineSteps, ts)
	}

	var totalMs int64
	if run.StartedAt != nil {
		if run.FinishedAt != nil {
			totalMs = run.FinishedAt.Sub(*run.StartedAt).Milliseconds()
		} else {
			totalMs = now.Sub(*run.StartedAt).Milliseconds()
		}
	}

	return domain.TimelineResponse{
		WorkflowRunID: run.ID,
		Status:        string(run.Status),
		StartedAt:     run.StartedAt,
		FinishedAt:    run.FinishedAt,
		TotalMs:       totalMs,
		Steps:         timelineSteps,
	}
}

func buildWorkflowTimelineRelationships(windows []workflowTimelineWindow) (map[string][]string, map[string]bool) {
	parallelMap := make(map[string][]string, len(windows))
	criticalRefs := make(map[string]bool, len(windows))
	if len(windows) == 0 {
		return parallelMap, criticalRefs
	}

	for _, w := range windows {
		criticalRefs[w.ref] = true
	}

	counts, totalParallelRefs := countWorkflowTimelineRelationships(windows, criticalRefs)
	if totalParallelRefs == 0 {
		return parallelMap, criticalRefs
	}

	storage := make([]string, totalParallelRefs)
	offset := 0
	for i, count := range counts {
		if count == 0 {
			continue
		}
		parallelMap[windows[i].ref] = storage[offset : offset : offset+count]
		offset += count
	}
	fillWorkflowTimelineRelationships(windows, parallelMap)
	return parallelMap, criticalRefs
}

func countWorkflowTimelineRelationships(
	windows []workflowTimelineWindow,
	criticalRefs map[string]bool,
) ([]int, int) {
	var counts []int
	totalParallelRefs := 0
	countOverlap := func(windowIdx int) {
		if counts == nil {
			counts = make([]int, len(windows))
		}
		counts[windowIdx]++
		totalParallelRefs++
	}

	activeCap := min(len(windows), 64)
	active := make([]workflowTimelineWindow, 0, activeCap)
	for i, a := range windows {
		kept := active[:0]
		for _, prior := range active {
			if !prior.end.After(a.start) {
				continue
			}
			kept = append(kept, prior)
			countOverlap(i)
			if prior.end.After(a.end) {
				criticalRefs[a.ref] = false
			}
			if a.end.After(prior.end) {
				criticalRefs[prior.ref] = false
			}
		}
		active = kept

		for j := i + 1; j < len(windows) && windows[j].start.Before(a.end); j++ {
			next := windows[j]
			countOverlap(i)
			if next.end.After(a.end) {
				criticalRefs[a.ref] = false
			}
			if a.end.After(next.end) {
				criticalRefs[next.ref] = false
			}
		}
		active = append(active, a)
	}
	return counts, totalParallelRefs
}

func fillWorkflowTimelineRelationships(windows []workflowTimelineWindow, parallelMap map[string][]string) {
	activeCap := min(len(windows), 64)
	active := make([]workflowTimelineWindow, 0, activeCap)
	for i, a := range windows {
		kept := active[:0]
		for _, prior := range active {
			if !prior.end.After(a.start) {
				continue
			}
			kept = append(kept, prior)
			parallelMap[a.ref] = append(parallelMap[a.ref], prior.ref)
		}
		active = kept

		for j := i + 1; j < len(windows) && windows[j].start.Before(a.end); j++ {
			parallelMap[a.ref] = append(parallelMap[a.ref], windows[j].ref)
		}
		active = append(active, a)
	}
}

func buildWorkflowTimelineParallelMap(windows []workflowTimelineWindow) map[string][]string {
	parallelMap := make(map[string][]string, len(windows))
	for i, a := range windows {
		for j, b := range windows {
			if i == j {
				continue
			}
			// Two windows overlap if a.start < b.end AND b.start < a.end.
			if a.start.Before(b.end) && b.start.Before(a.end) {
				parallelMap[a.ref] = append(parallelMap[a.ref], b.ref)
			}
		}
	}
	return parallelMap
}

type workflowTimelineWaitTracker struct {
	finishedAt        []time.Time
	finishIdx         int
	mostRecentFinish  time.Time
	hasFinishedBefore bool
}

func newWorkflowTimelineWaitTracker(stepRuns []domain.WorkflowStepRun) workflowTimelineWaitTracker {
	finishedAt := make([]time.Time, 0, len(stepRuns))
	for _, sr := range stepRuns {
		if sr.FinishedAt != nil {
			finishedAt = append(finishedAt, *sr.FinishedAt)
		}
	}
	sort.Slice(finishedAt, func(i, j int) bool {
		return finishedAt[i].Before(finishedAt[j])
	})
	return workflowTimelineWaitTracker{finishedAt: finishedAt}
}

func (t *workflowTimelineWaitTracker) waitBefore(startedAt *time.Time) int64 {
	if startedAt == nil {
		return 0
	}
	for t.finishIdx < len(t.finishedAt) && !t.finishedAt[t.finishIdx].After(*startedAt) {
		t.mostRecentFinish = t.finishedAt[t.finishIdx]
		t.hasFinishedBefore = true
		t.finishIdx++
	}
	if !t.hasFinishedBefore {
		return 0
	}
	if gap := startedAt.Sub(t.mostRecentFinish).Milliseconds(); gap > 0 {
		return gap
	}
	return 0
}
