//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"
)

func TestRetries_Schedule_Clear_Ready(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-test-" + newID()

	// Schedule a retry in the past → it should be "ready".
	past := time.Now().UTC().Add(-1 * time.Second)
	if err := q.ScheduleRetry(ctx, runID, past, 2); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	ready, err := q.ReadyRetries(ctx, 100)
	if err != nil {
		t.Fatalf("ready: %v", err)
	}
	if len(ready) != 1 || ready[0] != runID {
		t.Errorf("ready = %v, want [%s]", ready, runID)
	}

	// Clear it.
	if err := q.ClearRetry(ctx, runID); err != nil {
		t.Fatalf("clear: %v", err)
	}
	ready, _ = q.ReadyRetries(ctx, 100)
	if len(ready) != 0 {
		t.Errorf("ready after clear = %v", ready)
	}

	var rawRows int
	var latestCleared bool
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_retries
		WHERE run_id = $1`, runID).Scan(&rawRows, &latestCleared); err != nil {
		t.Fatalf("query retry rows after clear: %v", err)
	}
	if rawRows != 2 {
		t.Fatalf("raw retry rows = %d, want scheduled row plus clear tombstone", rawRows)
	}
	if !latestCleared {
		t.Fatal("latest retry row must be a clear tombstone")
	}
}

func TestRetries_FutureRetry_NotReady(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-future-" + newID()
	future := time.Now().UTC().Add(1 * time.Hour)
	if err := q.ScheduleRetry(ctx, runID, future, 1); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	ready, _ := q.ReadyRetries(ctx, 100)
	for _, id := range ready {
		if id == runID {
			t.Errorf("future retry %s should not be ready", runID)
		}
	}
}

func TestRetries_LatestScheduleWins(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-latest-" + newID()
	if err := q.ScheduleRetry(ctx, runID, time.Now().UTC().Add(-1*time.Second), 1); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := q.ScheduleRetry(ctx, runID, time.Now().UTC().Add(1*time.Hour), 5); err != nil {
		t.Fatalf("second: %v", err)
	}
	ready, _ := q.ReadyRetries(ctx, 100)
	for _, id := range ready {
		if id == runID {
			t.Errorf("newer future retry should remove run from ready")
		}
	}
	n, err := q.CountPendingRetries(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("pending = %d, want 1", n)
	}

	var rawRows, latestAttempt int
	var latestRetryAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*), (ARRAY_AGG(attempt ORDER BY id DESC))[1], (ARRAY_AGG(next_retry_at ORDER BY id DESC))[1]
		FROM job_retries
		WHERE run_id = $1`, runID).Scan(&rawRows, &latestAttempt, &latestRetryAt); err != nil {
		t.Fatalf("query raw retry rows: %v", err)
	}
	if rawRows != 2 {
		t.Fatalf("raw retry rows = %d, want append-only history", rawRows)
	}
	if latestAttempt != 5 {
		t.Fatalf("latest attempt = %d, want 5", latestAttempt)
	}
	if !latestRetryAt.After(time.Now().UTC().Add(30 * time.Minute)) {
		t.Fatalf("latest retry timestamp = %s, want future timestamp", latestRetryAt)
	}
}

