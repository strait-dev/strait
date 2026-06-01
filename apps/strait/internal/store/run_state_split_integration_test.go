//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestRunStateSplit_UpdateRunStatusDoesNotTouchLedgerStateColumns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-update")
	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, map[string]any{
		"started_at": startedAt,
	}); err != nil {
		t.Fatalf("UpdateRunStatus() error = %v", err)
	}

	var ledgerStatus domain.RunStatus
	var ledgerStartedAt *time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT status, started_at
		FROM job_runs
		WHERE id = $1`,
		run.ID,
	).Scan(&ledgerStatus, &ledgerStartedAt); err != nil {
		t.Fatalf("query job_runs ledger fields: %v", err)
	}
	if ledgerStatus != domain.StatusQueued {
		t.Fatalf("job_runs status = %q, want immutable ledger status %q", ledgerStatus, domain.StatusQueued)
	}
	if ledgerStartedAt != nil {
		t.Fatalf("job_runs started_at = %v, want NULL to avoid fat-row churn", *ledgerStartedAt)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusDequeued {
		t.Fatalf("GetRun status = %q, want state status %q", got.Status, domain.StatusDequeued)
	}
	if got.StartedAt == nil || !got.StartedAt.Equal(startedAt) {
		t.Fatalf("GetRun started_at = %v, want %v", got.StartedAt, startedAt)
	}
}

func TestRunStateSplit_UpdateRunStatusForActiveRunKeepsLedgerStateImmutable(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-active-update")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	finishedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := q.UpdateRunStatusForActiveRun(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": finishedAt,
		"result":      []byte(`{"ok":true}`),
	}, run.Attempt); err != nil {
		t.Fatalf("UpdateRunStatusForActiveRun() error = %v", err)
	}

	var ledgerStatus domain.RunStatus
	var ledgerFinishedAt *time.Time
	var ledgerResult []byte
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT status, finished_at, result
		FROM job_runs
		WHERE id = $1`,
		run.ID,
	).Scan(&ledgerStatus, &ledgerFinishedAt, &ledgerResult); err != nil {
		t.Fatalf("query job_runs ledger fields: %v", err)
	}
	if ledgerStatus != domain.StatusExecuting {
		t.Fatalf("job_runs status = %q, want immutable ledger status %q", ledgerStatus, domain.StatusExecuting)
	}
	if ledgerFinishedAt != nil {
		t.Fatalf("job_runs finished_at = %v, want NULL to avoid fat-row churn", *ledgerFinishedAt)
	}
	if !jsonEqual(ledgerResult, []byte(`{"ok":true}`)) {
		t.Fatalf("job_runs result = %s, want terminal result retained on ledger", string(ledgerResult))
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusCompleted {
		t.Fatalf("GetRun status = %q, want state status %q", got.Status, domain.StatusCompleted)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finishedAt) {
		t.Fatalf("GetRun finished_at = %v, want %v", got.FinishedAt, finishedAt)
	}
	if !jsonEqual(got.Result, []byte(`{"ok":true}`)) {
		t.Fatalf("GetRun result = %s, want terminal result", string(got.Result))
	}
}

func TestRunStateSplit_UpdateRunStatusReturningOldDoesNotTouchLedgerStateColumns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-returning-old")
	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	oldStatus, err := q.UpdateRunStatusReturningOld(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, nil)
	if err != nil {
		t.Fatalf("UpdateRunStatusReturningOld() error = %v", err)
	}
	if oldStatus != domain.StatusQueued {
		t.Fatalf("old status = %q, want %q", oldStatus, domain.StatusQueued)
	}

	var ledgerStatus domain.RunStatus
	if err := testDB.Pool.QueryRow(ctx, `SELECT status FROM job_runs WHERE id = $1`, run.ID).Scan(&ledgerStatus); err != nil {
		t.Fatalf("query job_runs status: %v", err)
	}
	if ledgerStatus != domain.StatusQueued {
		t.Fatalf("job_runs status = %q, want immutable ledger status %q", ledgerStatus, domain.StatusQueued)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusDequeued {
		t.Fatalf("GetRun status = %q, want state status %q", got.Status, domain.StatusDequeued)
	}
}
