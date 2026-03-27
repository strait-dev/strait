package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(view.Steps))
	}
	if view.TotalDuration != 10000 {
		t.Errorf("total duration = %d ms, want 10000", view.TotalDuration)
	}
}

func TestDebugView_FailedStepHasError(t *testing.T) {
	t.Parallel()
	wfRun := &domain.WorkflowRun{ID: "wfr-1", Status: domain.WfStatusFailed}
	steps := []domain.WorkflowStep{{StepRef: "a", StepType: "job"}}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-a", StepRef: "a", Status: domain.StepFailed, Error: "connection refused"},
	}

	view, err := BuildDebugView(wfRun, steps, stepRuns, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.Steps[0].Error != "connection refused" {
		t.Errorf("error = %q, want 'connection refused'", view.Steps[0].Error)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.Steps[0].Cost != 1000 {
		t.Errorf("step a cost = %d, want 1000", view.Steps[0].Cost)
	}
	if view.TotalCost != 3500 {
		t.Errorf("total cost = %d, want 3500", view.TotalCost)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.DataFlow) != 1 {
		t.Fatalf("expected 1 data flow edge, got %d", len(view.DataFlow))
	}
	if view.DataFlow[0].FromStepRef != "a" || view.DataFlow[0].ToStepRef != "b" {
		t.Errorf("edge = %s->%s, want a->b", view.DataFlow[0].FromStepRef, view.DataFlow[0].ToStepRef)
	}
	if view.DataFlow[0].DataSize == 0 {
		t.Error("data size should be > 0 when output exists")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Steps) != 2 {
		t.Fatalf("pending step should be included")
	}
	if view.Steps[1].Status != "pending" {
		t.Errorf("step b status = %q, want pending", view.Steps[1].Status)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.Steps[0].Duration != 5000 {
		t.Errorf("step duration = %d ms, want 5000", view.Steps[0].Duration)
	}
}

func TestDebugView_NilRun(t *testing.T) {
	t.Parallel()
	_, err := BuildDebugView(nil, nil, nil, nil)
	if err == nil {
		t.Error("expected error for nil run")
	}
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
	if comp.StatusDiff != nil {
		t.Error("identical statuses should have no status diff")
	}
	if len(comp.StepDiffs) != 0 {
		t.Errorf("identical runs should have no step diffs, got %d", len(comp.StepDiffs))
	}
}

func TestCompare_DifferentStatuses(t *testing.T) {
	t.Parallel()
	runA := &domain.WorkflowRun{ID: "a", Status: domain.WfStatusCompleted}
	runB := &domain.WorkflowRun{ID: "b", Status: domain.WfStatusFailed}

	comp := CompareRuns(runA, nil, runB, nil)
	if comp.StatusDiff == nil {
		t.Fatal("expected status diff")
	}
	if comp.StatusDiff.A != "completed" || comp.StatusDiff.B != "failed" {
		t.Errorf("diff = %v, want completed vs failed", comp.StatusDiff)
	}
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
	if len(comp.StepDiffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(comp.StepDiffs))
	}
	if comp.StepDiffs[0].DurationA != 5000 || comp.StepDiffs[0].DurationB != 10000 {
		t.Errorf("durations = %d/%d, want 5000/10000", comp.StepDiffs[0].DurationA, comp.StepDiffs[0].DurationB)
	}
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
	if !found {
		t.Error("expected s2 to be marked as only_in_a")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Steps) != 1000 {
		t.Errorf("expected 1000 steps, got %d", len(view.Steps))
	}
}

func TestDebugView_CrossProjectAccess(t *testing.T) {
	t.Parallel()
	// The debug view itself doesn't enforce tenancy -- that's the API layer's job.
	// But it should correctly pass through project context.
	wfRun := &domain.WorkflowRun{ID: "wfr-1", WorkflowID: "wf-1", Status: domain.WfStatusCompleted}
	view, err := BuildDebugView(wfRun, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.WorkflowID != "wf-1" {
		t.Errorf("workflow_id = %q, want wf-1", view.WorkflowID)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.Status != "running" {
		t.Errorf("status = %q, want running", view.Status)
	}
	// TotalDuration should be 0 since workflow hasn't finished.
	if view.TotalDuration != 0 {
		t.Errorf("total duration should be 0 for running workflow, got %d", view.TotalDuration)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Steps[0].Output) < 1024*1024 {
		t.Error("large output should be preserved in debug view")
	}
}
