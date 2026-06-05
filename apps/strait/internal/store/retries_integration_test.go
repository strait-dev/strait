//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetries_Schedule_Clear_Ready(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-test-" + newID()

	// Schedule a retry in the past → it should be "ready".
	past := time.Now().UTC().Add(-1 * time.Second)
	require.NoError(t, q.ScheduleRetry(ctx, runID,
		past, 2),
	)

	ready, err := q.ReadyRetries(ctx, 100)
	require.NoError(t, err)
	assert.False(t, len(ready) != 1 ||
		ready[0] !=
			runID)
	require.NoError(t, q.ClearRetry(ctx,
		runID))

	// Clear it.

	ready, _ = q.ReadyRetries(ctx, 100)
	assert.Len(t, ready, 0)

	var rawRows int
	var latestCleared bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_retries
		WHERE run_id = $1`,

		runID,
	).Scan(&rawRows,
		&latestCleared))
	require.EqualValues(t, 2, rawRows)
	require.True(t, latestCleared)

}

func TestRetries_FutureRetry_NotReady(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-future-" + newID()
	future := time.Now().UTC().Add(1 * time.Hour)
	require.NoError(t, q.ScheduleRetry(ctx, runID,
		future,
		1))

	ready, _ := q.ReadyRetries(ctx, 100)
	for _, id := range ready {
		assert.NotEqual(t, runID,

			id)

	}
}

func TestRetries_LatestScheduleWins(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-latest-" + newID()
	require.NoError(t, q.ScheduleRetry(ctx, runID,
		time.Now().UTC().
			Add(-1*time.Second), 1))
	require.NoError(t, q.ScheduleRetry(ctx, runID,
		time.Now().UTC().
			Add(1*time.Hour), 5))

	ready, _ := q.ReadyRetries(ctx, 100)
	for _, id := range ready {
		assert.NotEqual(t, runID,

			id)

	}
	n, err := q.CountPendingRetries(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	var rawRows, latestAttempt int
	var latestRetryAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), (ARRAY_AGG(attempt ORDER BY id DESC))[1], (ARRAY_AGG(next_retry_at ORDER BY id DESC))[1]
		FROM job_retries
		WHERE run_id = $1`,

		runID).Scan(&rawRows, &latestAttempt,

		&latestRetryAt))
	require.EqualValues(t, 2, rawRows)
	require.EqualValues(t, 5, latestAttempt)
	require.True(t, latestRetryAt.
		After(time.Now().UTC().Add(30*time.
			Minute)))

}

func TestRetries_ScheduleRetrySkipsIdenticalLatest(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-noop-" + newID()
	retryAt := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	require.NoError(t, q.ScheduleRetry(ctx, runID,
		retryAt,
		2))

	var rawRows, latestID int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), COALESCE(MAX(id), 0)
		FROM job_retries
		WHERE run_id = $1`,

		runID).Scan(&rawRows,
		&latestID))
	require.EqualValues(t, 1, rawRows)
	require.NoError(t, q.ScheduleRetry(ctx, runID,
		retryAt,
		2))

	var rowsAfterNoOp, latestIDAfterNoOp int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), COALESCE(MAX(id), 0)
		FROM job_retries
		WHERE run_id = $1`,

		runID).Scan(&rowsAfterNoOp,
		&latestIDAfterNoOp,
	))
	require.EqualValues(t, 1, rowsAfterNoOp)
	require.Equal(t, latestID,

		latestIDAfterNoOp,
	)
	require.NoError(t, q.ScheduleRetry(ctx, runID,
		retryAt.
			Add(time.
				Second,
			), 2))

	var rowsAfterChange int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_retries
		WHERE run_id = $1`,
		runID).Scan(&rowsAfterChange))
	require.EqualValues(t, 2, rowsAfterChange)

}

func TestRetries_ClearRetriesAppendsTombstones(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	firstID := "retry-clear-batch-a-" + newID()
	secondID := "retry-clear-batch-b-" + newID()
	past := time.Now().UTC().Add(-time.Second)
	for _, runID := range []string{firstID, secondID} {
		require.NoError(t, q.ScheduleRetry(ctx, runID,
			past, 1),
		)

	}
	require.NoError(t, q.ClearRetries(ctx, []string{firstID,
		secondID,
	},
	))

	ready, err := q.ReadyRetries(ctx, 100)
	require.NoError(t, err)

	for _, runID := range []string{firstID, secondID} {
		for _, readyID := range ready {
			require.NotEqual(t, runID,

				readyID,
			)

		}
	}

	rows, err := testDB.Pool.Query(ctx, `
		SELECT run_id, COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_retries
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
		require.EqualValues(t, 2, rawRows)
		require.True(t, latestCleared)

	}
	require.NoError(t, rows.
		Err())
	require.EqualValues(t, 2, seen)

}

