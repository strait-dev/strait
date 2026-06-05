//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/stretchr/testify/require"
)

// GetLatestCheckpoint.

func TestRuns_GetLatestCheckpoint_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-latest-checkpoint")
	run := mustCreateRun(t, ctx, q, job)

	cp1 := &domain.RunCheckpoint{RunID: run.ID, Source: "sdk", State: json.RawMessage(`{"step":1}`)}
	require.NoError(t, q.CreateRunCheckpoint(ctx,
		cp1))

	cp2 := &domain.RunCheckpoint{RunID: run.ID, Source: "auto", State: json.RawMessage(`{"step":2}`)}
	require.NoError(t, q.CreateRunCheckpoint(ctx,
		cp2))

	latest, err := q.GetLatestCheckpoint(ctx, run.ID)
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.EqualValues(t, 2, latest.
		Sequence,
	)

}

func TestRuns_GetLatestCheckpoint_NoCheckpoints(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-latest-checkpoint-empty")
	run := mustCreateRun(t, ctx, q, job)

	latest, err := q.GetLatestCheckpoint(ctx, run.ID)
	require.NoError(t, err)
	require.Nil(t, latest)

}

func TestRuns_GetLatestCheckpoint_NonexistentRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	latest, err := q.GetLatestCheckpoint(ctx, newID())
	require.NoError(t, err)
	require.Nil(t, latest)

}

// ListFinishedRunsSince.

func TestRuns_ListFinishedRunsSince_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-finished-runs-since"
	job := mustCreateJob(t, ctx, q, projectID)

	cutoff := time.Now().UTC().Add(-1 * time.Hour)

	// Create a completed run with finished_at after cutoff.
	r1 := baseRun(job, newID())
	r1.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r1))

	finishedAt := time.Now().UTC()
	require.NoError(t, q.UpdateRunStatus(ctx, r1.
		ID, domain.StatusExecuting,

		domain.
			StatusCompleted,
		map[string]any{"finished_at": finishedAt}))

	runs, err := q.ListFinishedRunsSince(ctx, projectID, cutoff, "", 100)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, r1.ID,

		runs[0].ID,
	)

}

func TestRuns_ListFinishedRunsSince_ExcludesBeforeCutoff(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-finished-runs-before"
	job := mustCreateJob(t, ctx, q, projectID)

	// Create a run finished before cutoff - use direct SQL to control finished_at.
	r1 := baseRun(job, newID())
	r1.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r1))

	pastFinish := time.Now().UTC().Add(-2 * time.Hour)
	require.NoError(t, q.UpdateRunStatus(ctx, r1.
		ID, domain.StatusExecuting,

		domain.
			StatusCompleted,
		map[string]any{"finished_at": pastFinish}))

	cutoff := time.Now().UTC().Add(-1 * time.Hour)
	runs, err := q.ListFinishedRunsSince(ctx, projectID, cutoff, "", 100)
	require.NoError(t, err)
	require.Len(t, runs, 0)

}

func TestRuns_ListFinishedRunsSince_ExcludesNonTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-finished-runs-nonterminal"
	job := mustCreateJob(t, ctx, q, projectID)

	r1 := baseRun(job, newID())
	r1.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r1))

	cutoff := time.Now().UTC().Add(-1 * time.Hour)
	runs, err := q.ListFinishedRunsSince(ctx, projectID, cutoff, "", 100)
	require.NoError(t, err)
	require.Len(t, runs, 0)

}

// ListDLQDepthByJob.

func TestRuns_ListDLQDepthByJob_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-dlq-depth"
	job := baseJob(newID(), projectID)
	threshold := 2
	job.DLQAlertThreshold = &threshold
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Create dead_letter runs. Need to go through executing -> dead_letter.
	for range 3 {
		r := baseRun(job, newID())
		r.Status = domain.StatusExecuting
		require.NoError(t, q.CreateRun(ctx,
			r))
		require.NoError(t, q.UpdateRunStatus(ctx, r.
			ID, domain.StatusExecuting,

			domain.StatusDeadLetter,

			nil))

	}

	depths, err := q.ListDLQDepthByJob(ctx)
	require.NoError(t, err)
	require.Len(t, depths,
		1,
	)
	require.Equal(t, job.ID,

		depths[0].JobID)
	require.EqualValues(t, 3, depths[0].DLQCount)

}

func TestRuns_ListDLQDepthByJob_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	depths, err := q.ListDLQDepthByJob(ctx)
	require.NoError(t, err)
	require.Len(t, depths,
		0,
	)

}

func TestRuns_ListDLQDepthByJob_BelowThreshold(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-dlq-below-threshold"
	job := baseJob(newID(), projectID)
	threshold := 5
	job.DLQAlertThreshold = &threshold
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Only 2 dead_letter runs, threshold is 5.
	for range 2 {
		r := baseRun(job, newID())
		r.Status = domain.StatusExecuting
		require.NoError(t, q.CreateRun(ctx,
			r))
		require.NoError(t, q.UpdateRunStatus(ctx, r.
			ID, domain.StatusExecuting,

			domain.StatusDeadLetter,

			nil))

	}

	depths, err := q.ListDLQDepthByJob(ctx)
	require.NoError(t, err)
	require.Len(t, depths,
		0,
	)

}

// ListQueueDepthByJob.

func TestRuns_ListQueueDepthByJob_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-queue-depth"
	job := baseJob(newID(), projectID)
	threshold := 2
	job.QueueDepthAlertThreshold = &threshold
	require.NoError(t, q.CreateJob(ctx,
		job))

	for range 3 {
		r := baseRun(job, newID())
		r.Status = domain.StatusQueued
		require.NoError(t, q.CreateRun(ctx,
			r))

	}

	depths, err := q.ListQueueDepthByJob(ctx)
	require.NoError(t, err)
	require.Len(t, depths,
		1,
	)
	require.EqualValues(t, 3, depths[0].QueuedCount)

}

func TestRuns_ListQueueDepthByJob_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	depths, err := q.ListQueueDepthByJob(ctx)
	require.NoError(t, err)
	require.Len(t, depths,
		0,
	)

}

func TestRuns_ListQueueDepthByJob_ExcludesExecuting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-queue-depth-exclude"
	job := baseJob(newID(), projectID)
	threshold := 1
	job.QueueDepthAlertThreshold = &threshold
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Only executing runs, not queued.
	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r))

	depths, err := q.ListQueueDepthByJob(ctx)
	require.NoError(t, err)
	require.Len(t, depths,
		0,
	)

}

// GetRunsByIDs.

