//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestRunStateSplit_UpdateRunStatusDoesNotTouchLedgerStateColumns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-update")
	run := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run))

	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.StatusQueued,

		domain.StatusDequeued,

		map[string]any{"started_at": startedAt}))

	var ledgerStatus domain.RunStatus
	var ledgerStartedAt *time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT status, started_at
		FROM job_runs
		WHERE id = $1`,

		run.ID).
		Scan(&ledgerStatus, &ledgerStartedAt))
	require.Equal(t, domain.
		StatusQueued,
		ledgerStatus,
	)
	require.Nil(t, ledgerStartedAt)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusDequeued,
		got.
			Status)
	require.False(t, got.StartedAt ==
		nil || !got.
		StartedAt.Equal(startedAt))

}

func TestRunStateSplit_UpdateRunStatusForActiveRunKeepsLedgerStateImmutable(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-active-update")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	run.IdempotencyKey = newID()
	require.NoError(t, q.CreateRun(ctx,
		run))

	finishedAt := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, q.UpdateRunStatusForActiveRun(ctx, run.ID,
		domain.StatusExecuting,

		domain.
			StatusCompleted, map[string]any{"finished_at": finishedAt, "result": json.RawMessage(`{"ok":true}`)}, run.Attempt,
	))

	var ledgerStatus domain.RunStatus
	var ledgerFinishedAt *time.Time
	var ledgerResult []byte
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT status, finished_at, result
		FROM job_runs
		WHERE id = $1`,

		run.
			ID).Scan(&ledgerStatus, &ledgerFinishedAt, &ledgerResult))
	require.Equal(t, domain.
		StatusExecuting,
		ledgerStatus,
	)
	require.Nil(t, ledgerFinishedAt)
	require.Nil(t, ledgerResult)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusCompleted,
		got.
			Status)
	require.False(t, got.FinishedAt ==
		nil || !got.
		FinishedAt.Equal(finishedAt))
	require.True(t, jsonEqual(got.Result,
		[]byte(`{"ok":true}`)))

	cachedRun, _, err := q.GetRunWithCacheVersion(ctx, run.ID)
	require.NoError(t, err)
	require.True(t, jsonEqual(cachedRun.
		Result,
		[]byte(`{"ok":true}`)))

	byJob, err := q.ListRunsByJob(ctx, job.ID, 10, 0)
	require.NoError(t, err)
	require.False(t, len(byJob) != 1 ||
		!jsonEqual(byJob[0].Result,
			[]byte(
				`{"ok":true}`,
			)))

	byIdempotency, err := q.GetRunByIdempotencyKey(ctx, job.ID, run.IdempotencyKey)
	require.NoError(t, err)
	require.NotNil(t, byIdempotency)
	require.True(t, jsonEqual(byIdempotency.
		Result,
		[]byte(`{"ok":true}`)))

	finished, err := q.ListFinishedRunsSince(ctx, job.ProjectID, finishedAt.Add(-time.Minute), "", 10)
	require.NoError(t, err)
	require.False(t, len(finished) !=
		1 || finished[0].ID != run.
		ID)
	require.True(t, jsonEqual(finished[0].Result,
		[]byte(`{"ok":true}`)))

}

