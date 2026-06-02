//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

// Integration tests for the dlq_counts trigger + store helpers.

func TestDLQCounts_TriggerMaintainsCounter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-trg-" + newID()
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org-x", Name: "P"}); err != nil {
		t.Fatalf("project: %v", err)
	}
	job := baseJob(newID(), projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("job: %v", err)
	}

	// Transition 3 runs straight to dead_letter.
	var ids []string
	for range 3 {
		r := baseRun(job, newID())
		r.Status = domain.StatusDeadLetter
		if err := q.CreateRun(ctx, r); err != nil {
			t.Fatalf("run: %v", err)
		}
		ids = append(ids, r.ID)
	}

	depth, err := q.DLQDepth(ctx, projectID, job.ID)
	if err != nil {
		t.Fatalf("depth: %v", err)
	}
	if depth != 3 {
		t.Errorf("depth = %d, want 3", depth)
	}

	// Soft-delete one row; counter should drop.
	_, err = testDB.Pool.Exec(ctx, `UPDATE job_runs SET visible_until = NOW() WHERE id = $1`, ids[0])
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	depth, _ = q.DLQDepth(ctx, projectID, job.ID)
	if depth != 2 {
		t.Errorf("depth after mask = %d, want 2", depth)
	}

	// Project-level aggregate.
	pd, err := q.DLQDepthByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("project depth: %v", err)
	}
	if pd != 2 {
		t.Errorf("project depth = %d, want 2", pd)
	}
}

func TestDLQCounts_SplitStateTransitionsMaintainCounter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "proj-dlq-split-counter")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusDeadLetter, map[string]any{
		"finished_at": time.Now().UTC().Truncate(time.Microsecond),
	}); err != nil {
		t.Fatalf("UpdateRunStatus(dead_letter) error = %v", err)
	}

	var ledgerStatus, readStatus domain.RunStatus
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT jr.status, s.status
		FROM job_runs jr
		JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		run.ID,
	).Scan(&ledgerStatus, &readStatus); err != nil {
		t.Fatalf("query split state: %v", err)
	}
	if ledgerStatus != domain.StatusExecuting {
		t.Fatalf("job_runs status = %q, want immutable executing ledger status", ledgerStatus)
	}
	if readStatus != domain.StatusDeadLetter {
		t.Fatalf("read status = %q, want dead_letter", readStatus)
	}

	depth, err := q.DLQDepth(ctx, job.ProjectID, job.ID)
	if err != nil {
		t.Fatalf("DLQDepth() error = %v", err)
	}
	if depth != 1 {
		t.Fatalf("DLQDepth after terminal transition = %d, want 1", depth)
	}

	replayed, err := q.ReplayDeadLetterRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ReplayDeadLetterRun() error = %v", err)
	}
	if replayed.Status != domain.StatusQueued {
		t.Fatalf("replayed status = %q, want queued", replayed.Status)
	}

	depth, err = q.DLQDepth(ctx, job.ProjectID, job.ID)
	if err != nil {
		t.Fatalf("DLQDepth() after replay error = %v", err)
	}
	if depth != 0 {
		t.Fatalf("DLQDepth after replay = %d, want 0", depth)
	}
}

func TestMaskOldestDLQRow_PicksOldest(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-oldest-" + newID()
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org-x", Name: "P"}); err != nil {
		t.Fatalf("project: %v", err)
	}
	job := baseJob(newID(), projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("job: %v", err)
	}

	// Three DLQ rows with distinct finished_at timestamps.
	base := time.Now().UTC().Add(-1 * time.Hour)
	var oldest, middle, newest domain.JobRun
	for i, run := range []*domain.JobRun{&oldest, &middle, &newest} {
		run.ID = newID()
		run.JobID = job.ID
		run.ProjectID = projectID
		run.Status = domain.StatusDeadLetter
		run.TriggeredBy = domain.TriggerManual
		run.Attempt = 1
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		_, err := testDB.Pool.Exec(ctx,
			`UPDATE job_runs SET finished_at = $1 WHERE id = $2`,
			base.Add(time.Duration(i)*time.Minute),
			run.ID,
		)
		if err != nil {
			t.Fatalf("set finished_at %d: %v", i, err)
		}
	}

	got, err := q.MaskOldestDLQRow(ctx, projectID, job.ID)
	if err != nil {
		t.Fatalf("mask oldest: %v", err)
	}
	if got != oldest.ID {
		t.Errorf("masked %q, want oldest %q", got, oldest.ID)
	}

	// Counter should drop by one.
	depth, _ := q.DLQDepth(ctx, projectID, job.ID)
	if depth != 2 {
		t.Errorf("depth after mask = %d, want 2", depth)
	}

	// Calling again picks the next oldest.
	got2, _ := q.MaskOldestDLQRow(ctx, projectID, job.ID)
	if got2 != middle.ID {
		t.Errorf("second mask %q, want %q", got2, middle.ID)
	}
}

func TestDLQDepth_MissingRowReturnsZero(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	depth, err := q.DLQDepth(ctx, "no-project", "no-job")
	if err != nil {
		t.Fatalf("depth: %v", err)
	}
	if depth != 0 {
		t.Errorf("depth = %d, want 0", depth)
	}
}
