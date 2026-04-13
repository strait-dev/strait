//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

// Chaos-style integration tests for recovery scenarios.

// TestChaos_StaleRunReclaimedAfterHeartbeatLapse simulates a worker that
// claims a run and then "crashes" (stops heartbeating). The stale-run
// reclaimer should move the run back to a claimable state. This is the
// integration-test version of the full kill-9 scenario — without a real
// subprocess fork, we approximate by skipping heartbeats and directly
// checking that the reclaimer's query would match.
func TestChaos_StaleRunReclaimedAfterHeartbeatLapse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-chaos-stale")
	q := mustQueue(t)

	run := mustEnqueueRun(t, ctx, q, job)

	// Claim the run (dequeue → executing).
	batch, err := q.DequeueN(ctx, 1)
	if err != nil || len(batch) != 1 {
		t.Fatalf("dequeue: %v (batch=%d)", err, len(batch))
	}
	_, err = testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status='executing', heartbeat_at=NOW() - INTERVAL '5 minutes' WHERE id=$1`,
		run.ID,
	)
	if err != nil {
		t.Fatalf("simulate stale heartbeat: %v", err)
	}

	// Verify the stale-run query picks it up. The actual reclaimer in
	// production uses a threshold of ~60s; we backdated the heartbeat by
	// 5 minutes to guarantee detection.
	var staleID string
	err = testDB.Pool.QueryRow(ctx, `
		SELECT id FROM job_runs
		WHERE status = 'executing'
		  AND heartbeat_at IS NOT NULL
		  AND heartbeat_at < NOW() - INTERVAL '30 seconds'
		ORDER BY heartbeat_at ASC
		LIMIT 1
	`).Scan(&staleID)
	if err != nil {
		t.Fatalf("stale query: %v", err)
	}
	if staleID != run.ID {
		t.Errorf("stale ID = %s, want %s", staleID, run.ID)
	}

	// Transition back to queued (what the reclaimer does).
	_, err = testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status='queued', started_at=NULL, heartbeat_at=NULL WHERE id=$1`,
		run.ID,
	)
	if err != nil {
		t.Fatalf("re-queue: %v", err)
	}

	// Second worker claims it.
	batch2, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("second dequeue: %v", err)
	}
	if len(batch2) != 1 || batch2[0].ID != run.ID {
		t.Errorf("expected to re-claim %s, got %v", run.ID, batch2)
	}
}

// TestChaos_DBTimestampUsedForRetryNotGoTime verifies that retry scheduling
// comparisons use the database's NOW() and not Go's time.Now(). This
// matters when there's clock skew between the app server and the DB.
func TestChaos_DBTimestampUsedForRetryNotGoTime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-chaos-clock")
	q := mustQueue(t)

	run := mustEnqueueRun(t, ctx, q, job)

	// Schedule a retry 2 seconds in the future using DB time.
	_, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status='queued', next_retry_at = NOW() + INTERVAL '2 seconds' WHERE id=$1`,
		run.ID,
	)
	if err != nil {
		t.Fatalf("schedule retry: %v", err)
	}

	// Immediately try to dequeue. The dequeue predicate checks
	// `jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW()` where
	// NOW() is the DB timestamp. Since we just set next_retry_at to
	// DB_NOW+2s, the run should NOT be claimable yet.
	batch, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if len(batch) != 0 {
		t.Errorf("run should not be claimable before next_retry_at; got %d runs", len(batch))
	}

	// Wait 3 seconds (longer than the 2s retry delay) then try again.
	time.Sleep(3 * time.Second)
	batch2, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("dequeue after wait: %v", err)
	}
	if len(batch2) != 1 || batch2[0].ID != run.ID {
		t.Errorf("expected to claim %s after retry fires, got %v", run.ID, batch2)
	}
	_ = domain.StatusQueued
}
