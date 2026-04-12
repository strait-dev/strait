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
)

// Suite 2: Concurrency and race condition tests.

func TestConcurrency_50WorkersMaxConcurrency5(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-conc-mc5")
	_, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency = 5 WHERE id = $1`, job.ID)
	if err != nil {
		t.Fatalf("set mc: %v", err)
	}
	q := mustQueue(t)
	for range 100 {
		mustEnqueueRun(t, ctx, q, job)
	}

	var claimed atomic.Int64
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				batch, err := q.DequeueN(ctx, 1)
				if err != nil || len(batch) == 0 {
					return
				}
				claimed.Add(1)
			}
		}()
	}
	wg.Wait()
	if claimed.Load() > 5 {
		t.Errorf("claimed %d, want <= 5 (max_concurrency)", claimed.Load())
	}
}

func TestConcurrency_ThunderingHerdAfterNotify(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-conc-herd")
	q := mustQueue(t)
	mustEnqueueRun(t, ctx, q, job)

	// 20 workers all try to claim the single run simultaneously.
	var claimed atomic.Int64
	var wg sync.WaitGroup
	for range 20 {
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
		t.Errorf("claimed %d, want exactly 1 (SKIP LOCKED)", claimed.Load())
	}
}

func TestConcurrency_NoDuplicateClaimsUnder100Workers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-conc-nodup")
	q := mustQueue(t)
	for range 200 {
		mustEnqueueRun(t, ctx, q, job)
	}

	seen := &sync.Map{}
	var dupes atomic.Int64
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				batch, err := q.DequeueN(ctx, 5)
				if err != nil || len(batch) == 0 {
					return
				}
				for _, r := range batch {
					if _, loaded := seen.LoadOrStore(r.ID, true); loaded {
						dupes.Add(1)
					}
				}
			}
		}()
	}
	wg.Wait()
	if dupes.Load() > 0 {
		t.Errorf("found %d duplicate claims", dupes.Load())
	}
}

func TestConcurrency_HeartbeatConcurrentRegisterDeregister(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)

	// Concurrent upsert and delete on the heartbeat side table.
	var wg sync.WaitGroup
	for i := range 50 {
		id := newID()
		_ = i
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = st.UpsertHeartbeatSideTable(ctx, id)
		}()
		go func() {
			defer wg.Done()
			_ = st.DeleteHeartbeatSideTable(ctx, []string{id})
		}()
	}
	wg.Wait()
	// No panics, no deadlocks — that's the invariant.
}

func TestConcurrency_CursorAdvanceUnder20Goroutines(t *testing.T) {
	c := queue.NewClaimCursor(60 * time.Second)
	var wg sync.WaitGroup
	base := time.Now()
	for g := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 500 {
				ts := base.Add(time.Duration(g*500+i) * time.Microsecond)
				c.Advance(ts, newID())
			}
		}()
	}
	wg.Wait()
	_, _, ok := c.Snapshot()
	if !ok {
		t.Error("cursor should be valid after 10k advances")
	}
}

func TestConcurrency_CounterInvariantUnderStorm(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-conc-storm")
	q := mustQueue(t)

	// Enqueue 50 runs.
	for range 50 {
		mustEnqueueRun(t, ctx, q, job)
	}

	// 10 workers claim, complete, and fail runs in parallel.
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				batch, err := q.DequeueN(ctx, 1)
				if err != nil || len(batch) == 0 {
					return
				}
				// Half complete, half fail.
				for _, r := range batch {
					if r.Priority%2 == 0 {
						_, _ = testDB.Pool.Exec(ctx, `UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, r.ID)
					} else {
						_, _ = testDB.Pool.Exec(ctx, `UPDATE job_runs SET status='dead_letter', finished_at=NOW() WHERE id=$1`, r.ID)
					}
				}
			}
		}()
	}
	wg.Wait()

	// Counter must match ground truth.
	assertActiveCountsInvariant(t, ctx, job.ID)
	assertDLQCountsInvariant(t, ctx, job.ID)
}

func TestConcurrency_EnqueueAndDequeueSimultaneously(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-conc-enqdq")
	q := mustQueue(t)

	var enqueued, dequeued atomic.Int64
	var wg sync.WaitGroup
	// Enqueue goroutines.
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				r := &domain.JobRun{
					ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
				}
				if err := q.Enqueue(ctx, r); err == nil {
					enqueued.Add(1)
				}
			}
		}()
	}
	// Dequeue goroutines.
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				batch, err := q.DequeueN(ctx, 2)
				if err == nil {
					dequeued.Add(int64(len(batch)))
				}
			}
		}()
	}
	wg.Wait()
	t.Logf("enqueued=%d dequeued=%d", enqueued.Load(), dequeued.Load())
	if dequeued.Load() > enqueued.Load() {
		t.Errorf("dequeued more than enqueued: %d > %d", dequeued.Load(), enqueued.Load())
	}
}

func TestConcurrency_BackpressureConcurrentConsume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    20,
		DefaultRefillPerSec: 0,
	}, true)
	project := "proj-conc-bp"

	var allowed, throttled atomic.Int64
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := bp.TryConsume(ctx, project)
			if err == nil {
				allowed.Add(1)
			} else if _, ok := queue.AsThrottled(err); ok {
				throttled.Add(1)
			}
		}()
	}
	wg.Wait()
	total := allowed.Load() + throttled.Load()
	if total != 50 {
		t.Errorf("total = %d, want 50", total)
	}
	if allowed.Load() > 21 {
		t.Errorf("allowed %d > max 21 (20 + first-insert)", allowed.Load())
	}
}
