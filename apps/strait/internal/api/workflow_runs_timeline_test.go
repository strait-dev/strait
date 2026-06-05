package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sort"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestHandleGetWorkflowRunTimeline_Success(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	start := now.Add(-10 * time.Second)
	mid := now.Add(-5 * time.Second)
	end := now.Add(-1 * time.Second)

	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:         "wr-1",
				Status:     domain.WfStatusCompleted,
				StartedAt:  &start,
				FinishedAt: &end,
			}, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{
					ID:         "sr-1",
					StepRef:    "step-a",
					Status:     domain.StepCompleted,
					StartedAt:  &start,
					FinishedAt: &mid,
				},
				{
					ID:         "sr-2",
					StepRef:    "step-b",
					Status:     domain.StepCompleted,
					StartedAt:  &mid,
					FinishedAt: &end,
				},
			}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/timeline", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp domain.TimelineResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.Equal(t, "wr-1", resp.
		WorkflowRunID,
	)
	require.Equal(t, "completed",
		resp.Status,
	)
	require.Len(t,
		resp.Steps, 2)
	require.Equal(t, "step-a", resp.
		Steps[0].StepRef)
	require.False(t, resp.Steps[0].
		DurationMs <=
		0)
	require.False(t, resp.TotalMs <=
		0)

}

func TestHandleGetWorkflowRunTimeline_ParallelDetection(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	start := now.Add(-10 * time.Second)
	mid := now.Add(-5 * time.Second)
	end := now.Add(-1 * time.Second)

	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:         "wr-1",
				Status:     domain.WfStatusCompleted,
				StartedAt:  &start,
				FinishedAt: &end,
			}, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			// Two overlapping steps (parallel).
			return []domain.WorkflowStepRun{
				{
					ID:         "sr-1",
					StepRef:    "step-a",
					Status:     domain.StepCompleted,
					StartedAt:  &start,
					FinishedAt: &mid,
				},
				{
					ID:         "sr-2",
					StepRef:    "step-b",
					Status:     domain.StepCompleted,
					StartedAt:  &start,
					FinishedAt: &end,
				},
			}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/timeline", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp domain.TimelineResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.False(t, len(resp.Steps[0].ParallelWith) !=
		1 || resp.Steps[0].
		ParallelWith[0] !=
		"step-b",
	)
	require.False(t, len(resp.Steps[1].ParallelWith) !=
		1 || resp.Steps[1].
		ParallelWith[0] !=
		"step-a",
	)

	// Both steps should be parallel with each other.

}

func TestHandleGetWorkflowRunTimeline_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return nil, store.ErrWorkflowRunNotFound
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-missing/timeline", ""))
	require.Equal(t, http.StatusNotFound,

		w.Code)

}

func TestHandleGetWorkflowRunTimeline_EmptySteps(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:        "wr-1",
				Status:    domain.WfStatusRunning,
				StartedAt: &now,
			}, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/timeline", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp domain.TimelineResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.Len(t,
		resp.Steps, 0)

}

func TestBuildWorkflowRunTimeline_WaitUsesMostRecentFinishedStepBeforeStart(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	run := &domain.WorkflowRun{ID: "wr-1", Status: domain.WfStatusRunning, StartedAt: &base}
	longStart := base
	longEnd := base.Add(100 * time.Second)
	shortStart := base.Add(10 * time.Second)
	shortEnd := base.Add(20 * time.Second)
	waitingStart := base.Add(50 * time.Second)
	waitingEnd := base.Add(60 * time.Second)

	resp := buildWorkflowRunTimeline(run, []domain.WorkflowStepRun{
		{ID: "sr-long", StepRef: "long", Status: domain.StepCompleted, StartedAt: &longStart, FinishedAt: &longEnd},
		{ID: "sr-short", StepRef: "short", Status: domain.StepCompleted, StartedAt: &shortStart, FinishedAt: &shortEnd},
		{ID: "sr-waiting", StepRef: "waiting", Status: domain.StepCompleted, StartedAt: &waitingStart, FinishedAt: &waitingEnd},
	}, base.Add(2*time.Minute))

	step := timelineStepByRef(t, resp.Steps, "waiting")
	require.Equal(t, int64(30*time.
		Second/
		time.Millisecond,
	), step.WaitMs,
	)

}

func TestBuildWorkflowRunTimeline_CriticalRefsDenseOverlap(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	run := &domain.WorkflowRun{ID: "wr-1", Status: domain.WfStatusRunning, StartedAt: &base}
	start := base
	endA := base.Add(10 * time.Second)
	endB := base.Add(20 * time.Second)
	endC := base.Add(30 * time.Second)

	resp := buildWorkflowRunTimeline(run, []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted, StartedAt: &start, FinishedAt: &endA},
		{ID: "sr-b", StepRef: "b", Status: domain.StepCompleted, StartedAt: &start, FinishedAt: &endB},
		{ID: "sr-c", StepRef: "c", Status: domain.StepCompleted, StartedAt: &start, FinishedAt: &endC},
	}, base.Add(time.Minute))
	require.False(t, timelineStepByRef(t,
		resp.Steps,
		"a").OnCriticalPath)
	require.False(t, timelineStepByRef(t,
		resp.Steps,
		"b").OnCriticalPath)
	require.True(
		t, timelineStepByRef(t,
			resp.Steps,
			"c").OnCriticalPath)

}

