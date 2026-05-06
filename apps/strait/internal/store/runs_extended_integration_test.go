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

// SumRunTotalTokens.

func TestRuns_SumRunTotalTokens_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-sum-tokens")
	run := mustCreateRun(t, ctx, q, job)

	u1 := &domain.RunUsage{RunID: run.ID, Provider: "openai", Model: "gpt-4", TotalTokens: 100, CostMicrousd: 1}
	if err := q.CreateRunUsage(ctx, u1); err != nil {
		t.Fatalf("CreateRunUsage(1) error = %v", err)
	}
	u2 := &domain.RunUsage{RunID: run.ID, Provider: "openai", Model: "gpt-4", TotalTokens: 200, CostMicrousd: 2}
	if err := q.CreateRunUsage(ctx, u2); err != nil {
		t.Fatalf("CreateRunUsage(2) error = %v", err)
	}

	total, err := q.SumRunTotalTokens(ctx, run.ID)
	if err != nil {
		t.Fatalf("SumRunTotalTokens() error = %v", err)
	}
	if total != 300 {
		t.Fatalf("total = %d, want 300", total)
	}
}

func TestRuns_SumRunTotalTokens_NoUsage(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-sum-tokens-empty")
	run := mustCreateRun(t, ctx, q, job)

	total, err := q.SumRunTotalTokens(ctx, run.ID)
	if err != nil {
		t.Fatalf("SumRunTotalTokens() error = %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
}

func TestRuns_SumRunTotalTokens_NonexistentRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	total, err := q.SumRunTotalTokens(ctx, newID())
	if err != nil {
		t.Fatalf("SumRunTotalTokens() error = %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
}

// CountRunToolCalls.

func TestRuns_CountRunToolCalls_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-count-tool-calls")
	run := mustCreateRun(t, ctx, q, job)

	for i := range 3 {
		call := &domain.RunToolCall{RunID: run.ID, ToolName: "tool-" + newID(), Input: json.RawMessage(`{}`), DurationMs: i}
		if err := q.CreateRunToolCall(ctx, call); err != nil {
			t.Fatalf("CreateRunToolCall(%d) error = %v", i, err)
		}
	}

	count, err := q.CountRunToolCalls(ctx, run.ID)
	if err != nil {
		t.Fatalf("CountRunToolCalls() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
}

func TestRuns_CountRunToolCalls_NoToolCalls(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-count-tool-calls-empty")
	run := mustCreateRun(t, ctx, q, job)

	count, err := q.CountRunToolCalls(ctx, run.ID)
	if err != nil {
		t.Fatalf("CountRunToolCalls() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestRuns_CountRunToolCalls_NonexistentRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CountRunToolCalls(ctx, newID())
	if err != nil {
		t.Fatalf("CountRunToolCalls() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
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