func TestRunStateSplit_TerminalErrorFieldsReadFromLifecycleEvent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-terminal-error")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))

	finishedAt := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.StatusExecuting,

		domain.StatusFailed,

		map[string]any{"finished_at": finishedAt, "error": "worker failed", "error_class": "server"}))

	var ledgerError *string
	var ledgerErrorClass *string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT error, error_class
		FROM job_runs
		WHERE id = $1`,

		run.ID).
		Scan(&ledgerError, &ledgerErrorClass))
	require.Nil(t, ledgerError)
	require.Nil(t, ledgerErrorClass)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "worker failed",

		got.Error)
	require.Equal(t, "server",

		got.ErrorClass,
	)

	status := domain.StatusFailed
	errorClass := "server"
	listed, err := q.ListRunsByProject(ctx, job.ProjectID, &status, nil, nil, nil, nil, nil, nil, &errorClass, 10, nil)
	require.NoError(t, err)
	require.False(t, len(listed) != 1 ||
		listed[0].ID != run.ID)
	require.False(t, listed[0].Error !=
		"worker failed" ||
		listed[0].ErrorClass !=
			"server",
	)

	filtered, err := q.ListRunsByProjectFiltered(ctx, job.ProjectID, nil, []domain.RunStatus{domain.StatusFailed}, "", "", nil, nil, nil, nil, nil, nil, nil, &errorClass, 10, nil)
	require.NoError(t, err)
	require.False(t, len(filtered) !=
		1 || filtered[0].ID != run.
		ID)
	require.False(t, filtered[0].Error !=
		"worker failed" ||
		filtered[0].ErrorClass !=
			"server",
	)

	finished, err := q.ListFinishedRunsSince(ctx, job.ProjectID, finishedAt.Add(-time.Minute), "", 10)
	require.NoError(t, err)
	require.False(t, len(finished) !=
		1 || finished[0].ID != run.
		ID)
	require.False(t, finished[0].Error !=
		"worker failed" ||
		finished[0].ErrorClass !=
			"server",
	)

}

func TestRunStateSplit_UpdateRunStatusReturningOldDoesNotTouchLedgerStateColumns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-returning-old")
	run := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run))

	oldStatus, err := q.UpdateRunStatusReturningOld(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, nil)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		oldStatus,
	)

	var ledgerStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_runs WHERE id = $1`,

		run.
			ID).Scan(&ledgerStatus),
	)
	require.Equal(t, domain.
		StatusQueued,
		ledgerStatus,
	)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusDequeued,
		got.
			Status)

}

func TestRunStateSplit_DeadLetterTransitionUsesColdTerminalState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-dead-letter")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))

	finishedAt := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.StatusExecuting,

		domain.StatusDeadLetter,

		map[string]any{"error": "worker gave up", "error_class": "terminal", "finished_at": finishedAt}))

	var ledgerStatus domain.RunStatus
	var ledgerFinishedAt *time.Time
	var ledgerError *string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT status, finished_at, error
		FROM job_runs
		WHERE id = $1`,

		run.
			ID).Scan(&ledgerStatus, &ledgerFinishedAt, &ledgerError))
	require.Equal(t, domain.
		StatusExecuting,
		ledgerStatus,
	)
	require.Nil(t, ledgerFinishedAt)
	require.Nil(t, ledgerError)

	var hotStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_run_state WHERE run_id = $1`,

		run.ID).Scan(&hotStatus))
	require.Equal(t, domain.
		StatusExecuting,
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

		run.ID).Scan(&terminalStatus, &terminalFinishedAt))
	require.Equal(t, domain.
		StatusDeadLetter,
		terminalStatus,
	)
	require.True(t, terminalFinishedAt.
		Equal(finishedAt))

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusDeadLetter,
		got.
			Status)
	require.False(t, got.Error !=
		"worker gave up" ||
		got.ErrorClass !=
			"terminal",
	)

	deadLetters, err := q.ListDeadLetterRuns(ctx, job.ProjectID, 10, nil)
	require.NoError(t, err)
	require.False(t, len(deadLetters) !=
		1 || deadLetters[0].ID !=
		run.ID)
	require.False(t, deadLetters[0].Error !=
		"worker gave up" ||
		deadLetters[0].ErrorClass !=
			"terminal",
	)

	visible := false
	filtered, err := q.ListDeadLetterRunsFiltered(ctx, job.ProjectID, &job.ID, &visible, 10, nil)
	require.NoError(t, err)
	require.False(t, len(filtered) !=
		1 || filtered[0].ID != run.
		ID)
	require.False(t, filtered[0].Error !=
		"worker gave up" ||
		filtered[0].ErrorClass !=
			"terminal",
	)

}

