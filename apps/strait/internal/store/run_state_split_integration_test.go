//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"slices"
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
	run.IdempotencyKey = newID()
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

	byIdempotency, err := q.GetRunByIdempotencyKey(ctx, job.ID, run.IdempotencyKey)
	if err != nil {
		t.Fatalf("GetRunByIdempotencyKey() error = %v", err)
	}
	if byIdempotency == nil {
		t.Fatal("GetRunByIdempotencyKey() returned nil run")
	}
	if !jsonEqual(byIdempotency.Result, []byte(`{"ok":true}`)) {
		t.Fatalf("GetRunByIdempotencyKey result = %s, want terminal result", string(byIdempotency.Result))
	}

	finished, err := q.ListFinishedRunsSince(ctx, job.ProjectID, finishedAt.Add(-time.Minute), "", 10)
	if err != nil {
		t.Fatalf("ListFinishedRunsSince() error = %v", err)
	}
	if len(finished) != 1 || finished[0].ID != run.ID {
		t.Fatalf("ListFinishedRunsSince = %+v, want run %s", finished, run.ID)
	}
	if !jsonEqual(finished[0].Result, []byte(`{"ok":true}`)) {
		t.Fatalf("ListFinishedRunsSince result = %s, want terminal result", string(finished[0].Result))
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

	finishedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusFailed, map[string]any{
		"finished_at": finishedAt,
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

	finished, err := q.ListFinishedRunsSince(ctx, job.ProjectID, finishedAt.Add(-time.Minute), "", 10)
	if err != nil {
		t.Fatalf("ListFinishedRunsSince() error = %v", err)
	}
	if len(finished) != 1 || finished[0].ID != run.ID {
		t.Fatalf("ListFinishedRunsSince = %+v, want failed run %s", finished, run.ID)
	}
	if finished[0].Error != "worker failed" || finished[0].ErrorClass != "server" {
		t.Fatalf("ListFinishedRunsSince error fields = %q/%q, want worker failed/server", finished[0].Error, finished[0].ErrorClass)
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

func TestRunStateSplit_DeadLetterTransitionUsesColdTerminalState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-dead-letter")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	finishedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusDeadLetter, map[string]any{
		"error":       "worker gave up",
		"error_class": "terminal",
		"finished_at": finishedAt,
	}); err != nil {
		t.Fatalf("UpdateRunStatus(dead_letter) error = %v", err)
	}

	var ledgerStatus domain.RunStatus
	var ledgerFinishedAt *time.Time
	var ledgerError *string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT status, finished_at, error
		FROM job_runs
		WHERE id = $1`,
		run.ID,
	).Scan(&ledgerStatus, &ledgerFinishedAt, &ledgerError); err != nil {
		t.Fatalf("query job_runs ledger fields: %v", err)
	}
	if ledgerStatus != domain.StatusExecuting {
		t.Fatalf("job_runs status = %q, want immutable ledger status %q", ledgerStatus, domain.StatusExecuting)
	}
	if ledgerFinishedAt != nil {
		t.Fatalf("job_runs finished_at = %v, want NULL to avoid fat-row churn", *ledgerFinishedAt)
	}
	if ledgerError != nil {
		t.Fatalf("job_runs error = %q, want NULL to avoid fat-row churn", *ledgerError)
	}

	var hotStatus domain.RunStatus
	if err := testDB.Pool.QueryRow(ctx, `SELECT status FROM job_run_state WHERE run_id = $1`, run.ID).Scan(&hotStatus); err != nil {
		t.Fatalf("query job_run_state status: %v", err)
	}
	if hotStatus != domain.StatusExecuting {
		t.Fatalf("job_run_state status = %q, want pre-terminal hot state %q", hotStatus, domain.StatusExecuting)
	}

	var terminalStatus domain.RunStatus
	var terminalFinishedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT status, finished_at
		FROM job_run_terminal_state
		WHERE run_id = $1`,
		run.ID,
	).Scan(&terminalStatus, &terminalFinishedAt); err != nil {
		t.Fatalf("query job_run_terminal_state: %v", err)
	}
	if terminalStatus != domain.StatusDeadLetter {
		t.Fatalf("terminal status = %q, want dead_letter", terminalStatus)
	}
	if !terminalFinishedAt.Equal(finishedAt) {
		t.Fatalf("terminal finished_at = %v, want %v", terminalFinishedAt, finishedAt)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusDeadLetter {
		t.Fatalf("GetRun status = %q, want dead_letter", got.Status)
	}
	if got.Error != "worker gave up" || got.ErrorClass != "terminal" {
		t.Fatalf("GetRun error fields = %q/%q, want worker gave up/terminal", got.Error, got.ErrorClass)
	}

	deadLetters, err := q.ListDeadLetterRuns(ctx, job.ProjectID, 10, nil)
	if err != nil {
		t.Fatalf("ListDeadLetterRuns() error = %v", err)
	}
	if len(deadLetters) != 1 || deadLetters[0].ID != run.ID {
		t.Fatalf("ListDeadLetterRuns = %+v, want run %s", deadLetters, run.ID)
	}
	if deadLetters[0].Error != "worker gave up" || deadLetters[0].ErrorClass != "terminal" {
		t.Fatalf("ListDeadLetterRuns error fields = %q/%q, want worker gave up/terminal", deadLetters[0].Error, deadLetters[0].ErrorClass)
	}

	visible := false
	filtered, err := q.ListDeadLetterRunsFiltered(ctx, job.ProjectID, &job.ID, &visible, 10, nil)
	if err != nil {
		t.Fatalf("ListDeadLetterRunsFiltered() error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != run.ID {
		t.Fatalf("ListDeadLetterRunsFiltered = %+v, want run %s", filtered, run.ID)
	}
	if filtered[0].Error != "worker gave up" || filtered[0].ErrorClass != "terminal" {
		t.Fatalf("ListDeadLetterRunsFiltered error fields = %q/%q, want worker gave up/terminal", filtered[0].Error, filtered[0].ErrorClass)
	}
}

func TestRunStateSplit_NonTerminalRetryErrorFieldsReadFromLifecycleEvent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-retry-error")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, map[string]any{
		"attempt":     2,
		"error":       "execution timed out",
		"error_class": "transient",
		"started_at":  nil,
		"finished_at": nil,
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
	if got.Status != domain.StatusQueued || got.Attempt != 2 {
		t.Fatalf("GetRun status/attempt = %q/%d, want queued/2", got.Status, got.Attempt)
	}
	if got.Error != "execution timed out" {
		t.Fatalf("GetRun error = %q, want execution timed out", got.Error)
	}
	if got.ErrorClass != "transient" {
		t.Fatalf("GetRun error_class = %q, want transient", got.ErrorClass)
	}
}