func TestRuns_GetRunsByIDs_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-runs-by-ids")
	r1 := mustCreateRun(t, ctx, q, job)
	r2 := mustCreateRun(t, ctx, q, job)

	result, err := q.GetRunsByIDs(ctx, []string{r1.ID, r2.ID})
	require.NoError(t, err)
	require.Len(t, result,
		2,
	)
	require.False(t, result[r1.ID] ==
		nil || result[r2.ID] == nil,
	)

}

func TestRuns_GetRunsByIDs_ReadsSplitRunState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-runs-by-ids-state")
	run := mustCreateRun(t, ctx, q, job)
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status = 'executing', priority = 1 WHERE id = $1`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"force ledger state: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_run_state
		 SET status = 'queued', priority = 77, scheduled_at = NULL, updated_at = NOW()
		 WHERE run_id = $1`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"force mutable state: %v", err)
	}

	result, err := q.GetRunsByIDs(ctx, []string{run.ID})
	require.NoError(t, err)

	got := result[run.ID]
	require.NotNil(t, got)
	require.Equal(t, domain.
		StatusQueued,
		got.Status,
	)
	require.EqualValues(t, 77, got.
		Priority,
	)

}

func TestRuns_GetRunsByIDs_SomeMissing(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-runs-by-ids-missing")
	r1 := mustCreateRun(t, ctx, q, job)
	missingID := newID()

	result, err := q.GetRunsByIDs(ctx, []string{r1.ID, missingID})
	require.NoError(t, err)
	require.Len(t, result,
		1,
	)
	require.NotNil(t, result[r1.ID])
	require.Nil(t, result[missingID])

}

func TestRuns_GetRunsByIDs_EmptySlice(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	result, err := q.GetRunsByIDs(ctx, []string{})
	require.NoError(t, err)
	require.Nil(t, result)

}

// GetRunErrorClass.

func TestRuns_GetRunErrorClass_WithErrorClass(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-error-class")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.StatusExecuting,

		domain.
			StatusFailed, map[string]any{"error": "something broke",
			"error_class": domain.
				ErrorClassRateLimited,

			"finished_at": time.Now().UTC()}))

	ec, err := q.GetRunErrorClass(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		ErrorClassRateLimited,

		ec)

}

func TestRuns_GetRunErrorClass_WithoutErrorClass(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-error-class-empty")
	run := mustCreateRun(t, ctx, q, job)

	ec, err := q.GetRunErrorClass(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "", ec)

}

func TestRuns_GetRunErrorClass_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetRunErrorClass(ctx, newID())
	require.Error(t, err)

}

// CancelJobRunsByWorkflowRun.

func TestRuns_CancelJobRunsByWorkflowRun_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-cancel-wf-runs"

	// Create workflow, step, run, step run linked to a job run.
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID: new(stepJob.ID),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	// Create a job run and link it via workflow_step_run.
	jobRun := baseRun(stepJob, newID())
	jobRun.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		jobRun))

	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		JobRunID: new(jobRun.ID),
	})

	now := time.Now().UTC()
	count, err := q.CancelJobRunsByWorkflowRun(ctx, wfRun.ID, now, "workflow canceled")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	got, err := q.GetRun(ctx, jobRun.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusCanceled,
		got.
			Status)

	assertBulkCanceledViaTerminalState(t, ctx, jobRun.ID, domain.StatusExecuting, "workflow canceled")
}

func TestRuns_CancelJobRunsByWorkflowRun_PgQueActiveClaimDoesNotTouchActiveCounter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	fixture := seedPgQueClaimedWorkflowStepRun(t, ctx, q, "cancel")
	count, err := q.CancelJobRunsByWorkflowRun(ctx, fixture.workflowRunID, time.Now().UTC(), "workflow canceled")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	got, err := q.GetRun(ctx, fixture.runID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusCanceled,
		got.
			Status)

	assertActiveCountTimestampUnchanged(t, ctx, fixture.jobID, fixture.counterUpdatedAt, "workflow cancel")
	assertBulkCanceledViaTerminalState(t, ctx, fixture.runID, domain.StatusQueued, "workflow canceled")
	assertRunLifecycleTransition(t, ctx, fixture.runID, domain.StatusExecuting, domain.StatusCanceled)
}

func TestRuns_CancelJobRunsByWorkflowRun_SkipsAlreadyTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-cancel-wf-terminal"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID: new(stepJob.ID),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	// Create already-completed job run.
	jobRun := baseRun(stepJob, newID())
	jobRun.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		jobRun))
	require.NoError(t, q.UpdateRunStatus(ctx, jobRun.
		ID, domain.StatusExecuting,

		domain.
			StatusCompleted, map[string]any{"finished_at": time.
			Now().UTC()}))

	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		JobRunID: new(jobRun.ID),
	})

	count, err := q.CancelJobRunsByWorkflowRun(ctx, wfRun.ID, time.Now().UTC(), "cancel")
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func TestRuns_CancelJobRunsByWorkflowRun_UnrelatedUntouched(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-cancel-wf-unrelated"
	job := mustCreateJob(t, ctx, q, projectID)

	// Create an unrelated executing run.
	unrelated := baseRun(job, newID())
	unrelated.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		unrelated,
	))

	// Cancel by a nonexistent workflow run ID.
	count, err := q.CancelJobRunsByWorkflowRun(ctx, newID(), time.Now().UTC(), "cancel")
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

	got, err := q.GetRun(ctx, unrelated.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusExecuting,
		got.
			Status)

}

// MarkJobRunsPausedByWorkflowRun.

func TestRuns_MarkJobRunsPausedByWorkflowRun_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-pause-wf-runs"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID: new(stepJob.ID),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	jobRun := baseRun(stepJob, newID())
	jobRun.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		jobRun))

	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		JobRunID: new(jobRun.ID),
	})

	count, err := q.MarkJobRunsPausedByWorkflowRun(ctx, wfRun.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	got, err := q.GetRun(ctx, jobRun.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusPaused,
		got.Status,
	)

	var ledgerStatus, stateStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,

		jobRun.
			ID).Scan(&ledgerStatus,

		&stateStatus))
	require.Equal(t, domain.
		StatusExecuting,
		ledgerStatus,
	)
	require.Equal(t, domain.
		StatusPaused,
		stateStatus,
	)

}

func TestRuns_MarkJobRunsPausedByWorkflowRun_PgQueActiveClaimDoesNotTouchActiveCounter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	fixture := seedPgQueClaimedWorkflowStepRun(t, ctx, q, "pause")
	count, err := q.MarkJobRunsPausedByWorkflowRun(ctx, fixture.workflowRunID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	got, err := q.GetRun(ctx, fixture.runID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusPaused,
		got.Status,
	)

	assertActiveCountTimestampUnchanged(t, ctx, fixture.jobID, fixture.counterUpdatedAt, "workflow pause")

	var activeClaims int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = $1`,

		fixture.runID).Scan(&activeClaims))
	require.EqualValues(t, 1, activeClaims)

	deleted, err := q.DeleteInactiveActiveClaims(ctx, 100)
	require.NoError(t, err)
	require.GreaterOrEqual(
		t,
		deleted,
		int64(1))
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = $1`,

		fixture.runID).Scan(&activeClaims))
	require.EqualValues(t, 0, activeClaims)

}