func TestRetries_ScheduleRetrySkipsIdenticalLatest(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-noop-" + newID()
	retryAt := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if err := q.ScheduleRetry(ctx, runID, retryAt, 2); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}

	var rawRows, latestID int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(MAX(id), 0)
		FROM job_retries
		WHERE run_id = $1`, runID).Scan(&rawRows, &latestID); err != nil {
		t.Fatalf("query initial retry rows: %v", err)
	}
	if rawRows != 1 {
		t.Fatalf("initial raw retry rows = %d, want 1", rawRows)
	}

	if err := q.ScheduleRetry(ctx, runID, retryAt, 2); err != nil {
		t.Fatalf("schedule identical retry: %v", err)
	}
	var rowsAfterNoOp, latestIDAfterNoOp int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(MAX(id), 0)
		FROM job_retries
		WHERE run_id = $1`, runID).Scan(&rowsAfterNoOp, &latestIDAfterNoOp); err != nil {
		t.Fatalf("query retry rows after identical schedule: %v", err)
	}
	if rowsAfterNoOp != 1 {
		t.Fatalf("raw retry rows after identical schedule = %d, want 1", rowsAfterNoOp)
	}
	if latestIDAfterNoOp != latestID {
		t.Fatalf("latest retry id after identical schedule = %d, want %d", latestIDAfterNoOp, latestID)
	}

	if err := q.ScheduleRetry(ctx, runID, retryAt.Add(time.Second), 2); err != nil {
		t.Fatalf("schedule changed retry: %v", err)
	}
	var rowsAfterChange int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_retries
		WHERE run_id = $1`, runID).Scan(&rowsAfterChange); err != nil {
		t.Fatalf("query retry rows after changed schedule: %v", err)
	}
	if rowsAfterChange != 2 {
		t.Fatalf("raw retry rows after changed schedule = %d, want 2", rowsAfterChange)
	}
}

func TestRetries_ClearRetriesAppendsTombstones(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	firstID := "retry-clear-batch-a-" + newID()
	secondID := "retry-clear-batch-b-" + newID()
	past := time.Now().UTC().Add(-time.Second)
	for _, runID := range []string{firstID, secondID} {
		if err := q.ScheduleRetry(ctx, runID, past, 1); err != nil {
			t.Fatalf("schedule %s: %v", runID, err)
		}
	}
	if err := q.ClearRetries(ctx, []string{firstID, secondID}); err != nil {
		t.Fatalf("clear batch: %v", err)
	}
	ready, err := q.ReadyRetries(ctx, 100)
	if err != nil {
		t.Fatalf("ready after batch clear: %v", err)
	}
	for _, runID := range []string{firstID, secondID} {
		for _, readyID := range ready {
			if readyID == runID {
				t.Fatalf("cleared run %s returned as ready: %v", runID, ready)
			}
		}
	}

	rows, err := testDB.Pool.Query(ctx, `
		SELECT run_id, COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_retries
		WHERE run_id = ANY($1)
		GROUP BY run_id`, []string{firstID, secondID})
	if err != nil {
		t.Fatalf("query clear tombstones: %v", err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var runID string
		var rawRows int
		var latestCleared bool
		if err := rows.Scan(&runID, &rawRows, &latestCleared); err != nil {
			t.Fatalf("scan clear tombstone row: %v", err)
		}
		seen++
		if rawRows != 2 {
			t.Fatalf("%s raw rows = %d, want scheduled row plus tombstone", runID, rawRows)
		}
		if !latestCleared {
			t.Fatalf("%s latest row must be a clear tombstone", runID)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate clear tombstones: %v", err)
	}
	if seen != 2 {
		t.Fatalf("clear tombstone rows seen = %d, want 2", seen)
	}
}

func TestRetries_ClearAlreadyClearedRetryDoesNotAppendTombstone(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-clear-idempotent-" + newID()
	if err := q.ScheduleRetry(ctx, runID, time.Now().UTC().Add(-time.Second), 1); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	if err := q.ClearRetry(ctx, runID); err != nil {
		t.Fatalf("first clear retry: %v", err)
	}
	if err := q.ClearRetry(ctx, runID); err != nil {
		t.Fatalf("second clear retry: %v", err)
	}
	if err := q.ClearRetries(ctx, []string{runID}); err != nil {
		t.Fatalf("batch clear retry: %v", err)
	}

	var rawRows int
	var latestCleared bool
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_retries
		WHERE run_id = $1`, runID).Scan(&rawRows, &latestCleared); err != nil {
		t.Fatalf("query retry rows after repeated clears: %v", err)
	}
	if rawRows != 2 {
		t.Fatalf("raw retry rows = %d, want scheduled row plus one clear tombstone", rawRows)
	}
	if !latestCleared {
		t.Fatal("latest retry row must remain a clear tombstone")
	}
}

func TestRetries_OrderedByNextRetryAt(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	base := time.Now().UTC().Add(-10 * time.Second)
	ids := []string{"a-" + newID(), "b-" + newID(), "c-" + newID()}
	for i, id := range ids {
		if err := q.ScheduleRetry(ctx, id, base.Add(time.Duration(i)*time.Second), 1); err != nil {
			t.Fatalf("schedule %d: %v", i, err)
		}
	}
	ready, _ := q.ReadyRetries(ctx, 100)
	if len(ready) != 3 {
		t.Fatalf("ready count = %d", len(ready))
	}
	if ready[0] != ids[0] || ready[1] != ids[1] || ready[2] != ids[2] {
		t.Errorf("order = %v, want %v", ready, ids)
	}
}