func TestRunStateSplit_ReplayDeadLetterReactivatesHotState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-dead-letter-replay")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusDeadLetter, map[string]any{
		"error":       "exhausted retries",
		"error_class": "retry",
	}); err != nil {
		t.Fatalf("UpdateRunStatus(dead_letter) error = %v", err)
	}

	var beforeGeneration int64
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT ready_generation
		FROM job_run_state
		WHERE run_id = $1`,
		run.ID,
	).Scan(&beforeGeneration); err != nil {
		t.Fatalf("query ready_generation before replay: %v", err)
	}

	replayed, err := q.ReplayDeadLetterRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ReplayDeadLetterRun() error = %v", err)
	}
	if replayed.Status != domain.StatusQueued {
		t.Fatalf("replayed status = %q, want queued", replayed.Status)
	}
	if replayed.Error != "" || replayed.ErrorClass != "" {
		t.Fatalf("replayed error fields = %q/%q, want empty", replayed.Error, replayed.ErrorClass)
	}

	var terminalRows int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_terminal_state
		WHERE run_id = $1`,
		run.ID,
	).Scan(&terminalRows); err != nil {
		t.Fatalf("count job_run_terminal_state: %v", err)
	}
	if terminalRows != 0 {
		t.Fatalf("terminal rows = %d, want 0 after replay", terminalRows)
	}

	var ledgerStatus domain.RunStatus
	var stateStatus domain.RunStatus
	var afterGeneration int64
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT jr.status, s.status, s.ready_generation
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		run.ID,
	).Scan(&ledgerStatus, &stateStatus, &afterGeneration); err != nil {
		t.Fatalf("query replayed state: %v", err)
	}
	if ledgerStatus != domain.StatusExecuting {
		t.Fatalf("job_runs status = %q, want immutable ledger status %q", ledgerStatus, domain.StatusExecuting)
	}
	if stateStatus != domain.StatusQueued {
		t.Fatalf("job_run_state status = %q, want queued", stateStatus)
	}
	if afterGeneration != beforeGeneration+1 {
		t.Fatalf("ready_generation = %d, want %d", afterGeneration, beforeGeneration+1)
	}
}