func TestRuns_MarkJobRunsPausedByWorkflowRun_SkipsNonExecuting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-pause-wf-skip"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID: new(stepJob.ID),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	// Queued run should not be paused.
	jobRun := baseRun(stepJob, newID())
	jobRun.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		jobRun))

	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		JobRunID: new(jobRun.ID),
	})

	count, err := q.MarkJobRunsPausedByWorkflowRun(ctx, wfRun.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func TestRuns_MarkJobRunsPausedByWorkflowRun_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.MarkJobRunsPausedByWorkflowRun(ctx, newID())
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

type pgQueClaimedWorkflowFixture struct {
	workflowRunID    string
	jobID            string
	runID            string
	counterUpdatedAt time.Time
}

func seedPgQueClaimedWorkflowStepRun(
	t *testing.T,
	ctx context.Context,
	q *store.Queries,
	suffix string,
) pgQueClaimedWorkflowFixture {
	t.Helper()

	projectID := "project-pgque-workflow-" + suffix
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{ProjectID: new(projectID)})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: new(stepJob.ID)})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})

	jobRun := baseRun(stepJob, newID())
	jobRun.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		jobRun))

	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{JobRunID: new(jobRun.ID)})
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE run_id = $1`, jobRun.ID); err != nil {
		require.Failf(t, "test failure",

			"mark limited workflow run: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
		SELECT run_id, ready_generation, attempt, NOW()
		FROM job_run_state
		WHERE run_id = $1`,
		jobRun.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert active claim: %v", err)
	}
	counterUpdatedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 0, $2)
		ON CONFLICT (job_id, concurrency_key)
		DO UPDATE SET count = 0, updated_at = EXCLUDED.updated_at`,
		stepJob.ID, counterUpdatedAt,
	); err != nil {
		require.Failf(t, "test failure",

			"seed active count row: %v", err)
	}

	return pgQueClaimedWorkflowFixture{
		workflowRunID:    wfRun.ID,
		jobID:            stepJob.ID,
		runID:            jobRun.ID,
		counterUpdatedAt: counterUpdatedAt,
	}
}

func assertActiveCountTimestampUnchanged(
	t *testing.T,
	ctx context.Context,
	jobID string,
	want time.Time,
	action string,
) {
	t.Helper()

	var got time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT updated_at
		FROM job_active_counts
		WHERE job_id = $1 AND concurrency_key = ''`,

		jobID).Scan(&got))
	require.True(t, got.Equal(want))

}

func assertRunLifecycleTransition(
	t *testing.T,
	ctx context.Context,
	runID string,
	from domain.RunStatus,
	to domain.RunStatus,
) {
	t.Helper()

	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_lifecycle_events
		WHERE run_id = $1
		  AND from_status = $2
		  AND to_status = $3`,

		runID, from, to).Scan(&count))
	require.EqualValues(t, 1, count)

}

// RequeuePausedJobRuns.

func TestRuns_RequeuePausedJobRuns_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-requeue-paused"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID: new(stepJob.ID),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	// Create executing run, pause it, then requeue.
	jobRun := baseRun(stepJob, newID())
	jobRun.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		jobRun))

	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		JobRunID: new(jobRun.ID),
	})

	_, err := q.MarkJobRunsPausedByWorkflowRun(ctx, wfRun.ID)
	require.NoError(t, err)

	var beforeGeneration int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT ready_generation FROM job_run_state WHERE run_id = $1`,

		jobRun.ID).Scan(&beforeGeneration))

	count, err := q.RequeuePausedJobRuns(ctx, wfRun.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	got, err := q.GetRun(ctx, jobRun.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		got.Status,
	)

	var ledgerStatus, stateStatus domain.RunStatus
	var afterGeneration int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status, s.ready_generation
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,

		jobRun.ID,
	).Scan(&ledgerStatus, &stateStatus, &afterGeneration))
	require.Equal(t, domain.
		StatusExecuting,
		ledgerStatus,
	)
	require.Equal(t, domain.
		StatusQueued,
		stateStatus,
	)
	require.Equal(t, beforeGeneration+
		1, afterGeneration,
	)

}

func TestRuns_RequeuePausedJobRuns_SkipsNonPaused(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.RequeuePausedJobRuns(ctx, newID())
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

// ActivateDueRuns.

func TestRuns_ActivateDueRuns_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-activate-due")

	// Create a delayed run scheduled in the past.
	r := baseRun(job, newID())
	past := time.Now().UTC().Add(-10 * time.Minute)
	r.ScheduledAt = &past
	r.Status = domain.StatusDelayed
	require.NoError(t, q.CreateRun(ctx,
		r))

	count, err := q.ActivateDueRuns(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	got, err := q.GetRun(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		got.Status,
	)

	var ledgerStatus, stateStatus, readStatus domain.RunStatus
	var readyEvents, lifecycleEvents int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status, rs.status,
		       (SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = jr.id AND reason = 'delayed_due'),
		       (SELECT COUNT(*) FROM job_run_lifecycle_events WHERE run_id = jr.id AND from_status = 'delayed' AND to_status = 'queued')
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		JOIN job_run_read_state rs ON rs.run_id = jr.id
		WHERE jr.id = $1`,

		r.ID).Scan(&ledgerStatus, &stateStatus, &readStatus, &readyEvents,
		&lifecycleEvents))
	require.Equal(t, domain.
		StatusDelayed,
		ledgerStatus,
	)
	require.Equal(t, domain.
		StatusDelayed,
		stateStatus,
	)
	require.Equal(t, domain.
		StatusQueued,
		readStatus,
	)
	require.EqualValues(t, 1, readyEvents)
	require.EqualValues(t, 1, lifecycleEvents)

	count, err = q.ActivateDueRuns(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

	var duplicateReadyEvents int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_ready_events
		WHERE run_id = $1 AND reason = 'delayed_due'`,

		r.ID).Scan(&duplicateReadyEvents))
	require.EqualValues(t, 1, duplicateReadyEvents)

}

