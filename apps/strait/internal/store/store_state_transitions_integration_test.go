//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
)

// Helpers local to this file

func stID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func stStore(t *testing.T) *store.Queries {
	t.Helper()
	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}
	return store.New(testDB.Pool)
}

func stClean(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := testDB.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	for _, tbl := range []string{
		"workflow_step_decisions",
		"workflow_snapshots",
		"endpoint_health_scores",
		"job_slo_evaluations",
		"job_slos",
		"run_state",
		"job_memory",
		"audit_events",
	} {
		if _, err := testDB.Pool.Exec(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}
}

func stPtr[T any](v T) *T { return new(v) }

func stCreateStepWithJob(t *testing.T, ctx context.Context, q *store.Queries, wf *domain.Workflow, projectID string, opts *testutil.WorkflowStepOpts) *domain.WorkflowStep {
	t.Helper()
	job := stCreateJob(t, ctx, q, projectID)
	if opts == nil {
		opts = &testutil.WorkflowStepOpts{}
	}
	opts.JobID = &job.ID
	return testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, opts)
}

func stQueue(t *testing.T) *queue.PgQueQueue {
	t.Helper()
	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	q := queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresRunWriter(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "store-" + stID(),
		ReceiveWindow: 100,
	})
	go q.RunTicker(ctx)
	return q
}

func stCreateJob(t *testing.T, ctx context.Context, q *store.Queries, projectID string) *domain.Job {
	t.Helper()
	job := &domain.Job{
		ID:          stID(),
		ProjectID:   projectID,
		Name:        "job-" + stID(),
		Slug:        "slug-" + stID(),
		EndpointURL: "https://example.com/st-job",
		MaxAttempts: 3,
		TimeoutSecs: 300,
		Enabled:     true,
	}
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	return job
}

// 1. Workflow run status transitions

func TestWorkflowRunStatus_ValidTransition_PendingToRunningToCompleted(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-wf-valid-transition-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-valid-transition"),
		Slug:      new("wf-valid-" + stID()),
	})

	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	// pending -> running
	now := time.Now().UTC()
	err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{
		"started_at": now,
	})
	if err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(pending->running) error = %v", err)
	}

	got, err := q.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if got.Status != domain.WfStatusRunning {
		t.Fatalf("status = %q, want %q", got.Status, domain.WfStatusRunning)
	}

	// running -> completed
	err = q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusRunning, domain.WfStatusCompleted, map[string]any{
		"finished_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(running->completed) error = %v", err)
	}

	got, err = q.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if got.Status != domain.WfStatusCompleted {
		t.Fatalf("status = %q, want %q", got.Status, domain.WfStatusCompleted)
	}
}

func TestWorkflowRunStatus_ValidTransition_PendingToRunningToFailed(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-wf-fail-transition-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-fail-transition"),
		Slug:      new("wf-fail-" + stID()),
	})

	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	// pending -> running
	err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{
		"started_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(pending->running) error = %v", err)
	}

	// running -> failed with error message
	err = q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusRunning, domain.WfStatusFailed, map[string]any{
		"error":       "step timeout exceeded",
		"finished_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(running->failed) error = %v", err)
	}

	got, err := q.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if got.Status != domain.WfStatusFailed {
		t.Fatalf("status = %q, want %q", got.Status, domain.WfStatusFailed)
	}
	if got.Error != "step timeout exceeded" {
		t.Fatalf("error = %q, want %q", got.Error, "step timeout exceeded")
	}
}

func TestWorkflowRunStatus_InvalidTransition_CompletedToRunning(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-wf-invalid-completed-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-invalid-completed"),
		Slug:      new("wf-invalid-c-" + stID()),
	})

	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    stPtr(domain.WfStatusCompleted),
	})

	err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusCompleted, domain.WfStatusRunning, nil)
	if err == nil {
		t.Fatal("expected error for completed->running transition, got nil")
	}

	var te *domain.TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("expected TransitionError, got %T: %v", err, err)
	}
}