func TestRunStateSplit_NonTerminalRetryErrorFieldsReadFromLifecycleEvent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-retry-error")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.StatusExecuting,

		domain.StatusQueued,

		map[string]any{"attempt": 2, "error": "execution timed out", "error_class": "transient", "started_at": nil, "finished_at": nil}))

	var ledgerError *string
	var ledgerErrorClass *string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT error, error_class
		FROM job_runs
		WHERE id = $1`,

		run.ID).
		Scan(&ledgerError, &ledgerErrorClass))
	require.Nil(t, ledgerError)
	require.Nil(t, ledgerErrorClass)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.False(t, got.Status !=
		domain.
			StatusQueued ||
		got.Attempt !=
			2)
	require.Equal(t, "execution timed out",

		got.
			Error)
	require.Equal(t, "transient",

		got.
			ErrorClass,
	)

}

func TestRunStateSplit_ReplayDeadLetterReactivatesHotState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-dead-letter-replay")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.StatusExecuting,

		domain.StatusDeadLetter,

		map[string]any{"error": "exhausted retries", "error_class": "retry"}))

	var beforeGeneration int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT ready_generation
		FROM job_run_state
		WHERE run_id = $1`,

		run.
			ID).Scan(&beforeGeneration))

	replayed, err := q.ReplayDeadLetterRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		replayed.
			Status)
	require.False(t, replayed.
		Error !=
		"" || replayed.
		ErrorClass !=
		"")

	var terminalRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_terminal_state
		WHERE run_id = $1`,

		run.
			ID).Scan(&terminalRows))
	require.EqualValues(t, 0, terminalRows)

	var ledgerStatus domain.RunStatus
	var stateStatus domain.RunStatus
	var afterGeneration int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status, s.ready_generation
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,

		run.ID).Scan(&ledgerStatus,
		&stateStatus, &afterGeneration,
	))
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

func TestRunStateSplit_UnmaskDLQRunUsesReadState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-dlq-unmask")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))

	finishedAt := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.StatusExecuting,

		domain.StatusDeadLetter,

		map[string]any{"finished_at": finishedAt, "error": "permanent failure"}))

	maskedID, err := q.MaskOldestDLQRow(ctx, job.ProjectID, job.ID)
	require.NoError(t, err)
	require.Equal(t, run.ID,

		maskedID,
	)

	depth, err := q.DLQDepth(ctx, job.ProjectID, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, depth)
	require.NoError(t, q.UnmaskDLQRun(ctx, run.ID))

	var ledgerStatus, readStatus domain.RunStatus
	var visibleUntil *time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status, jr.visible_until
		FROM job_runs jr
		JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,

		run.ID).Scan(&ledgerStatus,
		&readStatus, &visibleUntil,
	))
	require.Equal(t, domain.
		StatusExecuting,
		ledgerStatus,
	)
	require.Equal(t, domain.
		StatusDeadLetter,
		readStatus,
	)
	require.Nil(t, visibleUntil)

	var visibilityEvents int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_run_visibility_events WHERE run_id = $1`,

		run.
			ID,
	).
		Scan(&visibilityEvents))
	require.EqualValues(t, 2, visibilityEvents)

	depth, err = q.DLQDepth(ctx, job.ProjectID, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, depth)

}

func TestRunStateSplit_PurgeDLQRunDeletesSplitStateRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-state-dlq-purge")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.StatusExecuting,

		domain.StatusDeadLetter,

		map[string]any{"finished_at": time.Now().UTC().Truncate(time.Microsecond), "error": "terminal failure"}))
	require.NoError(t, q.UpdateHeartbeat(ctx, run.
		ID))

	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
		SELECT run_id, ready_generation, attempt, NOW()
		FROM job_run_state
		WHERE run_id = $1
		ON CONFLICT DO NOTHING`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert active claim: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
		SELECT run_id, ready_generation, attempt, 'purge_test'
		FROM job_run_state
		WHERE run_id = $1
		ON CONFLICT DO NOTHING`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert ready event: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at, cleared)
		VALUES ($1, NOW() + INTERVAL '1 minute', 2, NOW(), FALSE)`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert retry event: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_priority_events (run_id, priority)
		VALUES ($1, 10)`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert priority event: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_visibility_events (run_id, visible_until)
		VALUES ($1, NOW() + INTERVAL '1 hour')`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert visibility event: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_cache_versions (run_id, cache_version)
		VALUES ($1, 2)`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert cache version: %v", err)
	}

	depth, err := q.DLQDepth(ctx, job.ProjectID, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, depth)
	require.NoError(t, q.PurgeDLQRun(
		ctx, run.ID,
	))

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
			require.Failf(t, "test failure", "unknown table %q", table)
		}
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			query, run.ID).Scan(&count))
		require.EqualValues(t, 0, count)

	}

	depth, err = q.DLQDepth(ctx, job.ProjectID, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, depth)

}