func TestRuns_ActivateDueRuns_KeepsFuture(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-activate-future")

	future := time.Now().UTC().Add(1 * time.Hour)
	r := baseRun(job, newID())
	r.ScheduledAt = &future
	r.Status = domain.StatusDelayed
	require.NoError(t, q.CreateRun(ctx,
		r))

	count, err := q.ActivateDueRuns(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func TestRuns_ActivateDueRuns_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.ActivateDueRuns(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

// BulkCancelRuns.

func TestRuns_BulkCancelRuns_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-bulk-cancel")
	r1 := baseRun(job, newID())
	r1.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r1))

	r2 := baseRun(job, newID())
	r2.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		r2))

	now := time.Now().UTC()
	results, err := q.BulkCancelRuns(ctx, []string{r1.ID, r2.ID}, now, "bulk cancel")
	require.NoError(t, err)
	require.Len(t, results,

		2)

	assertBulkCanceledViaTerminalState(t, ctx, r1.ID, domain.StatusExecuting, "bulk cancel")
	assertBulkCanceledViaTerminalState(t, ctx, r2.ID, domain.StatusQueued, "bulk cancel")
}

func TestRuns_BulkCancelRuns_ReleasesActiveCounterWithoutMutatingHotState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-bulk-cancel-counter")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET job_max_concurrency = 1
		WHERE run_id = $1`, run.ID); err != nil {
		require.Failf(t, "test failure",

			"mark constrained state: %v", err)
	}

	var before int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COALESCE(SUM(count), 0)
		FROM job_active_counts
		WHERE job_id = $1`,

		job.ID,
	).Scan(&before))
	require.EqualValues(t, 1, before)

	results, err := q.BulkCancelRuns(ctx, []string{run.ID}, time.Now().UTC(), "bulk cancel")
	require.NoError(t, err)
	require.Len(t, results,

		1)

	var after int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COALESCE(SUM(count), 0)
		FROM job_active_counts
		WHERE job_id = $1`,

		job.ID,
	).Scan(&after))
	require.EqualValues(t, 0, after)

	assertBulkCanceledViaTerminalState(t, ctx, run.ID, domain.StatusExecuting, "bulk cancel")
}

func TestRuns_BulkCancelRuns_DoesNotRewriteZeroActiveCounter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-bulk-cancel-zero-counter")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET job_max_concurrency = 1
		WHERE run_id = $1`, run.ID); err != nil {
		require.Failf(t, "test failure",

			"mark constrained state: %v", err)
	}

	counterUpdatedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 0, $2)
		ON CONFLICT (job_id, concurrency_key)
		DO UPDATE SET count = 0, updated_at = EXCLUDED.updated_at`,
		job.ID, counterUpdatedAt,
	); err != nil {
		require.Failf(t, "test failure",

			"seed zero active count: %v", err)
	}

	results, err := q.BulkCancelRuns(ctx, []string{run.ID}, time.Now().UTC(), "bulk cancel")
	require.NoError(t, err)
	require.Len(t, results,

		1)

	var count int
	var updatedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT count, updated_at
		FROM job_active_counts
		WHERE job_id = $1
		  AND concurrency_key = ''`,

		job.ID).Scan(
		&count, &updatedAt))
	require.EqualValues(t, 0, count)
	require.True(t, updatedAt.
		Equal(counterUpdatedAt))

	assertBulkCanceledViaTerminalState(t, ctx, run.ID, domain.StatusExecuting, "bulk cancel")
}

func TestRuns_BulkCancelRuns_SkipsTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-bulk-cancel-terminal")
	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r))
	require.NoError(t, q.UpdateRunStatus(ctx, r.
		ID, domain.StatusExecuting,

		domain.StatusCompleted,

		map[string]any{"finished_at": time.Now().UTC()}))

	results, err := q.BulkCancelRuns(ctx, []string{r.ID}, time.Now().UTC(), "cancel")
	require.NoError(t, err)
	require.Len(t, results,

		0)

}

func TestRuns_BulkCancelRuns_EmptyIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	results, err := q.BulkCancelRuns(ctx, []string{}, time.Now().UTC(), "cancel")
	require.NoError(t, err)
	require.Nil(t, results)

}

func assertBulkCanceledViaTerminalState(
	t *testing.T,
	ctx context.Context,
	runID string,
	wantHotStatus domain.RunStatus,
	wantReason string,
) {
	t.Helper()

	var hotStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT status
		FROM job_run_state
		WHERE run_id = $1`,
		runID,
	).Scan(&hotStatus))
	require.Equal(t, wantHotStatus,

		hotStatus,
	)

	var terminalStatus domain.RunStatus
	var terminalFinishedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT status, finished_at
		FROM job_run_terminal_state
		WHERE run_id = $1`,

		runID,
	).Scan(&terminalStatus, &terminalFinishedAt))
	require.Equal(t, domain.
		StatusCanceled,
		terminalStatus,
	)
	require.False(t, terminalFinishedAt.
		IsZero(),
	)

	got, err := mustStore(t).GetRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusCanceled,
		got.
			Status)
	require.False(t, wantReason !=
		"" &&
		got.Error !=
			wantReason)

}

// CancelChildRunsByParentIDs.

func TestRuns_CancelChildRunsByParentIDs_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cancel-children")
	parent := mustCreateRun(t, ctx, q, job)

	child := baseRun(job, newID())
	child.ParentRunID = parent.ID
	child.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		child))

	count, err := q.CancelChildRunsByParentIDs(ctx, []string{parent.ID}, time.Now().UTC(), "parent canceled")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	got, err := q.GetRun(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusCanceled,
		got.
			Status)

	assertBulkCanceledViaTerminalState(t, ctx, child.ID, domain.StatusExecuting, "parent canceled")
}

func TestRuns_CancelChildRunsByParentIDs_NoChildren(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CancelChildRunsByParentIDs(ctx, []string{newID()}, time.Now().UTC(), "cancel")
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func TestRuns_CancelChildRunsByParentIDs_EmptyParentIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CancelChildRunsByParentIDs(ctx, []string{}, time.Now().UTC(), "cancel")
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

// BulkReplayDeadLetterRuns.

func TestRuns_BulkReplayDeadLetterRuns_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-bulk-replay")

	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r))
	require.NoError(t, q.UpdateRunStatus(ctx, r.
		ID, domain.StatusExecuting,

		domain.StatusDeadLetter,

		nil))

	replayed, err := q.BulkReplayDeadLetterRuns(ctx, []string{r.ID}, "", 0)
	require.NoError(t, err)
	require.Len(t, replayed,

		1)
	require.Equal(t, domain.
		StatusQueued,
		replayed[0].Status)

}

func TestRuns_BulkReplayDeadLetterRuns_ByProjectID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-bulk-replay-project"
	job := mustCreateJob(t, ctx, q, projectID)

	for range 3 {
		r := baseRun(job, newID())
		r.Status = domain.StatusExecuting
		require.NoError(t, q.CreateRun(ctx,
			r))
		require.NoError(t, q.UpdateRunStatus(ctx, r.
			ID, domain.StatusExecuting,

			domain.StatusDeadLetter,

			nil))

	}

	replayed, err := q.BulkReplayDeadLetterRuns(ctx, nil, projectID, 100)
	require.NoError(t, err)
	require.Len(t, replayed,

		3)

}

