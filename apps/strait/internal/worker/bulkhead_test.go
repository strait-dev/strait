package worker

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"

	"strait/internal/domain"
)

func TestShardedBulkhead_AcquireUpToLimit(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for i := range 5 {
		if !b.TryAcquire("job-1", 5) {
			t.Fatalf("slot %d should be acquired", i+1)
		}
	}
}

func TestShardedBulkhead_RejectsOverLimit(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for range 3 {
		b.TryAcquire("job-1", 3)
	}
	if b.TryAcquire("job-1", 3) {
		t.Fatal("should reject over limit")
	}
}

func TestShardedBulkhead_ReleaseAllowsReacquire(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for range 3 {
		b.TryAcquire("job-1", 3)
	}
	b.Release("job-1", 3)
	if !b.TryAcquire("job-1", 3) {
		t.Fatal("should acquire after release")
	}
}

func TestShardedBulkhead_ReleaseAll(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for range 5 {
		b.TryAcquire("job-1", 10)
	}
	for range 5 {
		b.Release("job-1", 10)
	}
	if count := b.ActiveCount("job-1"); count != 0 {
		t.Fatalf("ActiveCount = %d, want 0", count)
	}
}

func TestShardedBulkhead_MultipleJobs_Independent(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for range 3 {
		b.TryAcquire("job-A", 3)
	}
	if !b.TryAcquire("job-B", 3) {
		t.Fatal("job-B should be independent of job-A")
	}
}

func TestShardedBulkhead_MultipleJobs_EachHasOwnLimit(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for range 3 {
		b.TryAcquire("job-A", 3)
	}
	for range 5 {
		b.TryAcquire("job-B", 5)
	}
	if b.TryAcquire("job-A", 3) {
		t.Fatal("job-A should be at limit 3")
	}
	if b.TryAcquire("job-B", 5) {
		t.Fatal("job-B should be at limit 5")
	}
}

func TestShardedBulkhead_DefaultLimitApplied(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(3)
	for range 3 {
		b.TryAcquire("job-1", 0)
	}
	if b.TryAcquire("job-1", 0) {
		t.Fatal("default limit 3 should reject 4th acquire")
	}
}

func TestShardedBulkhead_ExplicitOverridesDefault(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(3)
	for range 5 {
		if !b.TryAcquire("job-1", 5) {
			t.Fatal("explicit limit 5 should allow up to 5")
		}
	}
	if b.TryAcquire("job-1", 5) {
		t.Fatal("should reject at explicit limit 5")
	}
}

func TestShardedBulkhead_DefaultZeroUnlimited(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for i := range 1000 {
		if !b.TryAcquire("job-1", 0) {
			t.Fatalf("slot %d should succeed with no limit", i+1)
		}
	}
}

func TestShardedBulkhead_CleanupOnFullRelease(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	b.TryAcquire("job-1", 5)
	b.Release("job-1", 5)
	if count := b.ActiveCount("job-1"); count != 0 {
		t.Fatalf("ActiveCount = %d after full release, want 0", count)
	}
}

func TestShardedBulkhead_ConcurrentSameJob(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	const limit = 50
	const goroutines = 200

	start := make(chan struct{})
	var successes atomic.Int32
	var wg conc.WaitGroup

	for range goroutines {
		wg.Go(func() {
			<-start
			if b.TryAcquire("job-1", limit) {
				successes.Add(1)
			}
		})
	}

	close(start)
	wg.Wait()

	if got := successes.Load(); got != limit {
		t.Fatalf("successes = %d, want %d", got, limit)
	}
}

func TestShardedBulkhead_ConcurrentMultipleJobs(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	const jobCount = 10
	const limit = 5
	const goroutinesPerJob = 10

	var wg conc.WaitGroup
	results := make([]atomic.Int32, jobCount)

	for j := range jobCount {
		jobID := fmt.Sprintf("job-%d", j)
		for range goroutinesPerJob {
			wg.Go(func() {
				if b.TryAcquire(jobID, limit) {
					results[j].Add(1)
				}
			})
		}
	}

	wg.Wait()

	for j := range jobCount {
		if got := results[j].Load(); got > limit {
			t.Fatalf("job-%d: acquired %d slots, max should be %d", j, got, limit)
		}
	}
}

func TestShardedBulkhead_ConcurrentAcquireRelease(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	const goroutines = 100
	const iterations = 1000
	const limit = 50

	var wg conc.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range iterations {
				if b.TryAcquire("job-1", limit) {
					b.Release("job-1", limit)
				}
			}
		})
	}

	wg.Wait()

	if count := b.ActiveCount("job-1"); count != 0 {
		t.Fatalf("ActiveCount = %d after all releases, want 0", count)
	}
}