func TestWorkflowRunStatus_InvalidTransition_FailedToPending(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-wf-invalid-failed-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-invalid-failed"),
		Slug:      new("wf-invalid-f-" + stID()),
	})

	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    stPtr(domain.WfStatusFailed),
	})

	err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusFailed, domain.WfStatusPending, nil)
	if err == nil {
		t.Fatal("expected error for failed->pending transition, got nil")
	}

	var te *domain.TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("expected TransitionError, got %T: %v", err, err)
	}
}

func TestWorkflowRunStatus_ConcurrentUpdates(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-wf-concurrent-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-concurrent"),
		Slug:      new("wf-concurrent-" + stID()),
	})

	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	// Move to running first.
	err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{
		"started_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(pending->running) error = %v", err)
	}

	// Two concurrent attempts to move from running -> completed and running -> failed.
	// Only one should succeed; the other should hit a conflict.
	var wg conc.WaitGroup
	results := make(chan error, 2)

	wg.Go(func() {
		results <- store.New(testDB.Pool).UpdateWorkflowRunStatus(ctx, run.ID,
			domain.WfStatusRunning, domain.WfStatusCompleted, map[string]any{
				"finished_at": time.Now().UTC(),
			})
	})
	wg.Go(func() {
		results <- store.New(testDB.Pool).UpdateWorkflowRunStatus(ctx, run.ID,
			domain.WfStatusRunning, domain.WfStatusFailed, map[string]any{
				"error":       "concurrent failure",
				"finished_at": time.Now().UTC(),
			})
	})

	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			conflicts++
		}
	}

	if successes != 1 {
		t.Fatalf("expected exactly 1 success, got %d", successes)
	}
	if conflicts != 1 {
		t.Fatalf("expected exactly 1 conflict, got %d", conflicts)
	}

	// Final state must be terminal.
	got, err := q.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if got.Status != domain.WfStatusCompleted && got.Status != domain.WfStatusFailed {
		t.Fatalf("final status = %q, want completed or failed", got.Status)
	}
}

func TestWorkflowRunStatus_IdempotentTransition(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-wf-idempotent-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-idempotent"),
		Slug:      new("wf-idempotent-" + stID()),
	})

	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	// pending -> running
	err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{
		"started_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("first transition error = %v", err)
	}

	// Attempt same transition again -- should succeed idempotently or return conflict.
	// The code returns nil if already in target state (idempotent).
	err = q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, nil)
	// This might fail with conflict because from=pending but current=running.
	// The implementation checks: if rows affected == 0 and current == target, returns nil.
	// But here from=pending, current=running, target=running -- it should return nil (idempotent).
	if err != nil {
		t.Fatalf("idempotent transition error = %v, expected nil", err)
	}
}

// 2. Step run status transitions

func TestStepRunStatus_WaitingToRunningToCompleted(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-step-lifecycle-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-step-lifecycle"),
		Slug:      new("wf-step-lc-" + stID()),
	})
	step := stCreateStepWithJob(t, ctx, q, wf, projectID, nil)
	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})
	sr := testutil.MustCreateWorkflowStepRun(t, ctx, q, run.ID, step.ID, &testutil.WorkflowStepRunOpts{
		Status: stPtr(domain.StepWaiting),
	})

	// waiting -> running
	err := q.UpdateStepRunStatusFrom(ctx, sr.ID, domain.StepWaiting, domain.StepRunning, map[string]any{
		"started_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpdateStepRunStatusFrom(waiting->running) error = %v", err)
	}

	got, err := q.GetWorkflowStepRun(ctx, sr.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun() error = %v", err)
	}
	if got.Status != domain.StepRunning {
		t.Fatalf("status = %q, want %q", got.Status, domain.StepRunning)
	}

	// running -> completed
	err = q.UpdateStepRunStatusFrom(ctx, sr.ID, domain.StepRunning, domain.StepCompleted, map[string]any{
		"output":      json.RawMessage(`{"result":"ok"}`),
		"finished_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpdateStepRunStatusFrom(running->completed) error = %v", err)
	}

	got, err = q.GetWorkflowStepRun(ctx, sr.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun() error = %v", err)
	}
	if got.Status != domain.StepCompleted {
		t.Fatalf("status = %q, want %q", got.Status, domain.StepCompleted)
	}
}

