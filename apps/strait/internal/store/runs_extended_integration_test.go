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
)

// GetLatestCheckpoint.

func TestRuns_GetLatestCheckpoint_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-latest-checkpoint")
	run := mustCreateRun(t, ctx, q, job)

	cp1 := &domain.RunCheckpoint{RunID: run.ID, Source: "sdk", State: json.RawMessage(`{"step":1}`)}
	if err := q.CreateRunCheckpoint(ctx, cp1); err != nil {
		t.Fatalf("CreateRunCheckpoint(1) error = %v", err)
	}
	cp2 := &domain.RunCheckpoint{RunID: run.ID, Source: "auto", State: json.RawMessage(`{"step":2}`)}
	if err := q.CreateRunCheckpoint(ctx, cp2); err != nil {
		t.Fatalf("CreateRunCheckpoint(2) error = %v", err)
	}

	latest, err := q.GetLatestCheckpoint(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetLatestCheckpoint() error = %v", err)
	}
	if latest == nil {
		t.Fatal("GetLatestCheckpoint() = nil")
	}
	if latest.Sequence != 2 {
		t.Fatalf("sequence = %d, want 2", latest.Sequence)
	}
}

func TestRuns_GetLatestCheckpoint_NoCheckpoints(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-latest-checkpoint-empty")
	run := mustCreateRun(t, ctx, q, job)

	latest, err := q.GetLatestCheckpoint(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetLatestCheckpoint() error = %v", err)
	}
	if latest != nil {
		t.Fatalf("GetLatestCheckpoint() = %+v, want nil", latest)
	}
}

func TestRuns_GetLatestCheckpoint_NonexistentRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	latest, err := q.GetLatestCheckpoint(ctx, newID())
	if err != nil {
		t.Fatalf("GetLatestCheckpoint() error = %v", err)
	}
	if latest != nil {
		t.Fatalf("expected nil for nonexistent run, got %+v", latest)
	}
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
	if err := q.CreateRun(ctx, r1); err != nil {
		t.Fatalf("CreateRun(r1) error = %v", err)
	}
	finishedAt := time.Now().UTC()
	if err := q.UpdateRunStatus(ctx, r1.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": finishedAt,
	}); err != nil {
		t.Fatalf("UpdateRunStatus(r1) error = %v", err)
	}

	runs, err := q.ListFinishedRunsSince(ctx, projectID, cutoff, "", 100)
	if err != nil {
		t.Fatalf("ListFinishedRunsSince() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len = %d, want 1", len(runs))
	}
	if runs[0].ID != r1.ID {
		t.Fatalf("id = %s, want %s", runs[0].ID, r1.ID)
	}
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
	if err := q.CreateRun(ctx, r1); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	pastFinish := time.Now().UTC().Add(-2 * time.Hour)
	if err := q.UpdateRunStatus(ctx, r1.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": pastFinish,
	}); err != nil {
		t.Fatalf("UpdateRunStatus error = %v", err)
	}

	cutoff := time.Now().UTC().Add(-1 * time.Hour)
	runs, err := q.ListFinishedRunsSince(ctx, projectID, cutoff, "", 100)
	if err != nil {
		t.Fatalf("ListFinishedRunsSince() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("len = %d, want 0", len(runs))
	}
}

func TestRuns_ListFinishedRunsSince_ExcludesNonTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-finished-runs-nonterminal"
	job := mustCreateJob(t, ctx, q, projectID)

	r1 := baseRun(job, newID())
	r1.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, r1); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	cutoff := time.Now().UTC().Add(-1 * time.Hour)
	runs, err := q.ListFinishedRunsSince(ctx, projectID, cutoff, "", 100)
	if err != nil {
		t.Fatalf("ListFinishedRunsSince() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("len = %d, want 0 (executing runs excluded)", len(runs))
	}
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
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob error = %v", err)
	}

	// Create dead_letter runs. Need to go through executing -> dead_letter.
	for range 3 {
		r := baseRun(job, newID())
		r.Status = domain.StatusExecuting
		if err := q.CreateRun(ctx, r); err != nil {
			t.Fatalf("CreateRun error = %v", err)
		}
		if err := q.UpdateRunStatus(ctx, r.ID, domain.StatusExecuting, domain.StatusDeadLetter, nil); err != nil {
			t.Fatalf("UpdateRunStatus error = %v", err)
		}
	}

	depths, err := q.ListDLQDepthByJob(ctx)
	if err != nil {
		t.Fatalf("ListDLQDepthByJob() error = %v", err)
	}
	if len(depths) != 1 {
		t.Fatalf("len = %d, want 1", len(depths))
	}
	if depths[0].JobID != job.ID {
		t.Fatalf("job_id = %s, want %s", depths[0].JobID, job.ID)
	}
	if depths[0].DLQCount != 3 {
		t.Fatalf("dlq_count = %d, want 3", depths[0].DLQCount)
	}
}

func TestRuns_ListDLQDepthByJob_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	depths, err := q.ListDLQDepthByJob(ctx)
	if err != nil {
		t.Fatalf("ListDLQDepthByJob() error = %v", err)
	}
	if len(depths) != 0 {
		t.Fatalf("len = %d, want 0", len(depths))
	}
}

func TestRuns_ListDLQDepthByJob_BelowThreshold(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-dlq-below-threshold"
	job := baseJob(newID(), projectID)
	threshold := 5
	job.DLQAlertThreshold = &threshold
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob error = %v", err)
	}

	// Only 2 dead_letter runs, threshold is 5.
	for range 2 {
		r := baseRun(job, newID())
		r.Status = domain.StatusExecuting
		if err := q.CreateRun(ctx, r); err != nil {
			t.Fatalf("CreateRun error = %v", err)
		}
		if err := q.UpdateRunStatus(ctx, r.ID, domain.StatusExecuting, domain.StatusDeadLetter, nil); err != nil {
			t.Fatalf("UpdateRunStatus error = %v", err)
		}
	}

	depths, err := q.ListDLQDepthByJob(ctx)
	if err != nil {
		t.Fatalf("ListDLQDepthByJob() error = %v", err)
	}
	if len(depths) != 0 {
		t.Fatalf("len = %d, want 0 (below threshold)", len(depths))
	}
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
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob error = %v", err)
	}

	for range 3 {
		r := baseRun(job, newID())
		r.Status = domain.StatusQueued
		if err := q.CreateRun(ctx, r); err != nil {
			t.Fatalf("CreateRun error = %v", err)
		}
	}

	depths, err := q.ListQueueDepthByJob(ctx)
	if err != nil {
		t.Fatalf("ListQueueDepthByJob() error = %v", err)
	}
	if len(depths) != 1 {
		t.Fatalf("len = %d, want 1", len(depths))
	}
	if depths[0].QueuedCount != 3 {
		t.Fatalf("queued_count = %d, want 3", depths[0].QueuedCount)
	}
}

func TestRuns_ListQueueDepthByJob_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	depths, err := q.ListQueueDepthByJob(ctx)
	if err != nil {
		t.Fatalf("ListQueueDepthByJob() error = %v", err)
	}
	if len(depths) != 0 {
		t.Fatalf("len = %d, want 0", len(depths))
	}
}