func TestRuns_BulkReplayDeadLetterRuns_NoneAvailable(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.BulkReplayDeadLetterRuns(ctx, nil, "nonexistent-project", 100)
	require.Error(t, err)

}

// BatchUpdateHeartbeat.

func TestRuns_BatchUpdateHeartbeat_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-batch-heartbeat")
	r1 := mustCreateRun(t, ctx, q, job)
	r2 := mustCreateRun(t, ctx, q, job)
	require.NoError(t, q.BatchUpdateHeartbeat(ctx,
		[]string{r1.ID,
			r2.ID}))

	got1, _ := q.GetRun(ctx, r1.ID)
	got2, _ := q.GetRun(ctx, r2.ID)
	require.NotNil(t, got1.
		HeartbeatAt,
	)
	require.NotNil(t, got2.
		HeartbeatAt,
	)

}

func TestRuns_BatchUpdateHeartbeat_EmptyIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	require.NoError(t, q.BatchUpdateHeartbeat(ctx,
		[]string{}))

}

func TestRuns_BatchUpdateHeartbeat_NonexistentIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	require.NoError(t, q.BatchUpdateHeartbeat(ctx,
		[]string{newID(), newID()}))

	// Should not error even with nonexistent IDs.

}

func BenchmarkHeartbeatWritePath(b *testing.B) {
	ctx := context.Background()
	if testDB == nil || testDB.Pool == nil {
		b.Fatal("testDB is not initialized")
	}
	q := store.New(testDB.Pool)
	if err := testDB.CleanTables(ctx); err != nil {
		b.Fatalf("CleanTables() error = %v", err)
	}

	job := baseJob(newID(), "project-heartbeat-bench")
	if err := q.CreateJob(ctx, job); err != nil {
		b.Fatalf("CreateJob() error = %v", err)
	}

	const runCount = 1000
	ids := make([]string, 0, runCount)
	for range runCount {
		run := baseRun(job, newID())
		run.Status = domain.StatusExecuting
		now := time.Now().UTC()
		run.StartedAt = &now
		run.HeartbeatAt = &now
		if err := q.CreateRun(ctx, run); err != nil {
			b.Fatalf("CreateRun() error = %v", err)
		}
		ids = append(ids, run.ID)
	}
	if err := q.BatchUpsertHeartbeatSideTable(ctx, ids); err != nil {
		b.Fatalf("seed side-table heartbeats: %v", err)
	}

	b.Run("job_runs_heartbeat_at", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if err := q.BatchUpdateHeartbeat(ctx, ids); err != nil {
				b.Fatalf("BatchUpdateHeartbeat() error = %v", err)
			}
		}
	})

	b.Run("side_table_upsert", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if err := q.BatchUpsertHeartbeatSideTable(ctx, ids); err != nil {
				b.Fatalf("BatchUpsertHeartbeatSideTable() error = %v", err)
			}
		}
	})
}

// ResetRunIdempotencyKey.

func TestRuns_ResetRunIdempotencyKey_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-reset-idemp")
	r := baseRun(job, newID())
	r.IdempotencyKey = "unique-key-123"
	require.NoError(t, q.CreateRun(ctx,
		r))
	require.NoError(t, q.ResetRunIdempotencyKey(
		ctx, r.ID))

	got, err := q.GetRun(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, "", got.
		IdempotencyKey,
	)

}

func TestRuns_ResetRunIdempotencyKey_AlreadyEmpty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-reset-idemp-empty")
	r := mustCreateRun(t, ctx, q, job)
	require.NoError(t, q.ResetRunIdempotencyKey(
		ctx, r.ID))

	// Should be a no-op when idempotency_key is already empty.

}

func TestRuns_ResetRunIdempotencyKey_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.ResetRunIdempotencyKey(ctx, newID())
	require.Error(t, err)

}

// RescheduleRun.

func TestRuns_RescheduleRun_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-reschedule")
	r := baseRun(job, newID())
	r.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		r))

	newSchedule := time.Now().UTC().Add(2 * time.Hour)
	require.NoError(t, q.RescheduleRun(ctx, r.ID,
		newSchedule, nil,
	))

	got, err := q.GetRun(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusDelayed,
		got.Status,
	)
	require.False(t, got.ScheduledAt ==
		nil || got.
		ScheduledAt.Before(time.Now().UTC()))

	var ledgerStatus domain.RunStatus
	var ledgerScheduledAt *time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT status, scheduled_at
		FROM job_runs
		WHERE id = $1
	`,
		r.ID).Scan(&ledgerStatus,
		&ledgerScheduledAt))
	require.Equal(t, domain.
		StatusQueued,
		ledgerStatus,
	)
	require.Nil(t, ledgerScheduledAt)

}

func TestRuns_RescheduleRun_SameDelayedScheduleDoesNotRewriteState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-reschedule-noop")
	scheduledAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Microsecond)
	r := baseRun(job, newID())
	r.Status = domain.StatusDelayed
	r.ScheduledAt = &scheduledAt
	require.NoError(t, q.CreateRun(ctx,
		r))

	var beforeUpdatedAt time.Time
	var beforeReadyGeneration int64
	var beforeCacheVersions int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT s.updated_at, s.ready_generation,
		       (SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = s.run_id)
		FROM job_run_state s
		WHERE s.run_id = $1
	`,

		r.ID).Scan(&beforeUpdatedAt, &beforeReadyGeneration, &beforeCacheVersions))

	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.RescheduleRun(ctx, r.ID,
		scheduledAt, nil,
	))

	var afterUpdatedAt time.Time
	var afterReadyGeneration int64
	var afterCacheVersions int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT s.updated_at, s.ready_generation,
		       (SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = s.run_id)
		FROM job_run_state s
		WHERE s.run_id = $1
	`,

		r.ID).Scan(&afterUpdatedAt, &afterReadyGeneration,
		&afterCacheVersions,
	))
	require.True(t, afterUpdatedAt.
		Equal(beforeUpdatedAt))
	require.Equal(t, beforeReadyGeneration,

		afterReadyGeneration,
	)
	require.Equal(t, beforeCacheVersions,

		afterCacheVersions,
	)

}

func TestRuns_RescheduleRun_SamePayloadDoesNotRewriteLedger(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-reschedule-payload-noop")
	scheduledAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Microsecond)
	payload := json.RawMessage(`{"kind":"same","value":1}`)
	r := baseRun(job, newID())
	r.Status = domain.StatusDelayed
	r.ScheduledAt = &scheduledAt
	r.Payload = payload
	require.NoError(t, q.CreateRun(ctx,
		r))

	var beforeLedgerXmin string
	var beforeStateXmin string
	var beforeCacheVersions int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT
			jr.xmin::text,
			s.xmin::text,
			(SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = jr.id)
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1
	`,

		r.
			ID).Scan(&beforeLedgerXmin, &beforeStateXmin, &beforeCacheVersions),
	)
	require.NoError(t, q.RescheduleRun(ctx, r.ID,
		scheduledAt, payload,
	))

	var afterLedgerXmin string
	var afterStateXmin string
	var afterCacheVersions int
	var gotPayload json.RawMessage
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT
			jr.xmin::text,
			s.xmin::text,
			(SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = jr.id),
			jr.payload
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1
	`,

		r.ID).Scan(&afterLedgerXmin,
		&afterStateXmin,
		&afterCacheVersions,

		&gotPayload))
	require.Equal(t, beforeLedgerXmin,

		afterLedgerXmin,
	)
	require.Equal(t, beforeStateXmin,

		afterStateXmin,
	)
	require.Equal(t, beforeCacheVersions,

		afterCacheVersions,
	)
	require.True(t, jsonEqual(gotPayload,
		payload,
	))

	changedPayload := json.RawMessage(`{"kind":"changed","value":2}`)
	require.NoError(t, q.RescheduleRun(ctx, r.ID,
		scheduledAt, changedPayload,
	))

	var changedLedgerXmin string
	var changedCacheVersions int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT
			jr.xmin::text,
			(SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = jr.id)
		FROM job_runs jr
		WHERE jr.id = $1
	`,

		r.
			ID).Scan(&changedLedgerXmin, &changedCacheVersions))
	require.NotEqual(t, afterLedgerXmin,

		changedLedgerXmin,
	)
	require.Equal(t, afterCacheVersions+
		1, changedCacheVersions,
	)

}

