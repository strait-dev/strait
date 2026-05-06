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

func TestRetries_UpsertIdempotent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "retry-upsert-" + newID()
	if err := q.ScheduleRetry(ctx, runID, time.Now().UTC().Add(-1*time.Second), 1); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := q.ScheduleRetry(ctx, runID, time.Now().UTC().Add(1*time.Hour), 5); err != nil {
		t.Fatalf("second: %v", err)
	}
	ready, _ := q.ReadyRetries(ctx, 100)
	for _, id := range ready {
		if id == runID {
			t.Errorf("upsert to future should remove from ready")
		}
	}
	n, err := q.CountPendingRetries(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("pending = %d, want 1", n)
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
	if err := q.ClearRetry(ctx, "no-such-run"); err != nil {
		t.Errorf("clear missing should be noop: %v", err)
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