func TestRunStateSplit_ActiveClaimRequeueRetainsInactiveClaimAndBumpsGeneration(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-active-claim-requeue")
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		run))

	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE run_id = $1`, run.ID); err != nil {
		require.Failf(t, "test failure",

			"mark limited job state: %v", err)
	}

	var beforeGeneration int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT ready_generation
		FROM job_run_state
		WHERE run_id = $1`,

		run.
			ID).Scan(&beforeGeneration))

	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
		VALUES ($1, $2, 1, NOW())`,
		run.ID, beforeGeneration,
	); err != nil {
		require.Failf(t, "test failure",

			"insert active claim: %v", err)
	}

	claimed, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusExecuting,
		claimed.
			Status)

	counterUpdatedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_active_counts (job_id, concurrency_key, count, updated_at)
		VALUES ($1, '', 0, $2)
		ON CONFLICT (job_id, concurrency_key)
		DO UPDATE SET count = 0, updated_at = EXCLUDED.updated_at`,
		job.ID, counterUpdatedAt,
	); err != nil {
		require.Failf(t, "test failure",

			"seed active count row: %v", err)
	}
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.StatusExecuting,

		domain.StatusQueued,

		map[string]any{"error": nil,
			"error_class": nil, "started_at": nil, "finished_at": nil, "heartbeat_at": nil}))

	var ledgerStatus, stateStatus domain.RunStatus
	var afterGeneration int64
	var activeClaims int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status, s.ready_generation,
		       (SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = s.run_id)
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,

		run.ID).Scan(&ledgerStatus,
		&stateStatus, &afterGeneration, &activeClaims))
	require.Equal(t, domain.
		StatusQueued,
		ledgerStatus,
	)
	require.Equal(t, domain.
		StatusQueued,
		stateStatus,
	)
	require.Equal(t, beforeGeneration+
		1, afterGeneration,
	)
	require.EqualValues(t, 1, activeClaims)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		got.Status,
	)

	deleted, err := q.DeleteInactiveActiveClaims(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_active_claims
		WHERE run_id = $1`,

		run.
			ID).Scan(&activeClaims))
	require.EqualValues(t, 0, activeClaims)

	var afterCounterUpdatedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT updated_at
		FROM job_active_counts
		WHERE job_id = $1 AND concurrency_key = ''`,

		job.ID).Scan(&afterCounterUpdatedAt))
	require.True(t, afterCounterUpdatedAt.
		Equal(
			counterUpdatedAt),
	)

	var lifecycleRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_lifecycle_events
		WHERE run_id = $1
		  AND from_status = 'executing'
		  AND to_status = 'queued'`,

		run.ID).Scan(&lifecycleRows))
	require.EqualValues(t, 1, lifecycleRows)

}

func TestRunStateSplit_DeleteInactiveActiveClaimsKeepsCurrentClaimAndDeletesColdRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-active-claim-cleanup")
	current := baseRun(job, newID())
	current.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		current),
	)

	staleGeneration := baseRun(job, newID())
	staleGeneration.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		staleGeneration,
	))

	paused := baseRun(job, newID())
	paused.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		paused))

	pausedResumed := baseRun(job, newID())
	pausedResumed.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		pausedResumed,
	))

	terminal := baseRun(job, newID())
	terminal.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		terminal,
	))

	for _, runID := range []string{current.ID, staleGeneration.ID, paused.ID, pausedResumed.ID, terminal.ID} {
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
			SELECT run_id, ready_generation, attempt, NOW()
			FROM job_run_state
			WHERE run_id = $1`,
			runID,
		); err != nil {
			require.Failf(t, "test failure",

				"insert active claim for %s: %v", runID, err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET ready_generation = ready_generation + 1
		WHERE run_id = $1`,
		staleGeneration.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"mark stale generation: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET status = 'paused'
		WHERE run_id = $1`,
		paused.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"mark paused state: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET status = 'paused'
		WHERE run_id = $1`,
		pausedResumed.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"mark paused resumed state: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
		SELECT run_id, ready_generation, attempt, 'paused_resume'
		FROM job_run_state
		WHERE run_id = $1`,
		pausedResumed.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert paused resume ready event: %v", err)
	}
	require.NoError(t, q.UpdateRunStatus(ctx, terminal.
		ID, domain.
		StatusExecuting,
		domain.
			StatusCompleted,

		map[string]any{"finished_at": time.Now().UTC().Truncate(time.Microsecond)}))

	deleted, err := q.DeleteInactiveActiveClaims(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 3, deleted)

	var remaining int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_run_active_claims`,
	).
		Scan(&remaining))
	require.EqualValues(t, 2, remaining)

	rows, err := testDB.Pool.Query(ctx, `SELECT run_id FROM job_run_active_claims ORDER BY run_id`)
	require.NoError(t, err)

	defer rows.Close()

	remainingRunIDs := make([]string, 0, 2)
	for rows.Next() {
		var runID string
		require.NoError(t, rows.
			Scan(&runID))

		remainingRunIDs = append(remainingRunIDs, runID)
	}
	require.NoError(t, rows.
		Err())

	wantRemaining := []string{current.ID, pausedResumed.ID}
	slices.Sort(wantRemaining)
	require.True(t, slices.
		Equal(remainingRunIDs,

			wantRemaining))

}

func TestRunStateSplit_DeleteInactiveReadyEventsKeepsCurrentGenerationAndDeletesColdRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-ready-event-cleanup")
	current := baseRun(job, newID())
	current.Status = domain.StatusDelayed
	require.NoError(t, q.CreateRun(ctx,
		current),
	)

	staleGeneration := baseRun(job, newID())
	staleGeneration.Status = domain.StatusDelayed
	require.NoError(t, q.CreateRun(ctx,
		staleGeneration,
	))

	terminal := baseRun(job, newID())
	terminal.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		terminal,
	))

	orphanID := newID()

	for _, runID := range []string{current.ID, staleGeneration.ID, terminal.ID} {
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
			SELECT run_id, ready_generation, attempt, 'delayed_due'
			FROM job_run_state
			WHERE run_id = $1`,
			runID,
		); err != nil {
			require.Failf(t, "test failure",

				"insert current ready event for %s: %v", runID, err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
		VALUES ($1, 1, 1, 'delayed_due')`,
		orphanID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert orphan ready event: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET ready_generation = ready_generation + 1
		WHERE run_id = $1`,
		staleGeneration.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"mark stale ready generation: %v", err)
	}
	require.NoError(t, q.UpdateRunStatus(ctx, terminal.
		ID, domain.
		StatusQueued,
		domain.
			StatusExecuting,

		nil))
	require.NoError(t, q.UpdateRunStatus(ctx, terminal.
		ID, domain.
		StatusExecuting,
		domain.
			StatusCompleted,

		map[string]any{"finished_at": time.Now().UTC().Truncate(time.Microsecond)}))

	deleted, err := q.DeleteInactiveReadyEvents(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 3, deleted)

	var remaining int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_run_ready_events`,
	).
		Scan(&remaining))
	require.EqualValues(t, 1, remaining)

	var remainingRunID string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT run_id FROM job_run_ready_events`,
	).Scan(&remainingRunID))
	require.Equal(t, current.
		ID, remainingRunID,
	)

	run, err := q.GetRun(ctx, current.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		run.Status,
	)

}

