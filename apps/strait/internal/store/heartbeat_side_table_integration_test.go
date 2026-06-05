//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests for the unlogged heartbeat side table.

func TestHeartbeatSideTable_UpsertCreatesAndUpdates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "test-hb-upsert-" + newID()
	require.NoError(t, q.UpsertHeartbeatSideTable(ctx, runID))

	var ts1 time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT heartbeat_at
		FROM job_run_heartbeats
		WHERE run_id = $1 AND cleared = FALSE
		ORDER BY id DESC
		LIMIT 1`,

		runID).Scan(&ts1))

	// Ensure observable time passes before the second upsert.
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, q.UpsertHeartbeatSideTable(ctx, runID))

	var ts2 time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT heartbeat_at
		FROM job_run_heartbeats
		WHERE run_id = $1 AND cleared = FALSE
		ORDER BY id DESC
		LIMIT 1`,

		runID).Scan(&ts2))
	assert.True(t, ts2.After(ts1))

	var rawRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_run_heartbeats WHERE run_id = $1`,

		runID).Scan(&rawRows))
	require.EqualValues(t, 2, rawRows)

}

func TestHeartbeatSideTable_BatchUpsertAll(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	ids := make([]string, 100)
	for i := range ids {
		ids[i] = "hb-batch-" + newID()
	}
	require.NoError(t, q.BatchUpsertHeartbeatSideTable(ctx,
		ids))

	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM (
			SELECT DISTINCT ON (run_id) run_id, cleared
			FROM job_run_heartbeats
			WHERE run_id = ANY($1)
			ORDER BY run_id, id DESC
		) latest
		WHERE cleared = FALSE`,
		ids).Scan(&count))
	assert.EqualValues(t, 100, count)

}

func TestHeartbeatSideTable_BatchEmptyNoOp(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	require.NoError(t, q.BatchUpsertHeartbeatSideTable(ctx,
		nil))
	require.NoError(t, q.BatchUpsertHeartbeatSideTable(ctx,
		[]string{}))

	// Empty batch must succeed and not issue a query.

}

func TestHeartbeatSideTable_DeleteRemoves(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	ids := []string{"hb-del-1-" + newID(), "hb-del-2-" + newID(), "hb-del-3-" + newID()}
	require.NoError(t, q.BatchUpsertHeartbeatSideTable(ctx,
		ids))
	require.NoError(t, q.DeleteHeartbeatSideTable(ctx, ids[:2]))

	// Delete two of them.

	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM (
			SELECT DISTINCT ON (run_id) run_id, cleared
			FROM job_run_heartbeats
			WHERE run_id = ANY($1)
			ORDER BY run_id, id DESC
		) latest
		WHERE cleared = FALSE`,
		ids).Scan(&count))
	assert.EqualValues(t, 1, count)

}

func TestHeartbeatSideTable_DeleteSkipsAbsentAndAlreadyClearedRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "hb-clear-idempotent-" + newID()
	missingID := "hb-clear-missing-" + newID()
	require.NoError(t, q.UpsertHeartbeatSideTable(ctx, runID))
	require.NoError(t, q.DeleteHeartbeatSideTable(ctx, []string{runID,

		missingID}))
	require.NoError(t, q.DeleteHeartbeatSideTable(ctx, []string{runID,

		missingID}))

	var rawRows int
	var latestCleared bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_run_heartbeats
		WHERE run_id = $1`,

		runID).Scan(&rawRows, &latestCleared))
	require.EqualValues(t, 2, rawRows)
	require.True(t, latestCleared)
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_heartbeats
		WHERE run_id = $1`,

		missingID).Scan(&rawRows))
	require.EqualValues(t, 0, rawRows)

}

func TestHeartbeatSideTable_CompactSupersededKeepsLatestRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	firstID := "hb-compact-a-" + newID()
	secondID := "hb-compact-b-" + newID()
	for range 3 {
		require.NoError(t, q.BatchUpsertHeartbeatSideTable(ctx,
			[]string{firstID, secondID}))

	}
	require.NoError(t, q.DeleteHeartbeatSideTable(ctx, []string{secondID}))

	compacted, err := q.CompactSupersededHeartbeats(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 5, compacted)

	rows, err := testDB.Pool.Query(ctx, `
		SELECT run_id, COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_run_heartbeats
		WHERE run_id = ANY($1)
		GROUP BY run_id`, []string{firstID, secondID})
	require.NoError(t, err)

	defer rows.Close()
	seen := 0
	for rows.Next() {
		var runID string
		var rawRows int
		var latestCleared bool
		require.NoError(t, rows.
			Scan(&runID,
				&rawRows,
				&latestCleared,
			))

		seen++
		require.EqualValues(t, 1, rawRows)
		require.False(t, runID ==
			firstID &&
			latestCleared,
		)
		require.False(t, runID ==
			secondID &&
			!latestCleared,
		)

	}
	require.NoError(t, rows.
		Err())
	require.EqualValues(t, 2, seen)

}

func TestHeartbeatSideTable_DeleteOrphanedUsesSplitRunState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-hb-gc-split-state")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpdateHeartbeat(ctx, run.
		ID))
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.
		StatusExecuting,

		domain.StatusCompleted,

		map[string]any{"finished_at": time.Now()}))
	require.NoError(t, q.UpdateHeartbeat(ctx, run.
		ID))

	var ledgerStatus, readStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status
		FROM job_runs jr
		JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,

		run.ID).Scan(&ledgerStatus, &readStatus))
	require.False(t, ledgerStatus !=
		domain.StatusExecuting ||
		readStatus !=
			domain.StatusCompleted,
	)

	deleted, err := q.DeleteOrphanedHeartbeats(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM (
			SELECT cleared
			FROM job_run_heartbeats
			WHERE run_id = $1
			ORDER BY id DESC
			LIMIT 1
		) latest
		WHERE cleared = FALSE`, run.ID).Scan(&count))
	require.EqualValues(t, 0, count)

}