func TestRuns_UpdateRunStatus_SameLedgerPayloadDoesNotRewriteJobRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-status-ledger-payload-noop")
	payload := json.RawMessage(`{"kind":"same","value":1}`)
	unchanged := baseRun(job, newID())
	unchanged.Status = domain.StatusExecuting
	unchanged.Payload = payload
	require.NoError(t, q.CreateRun(ctx,
		unchanged,
	))

	var beforeXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM job_runs
		WHERE id = $1`,
		unchanged.
			ID).Scan(&beforeXmin),
	)
	require.NoError(t, q.UpdateRunStatus(ctx, unchanged.
		ID, domain.
		StatusExecuting,

		domain.
			StatusCompleted, map[string]any{"payload": payload,
			"finished_at": time.Now().UTC()}))

	var afterXmin string
	var terminalStatus domain.RunStatus
	var gotPayload json.RawMessage
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.xmin::text, rs.status, jr.payload
		FROM job_runs jr
		JOIN job_run_read_state rs ON rs.run_id = jr.id
		WHERE jr.id = $1`,

		unchanged.
			ID).Scan(&afterXmin, &terminalStatus, &gotPayload))
	require.Equal(t, beforeXmin,

		afterXmin,
	)
	require.Equal(t, domain.
		StatusCompleted,
		terminalStatus,
	)
	require.True(t, jsonEqual(gotPayload,
		payload,
	))

	changed := baseRun(job, newID())
	changed.Status = domain.StatusExecuting
	changed.Payload = payload
	require.NoError(t, q.CreateRun(ctx,
		changed),
	)
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM job_runs
		WHERE id = $1`,
		changed.
			ID).Scan(&beforeXmin))

	changedPayload := json.RawMessage(`{"kind":"changed","value":2}`)
	require.NoError(t, q.UpdateRunStatus(ctx, changed.
		ID, domain.
		StatusExecuting,
		domain.
			StatusCompleted, map[string]any{"payload": changedPayload,
			"finished_at": time.
				Now().UTC()}))
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, payload
		FROM job_runs
		WHERE id = $1`,

		changed.ID).Scan(&afterXmin,
		&gotPayload))
	require.NotEqual(t, beforeXmin,

		afterXmin,
	)
	require.True(t, jsonEqual(gotPayload,
		changedPayload,
	))

}

func TestRuns_RescheduleRun_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.RescheduleRun(ctx, newID(), time.Now().UTC().Add(time.Hour), nil)
	require.Error(t, err)

}

func TestRuns_RescheduleRun_CannotRescheduleExecuting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-reschedule-exec")
	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r))

	err := q.RescheduleRun(ctx, r.ID, time.Now().UTC().Add(time.Hour), nil)
	require.Error(t, err)

}

// BulkCancelByFilter.

func TestRuns_BulkCancelByFilter_ByJobID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-bulk-cancel-filter"
	job := mustCreateJob(t, ctx, q, projectID)

	r1 := baseRun(job, newID())
	r1.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		r1))

	r2 := baseRun(job, newID())
	r2.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		r2))

	ids, err := q.BulkCancelByFilter(ctx, projectID, store.BulkCancelFilter{JobID: job.ID}, time.Now().UTC(), "filter cancel")
	require.NoError(t, err)
	require.Len(t, ids, 2)

	assertBulkCanceledViaTerminalState(t, ctx, r1.ID, domain.StatusQueued, "filter cancel")
	assertBulkCanceledViaTerminalState(t, ctx, r2.ID, domain.StatusQueued, "filter cancel")
}

func TestRuns_BulkCancelByFilter_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	ids, err := q.BulkCancelByFilter(ctx, "nonexistent-project", store.BulkCancelFilter{}, time.Now().UTC(), "cancel")
	require.NoError(t, err)
	require.Len(t, ids, 0)

}

func TestRuns_BulkCancelByFilter_ExcludesExecuting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-bulk-cancel-filter-exec"
	job := mustCreateJob(t, ctx, q, projectID)

	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r))

	ids, err := q.BulkCancelByFilter(ctx, projectID, store.BulkCancelFilter{}, time.Now().UTC(), "cancel")
	require.NoError(t, err)
	require.Len(t, ids, 0)

}

// CountActiveRunsForJob.

func TestRuns_CountActiveRunsForJob_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-count-active")

	r1 := baseRun(job, newID())
	r1.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		r1))

	r2 := baseRun(job, newID())
	r2.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r2))

	r3 := baseRun(job, newID())
	r3.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r3))
	require.NoError(t, q.UpdateRunStatus(ctx, r3.
		ID, domain.StatusExecuting,

		domain.
			StatusCompleted,
		map[string]any{"finished_at": time.
			Now().UTC()}))

	// Transition r3 to completed -- should not be counted.

	count, err := q.CountActiveRunsForJob(ctx, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

}

