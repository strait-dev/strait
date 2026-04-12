//go:build integration

package queue_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
)

// Suite 5: Idempotency and replay tests.

func TestIdempotency_SameKeyNoDuplicate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-idem-same")
	q := mustQueue(t)
	r1 := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, IdempotencyKey: "key-x"}
	if err := q.Enqueue(ctx, r1); err != nil {
		t.Fatalf("first: %v", err)
	}
	r2 := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, IdempotencyKey: "key-x"}
	if err := q.Enqueue(ctx, r2); err != nil {
		t.Fatalf("second: %v", err)
	}
	var count int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs WHERE job_id=$1`, job.ID).Scan(&count)
	if count != 1 {
		t.Errorf("count = %d, want 1 (deduped)", count)
	}
}

func TestIdempotency_SameKeyAfterTerminalAllowsReenqueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-idem-terminal")
	q := mustQueue(t)
	r1 := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, IdempotencyKey: "key-y"}
	_ = q.Enqueue(ctx, r1)
	_, _ = testDB.Pool.Exec(ctx, `UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, r1.ID)

	r2 := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, IdempotencyKey: "key-y"}
	if err := q.Enqueue(ctx, r2); err != nil {
		t.Fatalf("re-enqueue after terminal: %v", err)
	}
	var count int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs WHERE job_id=$1`, job.ID).Scan(&count)
	if count != 2 {
		t.Errorf("count = %d, want 2 (terminal allows re-enqueue)", count)
	}
}

func TestIdempotency_ConcurrentSameKey10Goroutines(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-idem-conc")
	q := mustQueue(t)

	var wg sync.WaitGroup
	var successes atomic.Int64
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, IdempotencyKey: "key-race"}
			if err := q.Enqueue(ctx, r); err == nil && !r.CreatedAt.IsZero() {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	var count int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs WHERE job_id=$1`, job.ID).Scan(&count)
	if count != 1 {
		t.Errorf("concurrent idempotency: count = %d, want 1", count)
	}
}

func TestIdempotency_OutboxWriteSameIDTwice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-idem-outbox")

	fixedID := newID()
	for range 3 {
		tx, _ := testDB.Pool.Begin(ctx)
		_ = queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{{
			ID: fixedID, ProjectID: job.ProjectID, JobID: job.ID,
		}})
		_ = tx.Commit(ctx)
	}
	qs := store.New(testDB.Pool)
	count, _ := qs.CountUnconsumedOutbox(ctx)
	if count != 1 {
		t.Errorf("outbox count = %d, want 1 (ON CONFLICT DO NOTHING)", count)
	}
}

func TestIdempotency_BackpressureTryConsumeAtomic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    1,
		DefaultRefillPerSec: 0,
	}, true)
	project := "proj-idem-bp"

	var allowed atomic.Int64
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := bp.TryConsume(ctx, project); err == nil {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 1 {
		t.Errorf("allowed = %d, want exactly 1 (atomic consume)", allowed.Load())
	}
}

func TestIdempotency_SKIPLOCKEDExactlyOneClaimer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-idem-skip")
	q := mustQueue(t)
	mustEnqueueRun(t, ctx, q, job)

	var claimed atomic.Int64
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := q.Dequeue(ctx)
			if err == nil && r != nil {
				claimed.Add(1)
			}
		}()
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Errorf("claimed = %d, want exactly 1", claimed.Load())
	}
}
