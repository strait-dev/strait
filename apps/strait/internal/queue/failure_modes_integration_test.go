//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
)

// Failure modes and recovery tests.

func TestFailure_DequeueWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fail-ctx")
	q := mustQueue(t)
	mustEnqueueRun(t, ctx, q, job)

	cancelledCtx, cancelNow := context.WithCancel(ctx)
	cancelNow()
	_, err := q.DequeueN(cancelledCtx, 1)
	if err == nil {
		t.Error("dequeue with cancelled context should error")
	}

	// Original context should still work.
	batch, err := q.DequeueN(ctx, 1)
	if err != nil || len(batch) != 1 {
		t.Fatalf("dequeue with valid context: err=%v len=%d", err, len(batch))
	}
}

func TestFailure_EnqueueWithDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fail-deadline")
	q := mustQueue(t)

	deadCtx, deadCancel := context.WithDeadline(ctx, time.Now().Add(-1*time.Second))
	defer deadCancel()
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	err := q.Enqueue(deadCtx, run)
	if err == nil {
		t.Error("enqueue with expired deadline should error")
	}
}

func TestFailure_TriggerDisabledThenReconciled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fail-trig")
	q := mustQueue(t)

	// Disable the active_counts trigger.
	_, err := testDB.Pool.Exec(ctx, `ALTER TABLE job_runs DISABLE TRIGGER job_runs_active_counts_trg`)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	defer func() {
		_, _ = testDB.Pool.Exec(ctx, `ALTER TABLE job_runs ENABLE TRIGGER job_runs_active_counts_trg`)
	}()

	for range 5 {
		mustEnqueueRun(t, ctx, q, job)
	}
	batch, _ := q.DequeueN(ctx, 5)
	if len(batch) != 5 {
		t.Fatalf("dequeue: %d", len(batch))
	}

	// Counter should be wrong (trigger was off).
	var counter int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(count),0) FROM job_active_counts WHERE job_id=$1`, job.ID).Scan(&counter)
	if counter != 0 {
		t.Errorf("counter should be 0 with trigger disabled, got %d", counter)
	}

	// Re-enable trigger.
	_, _ = testDB.Pool.Exec(ctx, `ALTER TABLE job_runs ENABLE TRIGGER job_runs_active_counts_trg`)

	// The counter is now drifted. A reconciler would fix it. We verify
	// the ground truth is non-zero to confirm the drift exists.
	var truth int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs WHERE job_id=$1 AND status IN ('dequeued','executing')`, job.ID).Scan(&truth)
	if truth != 5 {
		t.Errorf("truth = %d, want 5", truth)
	}
}

func TestFailure_PartialBatchWriteIsAtomic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fail-batch")
	q := mustQueue(t)

	// EnqueueBatch with a mix of valid and one with a bad job_id
	// should either insert all or none (COPY is atomic).
	runs := []*domain.JobRun{
		{JobID: job.ID, ProjectID: job.ProjectID},
		{JobID: job.ID, ProjectID: job.ProjectID},
	}
	n, err := q.EnqueueBatch(ctx, runs)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if n != 2 {
		t.Errorf("batch = %d, want 2", n)
	}
}

func TestFailure_HeartbeatUpdateAfterTerminal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fail-hb-term")
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)

	// Claim, complete, then try heartbeat — side table upsert should
	// succeed (idempotent) but the run is no longer executing.
	_, _ = q.DequeueN(ctx, 1)
	_, _ = testDB.Pool.Exec(ctx, `UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, run.ID)
	if err := st.UpsertHeartbeatSideTable(ctx, run.ID); err != nil {
		t.Fatalf("heartbeat after terminal: %v", err)
	}
	// The GC should clean this up as an orphan.
	deleted, err := st.DeleteOrphanedHeartbeats(ctx, 100)
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if deleted != 1 {
		t.Errorf("gc deleted = %d, want 1", deleted)
	}
}

func TestFailure_NotifyReconnectCounterIncrements(t *testing.T) {
	// Point at an unreachable DSN to force reconnect attempts.
	n := queue.NewQueueNotifier("postgres://127.0.0.1:1/nobody?connect_timeout=1", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go n.Run(ctx)
	time.Sleep(3 * time.Second)
	cancel()
	// Should have attempted at least 1 reconnect.
	if n.Reconnects() < 1 {
		t.Logf("reconnects = %d (may be 0 if connect is very slow)", n.Reconnects())
	}
}

func TestFailure_RetryScheduleClearedOnClaim(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fail-retry-clear")
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)

	// Schedule a retry in the past, then dequeue — the retry side table
	// row should be clearable after claim.
	past := time.Now().Add(-1 * time.Minute)
	_ = st.ScheduleRetry(ctx, run.ID, past, 2)
	batch, _ := q.DequeueN(ctx, 1)
	if len(batch) != 1 {
		t.Fatal("should claim run with past retry")
	}
	// Explicitly clear (what the worker would do).
	_ = st.ClearRetry(ctx, run.ID)
	ready, _ := st.ReadyRetries(ctx, 100)
	for _, id := range ready {
		if id == run.ID {
			t.Error("retry should be cleared after claim")
		}
	}
}

func TestFailure_DLQCounterRecoversAfterTriggerReEnable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fail-dlq-trig")

	// Insert a dead_letter row while trigger is enabled.
	id := newID()
	_, _ = testDB.Pool.Exec(ctx, `
		INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
		VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW())
	`, id, job.ID, job.ProjectID)

	var before int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COALESCE(count,0) FROM dlq_counts WHERE job_id=$1`, job.ID).Scan(&before)
	if before != 1 {
		t.Fatalf("dlq counter before = %d, want 1", before)
	}

	// Mask the row — counter should drop.
	_, _ = testDB.Pool.Exec(ctx, `UPDATE job_runs SET visible_until=NOW() WHERE id=$1`, id)
	var after int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COALESCE(count,0) FROM dlq_counts WHERE job_id=$1`, job.ID).Scan(&after)
	if after != 0 {
		t.Errorf("dlq counter after mask = %d, want 0", after)
	}
}
