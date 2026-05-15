//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"
)

// TestClaim_DoesNotWriteHeartbeatAt verifies that the claim path no longer
// stamps job_runs.heartbeat_at. Liveness now lives in the job_run_heartbeats
// side table; the claim UPDATE leaves the indexed heartbeat_at column NULL
// so the row qualifies for a HOT update.
func TestClaim_DoesNotWriteHeartbeatAt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-no-hb")
	q := mustQueue(t)

	run := mustEnqueueRun(t, ctx, q, job)

	batch, err := q.DequeueN(ctx, 1)
	if err != nil || len(batch) != 1 || batch[0].ID != run.ID {
		t.Fatalf("dequeue: err=%v len=%d", err, len(batch))
	}

	var heartbeatAt *time.Time
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT heartbeat_at FROM job_runs WHERE id = $1`, run.ID,
	).Scan(&heartbeatAt); err != nil {
		t.Fatalf("scan heartbeat_at: %v", err)
	}
	if heartbeatAt != nil {
		t.Fatalf("heartbeat_at should be NULL after claim, got %v", *heartbeatAt)
	}

	var sideTableCount int
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_run_heartbeats WHERE run_id = $1`, run.ID,
	).Scan(&sideTableCount); err != nil {
		t.Fatalf("scan side table: %v", err)
	}
	if sideTableCount != 0 {
		t.Fatalf("side table should be empty before first tick, got %d rows", sideTableCount)
	}
}

// TestReaper_StaleRunBeforeFirstHeartbeat covers the started_at fallback
// path: a run was claimed but the worker died before the first heartbeat
// tick. With no side-table row, ListStaleRuns must still flag it stale
// based on started_at.
func TestReaper_StaleRunBeforeFirstHeartbeat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-reaper-pre-tick")
	q := mustQueue(t)

	run := mustEnqueueRun(t, ctx, q, job)

	batch, err := q.DequeueN(ctx, 1)
	if err != nil || len(batch) != 1 {
		t.Fatalf("dequeue: %v", err)
	}

	// Promote dequeued → executing (what the worker does immediately
	// after the claim) and backdate started_at past the stale threshold.
	old := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status = 'executing', started_at = $1 WHERE id = $2`, old, run.ID,
	); err != nil {
		t.Fatalf("backdate started_at: %v", err)
	}

	stale, err := st.ListStaleRuns(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("ListStaleRuns: %v", err)
	}
	found := false
	for _, r := range stale {
		if r.ID == run.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pre-first-tick stale run %s to be reaped via started_at fallback, got %d runs", run.ID, len(stale))
	}
}

// TestReaper_HealthyRunWithRecentHeartbeat verifies a healthy ticking run
// is NOT flagged stale. The side-table row dominates the started_at value.
func TestReaper_HealthyRunWithRecentHeartbeat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-reaper-healthy")
	q := mustQueue(t)

	run := mustEnqueueRun(t, ctx, q, job)

	if _, err := q.DequeueN(ctx, 1); err != nil {
		t.Fatalf("dequeue: %v", err)
	}

	// Promote to executing and backdate started_at (so the fallback would
	// flag it stale) but write a recent heartbeat into the side table —
	// the side-table row wins.
	old := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status = 'executing', started_at = $1 WHERE id = $2`, old, run.ID,
	); err != nil {
		t.Fatalf("backdate started_at: %v", err)
	}
	if err := st.UpsertHeartbeatSideTable(ctx, run.ID); err != nil {
		t.Fatalf("upsert side table: %v", err)
	}

	stale, err := st.ListStaleRuns(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("ListStaleRuns: %v", err)
	}
	for _, r := range stale {
		if r.ID == run.ID {
			t.Fatalf("healthy run %s incorrectly flagged stale", run.ID)
		}
	}
}

// TestReaper_HealthyRunWithoutSideTableEntry_PreFirstTick covers the
// just-claimed window. The run is younger than the stale threshold, no
// side-table row yet, and the reaper must not flag it.
func TestReaper_HealthyRunWithoutSideTableEntry_PreFirstTick(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-reaper-just-claimed")
	q := mustQueue(t)

	run := mustEnqueueRun(t, ctx, q, job)

	if _, err := q.DequeueN(ctx, 1); err != nil {
		t.Fatalf("dequeue: %v", err)
	}

	// Promote to executing with a fresh started_at, mirroring what happens
	// once the worker picks up the dequeued run.
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status = 'executing', started_at = NOW() WHERE id = $1`, run.ID,
	); err != nil {
		t.Fatalf("promote to executing: %v", err)
	}

	stale, err := st.ListStaleRuns(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("ListStaleRuns: %v", err)
	}
	for _, r := range stale {
		if r.ID == run.ID {
			t.Fatalf("just-claimed run %s incorrectly flagged stale (no side-table row, recent started_at)", run.ID)
		}
	}
}
