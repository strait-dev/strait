//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

// Integration tests for the unlogged heartbeat side table.

func TestHeartbeatSideTable_UpsertCreatesAndUpdates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "test-hb-upsert-" + newID()

	if err := q.UpsertHeartbeatSideTable(ctx, runID); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	var ts1 time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT heartbeat_at
		FROM job_run_heartbeats
		WHERE run_id = $1 AND cleared = FALSE
		ORDER BY id DESC
		LIMIT 1`, runID).Scan(&ts1); err != nil {
		t.Fatalf("query: %v", err)
	}
	// Ensure observable time passes before the second upsert.
	time.Sleep(20 * time.Millisecond)

	if err := q.UpsertHeartbeatSideTable(ctx, runID); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	var ts2 time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT heartbeat_at
		FROM job_run_heartbeats
		WHERE run_id = $1 AND cleared = FALSE
		ORDER BY id DESC
		LIMIT 1`, runID).Scan(&ts2); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !ts2.After(ts1) {
		t.Errorf("second upsert ts %v not after first %v", ts2, ts1)
	}
	var rawRows int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_run_heartbeats WHERE run_id = $1`, runID).Scan(&rawRows); err != nil {
		t.Fatalf("raw heartbeat count: %v", err)
	}
	if rawRows != 2 {
		t.Fatalf("raw heartbeat rows = %d, want append-only history", rawRows)
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
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT DISTINCT ON (run_id) run_id, cleared
			FROM job_run_heartbeats
			WHERE run_id = ANY($1)
			ORDER BY run_id, id DESC
		) latest
		WHERE cleared = FALSE`, ids).Scan(&count); err != nil {
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
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT DISTINCT ON (run_id) run_id, cleared
			FROM job_run_heartbeats
			WHERE run_id = ANY($1)
			ORDER BY run_id, id DESC
		) latest
		WHERE cleared = FALSE`, ids).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("remaining count = %d, want 1", count)
	}
}

func TestHeartbeatSideTable_DeleteSkipsAbsentAndAlreadyClearedRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := "hb-clear-idempotent-" + newID()
	missingID := "hb-clear-missing-" + newID()
	if err := q.UpsertHeartbeatSideTable(ctx, runID); err != nil {
		t.Fatalf("append heartbeat: %v", err)
	}
	if err := q.DeleteHeartbeatSideTable(ctx, []string{runID, missingID}); err != nil {
		t.Fatalf("first clear heartbeat: %v", err)
	}
	if err := q.DeleteHeartbeatSideTable(ctx, []string{runID, missingID}); err != nil {
		t.Fatalf("second clear heartbeat: %v", err)
	}

	var rawRows int
	var latestCleared bool
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_run_heartbeats
		WHERE run_id = $1`, runID).Scan(&rawRows, &latestCleared); err != nil {
		t.Fatalf("query cleared heartbeat rows: %v", err)
	}
	if rawRows != 2 {
		t.Fatalf("raw heartbeat rows = %d, want live row plus one clear tombstone", rawRows)
	}
	if !latestCleared {
		t.Fatal("latest heartbeat row must be a clear tombstone")
	}

	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_heartbeats
		WHERE run_id = $1`, missingID).Scan(&rawRows); err != nil {
		t.Fatalf("query missing heartbeat rows: %v", err)
	}
	if rawRows != 0 {
		t.Fatalf("missing heartbeat rows = %d, want no clear tombstone", rawRows)
	}
}

func TestHeartbeatSideTable_CompactSupersededKeepsLatestRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	firstID := "hb-compact-a-" + newID()
	secondID := "hb-compact-b-" + newID()
	for range 3 {
		if err := q.BatchUpsertHeartbeatSideTable(ctx, []string{firstID, secondID}); err != nil {
			t.Fatalf("append heartbeat batch: %v", err)
		}
	}
	if err := q.DeleteHeartbeatSideTable(ctx, []string{secondID}); err != nil {
		t.Fatalf("clear second heartbeat: %v", err)
	}

	compacted, err := q.CompactSupersededHeartbeats(ctx, 100)
	if err != nil {
		t.Fatalf("CompactSupersededHeartbeats() error = %v", err)
	}
	if compacted != 5 {
		t.Fatalf("compacted = %d, want 5 superseded rows", compacted)
	}

	rows, err := testDB.Pool.Query(ctx, `
		SELECT run_id, COUNT(*), COALESCE((ARRAY_AGG(cleared ORDER BY id DESC))[1], FALSE)
		FROM job_run_heartbeats
		WHERE run_id = ANY($1)
		GROUP BY run_id`, []string{firstID, secondID})
	if err != nil {
		t.Fatalf("query compacted heartbeats: %v", err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var runID string
		var rawRows int
		var latestCleared bool
		if err := rows.Scan(&runID, &rawRows, &latestCleared); err != nil {
			t.Fatalf("scan compacted heartbeat: %v", err)
		}
		seen++
		if rawRows != 1 {
			t.Fatalf("%s raw rows = %d, want only latest row after compaction", runID, rawRows)
		}
		if runID == firstID && latestCleared {
			t.Fatal("latest heartbeat for active run should remain live")
		}
		if runID == secondID && !latestCleared {
			t.Fatal("latest heartbeat for cleared run should remain tombstoned")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate compacted heartbeats: %v", err)
	}
	if seen != 2 {
		t.Fatalf("compacted heartbeat groups = %d, want 2", seen)
	}
}

func TestHeartbeatSideTable_DeleteOrphanedUsesSplitRunState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-hb-gc-split-state")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if err := q.UpdateHeartbeat(ctx, run.ID); err != nil {
		t.Fatalf("UpdateHeartbeat() error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": time.Now(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus(completed) error = %v", err)
	}
	if err := q.UpdateHeartbeat(ctx, run.ID); err != nil {
		t.Fatalf("reinsert leaked heartbeat: %v", err)
	}

	var ledgerStatus, readStatus domain.RunStatus
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT jr.status, s.status
		FROM job_runs jr
		JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		run.ID,
	).Scan(&ledgerStatus, &readStatus); err != nil {
		t.Fatalf("query split state: %v", err)
	}
	if ledgerStatus != domain.StatusExecuting || readStatus != domain.StatusCompleted {
		t.Fatalf("ledger/read status = %q/%q, want executing/completed", ledgerStatus, readStatus)
	}

	deleted, err := q.DeleteOrphanedHeartbeats(ctx, 100)
	if err != nil {
		t.Fatalf("DeleteOrphanedHeartbeats() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	var count int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT cleared
			FROM job_run_heartbeats
			WHERE run_id = $1
			ORDER BY id DESC
			LIMIT 1
		) latest
		WHERE cleared = FALSE`, run.ID).Scan(&count); err != nil {
		t.Fatalf("count heartbeat rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("heartbeat rows = %d, want 0", count)
	}
}

