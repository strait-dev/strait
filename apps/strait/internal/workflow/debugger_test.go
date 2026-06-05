package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTime(s string) *time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return &t
}

func TestDebugView_AllStepsIncluded(t *testing.T) {
	t.Parallel()
	wfRun := &domain.WorkflowRun{
		ID:         "wfr-1",
		WorkflowID: "wf-1",
		Status:     domain.WfStatusCompleted,
		StartedAt:  makeTime("2026-01-01T00:00:00Z"),
		FinishedAt: makeTime("2026-01-01T00:00:10Z"),
	}
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted,
			StartedAt: makeTime("2026-01-01T00:00:00Z"), FinishedAt: makeTime("2026-01-01T00:00:03Z")},
		{ID: "sr-b", StepRef: "b", Status: domain.StepCompleted,
			StartedAt: makeTime("2026-01-01T00:00:03Z"), FinishedAt: makeTime("2026-01-01T00:00:10Z")},
	}

	view, err := BuildDebugView(wfRun, steps, stepRuns, nil)
	require.NoError(t, err)
	require.Len(t, view.Steps,
		2)
	assert.EqualValues(t, 10000, view.
		TotalDuration,
	)

}

func TestDebugView_FailedStepHasError(t *testing.T) {
	t.Parallel()
	wfRun := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusFailed}
	steps := []domain.WorkflowStep{{StepRef: "a", StepType: "job"}}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepFailed, Error: "connection refused"},
	}

	view, err := BuildDebugView(wfRun, steps, stepRuns, nil)
	require.NoError(t, err)
	assert.Equal(t, "connection refused",

		view.
			Steps[0].Error,
	)

}

func TestDebugView_CostPerStep(t *testing.T) {
	t.Parallel()
	wfRun := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusCompleted}
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted},
		{ID: "sr-b", StepRef: "b", Status: domain.StepCompleted},
	}
	costs := map[string]int64{"sr-a": 1000, "sr-b": 2500}

	view, err := BuildDebugView(wfRun, steps, stepRuns, costs)
	require.NoError(t, err)
	assert.EqualValues(t, 1000, view.
		Steps[0].Cost)
	assert.EqualValues(t, 3500, view.
		TotalCost,
	)

}

func TestDebugView_DataFlowEdges(t *testing.T) {
	t.Parallel()
	wfRun := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusCompleted}
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted, Output: json.RawMessage(`{"data":"value"}`)},
		{ID: "sr-b", StepRef: "b", Status: domain.StepCompleted},
	}

	view, err := BuildDebugView(wfRun, steps, stepRuns, nil)
	require.NoError(t, err)
	require.Len(t, view.DataFlow,
		1,
	)
	assert.False(t, view.DataFlow[0].FromStepRef !=
		"a" ||
		view.DataFlow[0].ToStepRef != "b",
	)
	assert.NotEqual(t, 0, view.
		DataFlow[0].DataSize,
	)

}

func TestDebugView_PendingStepsIncluded(t *testing.T) {
	t.Parallel()
	wfRun := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusRunning}
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted},
		{ID: "sr-b", StepRef: "b", Status: domain.StepPending},
	}

	view, err := BuildDebugView(wfRun, steps, stepRuns, nil)
	require.NoError(t, err)
	require.Len(t, view.Steps,
		2)
	assert.Equal(t, "pending",
		view.
			Steps[1].Status,
	)

}

func TestDebugView_StepTimingAccuracy(t *testing.T) {
	t.Parallel()
	wfRun := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusCompleted}
	steps := []domain.WorkflowStep{{StepRef: "a", StepType: "job"}}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted,
			StartedAt: makeTime("2026-01-01T00:00:00Z"), FinishedAt: makeTime("2026-01-01T00:00:05Z")},
	}

	view, err := BuildDebugView(wfRun, steps, stepRuns, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 5000, view.
		Steps[0].Duration,
	)

}

func TestDebugView_NilRun(t *testing.T) {
	t.Parallel()
	_, err := BuildDebugView(nil, nil, nil, nil)
	assert.Error(t, err)

}

// Compare tests.

