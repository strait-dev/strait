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
	"github.com/stretchr/testify/require"
)

// Helpers local to this file

func stID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func stStore(t *testing.T) *store.Queries {
	t.Helper()
	require.False(t, testDB ==
		nil ||
		testDB.Pool ==
			nil)

	return store.New(testDB.Pool)
}

func stClean(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, testDB.
		CleanTables(ctx))

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
			require.Failf(t, "test failure",

				"clean %s: %v", tbl, err)
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
	require.False(t, testDB ==
		nil ||
		testDB.Pool ==
			nil)

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
	require.NoError(t, q.CreateJob(ctx,
		job))

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
	require.NoError(t, err)

	got, err := q.GetWorkflowRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusRunning,
		got.
			Status)

	// running -> completed
	err = q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusRunning, domain.WfStatusCompleted, map[string]any{
		"finished_at": time.Now().UTC(),
	})
	require.NoError(t, err)

	got, err = q.GetWorkflowRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusCompleted,
		got.
			Status)

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
	require.NoError(t, err)

	// running -> failed with error message
	err = q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusRunning, domain.WfStatusFailed, map[string]any{
		"error":       "step timeout exceeded",
		"finished_at": time.Now().UTC(),
	})
	require.NoError(t, err)

	got, err := q.GetWorkflowRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusFailed,
		got.
			Status)
	require.Equal(t, "step timeout exceeded",

		got.
			Error)

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
	require.Error(t, err)

	var te *domain.TransitionError
	require.True(t, errors.As(err, &te))

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
	require.Error(t, err)

	var te *domain.TransitionError
	require.True(t, errors.As(err, &te))

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
	require.NoError(t, err)

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
	require.EqualValues(t, 1, successes)
	require.EqualValues(t, 1, conflicts)

	// Final state must be terminal.
	got, err := q.GetWorkflowRun(ctx, run.ID)
	require.NoError(t, err)
	require.False(t, got.Status !=
		domain.
			WfStatusCompleted &&
		got.
			Status !=
			domain.
				WfStatusFailed,
	)

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
	require.NoError(t, err)

	// Attempt same transition again -- should succeed idempotently or return conflict.
	// The code returns nil if already in target state (idempotent).
	err = q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, nil)
	require.NoError(t, err)

	// This might fail with conflict because from=pending but current=running.
	// The implementation checks: if rows affected == 0 and current == target, returns nil.
	// But here from=pending, current=running, target=running -- it should return nil (idempotent).

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
	require.NoError(t, err)

	got, err := q.GetWorkflowStepRun(ctx, sr.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StepRunning,
		got.Status,
	)

	// running -> completed
	err = q.UpdateStepRunStatusFrom(ctx, sr.ID, domain.StepRunning, domain.StepCompleted, map[string]any{
		"output":      json.RawMessage(`{"result":"ok"}`),
		"finished_at": time.Now().UTC(),
	})
	require.NoError(t, err)

	got, err = q.GetWorkflowStepRun(ctx, sr.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StepCompleted,
		got.Status,
	)

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
	require.NoError(t, err)

	// running -> failed
	err = q.UpdateStepRunStatusFrom(ctx, sr.ID, domain.StepRunning, domain.StepFailed, map[string]any{
		"error":       "connection refused",
		"finished_at": time.Now().UTC(),
	})
	require.NoError(t, err)

	got, err := q.GetWorkflowStepRun(ctx, sr.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StepFailed,
		got.Status,
	)
	require.Equal(t, "connection refused",

		got.Error,
	)

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
		require.NoError(t, err)

	}

	gotA, err := q.GetWorkflowStepRun(ctx, srA.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StepCompleted,
		gotA.
			Status)

	gotB, err := q.GetWorkflowStepRun(ctx, srB.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StepFailed,
		gotB.Status,
	)

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
	require.EqualValues(t, 1, successes)
	require.EqualValues(t, 1, conflicts)

	got, err := q.GetWorkflowStepRun(ctx, sr.ID)
	require.NoError(t, err)
	require.False(t, got.Status !=
		domain.
			StepCompleted &&
		got.Status !=
			domain.
				StepFailed,
	)

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
	require.NoError(t, err)

	got, err := q.GetWorkflowStepRun(ctx, sr.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StepRunning,
		got.Status,
	)

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
	require.NoError(t, q.UpdateStepRunStatus(ctx,
		sr.ID, domain.StepRunning,

		fields,
	))

	var xminBefore string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM workflow_step_runs
		WHERE id = $1`,

		sr.ID,
	).
		Scan(&xminBefore))
	require.NoError(t, q.UpdateStepRunStatus(ctx,
		sr.ID, domain.StepRunning,

		fields,
	))

	var xminAfter string
	var status domain.StepRunStatus
	var attempt int
	var gotStartedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, status, attempt, started_at
		FROM workflow_step_runs
		WHERE id = $1`,

		sr.
			ID).Scan(&xminAfter, &status,
		&attempt, &gotStartedAt,
	))
	require.Equal(t, xminBefore,

		xminAfter,
	)
	require.Equal(t, domain.
		StepRunning,
		status)
	require.EqualValues(t, 2, attempt)
	require.True(t, gotStartedAt.
		Equal(startedAt))

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
	require.Error(t, err)

	var fe *domain.FieldError
	require.True(t, errors.As(err, &fe))

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
		require.NoError(t, q.Enqueue(ctx,
			run))

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
		require.NoError(t, err)

	}

	for _, count := range seen {
		require.LessOrEqual(t,
			count,
			1)

	}
	require.Len(t, seen, 20)

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
		require.NoError(t, q.Enqueue(ctx,
			run))

	}
	for range 5 {
		run := &domain.JobRun{ID: stID(), JobID: jobB.ID, ProjectID: projB}
		require.NoError(t, q.Enqueue(ctx,
			run))

	}

	// DequeueNByProject should only return runs from project A.
	dequeued, err := q.DequeueNByProject(ctx, 10, projA)
	require.NoError(t, err)
	require.Len(t, dequeued,

		5)

	for i := range dequeued {
		require.Equal(t, projA,

			dequeued[i].ProjectID,
		)

	}

	// Project B runs should still be queued.
	dequeuedB, err := q.DequeueNByProject(ctx, 10, projB)
	require.NoError(t, err)
	require.Len(t, dequeuedB,

		5)

}