func TestRuns_ListQueueDepthByJob_ExcludesExecuting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-queue-depth-exclude"
	job := baseJob(newID(), projectID)
	threshold := 1
	job.QueueDepthAlertThreshold = &threshold
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob error = %v", err)
	}

	// Only executing runs, not queued.
	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	depths, err := q.ListQueueDepthByJob(ctx)
	if err != nil {
		t.Fatalf("ListQueueDepthByJob() error = %v", err)
	}
	if len(depths) != 0 {
		t.Fatalf("len = %d, want 0 (executing excluded)", len(depths))
	}
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
	if err != nil {
		t.Fatalf("GetRunsByIDs() error = %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[r1.ID] == nil || result[r2.ID] == nil {
		t.Fatal("expected both runs in result")
	}
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
		t.Fatalf("force ledger state: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_run_state
		 SET status = 'queued', priority = 77, scheduled_at = NULL, updated_at = NOW()
		 WHERE run_id = $1`,
		run.ID,
	); err != nil {
		t.Fatalf("force mutable state: %v", err)
	}

	result, err := q.GetRunsByIDs(ctx, []string{run.ID})
	if err != nil {
		t.Fatalf("GetRunsByIDs() error = %v", err)
	}
	got := result[run.ID]
	if got == nil {
		t.Fatal("expected run in result")
	}
	if got.Status != domain.StatusQueued {
		t.Fatalf("status = %q, want queued from job_run_state", got.Status)
	}
	if got.Priority != 77 {
		t.Fatalf("priority = %d, want 77 from job_run_state", got.Priority)
	}
}

func TestRuns_GetRunsByIDs_SomeMissing(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-runs-by-ids-missing")
	r1 := mustCreateRun(t, ctx, q, job)
	missingID := newID()

	result, err := q.GetRunsByIDs(ctx, []string{r1.ID, missingID})
	if err != nil {
		t.Fatalf("GetRunsByIDs() error = %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[r1.ID] == nil {
		t.Fatal("expected r1 in result")
	}
	if result[missingID] != nil {
		t.Fatal("expected missing ID to be absent")
	}
}

func TestRuns_GetRunsByIDs_EmptySlice(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	result, err := q.GetRunsByIDs(ctx, []string{})
	if err != nil {
		t.Fatalf("GetRunsByIDs() error = %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil for empty ids, got %v", result)
	}
}

// GetRunErrorClass.

func TestRuns_GetRunErrorClass_WithErrorClass(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-error-class")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusFailed, map[string]any{
		"error":       "something broke",
		"error_class": domain.ErrorClassRateLimited,
		"finished_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus error = %v", err)
	}

	ec, err := q.GetRunErrorClass(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRunErrorClass() error = %v", err)
	}
	if ec != domain.ErrorClassRateLimited {
		t.Fatalf("error_class = %q, want %q", ec, domain.ErrorClassRateLimited)
	}
}

func TestRuns_GetRunErrorClass_WithoutErrorClass(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-error-class-empty")
	run := mustCreateRun(t, ctx, q, job)

	ec, err := q.GetRunErrorClass(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRunErrorClass() error = %v", err)
	}
	if ec != "" {
		t.Fatalf("error_class = %q, want empty", ec)
	}
}

func TestRuns_GetRunErrorClass_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetRunErrorClass(ctx, newID())
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
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
	if err := q.CreateRun(ctx, jobRun); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		JobRunID: new(jobRun.ID),
	})

	now := time.Now().UTC()
	count, err := q.CancelJobRunsByWorkflowRun(ctx, wfRun.ID, now, "workflow canceled")
	if err != nil {
		t.Fatalf("CancelJobRunsByWorkflowRun() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	got, err := q.GetRun(ctx, jobRun.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusCanceled {
		t.Fatalf("status = %s, want %s", got.Status, domain.StatusCanceled)
	}
	assertBulkCanceledViaTerminalState(t, ctx, jobRun.ID, domain.StatusExecuting, "workflow canceled")
}

func TestRuns_CancelJobRunsByWorkflowRun_PgQueActiveClaimDoesNotTouchActiveCounter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	fixture := seedPgQueClaimedWorkflowStepRun(t, ctx, q, "cancel")
	count, err := q.CancelJobRunsByWorkflowRun(ctx, fixture.workflowRunID, time.Now().UTC(), "workflow canceled")
	if err != nil {
		t.Fatalf("CancelJobRunsByWorkflowRun() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	got, err := q.GetRun(ctx, fixture.runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusCanceled {
		t.Fatalf("status = %s, want canceled", got.Status)
	}
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
	if err := q.CreateRun(ctx, jobRun); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, jobRun.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus error = %v", err)
	}
	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		JobRunID: new(jobRun.ID),
	})

	count, err := q.CancelJobRunsByWorkflowRun(ctx, wfRun.ID, time.Now().UTC(), "cancel")
	if err != nil {
		t.Fatalf("CancelJobRunsByWorkflowRun() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 (already terminal)", count)
	}
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
	if err := q.CreateRun(ctx, unrelated); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	// Cancel by a nonexistent workflow run ID.
	count, err := q.CancelJobRunsByWorkflowRun(ctx, newID(), time.Now().UTC(), "cancel")
	if err != nil {
		t.Fatalf("CancelJobRunsByWorkflowRun() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}

	got, err := q.GetRun(ctx, unrelated.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusExecuting {
		t.Fatalf("unrelated run status = %s, want executing", got.Status)
	}
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
	if err := q.CreateRun(ctx, jobRun); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		JobRunID: new(jobRun.ID),
	})

	count, err := q.MarkJobRunsPausedByWorkflowRun(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("MarkJobRunsPausedByWorkflowRun() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	got, err := q.GetRun(ctx, jobRun.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusPaused {
		t.Fatalf("status = %s, want paused", got.Status)
	}

	var ledgerStatus, stateStatus domain.RunStatus
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT jr.status, s.status
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		jobRun.ID,
	).Scan(&ledgerStatus, &stateStatus); err != nil {
		t.Fatalf("query split pause state: %v", err)
	}
	if ledgerStatus != domain.StatusExecuting {
		t.Fatalf("job_runs status = %s, want immutable executing ledger status", ledgerStatus)
	}
	if stateStatus != domain.StatusPaused {
		t.Fatalf("job_run_state status = %s, want paused", stateStatus)
	}
}

func TestRuns_MarkJobRunsPausedByWorkflowRun_PgQueActiveClaimDoesNotTouchActiveCounter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	fixture := seedPgQueClaimedWorkflowStepRun(t, ctx, q, "pause")
	count, err := q.MarkJobRunsPausedByWorkflowRun(ctx, fixture.workflowRunID)
	if err != nil {
		t.Fatalf("MarkJobRunsPausedByWorkflowRun() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	got, err := q.GetRun(ctx, fixture.runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusPaused {
		t.Fatalf("status = %s, want paused", got.Status)
	}
	assertActiveCountTimestampUnchanged(t, ctx, fixture.jobID, fixture.counterUpdatedAt, "workflow pause")

	var activeClaims int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = $1`, fixture.runID).Scan(&activeClaims); err != nil {
		t.Fatalf("query active claims: %v", err)
	}
	if activeClaims != 1 {
		t.Fatalf("active claims = %d, want retained inactive claim after pause", activeClaims)
	}
	deleted, err := q.DeleteInactiveActiveClaims(ctx, 100)
	if err != nil {
		t.Fatalf("DeleteInactiveActiveClaims() error = %v", err)
	}
	if deleted < 1 {
		t.Fatalf("deleted inactive active claims = %d, want at least target claim", deleted)
	}
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = $1`, fixture.runID).Scan(&activeClaims); err != nil {
		t.Fatalf("query active claims after cleanup: %v", err)
	}
	if activeClaims != 0 {
		t.Fatalf("active claims after cleanup = %d, want 0", activeClaims)
	}
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
	if err := q.CreateRun(ctx, jobRun); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		JobRunID: new(jobRun.ID),
	})

	count, err := q.MarkJobRunsPausedByWorkflowRun(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("MarkJobRunsPausedByWorkflowRun() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 (non-executing skipped)", count)
	}
}

func TestRuns_MarkJobRunsPausedByWorkflowRun_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.MarkJobRunsPausedByWorkflowRun(ctx, newID())
	if err != nil {
		t.Fatalf("MarkJobRunsPausedByWorkflowRun() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
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
	if err := q.CreateRun(ctx, jobRun); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{JobRunID: new(jobRun.ID)})
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE run_id = $1`, jobRun.ID); err != nil {
		t.Fatalf("mark limited workflow run: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
		SELECT run_id, ready_generation, attempt, NOW()
		FROM job_run_state
		WHERE run_id = $1`,
		jobRun.ID,
	); err != nil {
		t.Fatalf("insert active claim: %v", err)
	}
	counterUpdatedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 0, $2)
		ON CONFLICT (job_id, concurrency_key)
		DO UPDATE SET count = 0, updated_at = EXCLUDED.updated_at`,
		stepJob.ID, counterUpdatedAt,
	); err != nil {
		t.Fatalf("seed active count row: %v", err)
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
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT updated_at
		FROM job_active_counts
		WHERE job_id = $1 AND concurrency_key = ''`,
		jobID,
	).Scan(&got); err != nil {
		t.Fatalf("query active count timestamp after %s: %v", action, err)
	}
	if !got.Equal(want) {
		t.Fatalf("active count updated_at changed after %s: got %s want %s", action, got, want)
	}
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
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_lifecycle_events
		WHERE run_id = $1
		  AND from_status = $2
		  AND to_status = $3`,
		runID, from, to,
	).Scan(&count); err != nil {
		t.Fatalf("query lifecycle transition: %v", err)
	}
	if count != 1 {
		t.Fatalf("lifecycle transition %s -> %s rows = %d, want 1", from, to, count)
	}
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
	if err := q.CreateRun(ctx, jobRun); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		JobRunID: new(jobRun.ID),
	})

	_, err := q.MarkJobRunsPausedByWorkflowRun(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("MarkJobRunsPausedByWorkflowRun() error = %v", err)
	}
	var beforeGeneration int64
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT ready_generation FROM job_run_state WHERE run_id = $1`,
		jobRun.ID,
	).Scan(&beforeGeneration); err != nil {
		t.Fatalf("query ready_generation before requeue: %v", err)
	}

	count, err := q.RequeuePausedJobRuns(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("RequeuePausedJobRuns() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	got, err := q.GetRun(ctx, jobRun.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusQueued {
		t.Fatalf("status = %s, want queued", got.Status)
	}

	var ledgerStatus, stateStatus domain.RunStatus
	var afterGeneration int64
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT jr.status, s.status, s.ready_generation
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		jobRun.ID,
	).Scan(&ledgerStatus, &stateStatus, &afterGeneration); err != nil {
		t.Fatalf("query split requeue state: %v", err)
	}
	if ledgerStatus != domain.StatusExecuting {
		t.Fatalf("job_runs status = %s, want immutable executing ledger status", ledgerStatus)
	}
	if stateStatus != domain.StatusQueued {
		t.Fatalf("job_run_state status = %s, want queued", stateStatus)
	}
	if afterGeneration != beforeGeneration+1 {
		t.Fatalf("ready_generation = %d, want %d", afterGeneration, beforeGeneration+1)
	}
}

func TestRuns_RequeuePausedJobRuns_SkipsNonPaused(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.RequeuePausedJobRuns(ctx, newID())
	if err != nil {
		t.Fatalf("RequeuePausedJobRuns() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
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
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	count, err := q.ActivateDueRuns(ctx, 100)
	if err != nil {
		t.Fatalf("ActivateDueRuns() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	got, err := q.GetRun(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusQueued {
		t.Fatalf("status = %s, want queued", got.Status)
	}

	var ledgerStatus, stateStatus, readStatus domain.RunStatus
	var readyEvents, lifecycleEvents int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT jr.status, s.status, rs.status,
		       (SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = jr.id AND reason = 'delayed_due'),
		       (SELECT COUNT(*) FROM job_run_lifecycle_events WHERE run_id = jr.id AND from_status = 'delayed' AND to_status = 'queued')
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		JOIN job_run_read_state rs ON rs.run_id = jr.id
		WHERE jr.id = $1`,
		r.ID,
	).Scan(&ledgerStatus, &stateStatus, &readStatus, &readyEvents, &lifecycleEvents); err != nil {
		t.Fatalf("query split delayed activation state: %v", err)
	}
	if ledgerStatus != domain.StatusDelayed {
		t.Fatalf("job_runs status = %s, want immutable delayed ledger status", ledgerStatus)
	}
	if stateStatus != domain.StatusDelayed {
		t.Fatalf("job_run_state status = %s, want delayed hot state", stateStatus)
	}
	if readStatus != domain.StatusQueued {
		t.Fatalf("job_run_read_state status = %s, want queued readiness overlay", readStatus)
	}
	if readyEvents != 1 {
		t.Fatalf("delayed_due ready events = %d, want 1", readyEvents)
	}
	if lifecycleEvents != 1 {
		t.Fatalf("delayed->queued lifecycle events = %d, want 1", lifecycleEvents)
	}

	count, err = q.ActivateDueRuns(ctx, 100)
	if err != nil {
		t.Fatalf("duplicate ActivateDueRuns() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("duplicate count = %d, want 0", count)
	}
	var duplicateReadyEvents int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_ready_events
		WHERE run_id = $1 AND reason = 'delayed_due'`,
		r.ID,
	).Scan(&duplicateReadyEvents); err != nil {
		t.Fatalf("query duplicate delayed_due ready events: %v", err)
	}
	if duplicateReadyEvents != 1 {
		t.Fatalf("delayed_due ready events after duplicate = %d, want 1", duplicateReadyEvents)
	}
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
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	count, err := q.ActivateDueRuns(ctx, 100)
	if err != nil {
		t.Fatalf("ActivateDueRuns() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 (future)", count)
	}
}

func TestRuns_ActivateDueRuns_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.ActivateDueRuns(ctx, 100)
	if err != nil {
		t.Fatalf("ActivateDueRuns() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

// BulkCancelRuns.

func TestRuns_BulkCancelRuns_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-bulk-cancel")
	r1 := baseRun(job, newID())
	r1.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, r1); err != nil {
		t.Fatalf("CreateRun(r1) error = %v", err)
	}
	r2 := baseRun(job, newID())
	r2.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, r2); err != nil {
		t.Fatalf("CreateRun(r2) error = %v", err)
	}

	now := time.Now().UTC()
	results, err := q.BulkCancelRuns(ctx, []string{r1.ID, r2.ID}, now, "bulk cancel")
	if err != nil {
		t.Fatalf("BulkCancelRuns() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len = %d, want 2", len(results))
	}
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
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET job_max_concurrency = 1
		WHERE run_id = $1`, run.ID); err != nil {
		t.Fatalf("mark constrained state: %v", err)
	}

	var before int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(count), 0)
		FROM job_active_counts
		WHERE job_id = $1`, job.ID).Scan(&before); err != nil {
		t.Fatalf("active count before cancel: %v", err)
	}
	if before != 1 {
		t.Fatalf("active count before cancel = %d, want 1", before)
	}

	results, err := q.BulkCancelRuns(ctx, []string{run.ID}, time.Now().UTC(), "bulk cancel")
	if err != nil {
		t.Fatalf("BulkCancelRuns() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}

	var after int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(count), 0)
		FROM job_active_counts
		WHERE job_id = $1`, job.ID).Scan(&after); err != nil {
		t.Fatalf("active count after cancel: %v", err)
	}
	if after != 0 {
		t.Fatalf("active count after cancel = %d, want 0", after)
	}
	assertBulkCanceledViaTerminalState(t, ctx, run.ID, domain.StatusExecuting, "bulk cancel")
}