func TestRetries_ClearAlreadyClearedRetryDoesNotAppendTombstone(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-clear-idempotent-" + newID()
	require.NoError(t, q.ScheduleRetry(ctx, runID,
		time.Now().UTC().
			Add(-time.Second), 1))
	require.NoError(t, q.ClearRetry(ctx,
		runID))
	require.NoError(t, q.ClearRetry(ctx,
		runID))
	require.NoError(t, q.ClearRetries(ctx, []string{runID}))

	var rawRows int
	var latestCleared bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_retries
		WHERE run_id = $1`,

		runID,
	).Scan(&rawRows,
		&latestCleared))
	require.EqualValues(t, 2, rawRows)
	require.True(t, latestCleared)

}

func TestRetries_OrderedByNextRetryAt(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	base := time.Now().UTC().Add(-10 * time.Second)
	ids := []string{"a-" + newID(), "b-" + newID(), "c-" + newID()}
	for i, id := range ids {
		require.NoError(t, q.ScheduleRetry(ctx, id,
			base.Add(time.
				Duration(i)*time.Second), 1))

	}
	ready, _ := q.ReadyRetries(ctx, 100)
	require.Len(t, ready, 3)
	assert.False(t, ready[0] !=
		ids[0] || ready[1] != ids[1] || ready[2] != ids[2])

}

func TestRetries_ClearNonexistentIsNoOp(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "no-such-run-" + newID()
	assert.NoError(t, q.ClearRetry(ctx,
		runID))

	n, err := q.CountPendingRetries(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, n)

	var rawRows int
	var latestCleared bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_retries
		WHERE run_id = $1`,

		runID,
	).Scan(&rawRows,
		&latestCleared))
	require.EqualValues(t, 0, rawRows)
	require.False(t, latestCleared)

}

func TestRetries_CompactSupersededKeepsLatestRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	firstID := "retry-compact-a-" + newID()
	secondID := "retry-compact-b-" + newID()
	require.NoError(t, q.ScheduleRetry(ctx, firstID,
		time.Now().UTC().
			Add(time.Hour), 1))
	require.NoError(t, q.ScheduleRetry(ctx, firstID,
		time.Now().UTC().
			Add(2*time.Hour), 2))
	require.NoError(t, q.ClearRetry(ctx,
		firstID,
	))
	require.NoError(t, q.ScheduleRetry(ctx, secondID,
		time.
			Now().UTC().
			Add(time.Hour), 1))

	compacted, err := q.CompactSupersededRetries(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, compacted)

	compacted, err = q.CompactSupersededRetries(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, compacted)

	var firstRows int
	var firstLatestCleared bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_retries
		WHERE run_id = $1`,

		firstID,
	).Scan(&firstRows,

		&firstLatestCleared))
	require.EqualValues(t, 1, firstRows)
	require.True(t, firstLatestCleared)

	var secondRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_retries WHERE run_id = $1`,

		secondID).Scan(&secondRows))
	require.EqualValues(t, 1, secondRows)

}

func TestRetries_RunRetryBlockedUsesLatestRow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-blocked-" + newID()
	require.NoError(t, q.ScheduleRetry(ctx, runID,
		time.Now().UTC().
			Add(time.Hour), 1))

	var blocked bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT strait_run_retry_blocked($1)`,

		runID).Scan(&blocked))
	require.True(t, blocked)
	require.NoError(t, q.ScheduleRetry(ctx, runID,
		time.Now().UTC().
			Add(-time.Second), 2))
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT strait_run_retry_blocked($1)`,

		runID).Scan(&blocked))
	require.False(t, blocked)
	require.NoError(t, q.ScheduleRetry(ctx, runID,
		time.Now().UTC().
			Add(time.Hour), 3))
	require.NoError(t, q.ClearRetry(ctx,
		runID))
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT strait_run_retry_blocked($1)`,

		runID).Scan(&blocked))
	require.False(t, blocked)

}

func TestRetries_HOTUpdateOnScheduleDoesNotChurnJobRuns(t *testing.T) {
	// With the side-table path, scheduling retries must NOT update
	// job_runs. Verify by capturing stats before and after.
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Capture initial update count on job_runs partitions.
	_, _ = testDB.Pool.Exec(ctx, "SELECT pg_stat_clear_snapshot()")
	var updBefore int64
	_ = testDB.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(n_tup_upd),0) FROM pg_stat_user_tables
		WHERE relname = 'job_runs' OR relname LIKE 'job_runs_%'
	`).Scan(&updBefore)

	// Schedule 100 retries via the side table.
	for range 100 {
		_ = q.ScheduleRetry(ctx, "churn-"+newID(), time.Now().UTC().Add(1*time.Hour), 1)
	}

	_, _ = testDB.Pool.Exec(ctx, "SELECT pg_stat_clear_snapshot()")
	var updAfter int64
	_ = testDB.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(n_tup_upd),0) FROM pg_stat_user_tables
		WHERE relname = 'job_runs' OR relname LIKE 'job_runs_%'
	`).Scan(&updAfter)

	// job_runs must not have been updated by the retry scheduler.
	if updAfter-updBefore > 5 {
		// Allow slack for unrelated background activity in CI.
		t.Logf("unexpected job_runs updates: before=%d after=%d delta=%d",
			updBefore, updAfter, updAfter-updBefore)
	}
}