func TestHeartbeatSideTable_DeleteOrphanedKeepsWaitingRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-hb-gc-waiting")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusWaiting, nil); err != nil {
		t.Fatalf("UpdateRunStatus(waiting) error = %v", err)
	}
	if err := q.UpdateHeartbeatForActiveRun(ctx, run.ID, run.Attempt); err != nil {
		t.Fatalf("UpdateHeartbeatForActiveRun() error = %v", err)
	}

	deleted, err := q.DeleteOrphanedHeartbeats(ctx, 100)
	if err != nil {
		t.Fatalf("DeleteOrphanedHeartbeats() error = %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0 for waiting run", deleted)
	}

	var count int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT cleared
			FROM job_run_heartbeats
			WHERE run_id = $1
			ORDER BY id DESC
			LIMIT 1
		) latest
		WHERE cleared = FALSE`, run.ID).Scan(&count); err != nil {
		t.Fatalf("count heartbeat rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("heartbeat rows = %d, want 1", count)
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
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		VALUES ($1, NOW() - INTERVAL '5 minutes', FALSE)`, staleID); err != nil {
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
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if err := q.UpsertHeartbeatSideTable(ctx, run.ID); err != nil {
		t.Fatalf("fresh side-table heartbeat: %v", err)
	}

	runs, err := q.ListStaleRuns(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("ListStaleRuns() fresh side-table error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("ListStaleRuns() with fresh side-table heartbeat = %d, want 0", len(runs))
	}

	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		VALUES ($1, NOW() - INTERVAL '10 minutes', FALSE)`, run.ID); err != nil {
		t.Fatalf("backdate side-table heartbeat: %v", err)
	}

	runs, err = q.ListStaleRuns(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("ListStaleRuns() stale side-table error = %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("ListStaleRuns() = %v, want only %s", runs, run.ID)
	}
}

func TestHeartbeatSideTable_UpdateHeartbeatDoesNotTouchJobRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-hb-update-side-table")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if err := q.UpdateHeartbeat(ctx, run.ID); err != nil {
		t.Fatalf("UpdateHeartbeat() error = %v", err)
	}

	var ledgerHeartbeat *time.Time
	if err := testDB.Pool.QueryRow(ctx, `SELECT heartbeat_at FROM job_runs WHERE id = $1`, run.ID).Scan(&ledgerHeartbeat); err != nil {
		t.Fatalf("query job_runs heartbeat_at: %v", err)
	}
	if ledgerHeartbeat != nil {
		t.Fatalf("job_runs heartbeat_at = %v, want NULL to avoid fat-row churn", *ledgerHeartbeat)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.HeartbeatAt == nil {
		t.Fatal("GetRun heartbeat_at = nil, want side-table heartbeat")
	}
}

func TestHeartbeatSideTable_UpdateHeartbeatForActiveRunUsesRunState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-hb-active-state")
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_state
		SET status = $1, started_at = NOW()
		WHERE run_id = $2`,
		domain.StatusExecuting,
		run.ID,
	); err != nil {
		t.Fatalf("move state to executing: %v", err)
	}

	if err := q.UpdateHeartbeatForActiveRun(ctx, run.ID, run.Attempt); err != nil {
		t.Fatalf("UpdateHeartbeatForActiveRun() error = %v", err)
	}

	var ledgerHeartbeat *time.Time
	if err := testDB.Pool.QueryRow(ctx, `SELECT heartbeat_at FROM job_runs WHERE id = $1`, run.ID).Scan(&ledgerHeartbeat); err != nil {
		t.Fatalf("query job_runs heartbeat_at: %v", err)
	}
	if ledgerHeartbeat != nil {
		t.Fatalf("job_runs heartbeat_at = %v, want NULL to avoid fat-row churn", *ledgerHeartbeat)
	}
	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusExecuting {
		t.Fatalf("GetRun status = %q, want state status %q", got.Status, domain.StatusExecuting)
	}
	if got.HeartbeatAt == nil {
		t.Fatal("GetRun heartbeat_at = nil, want side-table heartbeat")
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