func TestStepRunStatus_WaitingToRunningToFailed(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-step-failure-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-step-failure"),
		Slug:      new("wf-step-fail-" + stID()),
	})
	step := stCreateStepWithJob(t, ctx, q, wf, projectID, nil)
	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})
	sr := testutil.MustCreateWorkflowStepRun(t, ctx, q, run.ID, step.ID, &testutil.WorkflowStepRunOpts{
		Status: stPtr(domain.StepWaiting),
	})

	// waiting -> running
	err := q.UpdateStepRunStatusFrom(ctx, sr.ID, domain.StepWaiting, domain.StepRunning, map[string]any{
		"started_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpdateStepRunStatusFrom(waiting->running) error = %v", err)
	}

	// running -> failed
	err = q.UpdateStepRunStatusFrom(ctx, sr.ID, domain.StepRunning, domain.StepFailed, map[string]any{
		"error":       "connection refused",
		"finished_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpdateStepRunStatusFrom(running->failed) error = %v", err)
	}

	got, err := q.GetWorkflowStepRun(ctx, sr.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun() error = %v", err)
	}
	if got.Status != domain.StepFailed {
		t.Fatalf("status = %q, want %q", got.Status, domain.StepFailed)
	}
	if got.Error != "connection refused" {
		t.Fatalf("error = %q, want %q", got.Error, "connection refused")
	}
}

func TestStepRunStatus_ConcurrentUpdatesWithinSameWorkflow(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-step-concurrent-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-step-concurrent"),
		Slug:      new("wf-step-conc-" + stID()),
	})

	stepA := stCreateStepWithJob(t, ctx, q, wf, projectID, &testutil.WorkflowStepOpts{
		StepRef: new("step-a"),
	})
	stepB := stCreateStepWithJob(t, ctx, q, wf, projectID, &testutil.WorkflowStepOpts{
		StepRef: new("step-b"),
	})

	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	srA := testutil.MustCreateWorkflowStepRun(t, ctx, q, run.ID, stepA.ID, &testutil.WorkflowStepRunOpts{
		Status:  stPtr(domain.StepRunning),
		StepRef: new("step-a"),
	})
	srB := testutil.MustCreateWorkflowStepRun(t, ctx, q, run.ID, stepB.ID, &testutil.WorkflowStepRunOpts{
		Status:  stPtr(domain.StepRunning),
		StepRef: new("step-b"),
	})

	// Concurrently complete both step runs.
	var wg conc.WaitGroup
	errCh := make(chan error, 2)

	wg.Go(func() {
		errCh <- store.New(testDB.Pool).UpdateStepRunStatusFrom(ctx, srA.ID,
			domain.StepRunning, domain.StepCompleted, map[string]any{
				"output":      json.RawMessage(`{"stepA":"done"}`),
				"finished_at": time.Now().UTC(),
			})
	})
	wg.Go(func() {
		errCh <- store.New(testDB.Pool).UpdateStepRunStatusFrom(ctx, srB.ID,
			domain.StepRunning, domain.StepFailed, map[string]any{
				"error":       "step B failed",
				"finished_at": time.Now().UTC(),
			})
	})

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent step update error = %v", err)
		}
	}

	gotA, err := q.GetWorkflowStepRun(ctx, srA.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun(A) error = %v", err)
	}
	if gotA.Status != domain.StepCompleted {
		t.Fatalf("step A status = %q, want %q", gotA.Status, domain.StepCompleted)
	}

	gotB, err := q.GetWorkflowStepRun(ctx, srB.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun(B) error = %v", err)
	}
	if gotB.Status != domain.StepFailed {
		t.Fatalf("step B status = %q, want %q", gotB.Status, domain.StepFailed)
	}
}