func TestRuns_BulkCancelRuns_DoesNotRewriteZeroActiveCounter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-bulk-cancel-zero-counter")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET job_max_concurrency = 1
		WHERE run_id = $1`, run.ID); err != nil {
		t.Fatalf("mark constrained state: %v", err)
	}

	counterUpdatedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 0, $2)
		ON CONFLICT (job_id, concurrency_key)
		DO UPDATE SET count = 0, updated_at = EXCLUDED.updated_at`,
		job.ID, counterUpdatedAt,
	); err != nil {
		t.Fatalf("seed zero active count: %v", err)
	}

	results, err := q.BulkCancelRuns(ctx, []string{run.ID}, time.Now().UTC(), "bulk cancel")
	if err != nil {
		t.Fatalf("BulkCancelRuns() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}

	var count int
	var updatedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT count, updated_at
		FROM job_active_counts
		WHERE job_id = $1
		  AND concurrency_key = ''`, job.ID).Scan(&count, &updatedAt); err != nil {
		t.Fatalf("active count after cancel: %v", err)
	}
	if count != 0 {
		t.Fatalf("active count after cancel = %d, want 0", count)
	}
	if !updatedAt.Equal(counterUpdatedAt) {
		t.Fatalf("active count updated_at changed for zero decrement: got %s want %s", updatedAt, counterUpdatedAt)
	}
	assertBulkCanceledViaTerminalState(t, ctx, run.ID, domain.StatusExecuting, "bulk cancel")
}

func TestRuns_BulkCancelRuns_SkipsTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-bulk-cancel-terminal")
	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, r.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus error = %v", err)
	}

	results, err := q.BulkCancelRuns(ctx, []string{r.ID}, time.Now().UTC(), "cancel")
	if err != nil {
		t.Fatalf("BulkCancelRuns() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("len = %d, want 0 (terminal skipped)", len(results))
	}
}

func TestRuns_BulkCancelRuns_EmptyIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	results, err := q.BulkCancelRuns(ctx, []string{}, time.Now().UTC(), "cancel")
	if err != nil {
		t.Fatalf("BulkCancelRuns() error = %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil for empty ids, got %v", results)
	}
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
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT status
		FROM job_run_state
		WHERE run_id = $1`, runID).Scan(&hotStatus); err != nil {
		t.Fatalf("query hot run state: %v", err)
	}
	if hotStatus != wantHotStatus {
		t.Fatalf("hot status = %q, want retained pre-terminal status %q", hotStatus, wantHotStatus)
	}

	var terminalStatus domain.RunStatus
	var terminalFinishedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT status, finished_at
		FROM job_run_terminal_state
		WHERE run_id = $1`, runID).Scan(&terminalStatus, &terminalFinishedAt); err != nil {
		t.Fatalf("query terminal run state: %v", err)
	}
	if terminalStatus != domain.StatusCanceled {
		t.Fatalf("terminal status = %q, want canceled", terminalStatus)
	}
	if terminalFinishedAt.IsZero() {
		t.Fatal("terminal finished_at is zero")
	}

	got, err := mustStore(t).GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusCanceled {
		t.Fatalf("GetRun status = %q, want canceled", got.Status)
	}
	if wantReason != "" && got.Error != wantReason {
		t.Fatalf("GetRun error = %q, want %q", got.Error, wantReason)
	}
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
	if err := q.CreateRun(ctx, child); err != nil {
		t.Fatalf("CreateRun(child) error = %v", err)
	}

	count, err := q.CancelChildRunsByParentIDs(ctx, []string{parent.ID}, time.Now().UTC(), "parent canceled")
	if err != nil {
		t.Fatalf("CancelChildRunsByParentIDs() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	got, err := q.GetRun(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusCanceled {
		t.Fatalf("status = %s, want canceled", got.Status)
	}
	assertBulkCanceledViaTerminalState(t, ctx, child.ID, domain.StatusExecuting, "parent canceled")
}

func TestRuns_CancelChildRunsByParentIDs_NoChildren(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CancelChildRunsByParentIDs(ctx, []string{newID()}, time.Now().UTC(), "cancel")
	if err != nil {
		t.Fatalf("CancelChildRunsByParentIDs() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestRuns_CancelChildRunsByParentIDs_EmptyParentIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CancelChildRunsByParentIDs(ctx, []string{}, time.Now().UTC(), "cancel")
	if err != nil {
		t.Fatalf("CancelChildRunsByParentIDs() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

// BulkReplayDeadLetterRuns.

func TestRuns_BulkReplayDeadLetterRuns_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-bulk-replay")

	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, r.ID, domain.StatusExecuting, domain.StatusDeadLetter, nil); err != nil {
		t.Fatalf("UpdateRunStatus error = %v", err)
	}

	replayed, err := q.BulkReplayDeadLetterRuns(ctx, []string{r.ID}, "", 0)
	if err != nil {
		t.Fatalf("BulkReplayDeadLetterRuns() error = %v", err)
	}
	if len(replayed) != 1 {
		t.Fatalf("len = %d, want 1", len(replayed))
	}
	if replayed[0].Status != domain.StatusQueued {
		t.Fatalf("status = %s, want queued", replayed[0].Status)
	}
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
		if err := q.CreateRun(ctx, r); err != nil {
			t.Fatalf("CreateRun error = %v", err)
		}
		if err := q.UpdateRunStatus(ctx, r.ID, domain.StatusExecuting, domain.StatusDeadLetter, nil); err != nil {
			t.Fatalf("UpdateRunStatus error = %v", err)
		}
	}

	replayed, err := q.BulkReplayDeadLetterRuns(ctx, nil, projectID, 100)
	if err != nil {
		t.Fatalf("BulkReplayDeadLetterRuns() error = %v", err)
	}
	if len(replayed) != 3 {
		t.Fatalf("len = %d, want 3", len(replayed))
	}
}

func TestRuns_BulkReplayDeadLetterRuns_NoneAvailable(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.BulkReplayDeadLetterRuns(ctx, nil, "nonexistent-project", 100)
	if err == nil {
		t.Fatal("expected error for no available runs")
	}
}

// BatchUpdateHeartbeat.

func TestRuns_BatchUpdateHeartbeat_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-batch-heartbeat")
	r1 := mustCreateRun(t, ctx, q, job)
	r2 := mustCreateRun(t, ctx, q, job)

	if err := q.BatchUpdateHeartbeat(ctx, []string{r1.ID, r2.ID}); err != nil {
		t.Fatalf("BatchUpdateHeartbeat() error = %v", err)
	}

	got1, _ := q.GetRun(ctx, r1.ID)
	got2, _ := q.GetRun(ctx, r2.ID)
	if got1.HeartbeatAt == nil {
		t.Fatal("r1 heartbeat_at should be set")
	}
	if got2.HeartbeatAt == nil {
		t.Fatal("r2 heartbeat_at should be set")
	}
}

func TestRuns_BatchUpdateHeartbeat_EmptyIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if err := q.BatchUpdateHeartbeat(ctx, []string{}); err != nil {
		t.Fatalf("BatchUpdateHeartbeat() error = %v", err)
	}
}

func TestRuns_BatchUpdateHeartbeat_NonexistentIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Should not error even with nonexistent IDs.
	if err := q.BatchUpdateHeartbeat(ctx, []string{newID(), newID()}); err != nil {
		t.Fatalf("BatchUpdateHeartbeat() error = %v", err)
	}
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
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	if err := q.ResetRunIdempotencyKey(ctx, r.ID); err != nil {
		t.Fatalf("ResetRunIdempotencyKey() error = %v", err)
	}

	got, err := q.GetRun(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.IdempotencyKey != "" {
		t.Fatalf("idempotency_key = %q, want empty", got.IdempotencyKey)
	}
}

func TestRuns_ResetRunIdempotencyKey_AlreadyEmpty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-reset-idemp-empty")
	r := mustCreateRun(t, ctx, q, job)

	// Should be a no-op when idempotency_key is already empty.
	if err := q.ResetRunIdempotencyKey(ctx, r.ID); err != nil {
		t.Fatalf("ResetRunIdempotencyKey() error = %v", err)
	}
}

func TestRuns_ResetRunIdempotencyKey_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.ResetRunIdempotencyKey(ctx, newID())
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
}

// RescheduleRun.

func TestRuns_RescheduleRun_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-reschedule")
	r := baseRun(job, newID())
	r.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	newSchedule := time.Now().UTC().Add(2 * time.Hour)
	if err := q.RescheduleRun(ctx, r.ID, newSchedule, nil); err != nil {
		t.Fatalf("RescheduleRun() error = %v", err)
	}

	got, err := q.GetRun(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusDelayed {
		t.Fatalf("status = %s, want delayed", got.Status)
	}
	if got.ScheduledAt == nil || got.ScheduledAt.Before(time.Now().UTC()) {
		t.Fatal("scheduled_at should be in the future")
	}

	var ledgerStatus domain.RunStatus
	var ledgerScheduledAt *time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT status, scheduled_at
		FROM job_runs
		WHERE id = $1
	`, r.ID).Scan(&ledgerStatus, &ledgerScheduledAt); err != nil {
		t.Fatalf("query ledger reschedule fields: %v", err)
	}
	if ledgerStatus != domain.StatusQueued {
		t.Fatalf("ledger status = %s, want original queued", ledgerStatus)
	}
	if ledgerScheduledAt != nil {
		t.Fatalf("ledger scheduled_at = %v, want nil", *ledgerScheduledAt)
	}
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
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	var beforeUpdatedAt time.Time
	var beforeReadyGeneration int64
	var beforeCacheVersions int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT s.updated_at, s.ready_generation,
		       (SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = s.run_id)
		FROM job_run_state s
		WHERE s.run_id = $1
	`, r.ID).Scan(&beforeUpdatedAt, &beforeReadyGeneration, &beforeCacheVersions); err != nil {
		t.Fatalf("query state before reschedule: %v", err)
	}

	time.Sleep(2 * time.Millisecond)
	if err := q.RescheduleRun(ctx, r.ID, scheduledAt, nil); err != nil {
		t.Fatalf("RescheduleRun() error = %v", err)
	}

	var afterUpdatedAt time.Time
	var afterReadyGeneration int64
	var afterCacheVersions int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT s.updated_at, s.ready_generation,
		       (SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = s.run_id)
		FROM job_run_state s
		WHERE s.run_id = $1
	`, r.ID).Scan(&afterUpdatedAt, &afterReadyGeneration, &afterCacheVersions); err != nil {
		t.Fatalf("query state after reschedule: %v", err)
	}

	if !afterUpdatedAt.Equal(beforeUpdatedAt) {
		t.Fatalf("updated_at = %v, want unchanged %v", afterUpdatedAt, beforeUpdatedAt)
	}
	if afterReadyGeneration != beforeReadyGeneration {
		t.Fatalf("ready_generation = %d, want unchanged %d", afterReadyGeneration, beforeReadyGeneration)
	}
	if afterCacheVersions != beforeCacheVersions {
		t.Fatalf("cache versions = %d, want unchanged %d", afterCacheVersions, beforeCacheVersions)
	}
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
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	var beforeLedgerXmin string
	var beforeStateXmin string
	var beforeCacheVersions int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT
			jr.xmin::text,
			s.xmin::text,
			(SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = jr.id)
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1
	`, r.ID).Scan(&beforeLedgerXmin, &beforeStateXmin, &beforeCacheVersions); err != nil {
		t.Fatalf("query reschedule rows before no-op: %v", err)
	}

	if err := q.RescheduleRun(ctx, r.ID, scheduledAt, payload); err != nil {
		t.Fatalf("RescheduleRun(no-op payload) error = %v", err)
	}

	var afterLedgerXmin string
	var afterStateXmin string
	var afterCacheVersions int
	var gotPayload json.RawMessage
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT
			jr.xmin::text,
			s.xmin::text,
			(SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = jr.id),
			jr.payload
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1
	`, r.ID).Scan(&afterLedgerXmin, &afterStateXmin, &afterCacheVersions, &gotPayload); err != nil {
		t.Fatalf("query reschedule rows after no-op: %v", err)
	}
	if afterLedgerXmin != beforeLedgerXmin {
		t.Fatalf("job_runs no-op reschedule changed xmin from %s to %s", beforeLedgerXmin, afterLedgerXmin)
	}
	if afterStateXmin != beforeStateXmin {
		t.Fatalf("job_run_state no-op reschedule changed xmin from %s to %s", beforeStateXmin, afterStateXmin)
	}
	if afterCacheVersions != beforeCacheVersions {
		t.Fatalf("cache versions = %d, want unchanged %d", afterCacheVersions, beforeCacheVersions)
	}
	if !jsonEqual(gotPayload, payload) {
		t.Fatalf("payload = %s, want %s", string(gotPayload), string(payload))
	}

	changedPayload := json.RawMessage(`{"kind":"changed","value":2}`)
	if err := q.RescheduleRun(ctx, r.ID, scheduledAt, changedPayload); err != nil {
		t.Fatalf("RescheduleRun(changed payload) error = %v", err)
	}

	var changedLedgerXmin string
	var changedCacheVersions int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT
			jr.xmin::text,
			(SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = jr.id)
		FROM job_runs jr
		WHERE jr.id = $1
	`, r.ID).Scan(&changedLedgerXmin, &changedCacheVersions); err != nil {
		t.Fatalf("query reschedule rows after changed payload: %v", err)
	}
	if changedLedgerXmin == afterLedgerXmin {
		t.Fatalf("changed payload kept job_runs xmin %s, want a real update", changedLedgerXmin)
	}
	if changedCacheVersions != afterCacheVersions+1 {
		t.Fatalf("cache versions after changed payload = %d, want %d", changedCacheVersions, afterCacheVersions+1)
	}
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
	if err := q.CreateRun(ctx, unchanged); err != nil {
		t.Fatalf("CreateRun unchanged error = %v", err)
	}

	var beforeXmin string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM job_runs
		WHERE id = $1`,
		unchanged.ID,
	).Scan(&beforeXmin); err != nil {
		t.Fatalf("query job_runs xmin before same-payload status update: %v", err)
	}
	if err := q.UpdateRunStatus(ctx, unchanged.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"payload":     payload,
		"finished_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus unchanged payload error = %v", err)
	}

	var afterXmin string
	var terminalStatus domain.RunStatus
	var gotPayload json.RawMessage
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT jr.xmin::text, rs.status, jr.payload
		FROM job_runs jr
		JOIN job_run_read_state rs ON rs.run_id = jr.id
		WHERE jr.id = $1`,
		unchanged.ID,
	).Scan(&afterXmin, &terminalStatus, &gotPayload); err != nil {
		t.Fatalf("query same-payload status update result: %v", err)
	}
	if afterXmin != beforeXmin {
		t.Fatalf("same-payload status update changed job_runs xmin from %s to %s", beforeXmin, afterXmin)
	}
	if terminalStatus != domain.StatusCompleted {
		t.Fatalf("terminal status = %q, want completed", terminalStatus)
	}
	if !jsonEqual(gotPayload, payload) {
		t.Fatalf("payload = %s, want %s", string(gotPayload), string(payload))
	}

	changed := baseRun(job, newID())
	changed.Status = domain.StatusExecuting
	changed.Payload = payload
	if err := q.CreateRun(ctx, changed); err != nil {
		t.Fatalf("CreateRun changed error = %v", err)
	}
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM job_runs
		WHERE id = $1`,
		changed.ID,
	).Scan(&beforeXmin); err != nil {
		t.Fatalf("query job_runs xmin before changed-payload status update: %v", err)
	}
	changedPayload := json.RawMessage(`{"kind":"changed","value":2}`)
	if err := q.UpdateRunStatus(ctx, changed.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"payload":     changedPayload,
		"finished_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus changed payload error = %v", err)
	}
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text, payload
		FROM job_runs
		WHERE id = $1`,
		changed.ID,
	).Scan(&afterXmin, &gotPayload); err != nil {
		t.Fatalf("query changed-payload status update result: %v", err)
	}
	if afterXmin == beforeXmin {
		t.Fatalf("changed-payload status update kept job_runs xmin %s, want a real update", afterXmin)
	}
	if !jsonEqual(gotPayload, changedPayload) {
		t.Fatalf("changed payload = %s, want %s", string(gotPayload), string(changedPayload))
	}
}

func TestRuns_RescheduleRun_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.RescheduleRun(ctx, newID(), time.Now().UTC().Add(time.Hour), nil)
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
}

func TestRuns_RescheduleRun_CannotRescheduleExecuting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-reschedule-exec")
	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	err := q.RescheduleRun(ctx, r.ID, time.Now().UTC().Add(time.Hour), nil)
	if err == nil {
		t.Fatal("expected error for executing run")
	}
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
	if err := q.CreateRun(ctx, r1); err != nil {
		t.Fatalf("CreateRun(r1) error = %v", err)
	}
	r2 := baseRun(job, newID())
	r2.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, r2); err != nil {
		t.Fatalf("CreateRun(r2) error = %v", err)
	}

	ids, err := q.BulkCancelByFilter(ctx, projectID, store.BulkCancelFilter{JobID: job.ID}, time.Now().UTC(), "filter cancel")
	if err != nil {
		t.Fatalf("BulkCancelByFilter() error = %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len = %d, want 2", len(ids))
	}
	assertBulkCanceledViaTerminalState(t, ctx, r1.ID, domain.StatusQueued, "filter cancel")
	assertBulkCanceledViaTerminalState(t, ctx, r2.ID, domain.StatusQueued, "filter cancel")
}

func TestRuns_BulkCancelByFilter_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	ids, err := q.BulkCancelByFilter(ctx, "nonexistent-project", store.BulkCancelFilter{}, time.Now().UTC(), "cancel")
	if err != nil {
		t.Fatalf("BulkCancelByFilter() error = %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("len = %d, want 0", len(ids))
	}
}

func TestRuns_BulkCancelByFilter_ExcludesExecuting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-bulk-cancel-filter-exec"
	job := mustCreateJob(t, ctx, q, projectID)

	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	ids, err := q.BulkCancelByFilter(ctx, projectID, store.BulkCancelFilter{}, time.Now().UTC(), "cancel")
	if err != nil {
		t.Fatalf("BulkCancelByFilter() error = %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("len = %d, want 0 (executing excluded by BulkCancelByFilter)", len(ids))
	}
}

// CountActiveRunsForJob.

func TestRuns_CountActiveRunsForJob_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-count-active")

	r1 := baseRun(job, newID())
	r1.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, r1); err != nil {
		t.Fatalf("CreateRun(queued) error = %v", err)
	}
	r2 := baseRun(job, newID())
	r2.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, r2); err != nil {
		t.Fatalf("CreateRun(executing) error = %v", err)
	}
	r3 := baseRun(job, newID())
	r3.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, r3); err != nil {
		t.Fatalf("CreateRun(executing2) error = %v", err)
	}
	// Transition r3 to completed -- should not be counted.
	if err := q.UpdateRunStatus(ctx, r3.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus error = %v", err)
	}

	count, err := q.CountActiveRunsForJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("CountActiveRunsForJob() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestRuns_CountActiveRunsForJob_Zero(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CountActiveRunsForJob(ctx, newID())
	if err != nil {
		t.Fatalf("CountActiveRunsForJob() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestRuns_CountActiveRunsForJob_CrossJobIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job1 := mustCreateJob(t, ctx, q, "project-count-active-iso")
	job2 := mustCreateJob(t, ctx, q, "project-count-active-iso")

	r1 := baseRun(job1, newID())
	r1.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, r1); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	r2 := baseRun(job2, newID())
	r2.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, r2); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	count, err := q.CountActiveRunsForJob(ctx, job1.ID)
	if err != nil {
		t.Fatalf("CountActiveRunsForJob() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (isolated to job1)", count)
	}
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
	if err := q.CreateRun(ctx, r1); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	r2 := baseRun(job, newID())
	r2.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, r2); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	canceled, err := q.CancelActiveRunsForJob(ctx, job.ID, "cron overlap")
	if err != nil {
		t.Fatalf("CancelActiveRunsForJob() error = %v", err)
	}
	if len(canceled) != 2 {
		t.Fatalf("len = %d, want 2", len(canceled))
	}
	byID := make(map[string]store.CanceledRun, len(canceled))
	for _, cr := range canceled {
		byID[cr.ID] = cr
	}
	if got := byID[r1.ID]; got.WorkflowStepRunID != "step-run-cancel-active" || got.JobID != job.ID || got.ProjectID != job.ProjectID || got.ExecutionMode != domain.ExecutionModeWorker {
		t.Fatalf("canceled run metadata = %+v", got)
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
	if err := q.CreateRun(ctx, oldRun); err != nil {
		t.Fatalf("CreateRun oldRun error = %v", err)
	}
	replacement := baseRun(job, newID())
	replacement.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, replacement); err != nil {
		t.Fatalf("CreateRun replacement error = %v", err)
	}

	canceled, err := q.CancelActiveRunsForJobExcept(ctx, job.ID, replacement.ID, "cron overlap")
	if err != nil {
		t.Fatalf("CancelActiveRunsForJobExcept() error = %v", err)
	}
	if len(canceled) != 1 || canceled[0].ID != oldRun.ID {
		t.Fatalf("canceled = %+v, want only old run %s", canceled, oldRun.ID)
	}

	gotReplacement, err := q.GetRun(ctx, replacement.ID)
	if err != nil {
		t.Fatalf("GetRun(replacement) error = %v", err)
	}
	if gotReplacement.Status != domain.StatusQueued {
		t.Fatalf("replacement status = %q, want queued", gotReplacement.Status)
	}
}

func TestRuns_CancelActiveRunsForJob_NoActiveRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	canceled, err := q.CancelActiveRunsForJob(ctx, newID(), "cancel")
	if err != nil {
		t.Fatalf("CancelActiveRunsForJob() error = %v", err)
	}
	if len(canceled) != 0 {
		t.Fatalf("len = %d, want 0", len(canceled))
	}
}

func TestRuns_CancelActiveRunsForJob_SkipsTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cancel-active-terminal")
	r := baseRun(job, newID())
	r.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, r.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus error = %v", err)
	}

	canceled, err := q.CancelActiveRunsForJob(ctx, job.ID, "cancel")
	if err != nil {
		t.Fatalf("CancelActiveRunsForJob() error = %v", err)
	}
	if len(canceled) != 0 {
		t.Fatalf("len = %d, want 0 (terminal skipped)", len(canceled))
	}
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
		if err := q.CreateRunIteration(ctx, iter); err != nil {
			t.Fatalf("CreateRunIteration(%d) error = %v", i, err)
		}
	}

	count, err := q.CountRunIterations(ctx, run.ID)
	if err != nil {
		t.Fatalf("CountRunIterations() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
}

func TestRuns_CountRunIterations_NoIterations(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-count-iterations-empty")
	run := mustCreateRun(t, ctx, q, job)

	count, err := q.CountRunIterations(ctx, run.ID)
	if err != nil {
		t.Fatalf("CountRunIterations() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestRuns_CountRunIterations_NonexistentRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CountRunIterations(ctx, newID())
	if err != nil {
		t.Fatalf("CountRunIterations() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

// CreateRunIteration.

func TestRuns_CreateRunIteration_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-create-iteration")
	run := mustCreateRun(t, ctx, q, job)

	iter := &domain.RunIteration{RunID: run.ID, Iteration: 1, Description: "first iteration"}
	if err := q.CreateRunIteration(ctx, iter); err != nil {
		t.Fatalf("CreateRunIteration() error = %v", err)
	}
	if iter.ID == "" {
		t.Fatal("ID should be set")
	}
	if iter.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be set")
	}
}

func TestRuns_CreateRunIteration_AutoID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-create-iteration-autoid")
	run := mustCreateRun(t, ctx, q, job)

	iter := &domain.RunIteration{RunID: run.ID, Iteration: 1}
	if err := q.CreateRunIteration(ctx, iter); err != nil {
		t.Fatalf("CreateRunIteration() error = %v", err)
	}
	if iter.ID == "" {
		t.Fatal("expected auto-generated ID")
	}
}

func TestRuns_CreateRunIteration_CustomID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-create-iteration-custom")
	run := mustCreateRun(t, ctx, q, job)

	customID := newID()
	iter := &domain.RunIteration{ID: customID, RunID: run.ID, Iteration: 1}
	if err := q.CreateRunIteration(ctx, iter); err != nil {
		t.Fatalf("CreateRunIteration() error = %v", err)
	}
	if iter.ID != customID {
		t.Fatalf("ID = %q, want %q", iter.ID, customID)
	}
}

// ListRunState.

func TestRunState_ListRunState_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-run-state")
	run := mustCreateRun(t, ctx, q, job)

	s1 := &domain.RunState{RunID: run.ID, StateKey: "alpha", Value: json.RawMessage(`"hello"`)}
	if err := q.UpsertRunState(ctx, s1); err != nil {
		t.Fatalf("UpsertRunState(alpha) error = %v", err)
	}
	s2 := &domain.RunState{RunID: run.ID, StateKey: "beta", Value: json.RawMessage(`42`)}
	if err := q.UpsertRunState(ctx, s2); err != nil {
		t.Fatalf("UpsertRunState(beta) error = %v", err)
	}

	items, err := q.ListRunState(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListRunState() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
	// Ordered by state_key ASC.
	if items[0].StateKey != "alpha" {
		t.Fatalf("first key = %q, want alpha", items[0].StateKey)
	}
	if items[1].StateKey != "beta" {
		t.Fatalf("second key = %q, want beta", items[1].StateKey)
	}
}

func TestRunState_UpsertSameValueDoesNotRewrite(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-noop")
	run := mustCreateRun(t, ctx, q, job)

	state := &domain.RunState{RunID: run.ID, StateKey: "cursor", Value: json.RawMessage(`{"page":1}`)}
	if err := q.UpsertRunState(ctx, state); err != nil {
		t.Fatalf("UpsertRunState(initial) error = %v", err)
	}
	initialUpdatedAt := state.UpdatedAt
	var beforeXmin string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM run_state
		WHERE run_id = $1 AND state_key = $2`,
		run.ID,
		"cursor",
	).Scan(&beforeXmin); err != nil {
		t.Fatalf("query run_state xmin before no-op: %v", err)
	}

	sameState := &domain.RunState{RunID: run.ID, StateKey: "cursor", Value: json.RawMessage(`{"page":1}`)}
	if err := q.UpsertRunState(ctx, sameState); err != nil {
		t.Fatalf("UpsertRunState(no-op) error = %v", err)
	}
	var afterNoopXmin string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM run_state
		WHERE run_id = $1 AND state_key = $2`,
		run.ID,
		"cursor",
	).Scan(&afterNoopXmin); err != nil {
		t.Fatalf("query run_state xmin after no-op: %v", err)
	}
	if afterNoopXmin != beforeXmin {
		t.Fatalf("run_state no-op changed xmin from %s to %s", beforeXmin, afterNoopXmin)
	}
	if !sameState.UpdatedAt.Equal(initialUpdatedAt) {
		t.Fatalf("run_state no-op updated_at = %v, want %v", sameState.UpdatedAt, initialUpdatedAt)
	}

	changedState := &domain.RunState{RunID: run.ID, StateKey: "cursor", Value: json.RawMessage(`{"page":2}`)}
	if err := q.UpsertRunState(ctx, changedState); err != nil {
		t.Fatalf("UpsertRunState(changed) error = %v", err)
	}
	var afterChangedXmin string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM run_state
		WHERE run_id = $1 AND state_key = $2`,
		run.ID,
		"cursor",
	).Scan(&afterChangedXmin); err != nil {
		t.Fatalf("query run_state xmin after change: %v", err)
	}
	if afterChangedXmin == beforeXmin {
		t.Fatalf("run_state changed value kept xmin %s, want a real update", afterChangedXmin)
	}
}

