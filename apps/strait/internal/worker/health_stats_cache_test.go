package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	orcstore "strait/internal/store"
)

func newHealthStatsTestExecutor(t *testing.T, store *mockExecutorStore, jobCacheTTL time.Duration) *Executor {
	t.Helper()
	pool := NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	exec := NewExecutor(ExecutorConfig{
		Pool:         pool,
		Store:        store,
		PollInterval: time.Millisecond,
		JobCacheTTL:  jobCacheTTL,
	})
	t.Cleanup(exec.CloseCache)
	return exec
}

func TestCachedJobHealthStats_MemoizesPerJob(t *testing.T) {
	t.Parallel()

	var loads atomic.Int64
	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			loads.Add(1)
			return &orcstore.JobHealthStats{P95DurationSecs: 1.5}, nil
		},
	}

	exec := newHealthStatsTestExecutor(t, store, 5*time.Minute)
	since := time.Now().Add(-24 * time.Hour)

	for range 4 {
		stats, err := exec.cachedJobHealthStats(context.Background(), "job-1", since)
		if err != nil {
			t.Fatalf("cachedJobHealthStats() error = %v", err)
		}
		if stats == nil || stats.P95DurationSecs != 1.5 {
			t.Fatalf("unexpected stats: %+v", stats)
		}
	}

	if n := loads.Load(); n != 1 {
		t.Fatalf("health stats loaded %d times, want 1 (cached after first)", n)
	}

	// A different job is a distinct cache key and triggers its own load.
	if _, err := exec.cachedJobHealthStats(context.Background(), "job-2", since); err != nil {
		t.Fatalf("cachedJobHealthStats(job-2) error = %v", err)
	}
	if n := loads.Load(); n != 2 {
		t.Fatalf("health stats loaded %d times, want 2 after second job", n)
	}
}

func TestCachedJobHealthStats_DisabledHitsStoreEachTime(t *testing.T) {
	t.Parallel()

	var loads atomic.Int64
	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			loads.Add(1)
			return &orcstore.JobHealthStats{}, nil
		},
	}

	exec := newHealthStatsTestExecutor(t, store, 0) // caching disabled
	since := time.Now().Add(-24 * time.Hour)

	for range 3 {
		if _, err := exec.cachedJobHealthStats(context.Background(), "job-1", since); err != nil {
			t.Fatalf("cachedJobHealthStats() error = %v", err)
		}
	}

	if n := loads.Load(); n != 3 {
		t.Fatalf("health stats loaded %d times, want 3 (cache disabled)", n)
	}
}

func TestCachedJobHealthStats_ErrorNotCached(t *testing.T) {
	t.Parallel()

	var loads atomic.Int64
	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			n := loads.Add(1)
			if n == 1 {
				return nil, context.DeadlineExceeded
			}
			return &orcstore.JobHealthStats{P95DurationSecs: 2.0}, nil
		},
	}

	exec := newHealthStatsTestExecutor(t, store, 5*time.Minute)
	since := time.Now().Add(-24 * time.Hour)

	if _, err := exec.cachedJobHealthStats(context.Background(), "job-1", since); err == nil {
		t.Fatal("expected error on first call")
	}
	stats, err := exec.cachedJobHealthStats(context.Background(), "job-1", since)
	if err != nil {
		t.Fatalf("second call error = %v", err)
	}
	if stats == nil || stats.P95DurationSecs != 2.0 {
		t.Fatalf("unexpected stats after retry: %+v", stats)
	}
	if n := loads.Load(); n != 2 {
		t.Fatalf("loads = %d, want 2 (error result must not be cached)", n)
	}
}
