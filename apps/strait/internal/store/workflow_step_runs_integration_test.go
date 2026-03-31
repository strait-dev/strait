//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

func TestListRunningStepRunsByWorkflowRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-running-step-runs", domain.StepRunning)

	// Create a second step run in pending status.
	pendingSR := testutil.BuildWorkflowStepRun(wfRun.ID, stepRun.WorkflowStepID, &testutil.WorkflowStepRunOpts{
		Status:  testutil.Ptr(domain.StepPending),
		StepRef: testutil.Ptr("pending-step"),
	})
	if err := q.CreateWorkflowStepRun(ctx, pendingSR); err != nil {
		t.Fatalf("CreateWorkflowStepRun(pending) error = %v", err)
	}

	running, err := q.ListRunningStepRunsByWorkflowRun(ctx, wfRun.ID, 100)
	if err != nil {
		t.Fatalf("ListRunningStepRunsByWorkflowRun() error = %v", err)
	}
	if len(running) != 1 {
		t.Fatalf("ListRunningStepRunsByWorkflowRun() len = %d, want 1", len(running))
	}
	if running[0].ID != stepRun.ID {
		t.Fatalf("ListRunningStepRunsByWorkflowRun() id = %q, want %q", running[0].ID, stepRun.ID)
	}

	// Empty case.
	empty, err := q.ListRunningStepRunsByWorkflowRun(ctx, newID(), 100)
	if err != nil {
		t.Fatalf("ListRunningStepRunsByWorkflowRun(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListRunningStepRunsByWorkflowRun(empty) len = %d, want 0", len(empty))
	}
}

func TestListStepRunStatusesByWorkflowRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-step-run-statuses", domain.StepRunning)

	statuses, err := q.ListStepRunStatusesByWorkflowRun(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("ListStepRunStatusesByWorkflowRun() error = %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("ListStepRunStatusesByWorkflowRun() len = %d, want 1", len(statuses))
	}
	if statuses[stepRun.StepRef] != domain.StepRunning {
		t.Fatalf("ListStepRunStatusesByWorkflowRun() status = %q, want %q", statuses[stepRun.StepRef], domain.StepRunning)
	}

	// Empty case.
	empty, err := q.ListStepRunStatusesByWorkflowRun(ctx, newID())
	if err != nil {
		t.Fatalf("ListStepRunStatusesByWorkflowRun(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListStepRunStatusesByWorkflowRun(empty) len = %d, want 0", len(empty))
	}
}

func TestUpdateStepRunStatusFrom(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, _, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-step-run-status-from", domain.StepPending)

	// Transition from pending to running.
	if err := q.UpdateStepRunStatusFrom(ctx, stepRun.ID, domain.StepPending, domain.StepRunning, map[string]any{
		"started_at": time.Now().UTC().Truncate(time.Microsecond),
	}); err != nil {
		t.Fatalf("UpdateStepRunStatusFrom() error = %v", err)
	}

	got, err := q.GetWorkflowStepRun(ctx, stepRun.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun() error = %v", err)
	}
	if got.Status != domain.StepRunning {
		t.Fatalf("status = %q, want %q", got.Status, domain.StepRunning)
	}

	// Conflict: try from pending again (already running).
	err = q.UpdateStepRunStatusFrom(ctx, stepRun.ID, domain.StepPending, domain.StepCompleted, nil)
	if err == nil {
		t.Fatal("UpdateStepRunStatusFrom() conflict expected error, got nil")
	}

	// Invalid field.
	err = q.UpdateStepRunStatusFrom(ctx, stepRun.ID, domain.StepRunning, domain.StepCompleted, map[string]any{
		"bad_field": "x",
	})
	if err == nil {
		t.Fatal("UpdateStepRunStatusFrom() bad field expected error, got nil")
	}
}

func TestCountNonTerminalStepRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, _ := mustCreateWorkflowStepFixture(t, ctx, q, "project-count-non-terminal", domain.StepPending)

	count, err := q.CountNonTerminalStepRuns(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("CountNonTerminalStepRuns() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("CountNonTerminalStepRuns() = %d, want 1", count)
	}

	// Empty case.
	zeroCount, err := q.CountNonTerminalStepRuns(ctx, newID())
	if err != nil {
		t.Fatalf("CountNonTerminalStepRuns(empty) error = %v", err)
	}
	if zeroCount != 0 {
		t.Fatalf("CountNonTerminalStepRuns(empty) = %d, want 0", zeroCount)
	}
}

func TestListFailedStepRunRefs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-failed-step-refs", domain.StepFailed)

	refs, err := q.ListFailedStepRunRefs(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("ListFailedStepRunRefs() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("ListFailedStepRunRefs() len = %d, want 1", len(refs))
	}
	if refs[0] != stepRun.StepRef {
		t.Fatalf("ListFailedStepRunRefs() ref = %q, want %q", refs[0], stepRun.StepRef)
	}

	// Empty case.
	empty, err := q.ListFailedStepRunRefs(ctx, newID())
	if err != nil {
		t.Fatalf("ListFailedStepRunRefs(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListFailedStepRunRefs(empty) len = %d, want 0", len(empty))
	}
}

func TestCancelNonTerminalStepRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, _ := mustCreateWorkflowStepFixture(t, ctx, q, "project-cancel-step-runs", domain.StepPending)

	now := time.Now().UTC()
	affected, err := q.CancelNonTerminalStepRuns(ctx, wfRun.ID, now, "workflow canceled")
	if err != nil {
		t.Fatalf("CancelNonTerminalStepRuns() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("CancelNonTerminalStepRuns() affected = %d, want 1", affected)
	}

	// Calling again should affect 0.
	affected2, err := q.CancelNonTerminalStepRuns(ctx, wfRun.ID, now, "workflow canceled")
	if err != nil {
		t.Fatalf("CancelNonTerminalStepRuns(again) error = %v", err)
	}
	if affected2 != 0 {
		t.Fatalf("CancelNonTerminalStepRuns(again) affected = %d, want 0", affected2)
	}
}

