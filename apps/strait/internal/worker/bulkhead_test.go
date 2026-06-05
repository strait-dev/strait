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
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

func TestShardedBulkhead_AcquireUpToLimit(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for range 5 {
		require.True(t,
			b.TryAcquire("job-1",
				5))

	}
}

func TestShardedBulkhead_RejectsOverLimit(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for range 3 {
		b.TryAcquire("job-1", 3)
	}
	require.False(t,
		b.TryAcquire("job-1",
			3))

}

func TestShardedBulkhead_ReleaseAllowsReacquire(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for range 3 {
		b.TryAcquire("job-1", 3)
	}
	b.Release("job-1", 3)
	require.True(t,
		b.TryAcquire("job-1",
			3))

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
	require.EqualValues(t, 0, b.ActiveCount("job-1"))

}

func TestShardedBulkhead_MultipleJobs_Independent(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for range 3 {
		b.TryAcquire("job-A", 3)
	}
	require.True(t,
		b.TryAcquire("job-B",
			3))

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
	require.False(t,
		b.TryAcquire("job-A",
			3))
	require.False(t,
		b.TryAcquire("job-B",
			5))

}

func TestShardedBulkhead_DefaultLimitApplied(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(3)
	for range 3 {
		b.TryAcquire("job-1", 0)
	}
	require.False(t,
		b.TryAcquire("job-1",
			0))

}

func TestShardedBulkhead_ExplicitOverridesDefault(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(3)
	for range 5 {
		require.True(t,
			b.TryAcquire("job-1",
				5))

	}
	require.False(t,
		b.TryAcquire("job-1",
			5))

}

func TestShardedBulkhead_DefaultZeroUnlimited(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	for range 1000 {
		require.True(t,
			b.TryAcquire("job-1",
				0))

	}
}

func TestShardedBulkhead_CleanupOnFullRelease(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	b.TryAcquire("job-1", 5)
	b.Release("job-1", 5)
	require.EqualValues(t, 0, b.ActiveCount("job-1"))

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
	require.EqualValues(t,
		limit,
		successes.Load())

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
		require.LessOrEqual(t,
			results[j].Load(), int32(limit),
		)

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
	require.EqualValues(t, 0, b.ActiveCount("job-1"))

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
	require.GreaterOrEqual(
		t, len(shardsSeen), 10)

}

func TestShardedBulkhead_ReleaseWithoutAcquire(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	// Should not panic.
	b.Release("never-acquired", 5)
	require.EqualValues(t, 0, b.ActiveCount("never-acquired"))

}

func TestShardedBulkhead_ExplicitLimitOne(t *testing.T) {
	t.Parallel()
	b := NewShardedBulkhead(0)
	require.True(t,
		b.TryAcquire("job-1",
			1))
	require.False(t,
		b.TryAcquire("job-1",
			1))

	b.Release("job-1", 1)
	require.True(t,
		b.TryAcquire("job-1",
			1))

}

func TestExecutorBulkhead_DefaultAppliedWhenJobHasNoLimit(t *testing.T) {
	t.Parallel()

	exec := newBulkheadTestExecutor(t, 3)

	for range 3 {
		require.True(t,
			exec.tryAcquireBulkheadSlot("job-1",
				0))

	}
	require.False(t,
		exec.tryAcquireBulkheadSlot("job-1",
			0))

}

func TestExecutorBulkhead_ExplicitOverridesDefault(t *testing.T) {
	t.Parallel()

	exec := newBulkheadTestExecutor(t, 3)

	for range 5 {
		require.True(t,
			exec.tryAcquireBulkheadSlot("job-1",
				5))

	}
	require.False(t,
		exec.tryAcquireBulkheadSlot("job-1",
			5))

}

func TestExecutorBulkhead_DefaultZeroDisabled(t *testing.T) {
	t.Parallel()

	exec := newBulkheadTestExecutor(t, 0)

	for range 100 {
		require.True(t,
			exec.tryAcquireBulkheadSlot("job-1",
				0))

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
	require.EqualValues(t, 0, called.
		Load())

	calls := store.statusUpdates()
	require.Len(t, calls,
		1,
	)
	require.False(t,
		calls[0].from != domain.
			StatusDequeued ||
			calls[0].
				to != domain.StatusQueued,
	)
	require.Equal(t,
		"job bulkhead at capacity",

		calls[0].fields["error"])

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
	require.Len(t, calls,
		2,
	)
	require.False(t,
		calls[0].to != domain.
			StatusExecuting ||
			calls[1].to !=
				domain.StatusCompleted,
	)
	require.EqualValues(t, 0, exec.
		bulkhead.ActiveCount("job-1"))

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