func TestHeartbeatSideTable_DeleteOrphanedKeepsWaitingRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-hb-gc-waiting")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.
		StatusExecuting,

		domain.StatusWaiting,

		nil))
	require.NoError(t, q.UpdateHeartbeatForActiveRun(ctx, run.
		ID, run.
		Attempt))

	deleted, err := q.DeleteOrphanedHeartbeats(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 0, deleted)

	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM (
			SELECT cleared
			FROM job_run_heartbeats
			WHERE run_id = $1
			ORDER BY id DESC
			LIMIT 1
		) latest
		WHERE cleared = FALSE`, run.ID).Scan(&count))
	require.EqualValues(t, 1, count)

}

func TestHeartbeatSideTable_StaleDetection(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	freshID := "hb-fresh-" + newID()
	staleID := "hb-stale-" + newID()
	require.NoError(t, q.BatchUpsertHeartbeatSideTable(ctx,
		[]string{freshID, staleID}))

	// Backdate the stale one.
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		VALUES ($1, NOW() - INTERVAL '5 minutes', FALSE)`, staleID); err != nil {
		require.Failf(t, "test failure",

			"backdate: %v", err)
	}

	stale, err := q.StaleHeartbeatSideTable(ctx, 1*time.Minute, 100)
	require.NoError(t, err)
	assert.False(t, len(stale) != 1 ||
		stale[0] !=
			staleID)

}

func TestHeartbeatSideTable_ListStaleRunsPrefersSideTable(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-hb-side-stale-runs")
	old := time.Now().UTC().Add(-10 * time.Minute)
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	run.StartedAt = &old
	run.HeartbeatAt = &old
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpsertHeartbeatSideTable(ctx, run.
		ID))

	runs, err := q.ListStaleRuns(ctx, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, runs, 0)

	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		VALUES ($1, NOW() - INTERVAL '10 minutes', FALSE)`, run.ID); err != nil {
		require.Failf(t, "test failure",

			"backdate side-table heartbeat: %v", err)
	}

	runs, err = q.ListStaleRuns(ctx, 5*time.Minute)
	require.NoError(t, err)
	require.False(t, len(runs) != 1 ||
		runs[0].ID !=
			run.ID,
	)

}

func TestHeartbeatSideTable_UpdateHeartbeatDoesNotTouchJobRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-hb-update-side-table")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpdateHeartbeat(ctx, run.
		ID))

	var ledgerHeartbeat *time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT heartbeat_at FROM job_runs WHERE id = $1`,

		run.ID).Scan(&ledgerHeartbeat))
	require.Nil(t, ledgerHeartbeat)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.NotNil(t, got.HeartbeatAt)

}

func TestHeartbeatSideTable_UpdateHeartbeatForActiveRunUsesRunState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-hb-active-state")
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		run))

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET status = $1, started_at = NOW()
		WHERE run_id = $2`,
		domain.StatusExecuting,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"move state to executing: %v", err)
	}
	require.NoError(t, q.UpdateHeartbeatForActiveRun(ctx, run.
		ID, run.
		Attempt))

	var ledgerHeartbeat *time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT heartbeat_at FROM job_runs WHERE id = $1`,

		run.ID).Scan(&ledgerHeartbeat))
	require.Nil(t, ledgerHeartbeat)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusExecuting,
		got.
			Status)
	require.NotNil(t, got.HeartbeatAt)

}

func TestHeartbeatSideTable_UnloggedSurvivesTruncate(t *testing.T) {
	// Crash recovery simulation: truncate the unlogged table and verify
	// the system continues to accept writes. This mirrors what happens
	// after a Postgres crash.
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "hb-crash-" + newID()
	require.NoError(t, q.UpsertHeartbeatSideTable(ctx, runID))

	if _, err := testDB.Pool.Exec(ctx, "TRUNCATE job_run_heartbeats"); err != nil {
		require.Failf(t, "test failure",

			"truncate: %v", err)
	}
	require.NoError(t, q.UpsertHeartbeatSideTable(ctx, runID))

	// Must succeed after truncate.

}