func TestRunStateSplit_UnmaskDLQRunUsesReadState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-dlq-unmask")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	finishedAt := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusDeadLetter, map[string]any{
		"finished_at": finishedAt,
		"error":       "permanent failure",
	}); err != nil {
		t.Fatalf("UpdateRunStatus(dead_letter) error = %v", err)
	}

	maskedID, err := q.MaskOldestDLQRow(ctx, job.ProjectID, job.ID)
	if err != nil {
		t.Fatalf("MaskOldestDLQRow() error = %v", err)
	}
	if maskedID != run.ID {
		t.Fatalf("masked run = %q, want %q", maskedID, run.ID)
	}

	depth, err := q.DLQDepth(ctx, job.ProjectID, job.ID)
	if err != nil {
		t.Fatalf("DLQDepth() after mask error = %v", err)
	}
	if depth != 0 {
		t.Fatalf("DLQDepth after mask = %d, want 0", depth)
	}

	if err := q.UnmaskDLQRun(ctx, run.ID); err != nil {
		t.Fatalf("UnmaskDLQRun() error = %v", err)
	}

	var ledgerStatus, readStatus domain.RunStatus
	var visibleUntil *time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT jr.status, s.status, jr.visible_until
		FROM job_runs jr
		JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		run.ID,
	).Scan(&ledgerStatus, &readStatus, &visibleUntil); err != nil {
		t.Fatalf("query unmasked run state: %v", err)
	}
	if ledgerStatus != domain.StatusExecuting {
		t.Fatalf("job_runs status = %q, want immutable executing ledger status", ledgerStatus)
	}
	if readStatus != domain.StatusDeadLetter {
		t.Fatalf("read status = %q, want dead_letter", readStatus)
	}
	if visibleUntil != nil {
		t.Fatalf("visible_until = %v, want NULL", *visibleUntil)
	}

	var visibilityEvents int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_run_visibility_events WHERE run_id = $1`, run.ID).Scan(&visibilityEvents); err != nil {
		t.Fatalf("query visibility events: %v", err)
	}
	if visibilityEvents != 2 {
		t.Fatalf("visibility events = %d, want mask and unmask events", visibilityEvents)
	}

	depth, err = q.DLQDepth(ctx, job.ProjectID, job.ID)
	if err != nil {
		t.Fatalf("DLQDepth() after unmask error = %v", err)
	}
	if depth != 1 {
		t.Fatalf("DLQDepth after unmask = %d, want 1", depth)
	}
}

func TestRunStateSplit_PurgeDLQRunDeletesSplitStateRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-dlq-purge")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusDeadLetter, map[string]any{
		"finished_at": time.Now().UTC().Truncate(time.Microsecond),
		"error":       "terminal failure",
	}); err != nil {
		t.Fatalf("UpdateRunStatus(dead_letter) error = %v", err)
	}
	if err := q.UpdateHeartbeat(ctx, run.ID); err != nil {
		t.Fatalf("UpdateHeartbeat() error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
		SELECT run_id, ready_generation, attempt, NOW()
		FROM job_run_state
		WHERE run_id = $1
		ON CONFLICT DO NOTHING`,
		run.ID,
	); err != nil {
		t.Fatalf("insert active claim: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
		SELECT run_id, ready_generation, attempt, 'purge_test'
		FROM job_run_state
		WHERE run_id = $1
		ON CONFLICT DO NOTHING`,
		run.ID,
	); err != nil {
		t.Fatalf("insert ready event: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at, cleared)
		VALUES ($1, NOW() + INTERVAL '1 minute', 2, NOW(), FALSE)`,
		run.ID,
	); err != nil {
		t.Fatalf("insert retry event: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_priority_events (run_id, priority)
		VALUES ($1, 10)`,
		run.ID,
	); err != nil {
		t.Fatalf("insert priority event: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_visibility_events (run_id, visible_until)
		VALUES ($1, NOW() + INTERVAL '1 hour')`,
		run.ID,
	); err != nil {
		t.Fatalf("insert visibility event: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_cache_versions (run_id, cache_version)
		VALUES ($1, 2)`,
		run.ID,
	); err != nil {
		t.Fatalf("insert cache version: %v", err)
	}

	depth, err := q.DLQDepth(ctx, job.ProjectID, job.ID)
	if err != nil {
		t.Fatalf("DLQDepth() before purge error = %v", err)
	}
	if depth != 1 {
		t.Fatalf("DLQDepth before purge = %d, want 1", depth)
	}

	if err := q.PurgeDLQRun(ctx, run.ID); err != nil {
		t.Fatalf("PurgeDLQRun() error = %v", err)
	}

	for _, table := range []string{
		"job_runs",
		"job_run_state",
		"job_run_terminal_state",
		"job_run_active_claims",
		"job_run_lifecycle_events",
		"job_run_ready_events",
		"job_retries",
		"job_run_priority_events",
		"job_run_visibility_events",
		"job_run_cache_versions",
		"job_run_heartbeats",
	} {
		var count int
		var query string
		switch table {
		case "job_runs":
			query = `SELECT COUNT(*) FROM job_runs WHERE id = $1`
		case "job_run_state":
			query = `SELECT COUNT(*) FROM job_run_state WHERE run_id = $1`
		case "job_run_terminal_state":
			query = `SELECT COUNT(*) FROM job_run_terminal_state WHERE run_id = $1`
		case "job_run_active_claims":
			query = `SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = $1`
		case "job_run_lifecycle_events":
			query = `SELECT COUNT(*) FROM job_run_lifecycle_events WHERE run_id = $1`
		case "job_run_ready_events":
			query = `SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = $1`
		case "job_retries":
			query = `SELECT COUNT(*) FROM job_retries WHERE run_id = $1`
		case "job_run_priority_events":
			query = `SELECT COUNT(*) FROM job_run_priority_events WHERE run_id = $1`
		case "job_run_visibility_events":
			query = `SELECT COUNT(*) FROM job_run_visibility_events WHERE run_id = $1`
		case "job_run_cache_versions":
			query = `SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = $1`
		case "job_run_heartbeats":
			query = `SELECT COUNT(*) FROM job_run_heartbeats WHERE run_id = $1`
		default:
			t.Fatalf("unknown table %q", table)
		}
		if err := testDB.Pool.QueryRow(ctx, query, run.ID).Scan(&count); err != nil {
			t.Fatalf("count %s rows: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows = %d, want 0", table, count)
		}
	}

	depth, err = q.DLQDepth(ctx, job.ProjectID, job.ID)
	if err != nil {
		t.Fatalf("DLQDepth() after purge error = %v", err)
	}
	if depth != 0 {
		t.Fatalf("DLQDepth after purge = %d, want 0", depth)
	}
}

func TestRunStateSplit_ActiveClaimRequeueRetainsInactiveClaimAndBumpsGeneration(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-active-claim-requeue")
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE run_id = $1`, run.ID); err != nil {
		t.Fatalf("mark limited job state: %v", err)
	}

	var beforeGeneration int64
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT ready_generation
		FROM job_run_state
		WHERE run_id = $1`,
		run.ID,
	).Scan(&beforeGeneration); err != nil {
		t.Fatalf("query ready_generation before active claim: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
		VALUES ($1, $2, 1, NOW())`,
		run.ID, beforeGeneration,
	); err != nil {
		t.Fatalf("insert active claim: %v", err)
	}

	claimed, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun claimed: %v", err)
	}
	if claimed.Status != domain.StatusExecuting {
		t.Fatalf("claimed status = %q, want executing", claimed.Status)
	}
	counterUpdatedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 0, $2)
		ON CONFLICT (job_id, concurrency_key)
		DO UPDATE SET count = 0, updated_at = EXCLUDED.updated_at`,
		job.ID, counterUpdatedAt,
	); err != nil {
		t.Fatalf("seed active count row: %v", err)
	}

	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, map[string]any{
		"error":        nil,
		"error_class":  nil,
		"started_at":   nil,
		"finished_at":  nil,
		"heartbeat_at": nil,
	}); err != nil {
		t.Fatalf("UpdateRunStatus(active claim requeue) error = %v", err)
	}

	var ledgerStatus, stateStatus domain.RunStatus
	var afterGeneration int64
	var activeClaims int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT jr.status, s.status, s.ready_generation,
		       (SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = s.run_id)
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		run.ID,
	).Scan(&ledgerStatus, &stateStatus, &afterGeneration, &activeClaims); err != nil {
		t.Fatalf("query active claim requeue state: %v", err)
	}
	if ledgerStatus != domain.StatusQueued {
		t.Fatalf("job_runs status = %q, want immutable queued ledger status", ledgerStatus)
	}
	if stateStatus != domain.StatusQueued {
		t.Fatalf("job_run_state status = %q, want queued", stateStatus)
	}
	if afterGeneration != beforeGeneration+1 {
		t.Fatalf("ready_generation = %d, want %d", afterGeneration, beforeGeneration+1)
	}
	if activeClaims != 1 {
		t.Fatalf("active claims = %d, want retained inactive claim", activeClaims)
	}
	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun after requeue: %v", err)
	}
	if got.Status != domain.StatusQueued {
		t.Fatalf("GetRun status = %q, want queued despite retained inactive claim", got.Status)
	}
	deleted, err := q.DeleteInactiveActiveClaims(ctx, 100)
	if err != nil {
		t.Fatalf("DeleteInactiveActiveClaims() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted inactive active claims = %d, want 1", deleted)
	}
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_active_claims
		WHERE run_id = $1`,
		run.ID,
	).Scan(&activeClaims); err != nil {
		t.Fatalf("query active claims after cleanup: %v", err)
	}
	if activeClaims != 0 {
		t.Fatalf("active claims after cleanup = %d, want 0", activeClaims)
	}
	var afterCounterUpdatedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT updated_at
		FROM job_active_counts
		WHERE job_id = $1 AND concurrency_key = ''`,
		job.ID,
	).Scan(&afterCounterUpdatedAt); err != nil {
		t.Fatalf("query active count timestamp: %v", err)
	}
	if !afterCounterUpdatedAt.Equal(counterUpdatedAt) {
		t.Fatalf("active count updated_at changed on active-claim requeue: got %s want %s", afterCounterUpdatedAt, counterUpdatedAt)
	}

	var lifecycleRows int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_lifecycle_events
		WHERE run_id = $1
		  AND from_status = 'executing'
		  AND to_status = 'queued'`,
		run.ID,
	).Scan(&lifecycleRows); err != nil {
		t.Fatalf("query lifecycle rows: %v", err)
	}
	if lifecycleRows != 1 {
		t.Fatalf("lifecycle rows = %d, want 1", lifecycleRows)
	}
}