func TestRunState_ListRunState_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-run-state-empty")
	run := mustCreateRun(t, ctx, q, job)

	items, err := q.ListRunState(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListRunState() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len = %d, want 0", len(items))
	}
}

func TestRunState_ListRunState_CrossRunIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-run-state-iso")
	run1 := mustCreateRun(t, ctx, q, job)
	run2 := mustCreateRun(t, ctx, q, job)

	s1 := &domain.RunState{RunID: run1.ID, StateKey: "key1", Value: json.RawMessage(`"v1"`)}
	if err := q.UpsertRunState(ctx, s1); err != nil {
		t.Fatalf("UpsertRunState error = %v", err)
	}
	s2 := &domain.RunState{RunID: run2.ID, StateKey: "key2", Value: json.RawMessage(`"v2"`)}
	if err := q.UpsertRunState(ctx, s2); err != nil {
		t.Fatalf("UpsertRunState error = %v", err)
	}

	items, err := q.ListRunState(ctx, run1.ID)
	if err != nil {
		t.Fatalf("ListRunState(run1) error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1 (only run1 state)", len(items))
	}
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
	if err := q.UpsertRunState(ctx, s1); err != nil {
		t.Fatalf("UpsertRunState error = %v", err)
	}
	s2 := &domain.RunState{RunID: source.ID, StateKey: "key-b", Value: json.RawMessage(`"value-b"`)}
	if err := q.UpsertRunState(ctx, s2); err != nil {
		t.Fatalf("UpsertRunState error = %v", err)
	}

	if err := q.CopyRunState(ctx, source.ID, target.ID); err != nil {
		t.Fatalf("CopyRunState() error = %v", err)
	}

	items, err := q.ListRunState(ctx, target.ID)
	if err != nil {
		t.Fatalf("ListRunState(target) error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
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
	if err := q.UpsertRunState(ctx, s); err != nil {
		t.Fatalf("UpsertRunState(source) error = %v", err)
	}

	// Target already has key-a = "original".
	t2 := &domain.RunState{RunID: target.ID, StateKey: "key-a", Value: json.RawMessage(`"original"`)}
	if err := q.UpsertRunState(ctx, t2); err != nil {
		t.Fatalf("UpsertRunState(target) error = %v", err)
	}

	if err := q.CopyRunState(ctx, source.ID, target.ID); err != nil {
		t.Fatalf("CopyRunState() error = %v", err)
	}

	got, err := q.GetRunState(ctx, target.ID, "key-a")
	if err != nil {
		t.Fatalf("GetRunState() error = %v", err)
	}
	if got == nil {
		t.Fatal("expected state entry")
	}
	if !jsonEqual(got.Value, json.RawMessage(`"original"`)) {
		t.Fatalf("value = %s, want original (not overwritten)", string(got.Value))
	}
}

func TestRunState_CopyRunState_EmptySource(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-copy-empty-source")
	source := mustCreateRun(t, ctx, q, job)
	target := mustCreateRun(t, ctx, q, job)

	if err := q.CopyRunState(ctx, source.ID, target.ID); err != nil {
		t.Fatalf("CopyRunState() error = %v", err)
	}

	items, err := q.ListRunState(ctx, target.ID)
	if err != nil {
		t.Fatalf("ListRunState() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len = %d, want 0", len(items))
	}
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
	if err := q.CreateRunResourceSnapshot(ctx, snap); err != nil {
		t.Fatalf("CreateRunResourceSnapshot() error = %v", err)
	}
	if snap.ID == "" {
		t.Fatal("ID should be set")
	}
	if snap.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be set")
	}
}

