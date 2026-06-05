package api

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"

	straitcache "strait/internal/cache"
	"strait/internal/store"
)

func newQuotaCacheWithLoader(ttl time.Duration, calls *atomic.Int64, q *store.ProjectQuota, err error) *quotaCache {
	return newQuotaCache(ttl, func(_ context.Context, _ string) (*store.ProjectQuota, error) {
		calls.Add(1)
		if err != nil {
			return nil, err
		}
		return q, nil
	})
}

func TestQuotaCache_HitAndMiss(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	q := &store.ProjectQuota{ProjectID: "p1", MaxQueuedRuns: 100}
	c := newQuotaCacheWithLoader(5*time.Second, &calls, q, nil)
	ctx := t.Context()

	// First call: miss, loads from DB.
	got, err := c.Get(ctx, "p1")
	require.NoError(t, err)
	require.False(t, got == nil ||
		got.MaxQueuedRuns !=
			100)
	require.EqualValues(t, 1, calls.Load())

	// Second call: hit, no DB call.
	got, err = c.Get(ctx, "p1")
	require.NoError(t, err)
	require.False(t, got == nil ||
		got.MaxQueuedRuns !=
			100)
	require.EqualValues(t, 1, calls.Load())

}

func TestQuotaCache_Invalidate(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	c := newQuotaCacheWithLoader(5*time.Second, &calls, &store.ProjectQuota{ProjectID: "p1"}, nil)
	ctx := t.Context()

	_, _ = c.Get(ctx, "p1")
	_, _ = c.Get(ctx, "p1")
	require.EqualValues(t, 1, calls.Load())

	c.Invalidate("p1")

	_, _ = c.Get(ctx, "p1")
	require.EqualValues(t, 2, calls.Load())

}

func TestQuotaCache_SingleflightDedupes(t *testing.T) {
	t.Parallel()

	const goroutines = 100

	var (
		calls atomic.Int64
		gate  = make(chan struct{})
	)
	c := newQuotaCache(5*time.Second, func(_ context.Context, _ string) (*store.ProjectQuota, error) {
		<-gate
		calls.Add(1)
		return &store.ProjectQuota{ProjectID: "p1", MaxQueuedRuns: 42}, nil
	})

	ctx := t.Context()
	var ready sync.WaitGroup
	ready.Add(goroutines)
	start := make(chan struct{})

	var wg conc.WaitGroup
	results := make([]*store.ProjectQuota, goroutines)
	errs := make([]error, goroutines)
	for i := range goroutines {
		wg.Go(func() {
			ready.Done()
			<-start
			got, err := c.Get(ctx, "p1")
			results[i] = got
			errs[i] = err
		})
	}

	ready.Wait()
	close(start)
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()
	require.EqualValues(t, 1, calls.Load())

	for i, err := range errs {
		require.NoError(t, err)
		require.False(t, results[i] ==
			nil || results[i].
			MaxQueuedRuns !=
			42)

	}
}

func TestQuotaCache_TTLExpiry(t *testing.T) {
	t.Parallel()

	// Otter's timer wheel granularity is ~1s, so TTL must be >= 1s.
	var calls atomic.Int64
	c := newQuotaCacheWithLoader(1*time.Second, &calls, &store.ProjectQuota{ProjectID: "p1"}, nil)
	ctx := t.Context()

	_, _ = c.Get(ctx, "p1")
	require.EqualValues(t, 1, calls.Load())

	time.Sleep(3 * time.Second)

	_, _ = c.Get(ctx, "p1")
	require.EqualValues(t, 2, calls.Load())

}

func TestQuotaCache_PropagatesError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("load failed")
	var calls atomic.Int64
	c := newQuotaCacheWithLoader(5*time.Second, &calls, nil, sentinel)
	ctx := t.Context()

	_, err := c.Get(ctx, "p1")
	require.True(
		t, errors.Is(err,
			sentinel),
	)

	// Failed loads must not poison the cache. A subsequent successful load
	// should still issue a DB call rather than returning the previous error.
	c = newQuotaCacheWithLoader(5*time.Second, &calls, &store.ProjectQuota{ProjectID: "p1"}, nil)
	got, err := c.Get(ctx, "p1")
	require.NoError(t, err)
	require.NotNil(t, got)

}

func TestQuotaCache_NilQuotaIsCached(t *testing.T) {
	t.Parallel()

	// "No project_quotas row" is a legitimate result the trigger path treats
	// as "no per-project cap"; we must cache it just as eagerly as a real row.
	var calls atomic.Int64
	c := newQuotaCacheWithLoader(5*time.Second, &calls, nil, nil)
	ctx := t.Context()

	got, err := c.Get(ctx, "p1")
	require.NoError(t, err)
	require.Nil(t, got)

	_, _ = c.Get(ctx, "p1")
	require.EqualValues(t, 1, calls.Load())

}

func TestQuotaCache_PreservesStoreCacheVersionInRedis(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	defer cleanup()

	var calls atomic.Int64
	c := newQuotaCache(5*time.Second, func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
		calls.Add(1)
		return &store.ProjectQuota{ProjectID: projectID, MaxQueuedRuns: 11, CacheVersion: 9}, nil
	}, deps)

	got, err := c.Get(t.Context(), "project-versioned")
	require.NoError(t, err)
	require.False(t, got == nil ||
		got.CacheVersion !=
			9)

	raw, err := deps.Redis.Get(t.Context(), "strait:cache:"+quotaCacheNamespace+":project-versioned").Bytes()
	require.NoError(t, err)

	var envelope struct {
		Version int64 `json:"version"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.EqualValues(t, 9, envelope.Version)
	require.EqualValues(t, 1, calls.Load())

}

func TestQuotaCache_StrongBarrierAllowsDBConfirmedNil(t *testing.T) {
	t.Parallel()

	deps, cleanup := newTestRedisCacheDeps(t, nil)
	defer cleanup()

	var calls atomic.Int64
	c := newQuotaCache(5*time.Second, func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
		calls.Add(1)
		return nil, nil
	}, deps)

	c.InvalidateWithVersion("project-nil", 10)
	got, err := c.Get(t.Context(), "project-nil")
	require.NoError(t, err)
	require.Nil(t, got)
	require.EqualValues(t, 1, calls.Load())

	raw, err := deps.Redis.Get(t.Context(), "strait:cache:"+quotaCacheNamespace+":project-nil").Bytes()
	require.NoError(t, err)

	var envelope struct {
		Version  int64 `json:"version"`
		Barrier  bool  `json:"barrier"`
		Negative bool  `json:"negative"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.False(t, envelope.Version !=
		10 ||
		envelope.
			Barrier ||
		!envelope.Negative,
	)

}

func TestQuotaCache_StrongBarrierRejectsStaleQuotaFill(t *testing.T) {
	t.Parallel()

	deps, cleanup := newTestRedisCacheDeps(t, nil)
	defer cleanup()

	c := newQuotaCache(5*time.Second, func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
		return &store.ProjectQuota{ProjectID: projectID, MaxQueuedRuns: 5, CacheVersion: 9}, nil
	}, deps)

	c.InvalidateWithVersion("project-stale", 10)
	_, err := c.Get(t.Context(), "project-stale")
	require.Error(t, err)

}

func TestQuotaCache_Disabled(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	c := newQuotaCacheWithLoader(0, &calls, &store.ProjectQuota{ProjectID: "p1"}, nil)
	ctx := t.Context()

	_, _ = c.Get(ctx, "p1")
	_, _ = c.Get(ctx, "p1")
	_, _ = c.Get(ctx, "p1")
	require.EqualValues(t, 3, calls.Load())

}
