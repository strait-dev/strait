//go:build integration

package store_test

import (
	"context"
	"encoding/json"
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
		"result":      json.RawMessage(`{"ok":true}`),
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
	if ledgerResult != nil {
		t.Fatalf("job_runs result = %s, want NULL to avoid fat-row churn", string(ledgerResult))
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

	cachedRun, _, err := q.GetRunWithCacheVersion(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRunWithCacheVersion() error = %v", err)
	}
	if !jsonEqual(cachedRun.Result, []byte(`{"ok":true}`)) {
		t.Fatalf("GetRunWithCacheVersion result = %s, want terminal result", string(cachedRun.Result))
	}

	byJob, err := q.ListRunsByJob(ctx, job.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListRunsByJob() error = %v", err)
	}
	if len(byJob) != 1 || !jsonEqual(byJob[0].Result, []byte(`{"ok":true}`)) {
		t.Fatalf("ListRunsByJob result = %+v, want terminal result", byJob)
	}
}

func TestRunStateSplit_TerminalErrorFieldsReadFromLifecycleEvent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-terminal-error")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusFailed, map[string]any{
		"error":       "worker failed",
		"error_class": "server",
	}); err != nil {
		t.Fatalf("UpdateRunStatus() error = %v", err)
	}

	var ledgerError *string
	var ledgerErrorClass *string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT error, error_class
		FROM job_runs
		WHERE id = $1`,
		run.ID,
	).Scan(&ledgerError, &ledgerErrorClass); err != nil {
		t.Fatalf("query job_runs error fields: %v", err)
	}
	if ledgerError != nil {
		t.Fatalf("job_runs error = %q, want NULL to avoid fat-row churn", *ledgerError)
	}
	if ledgerErrorClass != nil {
		t.Fatalf("job_runs error_class = %q, want NULL to avoid fat-row churn", *ledgerErrorClass)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Error != "worker failed" {
		t.Fatalf("GetRun error = %q, want worker failed", got.Error)
	}
	if got.ErrorClass != "server" {
		t.Fatalf("GetRun error_class = %q, want server", got.ErrorClass)
	}

	status := domain.StatusFailed
	errorClass := "server"
	listed, err := q.ListRunsByProject(ctx, job.ProjectID, &status, nil, nil, nil, nil, nil, nil, &errorClass, 10, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != run.ID {
		t.Fatalf("ListRunsByProject = %+v, want failed run %s", listed, run.ID)
	}
	if listed[0].Error != "worker failed" || listed[0].ErrorClass != "server" {
		t.Fatalf("ListRunsByProject error fields = %q/%q, want worker failed/server", listed[0].Error, listed[0].ErrorClass)
	}

	filtered, err := q.ListRunsByProjectFiltered(ctx, job.ProjectID, nil, []domain.RunStatus{domain.StatusFailed}, "", "", nil, nil, nil, nil, nil, nil, nil, &errorClass, 10, nil)
	if err != nil {
		t.Fatalf("ListRunsByProjectFiltered() error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != run.ID {
		t.Fatalf("ListRunsByProjectFiltered = %+v, want failed run %s", filtered, run.ID)
	}
	if filtered[0].Error != "worker failed" || filtered[0].ErrorClass != "server" {
		t.Fatalf("ListRunsByProjectFiltered error fields = %q/%q, want worker failed/server", filtered[0].Error, filtered[0].ErrorClass)
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
