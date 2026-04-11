//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

// TestDelayedRun_PromotedAndDequeuable exercises the end-to-end path a
// delayed run takes through the queue: created with status='delayed' and
// a past scheduled_at, then promoted to 'queued' by the scheduler's
// delayed poller (via store.ActivateDueRuns), then dequeued normally.
//
// This was previously not covered by any integration test. The store-
// level TestRuns_ActivateDueRuns_HappyPath proves the promotion UPDATE
// works in isolation, but nothing verified that the queue's Dequeue
// (a) correctly skips rows with status='delayed' before promotion, and
// (b) picks them up after promotion. PR #92's perf work added
// idx_job_runs_delayed_scheduled specifically for the poller query, so
// regressing the promotion logic would be silent without this test.
func TestDelayedRun_PromotedAndDequeuable(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-delayed-promotion")

	// Create a run directly in the store with status='delayed' and a
	// scheduled_at in the past. The queue's Enqueue uses scheduled_at >
	// NOW() to decide whether to mark a run delayed, so it cannot
	// produce this exact shape on its own — we have to insert via the
	// store, which is the same path the cron scheduler uses when it
	// enqueues future-scheduled work that then ages past its trigger.
	past := time.Now().UTC().Add(-10 * time.Minute)
	run := &domain.JobRun{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Status:      domain.StatusDelayed,
		Attempt:     1,
		Priority:    0,
		TriggeredBy: domain.TriggerManual,
		ScheduledAt: &past,
	}
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Before promotion: Dequeue must skip the delayed run because the
	// query filters WHERE status = 'queued'. If it returned the delayed
	// row, that would be a silent regression in the dequeue filter.
	pre, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("pre-promotion Dequeue: %v", err)
	}
	if pre != nil {
		t.Fatalf("pre-promotion Dequeue returned run %q, want nil (status=delayed must be skipped)", pre.ID)
	}

	// Promote. This is the call the scheduler's DelayedPoller makes on
	// every tick.
	promoted, err := st.ActivateDueRuns(ctx, 100)
	if err != nil {
		t.Fatalf("ActivateDueRuns: %v", err)
	}
	if promoted != 1 {
		t.Fatalf("ActivateDueRuns promoted = %d, want 1", promoted)
	}

	// Verify the row's status transitioned via a direct read.
	after, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun after ActivateDueRuns: %v", err)
	}
	if after.Status != domain.StatusQueued {
		t.Fatalf("status after ActivateDueRuns = %q, want %q", after.Status, domain.StatusQueued)
	}

	// Dequeue should now return the promoted run.
	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("post-promotion Dequeue: %v", err)
	}
	if dequeued == nil {
		t.Fatal("post-promotion Dequeue returned nil, want the promoted run")
	}
	if dequeued.ID != run.ID {
		t.Fatalf("Dequeue returned run %q, want %q", dequeued.ID, run.ID)
	}
	if dequeued.Status != domain.StatusDequeued {
		t.Fatalf("dequeued status = %q, want %q", dequeued.Status, domain.StatusDequeued)
	}
	if dequeued.StartedAt == nil {
		t.Fatal("Dequeue did not set started_at on the promoted run")
	}
}

// TestActivateDueRuns_LeavesFutureRunsDelayed is a companion test that
// proves ActivateDueRuns respects the scheduled_at gate: a delayed run
// with a future scheduled_at must not be promoted.
func TestActivateDueRuns_LeavesFutureRunsDelayed(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-queue-delayed-future")

	future := time.Now().UTC().Add(time.Hour)
	run := &domain.JobRun{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Status:      domain.StatusDelayed,
		Attempt:     1,
		Priority:    0,
		TriggeredBy: domain.TriggerManual,
		ScheduledAt: &future,
	}
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	promoted, err := st.ActivateDueRuns(ctx, 100)
	if err != nil {
		t.Fatalf("ActivateDueRuns: %v", err)
	}
	if promoted != 0 {
		t.Fatalf("ActivateDueRuns promoted = %d, want 0 (future scheduled_at)", promoted)
	}

	after, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if after.Status != domain.StatusDelayed {
		t.Fatalf("status = %q, want %q (future run must stay delayed)", after.Status, domain.StatusDelayed)
	}

	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if dequeued != nil {
		t.Fatalf("Dequeue returned run %q, want nil", dequeued.ID)
	}
}