func TestRunStateSplit_CompactSupersededRunEventsKeepsLatestRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-event-compaction")
	priorityRun := baseRun(job, newID())
	priorityRun.Priority = 1
	priorityRun.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		priorityRun,
	))

	visibilityRun := baseRun(job, newID())
	visibilityRun.Status = domain.StatusDeadLetter
	finishedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	visibilityRun.FinishedAt = &finishedAt
	require.NoError(t, q.CreateRun(ctx,
		visibilityRun,
	))

	cacheRun := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		cacheRun,
	))

	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_priority_events (run_id, priority)
		VALUES ($1, 10), ($1, 20), ($1, 30)`,
		priorityRun.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert priority events: %v", err)
	}
	maskedAt := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_visibility_events (run_id, visible_until)
		VALUES ($1, NULL), ($1, $2), ($1, NULL)`,
		visibilityRun.ID, maskedAt,
	); err != nil {
		require.Failf(t, "test failure",

			"insert visibility events: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_cache_versions (run_id, cache_version)
		VALUES ($1, 41), ($1, 42), ($1, 43)`,
		cacheRun.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"insert cache versions: %v", err)
	}

	compactedPriority, err := q.CompactSupersededPriorityEvents(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, compactedPriority)

	compactedPriority, err = q.CompactSupersededPriorityEvents(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, compactedPriority)

	compactedPriority, err = q.CompactSupersededPriorityEvents(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 0, compactedPriority)

	compactedVisibility, err := q.CompactSupersededVisibilityEvents(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, compactedVisibility)

	compactedVisibility, err = q.CompactSupersededVisibilityEvents(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, compactedVisibility)

	compactedVisibility, err = q.CompactSupersededVisibilityEvents(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 0, compactedVisibility)

	compactedCacheVersions, err := q.CompactSupersededRunCacheVersions(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, compactedCacheVersions)

	compactedCacheVersions, err = q.CompactSupersededRunCacheVersions(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, compactedCacheVersions)

	compactedCacheVersions, err = q.CompactSupersededRunCacheVersions(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 0, compactedCacheVersions)

	var priorityRows, latestPriority int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), MAX(priority)
		FROM job_run_priority_events
		WHERE run_id = $1`,

		priorityRun.ID).Scan(&priorityRows, &latestPriority))
	require.EqualValues(t, 1, priorityRows)
	require.EqualValues(t, 30, latestPriority)

	gotPriorityRun, err := q.GetRun(ctx, priorityRun.ID)
	require.NoError(t, err)
	require.EqualValues(t, 30, gotPriorityRun.
		Priority,
	)

	var visibilityRows int
	var latestVisibleUntil *time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), MAX(visible_until)
		FROM job_run_visibility_events
		WHERE run_id = $1`,

		visibilityRun.ID).Scan(&visibilityRows, &latestVisibleUntil))
	require.EqualValues(t, 1, visibilityRows)
	require.Nil(t, latestVisibleUntil)

	maskedRuns, err := q.ListDeadLetterRunsFiltered(ctx, visibilityRun.ProjectID, nil, ptr(true), 10, nil)
	require.NoError(t, err)
	require.Len(t, maskedRuns,

		0)

	unmaskedRuns, err := q.ListDeadLetterRunsFiltered(ctx, visibilityRun.ProjectID, nil, ptr(false), 10, nil)
	require.NoError(t, err)
	require.False(t, len(unmaskedRuns) != 1 || unmaskedRuns[0].ID !=
		visibilityRun.
			ID)

	var cacheVersionRows int
	var latestCacheVersion int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cache_version ORDER BY id DESC))[1], 0)
		FROM job_run_cache_versions
		WHERE run_id = $1`,

		cacheRun.ID).Scan(&cacheVersionRows,
		&latestCacheVersion,
	))
	require.EqualValues(t, 1, cacheVersionRows)
	require.EqualValues(t, 43, latestCacheVersion)

	gotCacheRun, version, err := q.GetRunWithCacheVersion(ctx, cacheRun.ID)
	require.NoError(t, err)
	require.False(t, gotCacheRun.
		CacheVersion !=
		43 || version !=
		43)

}