func TestRunResource_CreateRunResourceSnapshot_AutoID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-resource-snapshot-autoid")
	run := mustCreateRun(t, ctx, q, job)

	snap := &domain.RunResourceSnapshot{RunID: run.ID, CPUPercent: 10}
	if err := q.CreateRunResourceSnapshot(ctx, snap); err != nil {
		t.Fatalf("CreateRunResourceSnapshot() error = %v", err)
	}
	if snap.ID == "" {
		t.Fatal("expected auto-generated ID")
	}
}

func TestRunResource_CreateRunResourceSnapshot_CustomID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-resource-snapshot-custom")
	run := mustCreateRun(t, ctx, q, job)

	customID := newID()
	snap := &domain.RunResourceSnapshot{ID: customID, RunID: run.ID, CPUPercent: 10}
	if err := q.CreateRunResourceSnapshot(ctx, snap); err != nil {
		t.Fatalf("CreateRunResourceSnapshot() error = %v", err)
	}
	if snap.ID != customID {
		t.Fatalf("ID = %q, want %q", snap.ID, customID)
	}
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
		if err := q.CreateRunResourceSnapshot(ctx, snap); err != nil {
			t.Fatalf("CreateRunResourceSnapshot(%d) error = %v", i, err)
		}
	}

	snapshots, err := q.ListRunResourceSnapshots(ctx, run.ID, nil, nil, 10)
	if err != nil {
		t.Fatalf("ListRunResourceSnapshots() error = %v", err)
	}
	if len(snapshots) != 3 {
		t.Fatalf("len = %d, want 3", len(snapshots))
	}
}