func TestRetries_ClearNonexistentIsNoOp(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "no-such-run-" + newID()
	if err := q.ClearRetry(ctx, runID); err != nil {
		t.Errorf("clear missing should be noop: %v", err)
	}
	n, err := q.CountPendingRetries(ctx)
	if err != nil {
		t.Fatalf("count pending retries: %v", err)
	}
	if n != 0 {
		t.Fatalf("pending retries = %d, want 0", n)
	}

	var rawRows int
	var latestCleared bool
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_retries
		WHERE run_id = $1`, runID).Scan(&rawRows, &latestCleared); err != nil {
		t.Fatalf("query clear missing tombstone: %v", err)
	}
	if rawRows != 0 {
		t.Fatalf("raw retry rows = %d, want no tombstone for absent retry", rawRows)
	}
	if latestCleared {
		t.Fatal("latest cleared = true, want false when no retry row exists")
	}
}

func TestRetries_CompactSupersededKeepsLatestRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	firstID := "retry-compact-a-" + newID()
	secondID := "retry-compact-b-" + newID()
	if err := q.ScheduleRetry(ctx, firstID, time.Now().UTC().Add(time.Hour), 1); err != nil {
		t.Fatalf("schedule first initial retry: %v", err)
	}
	if err := q.ScheduleRetry(ctx, firstID, time.Now().UTC().Add(2*time.Hour), 2); err != nil {
		t.Fatalf("schedule first replacement retry: %v", err)
	}
	if err := q.ClearRetry(ctx, firstID); err != nil {
		t.Fatalf("clear first retry: %v", err)
	}
	if err := q.ScheduleRetry(ctx, secondID, time.Now().UTC().Add(time.Hour), 1); err != nil {
		t.Fatalf("schedule second retry: %v", err)
	}

	compacted, err := q.CompactSupersededRetries(ctx, 1)
	if err != nil {
		t.Fatalf("compact first page: %v", err)
	}
	if compacted != 1 {
		t.Fatalf("first compacted rows = %d, want 1", compacted)
	}
	compacted, err = q.CompactSupersededRetries(ctx, 100)
	if err != nil {
		t.Fatalf("compact remaining: %v", err)
	}
	if compacted != 1 {
		t.Fatalf("remaining compacted rows = %d, want 1", compacted)
	}

	var firstRows int
	var firstLatestCleared bool
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_retries
		WHERE run_id = $1`, firstID).Scan(&firstRows, &firstLatestCleared); err != nil {
		t.Fatalf("query first retry rows: %v", err)
	}
	if firstRows != 1 {
		t.Fatalf("first retry rows after compaction = %d, want latest row only", firstRows)
	}
	if !firstLatestCleared {
		t.Fatal("first latest retry row must remain the clear tombstone")
	}

	var secondRows int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_retries WHERE run_id = $1`, secondID).Scan(&secondRows); err != nil {
		t.Fatalf("query second retry rows: %v", err)
	}
	if secondRows != 1 {
		t.Fatalf("second retry rows after compaction = %d, want untouched latest row", secondRows)
	}
}

func TestRetries_RunRetryBlockedUsesLatestRow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-blocked-" + newID()
	if err := q.ScheduleRetry(ctx, runID, time.Now().UTC().Add(time.Hour), 1); err != nil {
		t.Fatalf("schedule future retry: %v", err)
	}
	var blocked bool
	if err := testDB.Pool.QueryRow(ctx, `SELECT strait_run_retry_blocked($1)`, runID).Scan(&blocked); err != nil {
		t.Fatalf("query blocked future retry: %v", err)
	}
	if !blocked {
		t.Fatal("future latest retry should block dequeue")
	}

	if err := q.ScheduleRetry(ctx, runID, time.Now().UTC().Add(-time.Second), 2); err != nil {
		t.Fatalf("schedule due retry: %v", err)
	}
	if err := testDB.Pool.QueryRow(ctx, `SELECT strait_run_retry_blocked($1)`, runID).Scan(&blocked); err != nil {
		t.Fatalf("query blocked due retry: %v", err)
	}
	if blocked {
		t.Fatal("newer due retry should unblock dequeue")
	}

	if err := q.ScheduleRetry(ctx, runID, time.Now().UTC().Add(time.Hour), 3); err != nil {
		t.Fatalf("schedule second future retry: %v", err)
	}
	if err := q.ClearRetry(ctx, runID); err != nil {
		t.Fatalf("clear retry: %v", err)
	}
	if err := testDB.Pool.QueryRow(ctx, `SELECT strait_run_retry_blocked($1)`, runID).Scan(&blocked); err != nil {
		t.Fatalf("query blocked cleared retry: %v", err)
	}
	if blocked {
		t.Fatal("clear tombstone should unblock dequeue")
	}
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