func TestCompare_IdenticalRuns(t *testing.T) {
	t.Parallel()
	runA := &domain.WorkflowRun{ID: "a", Status: domain.WfStatusCompleted}
	runB := &domain.WorkflowRun{ID: "b", Status: domain.WfStatusCompleted}
	stepsA := []domain.WorkflowStepRun{
		{StepRef: "s1", Status: domain.StepCompleted, StartedAt: makeTime("2026-01-01T00:00:00Z"), FinishedAt: makeTime("2026-01-01T00:00:05Z")},
	}
	stepsB := []domain.WorkflowStepRun{
		{StepRef: "s1", Status: domain.StepCompleted, StartedAt: makeTime("2026-01-01T00:00:00Z"), FinishedAt: makeTime("2026-01-01T00:00:05Z")},
	}

	comp := CompareRuns(runA, stepsA, runB, stepsB)
	assert.Nil(t, comp.
		StatusDiff,
	)
	assert.Len(t, comp.StepDiffs,
		0,
	)

}

func TestCompare_DifferentStatuses(t *testing.T) {
	t.Parallel()
	runA := &domain.WorkflowRun{ID: "a", Status: domain.WfStatusCompleted}
	runB := &domain.WorkflowRun{ID: "b", Status: domain.WfStatusFailed}

	comp := CompareRuns(runA, nil, runB, nil)
	require.NotNil(t, comp.StatusDiff)
	assert.False(t, comp.StatusDiff.
		A != "completed" ||
		comp.
			StatusDiff.
			B != "failed")

}

func TestCompare_DifferentTiming(t *testing.T) {
	t.Parallel()
	runA := &domain.WorkflowRun{ID: "a", Status: domain.WfStatusCompleted}
	runB := &domain.WorkflowRun{ID: "b", Status: domain.WfStatusCompleted}
	stepsA := []domain.WorkflowStepRun{
		{StepRef: "s1", Status: domain.StepCompleted, StartedAt: makeTime("2026-01-01T00:00:00Z"), FinishedAt: makeTime("2026-01-01T00:00:05Z")},
	}
	stepsB := []domain.WorkflowStepRun{
		{StepRef: "s1", Status: domain.StepCompleted, StartedAt: makeTime("2026-01-01T00:00:00Z"), FinishedAt: makeTime("2026-01-01T00:00:10Z")},
	}

	comp := CompareRuns(runA, stepsA, runB, stepsB)
	require.Len(t, comp.StepDiffs,

		1)
	assert.False(t, comp.StepDiffs[0].DurationA !=
		5000 ||
		comp.StepDiffs[0].DurationB != 10000,
	)

}

func TestCompare_MissingSteps(t *testing.T) {
	t.Parallel()
	runA := &domain.WorkflowRun{ID: "a", Status: domain.WfStatusCompleted}
	runB := &domain.WorkflowRun{ID: "b", Status: domain.WfStatusCompleted}
	stepsA := []domain.WorkflowStepRun{
		{StepRef: "s1", Status: domain.StepCompleted},
		{StepRef: "s2", Status: domain.StepCompleted},
	}
	stepsB := []domain.WorkflowStepRun{
		{StepRef: "s1", Status: domain.StepCompleted},
	}

	comp := CompareRuns(runA, stepsA, runB, stepsB)
	found := false
	for _, d := range comp.StepDiffs {
		if d.StepRef == "s2" && d.OnlyInA {
			found = true
		}
	}
	assert.True(t, found)

}

func TestCompare_MissingStepsInA(t *testing.T) {
	t.Parallel()
	runA := &domain.WorkflowRun{ID: "a", Status: domain.WfStatusCompleted}
	runB := &domain.WorkflowRun{ID: "b", Status: domain.WfStatusCompleted}
	stepsA := []domain.WorkflowStepRun{
		{StepRef: "s1", Status: domain.StepCompleted},
	}
	stepsB := []domain.WorkflowStepRun{
		{StepRef: "s1", Status: domain.StepCompleted},
		{StepRef: "s2", Status: domain.StepCompleted},
	}

	comp := CompareRuns(runA, stepsA, runB, stepsB)
	found := false
	for _, d := range comp.StepDiffs {
		if d.StepRef == "s2" && d.OnlyInB {
			found = true
		}
	}
	assert.True(t, found)

}

// Adversarial tests.