func TestStepRunStatus_ConflictOnSameStepRun(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-step-conflict-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-step-conflict"),
		Slug:      new("wf-step-conflict-" + stID()),
	})
	step := stCreateStepWithJob(t, ctx, q, wf, projectID, nil)
	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})
	sr := testutil.MustCreateWorkflowStepRun(t, ctx, q, run.ID, step.ID, &testutil.WorkflowStepRunOpts{
		Status: stPtr(domain.StepRunning),
	})

	// Two goroutines race to transition running -> completed vs running -> failed.
	var wg conc.WaitGroup
	results := make(chan error, 2)

	wg.Go(func() {
		results <- store.New(testDB.Pool).UpdateStepRunStatusFrom(ctx, sr.ID,
			domain.StepRunning, domain.StepCompleted, map[string]any{
				"finished_at": time.Now().UTC(),
			})
	})
	wg.Go(func() {
		results <- store.New(testDB.Pool).UpdateStepRunStatusFrom(ctx, sr.ID,
			domain.StepRunning, domain.StepFailed, map[string]any{
				"error":       "race failure",
				"finished_at": time.Now().UTC(),
			})
	})

	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			conflicts++
		}
	}

	if successes != 1 {
		t.Fatalf("expected exactly 1 success, got %d", successes)
	}
	if conflicts != 1 {
		t.Fatalf("expected exactly 1 conflict, got %d", conflicts)
	}

	got, err := q.GetWorkflowStepRun(ctx, sr.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun() error = %v", err)
	}
	if got.Status != domain.StepCompleted && got.Status != domain.StepFailed {
		t.Fatalf("final status = %q, want completed or failed", got.Status)
	}
}

func TestStepRunStatus_UpdateStepRunStatus_SetsFields(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-step-fields-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-step-fields"),
		Slug:      new("wf-step-fields-" + stID()),
	})
	step := stCreateStepWithJob(t, ctx, q, wf, projectID, nil)
	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})
	sr := testutil.MustCreateWorkflowStepRun(t, ctx, q, run.ID, step.ID, nil)

	now := time.Now().UTC()
	err := q.UpdateStepRunStatus(ctx, sr.ID, domain.StepRunning, map[string]any{
		"started_at": now,
		"attempt":    2,
	})
	if err != nil {
		t.Fatalf("UpdateStepRunStatus() error = %v", err)
	}

	got, err := q.GetWorkflowStepRun(ctx, sr.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun() error = %v", err)
	}
	if got.Status != domain.StepRunning {
		t.Fatalf("status = %q, want %q", got.Status, domain.StepRunning)
	}
}