func TestRuns_CountActiveRunsForJob_Zero(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CountActiveRunsForJob(ctx, newID())
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func TestRuns_CountActiveRunsForJob_CrossJobIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job1 := mustCreateJob(t, ctx, q, "project-count-active-iso")
	job2 := mustCreateJob(t, ctx, q, "project-count-active-iso")

	r1 := baseRun(job1, newID())
	r1.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		r1))

	r2 := baseRun(job2, newID())
	r2.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		r2))

	count, err := q.CountActiveRunsForJob(ctx, job1.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

}

// CancelActiveRunsForJob.

func TestRuns_CancelActiveRunsForJob_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cancel-active")

	r1 := baseRun(job, newID())
	r1.Status = domain.StatusQueued
	r1.WorkflowStepRunID = "step-run-cancel-active"
	r1.ExecutionMode = domain.ExecutionModeWorker
	require.NoError(t, q.CreateRun(ctx,
		r1))

	r2 := baseRun(job, newID())
	r2.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r2))

	canceled, err := q.CancelActiveRunsForJob(ctx, job.ID, "cron overlap")
	require.NoError(t, err)
	require.Len(t, canceled,

		2)

	byID := make(map[string]store.CanceledRun, len(canceled))
	for _, cr := range canceled {
		byID[cr.ID] = cr
	}
	if got := byID[r1.ID]; got.WorkflowStepRunID != "step-run-cancel-active" || got.JobID != job.ID || got.ProjectID != job.ProjectID || got.ExecutionMode != domain.ExecutionModeWorker {
		require.Failf(t, "test failure",

			"canceled run metadata = %+v", got)
	}
	assertBulkCanceledViaTerminalState(t, ctx, r1.ID, domain.StatusQueued, "cron overlap")
	assertBulkCanceledViaTerminalState(t, ctx, r2.ID, domain.StatusExecuting, "cron overlap")
}

func TestRuns_CancelActiveRunsForJobExcept_PreservesReplacementRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cancel-active-except")

	oldRun := baseRun(job, newID())
	oldRun.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		oldRun))

	replacement := baseRun(job, newID())
	replacement.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		replacement,
	))

	canceled, err := q.CancelActiveRunsForJobExcept(ctx, job.ID, replacement.ID, "cron overlap")
	require.NoError(t, err)
	require.False(t, len(canceled) !=
		1 || canceled[0].ID != oldRun.
		ID)

	gotReplacement, err := q.GetRun(ctx, replacement.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		gotReplacement.
			Status)

}

func TestRuns_CancelActiveRunsForJob_NoActiveRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	canceled, err := q.CancelActiveRunsForJob(ctx, newID(), "cancel")
	require.NoError(t, err)
	require.Len(t, canceled,

		0)

}

func TestRuns_CancelActiveRunsForJob_SkipsTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cancel-active-terminal")
	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		r))
	require.NoError(t, q.UpdateRunStatus(ctx, r.
		ID, domain.StatusExecuting,

		domain.StatusCompleted,

		map[string]any{"finished_at": time.Now().UTC()}))

	canceled, err := q.CancelActiveRunsForJob(ctx, job.ID, "cancel")
	require.NoError(t, err)
	require.Len(t, canceled,

		0)

}

// CountRunIterations.

func TestRuns_CountRunIterations_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-count-iterations")
	run := mustCreateRun(t, ctx, q, job)

	for i := range 3 {
		iter := &domain.RunIteration{RunID: run.ID, Iteration: i + 1, Description: "step"}
		require.NoError(t, q.CreateRunIteration(ctx,
			iter))

	}

	count, err := q.CountRunIterations(ctx, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

}

func TestRuns_CountRunIterations_NoIterations(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-count-iterations-empty")
	run := mustCreateRun(t, ctx, q, job)

	count, err := q.CountRunIterations(ctx, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func TestRuns_CountRunIterations_NonexistentRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CountRunIterations(ctx, newID())
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

// CreateRunIteration.

func TestRuns_CreateRunIteration_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-create-iteration")
	run := mustCreateRun(t, ctx, q, job)

	iter := &domain.RunIteration{RunID: run.ID, Iteration: 1, Description: "first iteration"}
	require.NoError(t, q.CreateRunIteration(ctx,
		iter))
	require.NotEqual(t, "",

		iter.ID)
	require.False(t, iter.CreatedAt.
		IsZero())

}

func TestRuns_CreateRunIteration_AutoID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-create-iteration-autoid")
	run := mustCreateRun(t, ctx, q, job)

	iter := &domain.RunIteration{RunID: run.ID, Iteration: 1}
	require.NoError(t, q.CreateRunIteration(ctx,
		iter))
	require.NotEqual(t, "",

		iter.ID)

}

func TestRuns_CreateRunIteration_CustomID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-create-iteration-custom")
	run := mustCreateRun(t, ctx, q, job)

	customID := newID()
	iter := &domain.RunIteration{ID: customID, RunID: run.ID, Iteration: 1}
	require.NoError(t, q.CreateRunIteration(ctx,
		iter))
	require.Equal(t, customID,

		iter.ID,
	)

}

// ListRunState.

func TestRunState_ListRunState_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-run-state")
	run := mustCreateRun(t, ctx, q, job)

	s1 := &domain.RunState{RunID: run.ID, StateKey: "alpha", Value: json.RawMessage(`"hello"`)}
	require.NoError(t, q.UpsertRunState(ctx, s1))

	s2 := &domain.RunState{RunID: run.ID, StateKey: "beta", Value: json.RawMessage(`42`)}
	require.NoError(t, q.UpsertRunState(ctx, s2))

	items, err := q.ListRunState(ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "alpha",

		items[0].StateKey)
	require.Equal(t, "beta",

		items[1].
			StateKey)

	// Ordered by state_key ASC.

}

