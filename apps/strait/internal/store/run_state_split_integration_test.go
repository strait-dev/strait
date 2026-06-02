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

func TestRunStateSplit_ActiveClaimRequeueDeletesClaimAndBumpsGeneration(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-active-claim-requeue")
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
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
	if activeClaims != 0 {
		t.Fatalf("active claims = %d, want 0", activeClaims)
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