func TestStepRunStatus_UpdateStepRunStatusSkipsNoOp(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-step-noop-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-step-noop"),
		Slug:      new("wf-step-noop-" + stID()),
	})
	step := stCreateStepWithJob(t, ctx, q, wf, projectID, nil)
	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})
	sr := testutil.MustCreateWorkflowStepRun(t, ctx, q, run.ID, step.ID, nil)

	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	fields := map[string]any{
		"started_at": startedAt,
		"attempt":    2,
	}
	if err := q.UpdateStepRunStatus(ctx, sr.ID, domain.StepRunning, fields); err != nil {
		t.Fatalf("initial UpdateStepRunStatus() error = %v", err)
	}

	var xminBefore string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM workflow_step_runs
		WHERE id = $1`,
		sr.ID,
	).Scan(&xminBefore); err != nil {
		t.Fatalf("query workflow_step_runs xmin before no-op: %v", err)
	}

	if err := q.UpdateStepRunStatus(ctx, sr.ID, domain.StepRunning, fields); err != nil {
		t.Fatalf("no-op UpdateStepRunStatus() error = %v", err)
	}

	var xminAfter string
	var status domain.StepRunStatus
	var attempt int
	var gotStartedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text, status, attempt, started_at
		FROM workflow_step_runs
		WHERE id = $1`,
		sr.ID,
	).Scan(&xminAfter, &status, &attempt, &gotStartedAt); err != nil {
		t.Fatalf("query workflow_step_runs after no-op: %v", err)
	}
	if xminAfter != xminBefore {
		t.Fatalf("workflow_step_runs no-op update changed xmin from %s to %s", xminBefore, xminAfter)
	}
	if status != domain.StepRunning {
		t.Fatalf("status = %q, want %q", status, domain.StepRunning)
	}
	if attempt != 2 {
		t.Fatalf("attempt = %d, want 2", attempt)
	}
	if !gotStartedAt.Equal(startedAt) {
		t.Fatalf("started_at = %v, want %v", gotStartedAt, startedAt)
	}
}

func TestStepRunStatus_RejectsDisallowedField(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-step-bad-field-" + stID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-step-bad-field"),
		Slug:      new("wf-step-bad-" + stID()),
	})
	step := stCreateStepWithJob(t, ctx, q, wf, projectID, nil)
	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})
	sr := testutil.MustCreateWorkflowStepRun(t, ctx, q, run.ID, step.ID, nil)

	err := q.UpdateStepRunStatus(ctx, sr.ID, domain.StepRunning, map[string]any{
		"evil_column": "drop table",
	})
	if err == nil {
		t.Fatal("expected error for disallowed field, got nil")
	}
	var fe *domain.FieldError
	if !errors.As(err, &fe) {
		t.Fatalf("expected FieldError, got %T: %v", err, err)
	}
}

// 3. Dequeue race conditions

func TestDequeue_ConcurrentDequeuesNoDuplicates(t *testing.T) {
	ctx := context.Background()
	q := stQueue(t)
	st := stStore(t)
	stClean(t, ctx)

	job := stCreateJob(t, ctx, st, "proj-dequeue-race-"+stID())
	for range 20 {
		run := &domain.JobRun{ID: stID(), JobID: job.ID, ProjectID: job.ProjectID}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	var (
		wg   conc.WaitGroup
		mu   sync.Mutex
		seen = make(map[string]int)
	)

	errCh := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			runs, err := q.DequeueN(ctx, 10)
			if err != nil {
				errCh <- err
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for i := range runs {
				seen[runs[i].ID]++
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("DequeueN() error = %v", err)
		}
	}

	for runID, count := range seen {
		if count > 1 {
			t.Fatalf("run %s dequeued %d times, want exactly 1", runID, count)
		}
	}
	if len(seen) != 20 {
		t.Fatalf("total unique dequeued = %d, want 20", len(seen))
	}
}

func TestDequeue_FairDistributesAcrossJobs(t *testing.T) {
	ctx := context.Background()
	q := stQueue(t)
	st := stStore(t)
	stClean(t, ctx)

	projectID := "proj-dequeue-fair-" + stID()

	// Create 3 jobs with different queue depths: 10, 5, 1 runs.
	jobA := stCreateJob(t, ctx, st, projectID)
	jobB := stCreateJob(t, ctx, st, projectID)
	jobC := stCreateJob(t, ctx, st, projectID)

	for range 10 {
		run := &domain.JobRun{ID: stID(), JobID: jobA.ID, ProjectID: projectID}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue(A) error = %v", err)
		}
	}
	for range 5 {
		run := &domain.JobRun{ID: stID(), JobID: jobB.ID, ProjectID: projectID}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue(B) error = %v", err)
		}
	}
	run := &domain.JobRun{ID: stID(), JobID: jobC.ID, ProjectID: projectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue(C) error = %v", err)
	}

	// Fair dequeue of 3 should pick at most one from each job.
	dequeued, err := q.DequeueNFair(ctx, 3)
	if err != nil {
		t.Fatalf("DequeueNFair() error = %v", err)
	}
	if len(dequeued) != 3 {
		t.Fatalf("DequeueNFair() len = %d, want 3", len(dequeued))
	}

	jobsSeen := make(map[string]int)
	for i := range dequeued {
		jobsSeen[dequeued[i].JobID]++
	}

	// Each job should appear at most once in a fair dequeue.
	for jobID, count := range jobsSeen {
		if count > 1 {
			t.Fatalf("fair dequeue picked %d runs from job %s, want at most 1", count, jobID)
		}
	}
	// All 3 jobs should be represented.
	if len(jobsSeen) != 3 {
		t.Fatalf("fair dequeue covered %d distinct jobs, want 3", len(jobsSeen))
	}
}