func TestRunState_UpsertSameValueDoesNotRewrite(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-noop")
	run := mustCreateRun(t, ctx, q, job)

	state := &domain.RunState{RunID: run.ID, StateKey: "cursor", Value: json.RawMessage(`{"page":1}`)}
	require.NoError(t, q.UpsertRunState(ctx, state))

	initialUpdatedAt := state.UpdatedAt
	var beforeXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM run_state
		WHERE run_id = $1 AND state_key = $2`,

		run.ID,
		"cursor").Scan(&beforeXmin),
	)

	sameState := &domain.RunState{RunID: run.ID, StateKey: "cursor", Value: json.RawMessage(`{"page":1}`)}
	require.NoError(t, q.UpsertRunState(ctx, sameState))

	var afterNoopXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM run_state
		WHERE run_id = $1 AND state_key = $2`,

		run.ID,
		"cursor").Scan(&afterNoopXmin),
	)
	require.Equal(t, beforeXmin,

		afterNoopXmin,
	)
	require.True(t, sameState.
		UpdatedAt.
		Equal(initialUpdatedAt))

	changedState := &domain.RunState{RunID: run.ID, StateKey: "cursor", Value: json.RawMessage(`{"page":2}`)}
	require.NoError(t, q.UpsertRunState(ctx, changedState))

	var afterChangedXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM run_state
		WHERE run_id = $1 AND state_key = $2`,

		run.ID,
		"cursor").Scan(&afterChangedXmin))
	require.NotEqual(t, beforeXmin,

		afterChangedXmin,
	)

}

func TestRunState_ListRunState_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-run-state-empty")
	run := mustCreateRun(t, ctx, q, job)

	items, err := q.ListRunState(ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, items, 0)

}

func TestRunState_ListRunState_CrossRunIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-run-state-iso")
	run1 := mustCreateRun(t, ctx, q, job)
	run2 := mustCreateRun(t, ctx, q, job)

	s1 := &domain.RunState{RunID: run1.ID, StateKey: "key1", Value: json.RawMessage(`"v1"`)}
	require.NoError(t, q.UpsertRunState(ctx, s1))

	s2 := &domain.RunState{RunID: run2.ID, StateKey: "key2", Value: json.RawMessage(`"v2"`)}
	require.NoError(t, q.UpsertRunState(ctx, s2))

	items, err := q.ListRunState(ctx, run1.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)

}

// CopyRunState.

func TestRunState_CopyRunState_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-copy-run-state")
	source := mustCreateRun(t, ctx, q, job)
	target := mustCreateRun(t, ctx, q, job)

	s1 := &domain.RunState{RunID: source.ID, StateKey: "key-a", Value: json.RawMessage(`"value-a"`)}
	require.NoError(t, q.UpsertRunState(ctx, s1))

	s2 := &domain.RunState{RunID: source.ID, StateKey: "key-b", Value: json.RawMessage(`"value-b"`)}
	require.NoError(t, q.UpsertRunState(ctx, s2))
	require.NoError(t, q.CopyRunState(ctx, source.
		ID, target.ID))

	items, err := q.ListRunState(ctx, target.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)

}

func TestRunState_CopyRunState_DoesNotOverwriteExisting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-copy-no-overwrite")
	source := mustCreateRun(t, ctx, q, job)
	target := mustCreateRun(t, ctx, q, job)

	// Source has key-a = "from-source".
	s := &domain.RunState{RunID: source.ID, StateKey: "key-a", Value: json.RawMessage(`"from-source"`)}
	require.NoError(t, q.UpsertRunState(ctx, s))

	// Target already has key-a = "original".
	t2 := &domain.RunState{RunID: target.ID, StateKey: "key-a", Value: json.RawMessage(`"original"`)}
	require.NoError(t, q.UpsertRunState(ctx, t2))
	require.NoError(t, q.CopyRunState(ctx, source.
		ID, target.ID))

	got, err := q.GetRunState(ctx, target.ID, "key-a")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.True(t, jsonEqual(got.Value,
		json.RawMessage(`"original"`)))

}

func TestRunState_CopyRunState_EmptySource(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-copy-empty-source")
	source := mustCreateRun(t, ctx, q, job)
	target := mustCreateRun(t, ctx, q, job)
	require.NoError(t, q.CopyRunState(ctx, source.
		ID, target.ID))

	items, err := q.ListRunState(ctx, target.ID)
	require.NoError(t, err)
	require.Len(t, items, 0)

}

// CreateRunResourceSnapshot.

func TestRunResource_CreateRunResourceSnapshot_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-resource-snapshot")
	run := mustCreateRun(t, ctx, q, job)

	snap := &domain.RunResourceSnapshot{
		RunID:          run.ID,
		CPUPercent:     45.5,
		MemoryMB:       256,
		MemoryLimitMB:  512,
		NetworkRxBytes: 1024,
		NetworkTxBytes: 2048,
	}
	require.NoError(t, q.CreateRunResourceSnapshot(ctx, snap))
	require.NotEqual(t, "",

		snap.ID)
	require.False(t, snap.CreatedAt.
		IsZero())

}

func TestRunResource_CreateRunResourceSnapshot_AutoID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-resource-snapshot-autoid")
	run := mustCreateRun(t, ctx, q, job)

	snap := &domain.RunResourceSnapshot{RunID: run.ID, CPUPercent: 10}
	require.NoError(t, q.CreateRunResourceSnapshot(ctx, snap))
	require.NotEqual(t, "",

		snap.ID)

}

func TestRunResource_CreateRunResourceSnapshot_CustomID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-resource-snapshot-custom")
	run := mustCreateRun(t, ctx, q, job)

	customID := newID()
	snap := &domain.RunResourceSnapshot{ID: customID, RunID: run.ID, CPUPercent: 10}
	require.NoError(t, q.CreateRunResourceSnapshot(ctx, snap))
	require.Equal(t, customID,

		snap.ID,
	)

}

// ListRunResourceSnapshots.

func TestRunResource_ListRunResourceSnapshots_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-snapshots")
	run := mustCreateRun(t, ctx, q, job)

	for i := range 3 {
		snap := &domain.RunResourceSnapshot{
			RunID:      run.ID,
			CPUPercent: float64(i * 10),
			MemoryMB:   float64(i * 100),
		}
		require.NoError(t, q.CreateRunResourceSnapshot(ctx, snap))

	}

	snapshots, err := q.ListRunResourceSnapshots(ctx, run.ID, nil, nil, 10)
	require.NoError(t, err)
	require.Len(t, snapshots,

		3)

}

func TestRunResource_ListRunResourceSnapshots_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-snapshots-empty")
	run := mustCreateRun(t, ctx, q, job)

	snapshots, err := q.ListRunResourceSnapshots(ctx, run.ID, nil, nil, 10)
	require.NoError(t, err)
	require.Len(t, snapshots,

		0)

}

func TestRunResource_ListRunResourceSnapshots_TimeFilter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-snapshots-time")
	run := mustCreateRun(t, ctx, q, job)

	for range 3 {
		snap := &domain.RunResourceSnapshot{RunID: run.ID, CPUPercent: 50}
		require.NoError(t, q.CreateRunResourceSnapshot(ctx, snap))

	}

	now := time.Now().UTC()
	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)

	snapshots, err := q.ListRunResourceSnapshots(ctx, run.ID, &from, &to, 10)
	require.NoError(t, err)
	require.Len(t, snapshots,

		3)

	// Filter with a past window that excludes everything.
	pastFrom := now.Add(-3 * time.Hour)
	pastTo := now.Add(-2 * time.Hour)
	snapshots, err = q.ListRunResourceSnapshots(ctx, run.ID, &pastFrom, &pastTo, 10)
	require.NoError(t, err)
	require.Len(t, snapshots,

		0)

}