func TestRunResource_ListRunResourceSnapshots_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-snapshots-empty")
	run := mustCreateRun(t, ctx, q, job)

	snapshots, err := q.ListRunResourceSnapshots(ctx, run.ID, nil, nil, 10)
	if err != nil {
		t.Fatalf("ListRunResourceSnapshots() error = %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("len = %d, want 0", len(snapshots))
	}
}

func TestRunResource_ListRunResourceSnapshots_TimeFilter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-snapshots-time")
	run := mustCreateRun(t, ctx, q, job)

	for range 3 {
		snap := &domain.RunResourceSnapshot{RunID: run.ID, CPUPercent: 50}
		if err := q.CreateRunResourceSnapshot(ctx, snap); err != nil {
			t.Fatalf("CreateRunResourceSnapshot error = %v", err)
		}
	}

	now := time.Now().UTC()
	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)

	snapshots, err := q.ListRunResourceSnapshots(ctx, run.ID, &from, &to, 10)
	if err != nil {
		t.Fatalf("ListRunResourceSnapshots() error = %v", err)
	}
	if len(snapshots) != 3 {
		t.Fatalf("len = %d, want 3", len(snapshots))
	}

	// Filter with a past window that excludes everything.
	pastFrom := now.Add(-3 * time.Hour)
	pastTo := now.Add(-2 * time.Hour)
	snapshots, err = q.ListRunResourceSnapshots(ctx, run.ID, &pastFrom, &pastTo, 10)
	if err != nil {
		t.Fatalf("ListRunResourceSnapshots(past) error = %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("len = %d, want 0 (past window)", len(snapshots))
	}
}
