//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"
)

// Phase 8 integration tests for the unlogged heartbeat side table.

func TestHeartbeatSideTable_UpsertCreatesAndUpdates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "test-hb-upsert-" + newID()

	if err := q.UpsertHeartbeatSideTable(ctx, runID); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	var ts1 time.Time
	if err := testDB.Pool.QueryRow(ctx, "SELECT heartbeat_at FROM job_run_heartbeats WHERE run_id=$1", runID).Scan(&ts1); err != nil {
		t.Fatalf("query: %v", err)
	}
	// Ensure observable time passes before the second upsert.
	time.Sleep(20 * time.Millisecond)

	if err := q.UpsertHeartbeatSideTable(ctx, runID); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	var ts2 time.Time
	if err := testDB.Pool.QueryRow(ctx, "SELECT heartbeat_at FROM job_run_heartbeats WHERE run_id=$1", runID).Scan(&ts2); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !ts2.After(ts1) {
		t.Errorf("second upsert ts %v not after first %v", ts2, ts1)
	}
}

func TestHeartbeatSideTable_BatchUpsertAll(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	ids := make([]string, 100)
	for i := range ids {
		ids[i] = "hb-batch-" + newID()
	}

	if err := q.BatchUpsertHeartbeatSideTable(ctx, ids); err != nil {
		t.Fatalf("batch upsert: %v", err)
	}

	var count int
	if err := testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM job_run_heartbeats WHERE run_id = ANY($1)", ids).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 100 {
		t.Errorf("count = %d, want 100", count)
	}
}

func TestHeartbeatSideTable_BatchEmptyNoOp(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	// Empty batch must succeed and not issue a query.
	if err := q.BatchUpsertHeartbeatSideTable(ctx, nil); err != nil {
		t.Fatalf("empty batch: %v", err)
	}
	if err := q.BatchUpsertHeartbeatSideTable(ctx, []string{}); err != nil {
		t.Fatalf("empty slice: %v", err)
	}
}

func TestHeartbeatSideTable_DeleteRemoves(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	ids := []string{"hb-del-1-" + newID(), "hb-del-2-" + newID(), "hb-del-3-" + newID()}
	if err := q.BatchUpsertHeartbeatSideTable(ctx, ids); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Delete two of them.
	if err := q.DeleteHeartbeatSideTable(ctx, ids[:2]); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var count int
	if err := testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM job_run_heartbeats WHERE run_id = ANY($1)", ids).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("remaining count = %d, want 1", count)
	}
}

func TestHeartbeatSideTable_StaleDetection(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	freshID := "hb-fresh-" + newID()
	staleID := "hb-stale-" + newID()

	if err := q.BatchUpsertHeartbeatSideTable(ctx, []string{freshID, staleID}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Backdate the stale one.
	if _, err := testDB.Pool.Exec(ctx,
		"UPDATE job_run_heartbeats SET heartbeat_at = NOW() - INTERVAL '5 minutes' WHERE run_id = $1",
		staleID,
	); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	stale, err := q.StaleHeartbeatSideTable(ctx, 1*time.Minute, 100)
	if err != nil {
		t.Fatalf("stale: %v", err)
	}
	if len(stale) != 1 || stale[0] != staleID {
		t.Errorf("stale = %v, want [%s]", stale, staleID)
	}
}

func TestHeartbeatSideTable_UnloggedSurvivesTruncate(t *testing.T) {
	// Crash recovery simulation: truncate the unlogged table and verify
	// the system continues to accept writes. This mirrors what happens
	// after a Postgres crash.
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "hb-crash-" + newID()
	if err := q.UpsertHeartbeatSideTable(ctx, runID); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, "TRUNCATE job_run_heartbeats"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// Must succeed after truncate.
	if err := q.UpsertHeartbeatSideTable(ctx, runID); err != nil {
		t.Fatalf("post-truncate upsert: %v", err)
	}
}