func TestSkipStepRunsByRefs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-skip-step-runs", domain.StepPending)

	now := time.Now().UTC()
	affected, err := q.SkipStepRunsByRefs(ctx, wfRun.ID, []string{stepRun.StepRef}, now)
	if err != nil {
		t.Fatalf("SkipStepRunsByRefs() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("SkipStepRunsByRefs() affected = %d, want 1", affected)
	}

	got, err := q.GetWorkflowStepRun(ctx, stepRun.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun() error = %v", err)
	}
	if got.Status != domain.StepSkipped {
		t.Fatalf("status = %q, want %q", got.Status, domain.StepSkipped)
	}

	// Empty refs returns 0.
	zero, err := q.SkipStepRunsByRefs(ctx, wfRun.ID, []string{}, now)
	if err != nil {
		t.Fatalf("SkipStepRunsByRefs(empty) error = %v", err)
	}
	if zero != 0 {
		t.Fatalf("SkipStepRunsByRefs(empty) affected = %d, want 0", zero)
	}
}

func TestGetStepOutputs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-step-outputs", domain.StepCompleted)

	// Set output on the step run.
	if err := q.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepCompleted, map[string]any{
		"output": json.RawMessage(`{"result":"ok"}`),
	}); err != nil {
		t.Fatalf("UpdateStepRunStatus() error = %v", err)
	}

	outputs, err := q.GetStepOutputs(ctx, wfRun.ID, []string{stepRun.StepRef})
	if err != nil {
		t.Fatalf("GetStepOutputs() error = %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("GetStepOutputs() len = %d, want 1", len(outputs))
	}
	outStr := string(outputs[stepRun.StepRef])
	if !strings.Contains(outStr, `"result"`) || !strings.Contains(outStr, `"ok"`) {
		t.Fatalf("GetStepOutputs() output = %s, expected to contain result:ok", outStr)
	}

	// Unknown step ref.
	empty, err := q.GetStepOutputs(ctx, wfRun.ID, []string{"nonexistent"})
	if err != nil {
		t.Fatalf("GetStepOutputs(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("GetStepOutputs(empty) len = %d, want 0", len(empty))
	}
}

func TestListRunnableStepRunsByWorkflowRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-runnable-step-runs"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: testutil.Ptr(projectID),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: testutil.Ptr(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: testutil.Ptr(stepJob.ID), StepRef: testutil.Ptr("runnable-step")})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: testutil.Ptr(projectID)})

	// Create a step run with deps_completed == deps_required (runnable).
	sr := &domain.WorkflowStepRun{
		WorkflowRunID:  wfRun.ID,
		WorkflowStepID: step.ID,
		StepRef:        step.StepRef,
		Status:         domain.StepPending,
		DepsCompleted:  1,
		DepsRequired:   1,
	}
	if err := q.CreateWorkflowStepRun(ctx, sr); err != nil {
		t.Fatalf("CreateWorkflowStepRun() error = %v", err)
	}

	runnable, err := q.ListRunnableStepRunsByWorkflowRun(ctx, wfRun.ID, 100)
	if err != nil {
		t.Fatalf("ListRunnableStepRunsByWorkflowRun() error = %v", err)
	}
	if len(runnable) != 1 {
		t.Fatalf("ListRunnableStepRunsByWorkflowRun() len = %d, want 1", len(runnable))
	}
	if runnable[0].ID != sr.ID {
		t.Fatalf("ListRunnableStepRunsByWorkflowRun() id = %q, want %q", runnable[0].ID, sr.ID)
	}

	// Empty case.
	empty, err := q.ListRunnableStepRunsByWorkflowRun(ctx, newID(), 100)
	if err != nil {
		t.Fatalf("ListRunnableStepRunsByWorkflowRun(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListRunnableStepRunsByWorkflowRun(empty) len = %d, want 0", len(empty))
	}
}

func TestGetCostGateDefaultAction(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Empty result for a nonexistent step run.
	action, err := q.GetCostGateDefaultAction(ctx, newID())
	if err != nil {
		t.Fatalf("GetCostGateDefaultAction() error = %v", err)
	}
	if action != "" {
		t.Fatalf("GetCostGateDefaultAction() = %q, want empty", action)
	}
}

func TestListOrphanedStepRuns_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orphans, err := q.ListOrphanedStepRuns(ctx)
	if err != nil {
		t.Fatalf("ListOrphanedStepRuns() error = %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("ListOrphanedStepRuns() len = %d, want 0", len(orphans))
	}
}

func TestGetWorkflowStepRun_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetWorkflowStepRun(ctx, newID())
	if !errors.Is(err, store.ErrWorkflowStepRunNotFound) {
		t.Fatalf("GetWorkflowStepRun() error = %v, want ErrWorkflowStepRunNotFound", err)
	}
}

func TestIncrementStepRunAttempt_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.IncrementStepRunAttempt(ctx, newID(), 2)
	if err == nil {
		t.Fatal("IncrementStepRunAttempt() expected error, got nil")
	}
	if !errors.Is(err, store.ErrWorkflowStepRunNotFound) {
		t.Fatalf("IncrementStepRunAttempt() error = %v, want ErrWorkflowStepRunNotFound", err)
	}
}