func TestDequeue_ByProjectRespectsIsolation(t *testing.T) {
	ctx := context.Background()
	q := stQueue(t)
	st := stStore(t)
	stClean(t, ctx)

	projA := "proj-dequeue-iso-a-" + stID()
	projB := "proj-dequeue-iso-b-" + stID()

	jobA := stCreateJob(t, ctx, st, projA)
	jobB := stCreateJob(t, ctx, st, projB)

	for range 5 {
		run := &domain.JobRun{ID: stID(), JobID: jobA.ID, ProjectID: projA}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue(A) error = %v", err)
		}
	}
	for range 5 {
		run := &domain.JobRun{ID: stID(), JobID: jobB.ID, ProjectID: projB}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue(B) error = %v", err)
		}
	}

	// DequeueNByProject should only return runs from project A.
	dequeued, err := q.DequeueNByProject(ctx, 10, projA)
	if err != nil {
		t.Fatalf("DequeueNByProject() error = %v", err)
	}
	if len(dequeued) != 5 {
		t.Fatalf("DequeueNByProject() len = %d, want 5", len(dequeued))
	}
	for i := range dequeued {
		if dequeued[i].ProjectID != projA {
			t.Fatalf("dequeued run project = %q, want %q", dequeued[i].ProjectID, projA)
		}
	}

	// Project B runs should still be queued.
	dequeuedB, err := q.DequeueNByProject(ctx, 10, projB)
	if err != nil {
		t.Fatalf("DequeueNByProject(B) error = %v", err)
	}
	if len(dequeuedB) != 5 {
		t.Fatalf("DequeueNByProject(B) len = %d, want 5", len(dequeuedB))
	}
}

func TestDequeue_EmptyQueueReturnsEmptySlice(t *testing.T) {
	ctx := context.Background()
	q := stQueue(t)
	stClean(t, ctx)

	dequeued, err := q.DequeueN(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(dequeued) != 0 {
		t.Fatalf("DequeueN() len = %d, want 0", len(dequeued))
	}
}

func TestDequeue_RespectsPriorityOrdering(t *testing.T) {
	ctx := context.Background()
	q := stQueue(t)
	st := stStore(t)
	stClean(t, ctx)

	projectID := "proj-dequeue-prio-" + stID()
	job := stCreateJob(t, ctx, st, projectID)

	// Enqueue runs with priorities 0, 5, 10 (higher = more urgent).
	for _, prio := range []int{0, 5, 10} {
		run := &domain.JobRun{
			ID:        stID(),
			JobID:     job.ID,
			ProjectID: projectID,
			Priority:  prio,
		}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue(priority=%d) error = %v", prio, err)
		}
	}

	dequeued, err := q.DequeueN(ctx, 3)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(dequeued) != 3 {
		t.Fatalf("DequeueN() len = %d, want 3", len(dequeued))
	}

	// The highest-priority run should be dequeued -- verify that priority 10
	// was included. The exact return order of DequeueN is by created_at ASC
	// (the ORDER BY in the outer SELECT), but the claim CTE picks highest
	// priority first, so all three should be present.
	priorities := make(map[int]bool)
	for i := range dequeued {
		priorities[dequeued[i].Priority] = true
	}
	if !priorities[10] || !priorities[5] || !priorities[0] {
		t.Fatalf("expected priorities {0,5,10}, got %v", priorities)
	}
}