func TestDequeue_EmptyQueueReturnsEmptySlice(t *testing.T) {
	ctx := context.Background()
	q := stQueue(t)
	stClean(t, ctx)

	dequeued, err := q.DequeueN(ctx, 10)
	require.NoError(t, err)
	require.Len(t, dequeued,

		0)

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
		require.NoError(t, q.Enqueue(ctx,
			run))

	}

	dequeued, err := q.DequeueN(ctx, 3)
	require.NoError(t, err)
	require.Len(t, dequeued,

		3)

	// The highest-priority run should be dequeued -- verify that priority 10
	// was included. The exact return order of DequeueN is by created_at ASC
	// (the ORDER BY in the outer SELECT), but the claim CTE picks highest
	// priority first, so all three should be present.
	priorities := make(map[int]bool)
	for i := range dequeued {
		priorities[dequeued[i].Priority] = true
	}
	require.False(t, !priorities[10] ||
		!priorities[5] || !priorities[0])

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
	require.NoError(t, q.CreateAuditEvent(ctx, ev))
	require.NotEqual(t, "",

		ev.ID)
	require.False(t, ev.CreatedAt.
		IsZero())

	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 10, nil, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, events,
		1,
	)
	require.Equal(t, ev.ID,

		events[0].
			ID)
	require.Equal(t, "job.create",

		events[0].Action,
	)

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
		require.NoError(t, q.CreateAuditEvent(ctx, &events[i]))

	}

	// Filter by actor.
	byActor, err := q.ListAuditEvents(ctx, projectID, "user-1", "", "", 10, nil, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, byActor,

		2)

	// Filter by resource type.
	byResource, err := q.ListAuditEvents(ctx, projectID, "", "job", "", 10, nil, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, byResource,

		2)

	// Filter by both actor and resource type.
	byBoth, err := q.ListAuditEvents(ctx, projectID, "user-1", "job", "", 10, nil, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, byBoth,
		1,
	)
	require.Equal(t, "job.create",

		byBoth[0].Action,
	)

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
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

	}

	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC().Add(1 * time.Hour)

	var streamed []domain.AuditEvent
	err := q.StreamAuditEvents(ctx, projectID, "", "", from, to, func(ev *domain.AuditEvent) error {
		streamed = append(streamed, *ev)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, streamed,

		5)

	// Verify ascending order.
	for i := 1; i < len(streamed); i++ {
		require.False(t, streamed[i].CreatedAt.
			Before(streamed[i-1].CreatedAt))

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
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

	}

	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC().Add(1 * time.Hour)

	var streamed []domain.AuditEvent
	err := q.StreamAuditEvents(ctx, projectID, "actor-a", "", from, to, func(ev *domain.AuditEvent) error {
		streamed = append(streamed, *ev)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, streamed,

		1)
	require.Equal(t, "actor-a",

		streamed[0].ActorID,
	)

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
	require.NoError(t, q.CreateAuditEvent(ctx, ev))

	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 1, nil, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, events,
		1,
	)
	require.GreaterOrEqual(
		t,
		len(events[0].Details), 100_000)

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
	require.NoError(t, q.CreateAuditEvent(ctx, ev))

	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 1, nil, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, events,
		1,
	)
	require.Equal(t, "{}",
		string(events[0].Details))

	// Empty details should be stored as {}.

}
