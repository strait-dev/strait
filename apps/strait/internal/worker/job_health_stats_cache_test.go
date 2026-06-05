package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	orcstore "strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newExecutorWithHealthStatsCache(t *testing.T, ttl time.Duration, store ExecutorStore) *Executor {
	t.Helper()
	e := &Executor{store: store}
	if ttl > 0 {
		e.jobHealthCache = newTierJobHealthCache(ttl)
	}
	return e
}

// Cache disabled (TTL=0) should reach the store on every call.
func TestGetJobHealthStats_DisabledPassesThrough(t *testing.T) {
	var calls atomic.Int32
	want := &orcstore.JobHealthStats{TotalRuns: 7, P95DurationSecs: 1.5}
	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			calls.Add(1)
			return want, nil
		},
	}
	e := newExecutorWithHealthStatsCache(t, 0, store)

	for range 5 {
		got, err := e.getJobHealthStats(t.Context(), "job-1", time.Now())
		require.NoError(
			t, err)
		require.True(t,
			sameJobHealthStats(got, want))
	}
	require.EqualValues(t, 5, calls.
		Load())
}

// With TTL set, repeated calls for the same jobID should hit the store once.
func TestGetJobHealthStats_TTLServesFromCache(t *testing.T) {
	var calls atomic.Int32
	want := &orcstore.JobHealthStats{TotalRuns: 42, P95DurationSecs: 0.25}
	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			calls.Add(1)
			return want, nil
		},
	}
	e := newExecutorWithHealthStatsCache(t, 5*time.Second, store)

	for range 20 {
		got, err := e.getJobHealthStats(t.Context(), "job-7", time.Now())
		require.NoError(
			t, err)
		require.True(t,
			sameJobHealthStats(got, want))
	}
	require.EqualValues(t, 1, calls.
		Load())
}

// Concurrent misses for the same jobID must collapse into a single store call
// via the singleflight group, since under 1000-VU load every dispatch hits this
// path simultaneously.
func TestGetJobHealthStats_SingleflightCoalescesMisses(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	want := &orcstore.JobHealthStats{TotalRuns: 11}
	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			calls.Add(1)
			<-release // hold the first call open so concurrent callers queue behind it
			return want, nil
		},
	}
	e := newExecutorWithHealthStatsCache(t, 5*time.Second, store)

	const fanout = 50
	var wg sync.WaitGroup
	wg.Add(fanout)
	for range fanout {
		go func() {
			defer wg.Done()
			if _, err := e.getJobHealthStats(t.Context(), "job-x", time.Now()); err != nil {
				assert.Failf(t, "test failure",

					"unexpected error: %v", err)
			}
		}()
	}
	// Give the goroutines a beat to all enter the singleflight before unblocking.
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	require.EqualValues(t, 1, calls.
		Load())
}

// Errors must propagate (and not poison the cache: a follow-up successful call
// can re-populate it).
func TestGetJobHealthStats_ErrorIsNotCached(t *testing.T) {
	var calls atomic.Int32
	storeErr := errors.New("transient")
	want := &orcstore.JobHealthStats{TotalRuns: 3}
	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			if calls.Add(1) == 1 {
				return nil, storeErr
			}
			return want, nil
		},
	}
	e := newExecutorWithHealthStatsCache(t, time.Minute, store)

	if _, err := e.getJobHealthStats(t.Context(), "job-err", time.Now()); !errors.Is(err, storeErr) {
		require.Failf(t, "test failure",

			"first call: err = %v, want %v", err, storeErr)
	}
	got, err := e.getJobHealthStats(t.Context(), "job-err", time.Now())
	require.NoError(
		t, err)
	require.True(t,
		sameJobHealthStats(got, want))
	require.EqualValues(t, 2, calls.
		Load())
}

func sameJobHealthStats(got, want *orcstore.JobHealthStats) bool {
	if got == nil || want == nil {
		return got == want
	}
	return got.TotalRuns == want.TotalRuns &&
		got.P95DurationSecs == want.P95DurationSecs &&
		got.P99DurationSecs == want.P99DurationSecs &&
		got.AvgDurationSecs == want.AvgDurationSecs
}