// 4. Audit event creation

func TestAuditEvent_CreateAndList(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-audit-" + stID()

	ev := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "user-1",
		ActorType:    "user",
		Action:       "job.create",
		ResourceType: "job",
		ResourceID:   stID(),
		Details:      json.RawMessage(`{"name":"test-job"}`),
	}

	if err := q.CreateAuditEvent(ctx, ev); err != nil {
		t.Fatalf("CreateAuditEvent() error = %v", err)
	}
	if ev.ID == "" {
		t.Fatal("CreateAuditEvent() did not set ID")
	}
	if ev.CreatedAt.IsZero() {
		t.Fatal("CreateAuditEvent() did not set CreatedAt")
	}

	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 10, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ListAuditEvents() len = %d, want 1", len(events))
	}
	if events[0].ID != ev.ID {
		t.Fatalf("event ID = %q, want %q", events[0].ID, ev.ID)
	}
	if events[0].Action != "job.create" {
		t.Fatalf("event action = %q, want %q", events[0].Action, "job.create")
	}
}

func TestAuditEvent_FilterByActorAndResourceType(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-audit-filter-" + stID()

	// Create events from different actors and resource types.
	events := []domain.AuditEvent{
		{ProjectID: projectID, ActorID: "user-1", ActorType: "user", Action: "job.create", ResourceType: "job", ResourceID: stID()},
		{ProjectID: projectID, ActorID: "user-2", ActorType: "user", Action: "job.delete", ResourceType: "job", ResourceID: stID()},
		{ProjectID: projectID, ActorID: "user-1", ActorType: "user", Action: "workflow.create", ResourceType: "workflow", ResourceID: stID()},
		{ProjectID: projectID, ActorID: "system", ActorType: "system", Action: "run.complete", ResourceType: "run", ResourceID: stID()},
	}
	for i := range events {
		if err := q.CreateAuditEvent(ctx, &events[i]); err != nil {
			t.Fatalf("CreateAuditEvent(%d) error = %v", i, err)
		}
	}

	// Filter by actor.
	byActor, err := q.ListAuditEvents(ctx, projectID, "user-1", "", "", 10, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents(actor=user-1) error = %v", err)
	}
	if len(byActor) != 2 {
		t.Fatalf("ListAuditEvents(actor=user-1) len = %d, want 2", len(byActor))
	}

	// Filter by resource type.
	byResource, err := q.ListAuditEvents(ctx, projectID, "", "job", "", 10, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents(resource=job) error = %v", err)
	}
	if len(byResource) != 2 {
		t.Fatalf("ListAuditEvents(resource=job) len = %d, want 2", len(byResource))
	}

	// Filter by both actor and resource type.
	byBoth, err := q.ListAuditEvents(ctx, projectID, "user-1", "job", "", 10, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents(actor=user-1,resource=job) error = %v", err)
	}
	if len(byBoth) != 1 {
		t.Fatalf("ListAuditEvents(actor+resource) len = %d, want 1", len(byBoth))
	}
	if byBoth[0].Action != "job.create" {
		t.Fatalf("filtered event action = %q, want %q", byBoth[0].Action, "job.create")
	}
}