func TestBuildWorkflowTimelineRelationships_NoBoundaryOverlap(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	windows := []workflowTimelineWindow{
		{ref: "a", start: base, end: base.Add(time.Second)},
		{ref: "b", start: base.Add(time.Second), end: base.Add(2 * time.Second)},
		{ref: "c", start: base.Add(2 * time.Second), end: base.Add(3 * time.Second)},
	}

	parallelMap, criticalRefs := buildWorkflowTimelineRelationships(windows)

	for _, ref := range []string{"a", "b", "c"} {
		require.Len(t,
			parallelMap[ref], 0)
		require.True(
			t, criticalRefs[ref])

	}
}

func TestBuildWorkflowTimelineRelationships_NestedOverlapOrdering(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	windows := []workflowTimelineWindow{
		{ref: "outer", start: base, end: base.Add(10 * time.Second)},
		{ref: "inner-a", start: base.Add(time.Second), end: base.Add(2 * time.Second)},
		{ref: "inner-b", start: base.Add(3 * time.Second), end: base.Add(4 * time.Second)},
		{ref: "tail", start: base.Add(10 * time.Second), end: base.Add(11 * time.Second)},
	}

	parallelMap, criticalRefs := buildWorkflowTimelineRelationships(windows)
	require.True(
		t, slices.Equal(parallelMap["outer"],
			[]string{"inner-a",
				"inner-b",
			}))
	require.True(
		t, slices.Equal(parallelMap["inner-a"], []string{"outer"}))
	require.True(
		t, slices.Equal(parallelMap["inner-b"], []string{"outer"}))
	require.Len(t,
		parallelMap["tail"], 0,
	)
	require.False(t, !criticalRefs["outer"] || criticalRefs["inner-a"] ||
		criticalRefs["inner-b"] ||
		!criticalRefs["tail"])

}

func TestEstimateWorkflowCriticalPath_DeterministicWideReadyQueue(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	steps := []domain.WorkflowStep{
		{StepRef: "root-c", StepType: domain.WorkflowStepTypeJob, TimeoutSecsOverride: 1},
		{StepRef: "root-a", StepType: domain.WorkflowStepTypeJob, TimeoutSecsOverride: 1},
		{StepRef: "root-b", StepType: domain.WorkflowStepTypeJob, TimeoutSecsOverride: 1},
		{StepRef: "join", StepType: domain.WorkflowStepTypeJob, DependsOn: []string{"root-b", "root-a", "root-c"}, TimeoutSecsOverride: 5},
	}

	path, estimateMS, remainingMS := estimateWorkflowCriticalPath(steps, nil, now)
	require.True(
		t, slices.Equal(path,
			[]string{"root-b",
				"join"}))
	require.EqualValues(t, 6_000, estimateMS)
	require.EqualValues(t, 6_000, remainingMS)

}

func TestEstimateWorkflowCriticalPath_IgnoresUnknownDependencies(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: domain.WorkflowStepTypeJob, DependsOn: []string{"external"}, TimeoutSecsOverride: 2},
		{StepRef: "b", StepType: domain.WorkflowStepTypeJob, DependsOn: []string{"a"}, TimeoutSecsOverride: 3},
	}

	path, estimateMS, remainingMS := estimateWorkflowCriticalPath(steps, nil, now)
	require.True(
		t, slices.Equal(path,
			[]string{"a",
				"b"}))
	require.EqualValues(t, 5_000, estimateMS)
	require.EqualValues(t, 5_000, remainingMS)

}

func TestEstimateStepTiming_RunningPastTimeoutClampsRemaining(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	startedAt := now.Add(-3 * time.Second)

	estimateMS, remainingMS := estimateStepTiming(
		domain.WorkflowStep{StepRef: "slow", StepType: domain.WorkflowStepTypeJob, TimeoutSecsOverride: 2},
		domain.WorkflowStepRun{StepRef: "slow", Status: domain.StepRunning, StartedAt: &startedAt},
		now,
	)
	require.EqualValues(t, 2_000, estimateMS)
	require.EqualValues(t, 0, remainingMS)

}

func timelineStepByRef(t *testing.T, steps []domain.TimelineStep, ref string) domain.TimelineStep {
	t.Helper()
	for _, step := range steps {
		if step.StepRef == ref {
			return step
		}
	}
	require.Failf(t, "test failure",

		"step %q not found", ref)
	return domain.TimelineStep{}
}

var timelineResponseSink domain.TimelineResponse
var criticalPathSink []string
var criticalPathEstimateSink int64
var criticalPathRemainingSink int64