func TestRunStateSplit_DeleteInactiveActiveClaimsKeepsCurrentClaimAndDeletesColdRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-active-claim-cleanup")
	current := baseRun(job, newID())
	current.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, current); err != nil {
		t.Fatalf("CreateRun current error = %v", err)
	}
	staleGeneration := baseRun(job, newID())
	staleGeneration.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, staleGeneration); err != nil {
		t.Fatalf("CreateRun stale generation error = %v", err)
	}
	paused := baseRun(job, newID())
	paused.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, paused); err != nil {
		t.Fatalf("CreateRun paused error = %v", err)
	}
	pausedResumed := baseRun(job, newID())
	pausedResumed.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, pausedResumed); err != nil {
		t.Fatalf("CreateRun paused resumed error = %v", err)
	}
	terminal := baseRun(job, newID())
	terminal.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, terminal); err != nil {
		t.Fatalf("CreateRun terminal error = %v", err)
	}

	for _, runID := range []string{current.ID, staleGeneration.ID, paused.ID, pausedResumed.ID, terminal.ID} {
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
			SELECT run_id, ready_generation, attempt, NOW()
			FROM job_run_state
			WHERE run_id = $1`,
			runID,
		); err != nil {
			t.Fatalf("insert active claim for %s: %v", runID, err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET ready_generation = ready_generation + 1
		WHERE run_id = $1`,
		staleGeneration.ID,
	); err != nil {
		t.Fatalf("mark stale generation: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET status = 'paused'
		WHERE run_id = $1`,
		paused.ID,
	); err != nil {
		t.Fatalf("mark paused state: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET status = 'paused'
		WHERE run_id = $1`,
		pausedResumed.ID,
	); err != nil {
		t.Fatalf("mark paused resumed state: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
		SELECT run_id, ready_generation, attempt, 'paused_resume'
		FROM job_run_state
		WHERE run_id = $1`,
		pausedResumed.ID,
	); err != nil {
		t.Fatalf("insert paused resume ready event: %v", err)
	}
	if err := q.UpdateRunStatus(ctx, terminal.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": time.Now().UTC().Truncate(time.Microsecond),
	}); err != nil {
		t.Fatalf("UpdateRunStatus terminal error = %v", err)
	}

	deleted, err := q.DeleteInactiveActiveClaims(ctx, 100)
	if err != nil {
		t.Fatalf("DeleteInactiveActiveClaims() error = %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted inactive active claims = %d, want 3", deleted)
	}
	var remaining int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_run_active_claims`).Scan(&remaining); err != nil {
		t.Fatalf("query remaining active claims: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("remaining active claims = %d, want 2 current claims", remaining)
	}
	rows, err := testDB.Pool.Query(ctx, `SELECT run_id FROM job_run_active_claims ORDER BY run_id`)
	if err != nil {
		t.Fatalf("query remaining active claim runs: %v", err)
	}
	defer rows.Close()

	remainingRunIDs := make([]string, 0, 2)
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			t.Fatalf("scan remaining active claim run: %v", err)
		}
		remainingRunIDs = append(remainingRunIDs, runID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("remaining active claim rows: %v", err)
	}
	wantRemaining := []string{current.ID, pausedResumed.ID}
	slices.Sort(wantRemaining)
	if !slices.Equal(remainingRunIDs, wantRemaining) {
		t.Fatalf("remaining active claim runs = %v, want %v", remainingRunIDs, wantRemaining)
	}
}

func TestRunStateSplit_DeleteInactiveReadyEventsKeepsCurrentGenerationAndDeletesColdRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-ready-event-cleanup")
	current := baseRun(job, newID())
	current.Status = domain.StatusDelayed
	if err := q.CreateRun(ctx, current); err != nil {
		t.Fatalf("CreateRun current error = %v", err)
	}
	staleGeneration := baseRun(job, newID())
	staleGeneration.Status = domain.StatusDelayed
	if err := q.CreateRun(ctx, staleGeneration); err != nil {
		t.Fatalf("CreateRun stale generation error = %v", err)
	}
	terminal := baseRun(job, newID())
	terminal.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, terminal); err != nil {
		t.Fatalf("CreateRun terminal error = %v", err)
	}
	orphanID := newID()

	for _, runID := range []string{current.ID, staleGeneration.ID, terminal.ID} {
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
			SELECT run_id, ready_generation, attempt, 'delayed_due'
			FROM job_run_state
			WHERE run_id = $1`,
			runID,
		); err != nil {
			t.Fatalf("insert current ready event for %s: %v", runID, err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
		VALUES ($1, 1, 1, 'delayed_due')`,
		orphanID,
	); err != nil {
		t.Fatalf("insert orphan ready event: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET ready_generation = ready_generation + 1
		WHERE run_id = $1`,
		staleGeneration.ID,
	); err != nil {
		t.Fatalf("mark stale ready generation: %v", err)
	}
	if err := q.UpdateRunStatus(ctx, terminal.ID, domain.StatusQueued, domain.StatusExecuting, nil); err != nil {
		t.Fatalf("UpdateRunStatus terminal executing error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, terminal.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": time.Now().UTC().Truncate(time.Microsecond),
	}); err != nil {
		t.Fatalf("UpdateRunStatus terminal completed error = %v", err)
	}

	deleted, err := q.DeleteInactiveReadyEvents(ctx, 100)
	if err != nil {
		t.Fatalf("DeleteInactiveReadyEvents() error = %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted inactive ready events = %d, want 3", deleted)
	}
	var remaining int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_run_ready_events`).Scan(&remaining); err != nil {
		t.Fatalf("query remaining ready events: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("remaining ready events = %d, want 1 current-generation event", remaining)
	}
	var remainingRunID string
	if err := testDB.Pool.QueryRow(ctx, `SELECT run_id FROM job_run_ready_events`).Scan(&remainingRunID); err != nil {
		t.Fatalf("query remaining ready event run: %v", err)
	}
	if remainingRunID != current.ID {
		t.Fatalf("remaining ready event run = %s, want %s", remainingRunID, current.ID)
	}

	run, err := q.GetRun(ctx, current.ID)
	if err != nil {
		t.Fatalf("GetRun current: %v", err)
	}
	if run.Status != domain.StatusQueued {
		t.Fatalf("current run read status = %q, want queued from ready overlay", run.Status)
	}
}

func TestRunStateSplit_CompactSupersededRunEventsKeepsLatestRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-event-compaction")
	priorityRun := baseRun(job, newID())
	priorityRun.Priority = 1
	priorityRun.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, priorityRun); err != nil {
		t.Fatalf("CreateRun priority error = %v", err)
	}
	visibilityRun := baseRun(job, newID())
	visibilityRun.Status = domain.StatusDeadLetter
	finishedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	visibilityRun.FinishedAt = &finishedAt
	if err := q.CreateRun(ctx, visibilityRun); err != nil {
		t.Fatalf("CreateRun visibility error = %v", err)
	}

	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_priority_events (run_id, priority)
		VALUES ($1, 10), ($1, 20), ($1, 30)`,
		priorityRun.ID,
	); err != nil {
		t.Fatalf("insert priority events: %v", err)
	}
	maskedAt := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_visibility_events (run_id, visible_until)
		VALUES ($1, NULL), ($1, $2), ($1, NULL)`,
		visibilityRun.ID, maskedAt,
	); err != nil {
		t.Fatalf("insert visibility events: %v", err)
	}

	compactedPriority, err := q.CompactSupersededPriorityEvents(ctx, 1)
	if err != nil {
		t.Fatalf("CompactSupersededPriorityEvents(limit=1) error = %v", err)
	}
	if compactedPriority != 1 {
		t.Fatalf("priority compacted first batch = %d, want 1", compactedPriority)
	}
	compactedPriority, err = q.CompactSupersededPriorityEvents(ctx, 100)
	if err != nil {
		t.Fatalf("CompactSupersededPriorityEvents(limit=100) error = %v", err)
	}
	if compactedPriority != 1 {
		t.Fatalf("priority compacted second batch = %d, want 1", compactedPriority)
	}
	compactedPriority, err = q.CompactSupersededPriorityEvents(ctx, 100)
	if err != nil {
		t.Fatalf("CompactSupersededPriorityEvents(empty) error = %v", err)
	}
	if compactedPriority != 0 {
		t.Fatalf("priority compacted empty batch = %d, want 0", compactedPriority)
	}

	compactedVisibility, err := q.CompactSupersededVisibilityEvents(ctx, 1)
	if err != nil {
		t.Fatalf("CompactSupersededVisibilityEvents(limit=1) error = %v", err)
	}
	if compactedVisibility != 1 {
		t.Fatalf("visibility compacted first batch = %d, want 1", compactedVisibility)
	}
	compactedVisibility, err = q.CompactSupersededVisibilityEvents(ctx, 100)
	if err != nil {
		t.Fatalf("CompactSupersededVisibilityEvents(limit=100) error = %v", err)
	}
	if compactedVisibility != 1 {
		t.Fatalf("visibility compacted second batch = %d, want 1", compactedVisibility)
	}
	compactedVisibility, err = q.CompactSupersededVisibilityEvents(ctx, 100)
	if err != nil {
		t.Fatalf("CompactSupersededVisibilityEvents(empty) error = %v", err)
	}
	if compactedVisibility != 0 {
		t.Fatalf("visibility compacted empty batch = %d, want 0", compactedVisibility)
	}

	var priorityRows, latestPriority int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(priority)
		FROM job_run_priority_events
		WHERE run_id = $1`,
		priorityRun.ID,
	).Scan(&priorityRows, &latestPriority); err != nil {
		t.Fatalf("query priority events after compaction: %v", err)
	}
	if priorityRows != 1 {
		t.Fatalf("priority event rows = %d, want 1", priorityRows)
	}
	if latestPriority != 30 {
		t.Fatalf("latest priority event = %d, want 30", latestPriority)
	}
	gotPriorityRun, err := q.GetRun(ctx, priorityRun.ID)
	if err != nil {
		t.Fatalf("GetRun priority: %v", err)
	}
	if gotPriorityRun.Priority != 30 {
		t.Fatalf("GetRun priority = %d, want latest event priority 30", gotPriorityRun.Priority)
	}

	var visibilityRows int
	var latestVisibleUntil *time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(visible_until)
		FROM job_run_visibility_events
		WHERE run_id = $1`,
		visibilityRun.ID,
	).Scan(&visibilityRows, &latestVisibleUntil); err != nil {
		t.Fatalf("query visibility events after compaction: %v", err)
	}
	if visibilityRows != 1 {
		t.Fatalf("visibility event rows = %d, want 1", visibilityRows)
	}
	if latestVisibleUntil != nil {
		t.Fatalf("latest visible_until = %v, want nil unmasked event", latestVisibleUntil)
	}

	maskedRuns, err := q.ListDeadLetterRunsFiltered(ctx, visibilityRun.ProjectID, nil, ptr(true), 10, nil)
	if err != nil {
		t.Fatalf("ListDeadLetterRuns(masked=true) error = %v", err)
	}
	if len(maskedRuns) != 0 {
		t.Fatalf("masked dead-letter runs len = %d, want 0 after latest unmask event", len(maskedRuns))
	}
	unmaskedRuns, err := q.ListDeadLetterRunsFiltered(ctx, visibilityRun.ProjectID, nil, ptr(false), 10, nil)
	if err != nil {
		t.Fatalf("ListDeadLetterRuns(masked=false) error = %v", err)
	}
	if len(unmaskedRuns) != 1 || unmaskedRuns[0].ID != visibilityRun.ID {
		t.Fatalf("unmasked dead-letter runs = %+v, want run %s", unmaskedRuns, visibilityRun.ID)
	}
}