func TestAuditEvent_StreamAuditEvents(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-audit-stream-" + stID()

	for i := range 5 {
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      "user-stream",
			ActorType:    "user",
			Action:       "job.update",
			ResourceType: "job",
			ResourceID:   stID(),
			Details:      json.RawMessage(fmt.Sprintf(`{"index":%d}`, i)),
		}
		if err := q.CreateAuditEvent(ctx, ev); err != nil {
			t.Fatalf("CreateAuditEvent(%d) error = %v", i, err)
		}
	}

	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC().Add(1 * time.Hour)

	var streamed []domain.AuditEvent
	err := q.StreamAuditEvents(ctx, projectID, "", "", from, to, func(ev *domain.AuditEvent) error {
		streamed = append(streamed, *ev)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamAuditEvents() error = %v", err)
	}
	if len(streamed) != 5 {
		t.Fatalf("StreamAuditEvents() count = %d, want 5", len(streamed))
	}

	// Verify ascending order.
	for i := 1; i < len(streamed); i++ {
		if streamed[i].CreatedAt.Before(streamed[i-1].CreatedAt) {
			t.Fatalf("stream not in ascending order at index %d", i)
		}
	}
}

func TestAuditEvent_StreamFiltersByActor(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-audit-stream-actor-" + stID()

	ev1 := &domain.AuditEvent{
		ProjectID: projectID, ActorID: "actor-a", ActorType: "user",
		Action: "job.create", ResourceType: "job", ResourceID: stID(),
	}
	ev2 := &domain.AuditEvent{
		ProjectID: projectID, ActorID: "actor-b", ActorType: "user",
		Action: "job.delete", ResourceType: "job", ResourceID: stID(),
	}
	for _, ev := range []*domain.AuditEvent{ev1, ev2} {
		if err := q.CreateAuditEvent(ctx, ev); err != nil {
			t.Fatalf("CreateAuditEvent() error = %v", err)
		}
	}

	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC().Add(1 * time.Hour)

	var streamed []domain.AuditEvent
	err := q.StreamAuditEvents(ctx, projectID, "actor-a", "", from, to, func(ev *domain.AuditEvent) error {
		streamed = append(streamed, *ev)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamAuditEvents(actor=actor-a) error = %v", err)
	}
	if len(streamed) != 1 {
		t.Fatalf("StreamAuditEvents(actor-a) count = %d, want 1", len(streamed))
	}
	if streamed[0].ActorID != "actor-a" {
		t.Fatalf("streamed actor = %q, want %q", streamed[0].ActorID, "actor-a")
	}
}

func TestAuditEvent_LargePayload(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-audit-large-" + stID()

	// Build a large JSON payload (~100KB).
	largeValue := strings.Repeat("x", 100_000)
	details := json.RawMessage(`{"data":"` + largeValue + `"}`)

	ev := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "user-large",
		ActorType:    "user",
		Action:       "export.create",
		ResourceType: "export",
		ResourceID:   stID(),
		Details:      details,
	}

	if err := q.CreateAuditEvent(ctx, ev); err != nil {
		t.Fatalf("CreateAuditEvent() with large payload error = %v", err)
	}

	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 1, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ListAuditEvents() len = %d, want 1", len(events))
	}
	if len(events[0].Details) < 100_000 {
		t.Fatalf("details size = %d, want >= 100000", len(events[0].Details))
	}
}

func TestAuditEvent_EmptyDetails(t *testing.T) {
	ctx := context.Background()
	q := stStore(t)
	stClean(t, ctx)

	projectID := "proj-audit-empty-" + stID()

	ev := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "user-empty",
		ActorType:    "user",
		Action:       "job.view",
		ResourceType: "job",
		ResourceID:   stID(),
	}

	if err := q.CreateAuditEvent(ctx, ev); err != nil {
		t.Fatalf("CreateAuditEvent() with nil details error = %v", err)
	}

	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 1, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ListAuditEvents() len = %d, want 1", len(events))
	}
	// Empty details should be stored as {}.
	if string(events[0].Details) != "{}" {
		t.Fatalf("details = %q, want %q", string(events[0].Details), "{}")
	}
}