func TestDebugView_1000StepWorkflow(t *testing.T) {
	t.Parallel()
	wfRun := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusCompleted}
	steps := make([]domain.WorkflowStep, 1000)
	stepRuns := make([]domain.WorkflowStepRun, 1000)
	for i := range 1000 {
		ref := fmt.Sprintf("step-%03d", i)
		steps[i] = domain.WorkflowStep{StepRef: ref, StepType: "job"}
		stepRuns[i] = domain.WorkflowStepRun{ID: "sr-" + ref, StepRef: ref, Status: domain.StepCompleted}
	}

	view, err := BuildDebugView(wfRun, steps, stepRuns, nil)
	require.NoError(t, err)
	assert.Len(t, view.Steps,
		1000)

}

func TestDebugView_CrossProjectAccess(t *testing.T) {
	t.Parallel()
	// The debug view itself doesn't enforce tenancy -- that's the API layer's job.
	// But it should correctly pass through project context.
	wfRun := &domain.WorkflowRun{ID: "wfr-1", WorkflowID: "wf-1", Status: domain.WfStatusCompleted}
	view, err := BuildDebugView(wfRun, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "wf-1",
		view.WorkflowID,
	)

}

func TestDebugView_RunInProgress(t *testing.T) {
	t.Parallel()
	now := time.Now()
	wfRun := &domain.WorkflowRun{
		ID:        "wfr-1",
		Status:    domain.WfStatusRunning,
		StartedAt: &now,
	}
	steps := []domain.WorkflowStep{
		{StepRef: "a", StepType: "job"},
		{StepRef: "b", StepType: "job", DependsOn: []string{"a"}},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted},
		{ID: "sr-b", StepRef: "b", Status: domain.StepRunning},
	}

	view, err := BuildDebugView(wfRun, steps, stepRuns, nil)
	require.NoError(t, err)
	assert.Equal(t, "running",
		view.
			Status)
	assert.EqualValues(t, 0, view.
		TotalDuration,
	)

	// TotalDuration should be 0 since workflow hasn't finished.

}

func TestDebugView_LargeOutputPayload(t *testing.T) {
	t.Parallel()
	wfRun := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusCompleted}
	steps := []domain.WorkflowStep{{StepRef: "a", StepType: "job"}}
	largeOutput := json.RawMessage(`{"data":"` + strings.Repeat("x", 1024*1024) + `"}`)
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted, Output: largeOutput},
	}

	view, err := BuildDebugView(wfRun, steps, stepRuns, nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t,
		len(view.
			Steps[0].
			Output), 1024*
			1024,
	)

}

func BenchmarkBuildDebugView_DataFlowChain1000(b *testing.B) {
	wfRun := &domain.WorkflowRun{ID: "wfr-1", WorkflowID: "wf-1", Status: domain.WfStatusCompleted}
	steps := make([]domain.WorkflowStep, 1000)
	stepRuns := make([]domain.WorkflowStepRun, 1000)
	for i := range steps {
		ref := fmt.Sprintf("step-%04d", i)
		steps[i] = domain.WorkflowStep{StepRef: ref, StepType: "job"}
		if i > 0 {
			steps[i].DependsOn = []string{fmt.Sprintf("step-%04d", i-1)}
		}
		stepRuns[i] = domain.WorkflowStepRun{
			ID:      "sr-" + ref,
			StepRef: ref,
			Status:  domain.StepCompleted,
			Output:  json.RawMessage(`{"ok":true}`),
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		view, err := BuildDebugView(wfRun, steps, stepRuns, nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(view.DataFlow) != len(steps)-1 {
			b.Fatalf("data flow edges = %d, want %d", len(view.DataFlow), len(steps)-1)
		}
	}
}

func BenchmarkCompareRuns_Identical1000Steps(b *testing.B) {
	runA := &domain.WorkflowRun{ID: "a", Status: domain.WfStatusCompleted}
	runB := &domain.WorkflowRun{ID: "b", Status: domain.WfStatusCompleted}
	stepsA := make([]domain.WorkflowStepRun, 1000)
	stepsB := make([]domain.WorkflowStepRun, 1000)
	for i := range stepsA {
		ref := fmt.Sprintf("step-%04d", i)
		stepsA[i] = domain.WorkflowStepRun{StepRef: ref, Status: domain.StepCompleted}
		stepsB[i] = domain.WorkflowStepRun{StepRef: ref, Status: domain.StepCompleted}
	}

	b.ReportAllocs()
	for b.Loop() {
		comp := CompareRuns(runA, stepsA, runB, stepsB)
		if len(comp.StepDiffs) != 0 {
			b.Fatalf("step diffs = %d, want 0", len(comp.StepDiffs))
		}
	}
}