func BenchmarkEstimateWorkflowCriticalPath_WideReadyQueue(b *testing.B) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	for _, size := range []int{128, 512} {
		b.Run(fmt.Sprintf("roots=%d", size), func(b *testing.B) {
			steps := make([]domain.WorkflowStep, 0, size+1)
			deps := make([]string, 0, size)
			for i := range size {
				ref := fmt.Sprintf("root-%04d", i)
				steps = append(steps, domain.WorkflowStep{
					StepRef:             ref,
					StepType:            domain.WorkflowStepTypeJob,
					TimeoutSecsOverride: 1,
				})
				deps = append(deps, ref)
			}
			steps = append(steps, domain.WorkflowStep{
				StepRef:             "join",
				StepType:            domain.WorkflowStepTypeJob,
				DependsOn:           deps,
				TimeoutSecsOverride: 10,
			})

			b.ReportAllocs()
			for b.Loop() {
				criticalPathSink, criticalPathEstimateSink, criticalPathRemainingSink = estimateWorkflowCriticalPath(steps, nil, now)
			}
		})
	}
}

func BenchmarkBuildWorkflowRunTimeline_DenseOverlap(b *testing.B) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)
	run := &domain.WorkflowRun{
		ID:         "wr-bench",
		Status:     domain.WfStatusRunning,
		StartedAt:  &start,
		FinishedAt: &end,
	}
	buildTimeline := buildWorkflowRunTimeline
	if os.Getenv("STRAIT_TIMELINE_BENCH_OLD") == "1" {
		buildTimeline = buildWorkflowRunTimelineOldForBenchmark
	}

	for _, size := range []int{32, 96} {
		b.Run(fmt.Sprintf("steps=%d", size), func(b *testing.B) {
			stepRuns := make([]domain.WorkflowStepRun, size)
			for i := range stepRuns {
				stepStart := start.Add(time.Duration(i) * time.Millisecond)
				stepRuns[i] = domain.WorkflowStepRun{
					ID:         fmt.Sprintf("sr-%d", i),
					StepRef:    fmt.Sprintf("step-%d", i),
					Status:     domain.StepCompleted,
					StartedAt:  &stepStart,
					FinishedAt: &end,
				}
			}

			b.ReportAllocs()
			for b.Loop() {
				timelineResponseSink = buildTimeline(run, stepRuns, now)
			}
		})
	}
}

func BenchmarkBuildWorkflowRunTimeline_SparseNoOverlap(b *testing.B) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	end := start.Add(time.Second)
	run := &domain.WorkflowRun{
		ID:         "wr-sparse-bench",
		Status:     domain.WfStatusCompleted,
		StartedAt:  &start,
		FinishedAt: &end,
	}

	for _, size := range []int{256, 1024} {
		b.Run(fmt.Sprintf("steps=%d", size), func(b *testing.B) {
			stepRuns := make([]domain.WorkflowStepRun, size)
			for i := range stepRuns {
				stepStart := start.Add(time.Duration(i*2) * time.Second)
				stepEnd := stepStart.Add(time.Second)
				stepRuns[i] = domain.WorkflowStepRun{
					ID:         fmt.Sprintf("sr-%d", i),
					StepRef:    fmt.Sprintf("step-%d", i),
					Status:     domain.StepCompleted,
					StartedAt:  &stepStart,
					FinishedAt: &stepEnd,
				}
			}

			b.ReportAllocs()
			for b.Loop() {
				timelineResponseSink = buildWorkflowRunTimeline(run, stepRuns, now)
			}
		})
	}
}

func buildWorkflowRunTimelineOldForBenchmark(run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, now time.Time) domain.TimelineResponse {
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

	parallelMap := buildWorkflowTimelineParallelMap(windows)
	criticalRefs := make(map[string]bool)
	for _, w := range windows {
		isOnCritical := true
		for _, pRef := range parallelMap[w.ref] {
			for _, w2 := range windows {
				if w2.ref == pRef && w2.end.After(w.end) {
					isOnCritical = false
					break
				}
			}
			if !isOnCritical {
				break
			}
		}
		if isOnCritical {
			criticalRefs[w.ref] = true
		}
	}

	timelineSteps := make([]domain.TimelineStep, 0, len(stepRuns))
	for i, sr := range stepRuns {
		var durationMs int64
		if sr.StartedAt != nil {
			if sr.FinishedAt != nil {
				durationMs = sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
			} else {
				durationMs = now.Sub(*sr.StartedAt).Milliseconds()
			}
		}

		var waitMs int64
		if sr.StartedAt != nil && i > 0 {
			for k := i - 1; k >= 0; k-- {
				if stepRuns[k].FinishedAt != nil {
					gap := sr.StartedAt.Sub(*stepRuns[k].FinishedAt).Milliseconds()
					if gap > 0 {
						waitMs = gap
					}
					break
				}
			}
		}

		timelineSteps = append(timelineSteps, domain.TimelineStep{
			StepRunID:      sr.ID,
			StepRef:        sr.StepRef,
			Status:         string(sr.Status),
			StartedAt:      sr.StartedAt,
			FinishedAt:     sr.FinishedAt,
			DurationMs:     durationMs,
			ParallelWith:   parallelMap[sr.StepRef],
			OnCriticalPath: criticalRefs[sr.StepRef],
			WaitMs:         waitMs,
		})
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