func TestShardedBulkhead_ShardDistribution(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	shardsSeen := make(map[uint32]bool)

	for i := range 100 {
		jobID := fmt.Sprintf("job-%d", i)
		b.TryAcquire(jobID, 100)

		h := fnv.New32a()
		_, _ = h.Write([]byte(jobID))
		shardsSeen[h.Sum32()%numShards] = true
	}

	if len(shardsSeen) < 10 {
		t.Fatalf("only %d shards used across 100 jobs, want at least 10", len(shardsSeen))
	}
}

func TestShardedBulkhead_ReleaseWithoutAcquire(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	// Should not panic.
	b.Release("never-acquired", 5)
	if count := b.ActiveCount("never-acquired"); count != 0 {
		t.Fatalf("ActiveCount = %d, want 0", count)
	}
}

func TestShardedBulkhead_ExplicitLimitOne(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	if !b.TryAcquire("job-1", 1) {
		t.Fatal("first acquire should succeed")
	}
	if b.TryAcquire("job-1", 1) {
		t.Fatal("second acquire should fail with limit 1")
	}
	b.Release("job-1", 1)
	if !b.TryAcquire("job-1", 1) {
		t.Fatal("acquire after release should succeed")
	}
}

func TestExecutorBulkhead_DefaultAppliedWhenJobHasNoLimit(t *testing.T) {
	t.Parallel()

	exec := newBulkheadTestExecutor(t, 3)

	for i := range 3 {
		if !exec.tryAcquireBulkheadSlot("job-1", 0) {
			t.Fatalf("slot %d should be acquired", i+1)
		}
	}
	if exec.tryAcquireBulkheadSlot("job-1", 0) {
		t.Fatal("4th slot should be rejected with default concurrency 3")
	}
}

func TestExecutorBulkhead_ExplicitOverridesDefault(t *testing.T) {
	t.Parallel()

	exec := newBulkheadTestExecutor(t, 3)

	for i := range 5 {
		if !exec.tryAcquireBulkheadSlot("job-1", 5) {
			t.Fatalf("slot %d should be acquired with explicit limit 5", i+1)
		}
	}
	if exec.tryAcquireBulkheadSlot("job-1", 5) {
		t.Fatal("6th slot should be rejected with explicit limit 5")
	}
}

func TestExecutorBulkhead_DefaultZeroDisabled(t *testing.T) {
	t.Parallel()

	exec := newBulkheadTestExecutor(t, 0)

	for i := range 100 {
		if !exec.tryAcquireBulkheadSlot("job-1", 0) {
			t.Fatalf("slot %d should be acquired with no limit", i+1)
		}
	}
}

func TestExecutor_Bulkheads_AtCapacityRequeues(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		job := testJob(server.URL, 3, 5)
		job.MaxConcurrency = 1
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	exec.bulkhead.TryAcquire("job-1", 1)

	run := testRun(1)
	exec.execute(context.Background(), run)

	if called.Load() != 0 {
		t.Fatalf("dispatch called %d times, want 0", called.Load())
	}

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].from != domain.StatusDequeued || calls[0].to != domain.StatusQueued {
		t.Fatalf("transition = %s->%s, want %s->%s", calls[0].from, calls[0].to, domain.StatusDequeued, domain.StatusQueued)
	}
	if calls[0].fields["error"] != "job bulkhead at capacity" {
		t.Fatalf("error field = %v, want %q", calls[0].fields["error"], "job bulkhead at capacity")
	}
}

func TestExecutor_Bulkheads_EnabledUnderLimitExecutes(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		job := testJob(server.URL, 1, 5)
		job.MaxConcurrency = 1
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())

	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[0].to != domain.StatusExecuting || calls[1].to != domain.StatusCompleted {
		t.Fatalf("transitions = %s then %s, want executing then completed", calls[0].to, calls[1].to)
	}

	if count := exec.bulkhead.ActiveCount("job-1"); count != 0 {
		t.Fatalf("bulkhead active count = %d, want 0 (released)", count)
	}
}

func newBulkheadTestExecutor(t *testing.T, defaultLimit int) *Executor {
	t.Helper()

	pool := NewPool(10)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	return NewExecutor(ExecutorConfig{
		Pool:                     pool,
		Queue:                    &mockExecQueue{},
		Store:                    &mockExecutorStore{},
		PollInterval:             time.Hour,
		DefaultJobMaxConcurrency: defaultLimit,
	})
}

func BenchmarkShardedBulkhead_Contention(b *testing.B) {
	bh := NewShardedBulkhead(0)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if bh.TryAcquire("job-1", 100) {
				bh.Release("job-1", 100)
			}
		}
	})
}

func BenchmarkShardedBulkhead_MultiJob(b *testing.B) {
	bh := NewShardedBulkhead(0)
	var counter atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		id := counter.Add(1)
		jobID := fmt.Sprintf("job-%d", id)
		for pb.Next() {
			if bh.TryAcquire(jobID, 100) {
				bh.Release(jobID, 100)
			}
		}
	})
}
