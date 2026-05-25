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
	ctx := context.Background()

	// First call: miss, loads from DB.
	got, err := c.Get(ctx, "p1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.MaxQueuedRuns != 100 {
		t.Fatalf("Get returned %+v, want MaxQueuedRuns=100", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("DB calls = %d, want 1", calls.Load())
	}

	// Second call: hit, no DB call.
	got, err = c.Get(ctx, "p1")
	if err != nil {
		t.Fatalf("Get (cached): %v", err)
	}
	if got == nil || got.MaxQueuedRuns != 100 {
		t.Fatalf("cached Get returned %+v", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("DB calls = %d, want 1 (cache hit)", calls.Load())
	}
}

func TestQuotaCache_Invalidate(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	c := newQuotaCacheWithLoader(5*time.Second, &calls, &store.ProjectQuota{ProjectID: "p1"}, nil)
	ctx := context.Background()

	_, _ = c.Get(ctx, "p1")
	_, _ = c.Get(ctx, "p1")
	if calls.Load() != 1 {
		t.Fatalf("pre-invalidate DB calls = %d, want 1", calls.Load())
	}

	c.Invalidate("p1")

	_, _ = c.Get(ctx, "p1")
	if calls.Load() != 2 {
		t.Fatalf("post-invalidate DB calls = %d, want 2", calls.Load())
	}
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

	ctx := context.Background()
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

	if calls.Load() != 1 {
		t.Fatalf("DB calls = %d, want 1 (singleflight dedupe)", calls.Load())
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
		if results[i] == nil || results[i].MaxQueuedRuns != 42 {
			t.Fatalf("goroutine %d: %+v", i, results[i])
		}
	}
}

func TestQuotaCache_TTLExpiry(t *testing.T) {
	t.Parallel()

	// Otter's timer wheel granularity is ~1s, so TTL must be >= 1s.
	var calls atomic.Int64
	c := newQuotaCacheWithLoader(1*time.Second, &calls, &store.ProjectQuota{ProjectID: "p1"}, nil)
	ctx := context.Background()

	_, _ = c.Get(ctx, "p1")
	if calls.Load() != 1 {
		t.Fatalf("initial DB calls = %d, want 1", calls.Load())
	}

	time.Sleep(3 * time.Second)

	_, _ = c.Get(ctx, "p1")
	if calls.Load() != 2 {
		t.Fatalf("post-expiry DB calls = %d, want 2", calls.Load())
	}
}

func TestQuotaCache_PropagatesError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("load failed")
	var calls atomic.Int64
	c := newQuotaCacheWithLoader(5*time.Second, &calls, nil, sentinel)
	ctx := context.Background()

	_, err := c.Get(ctx, "p1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wraps %v", err, sentinel)
	}

	// Failed loads must not poison the cache. A subsequent successful load
	// should still issue a DB call rather than returning the previous error.
	c = newQuotaCacheWithLoader(5*time.Second, &calls, &store.ProjectQuota{ProjectID: "p1"}, nil)
	got, err := c.Get(ctx, "p1")
	if err != nil {
		t.Fatalf("post-recovery Get: %v", err)
	}
	if got == nil {
		t.Fatal("post-recovery Get returned nil")
	}
}

func TestQuotaCache_NilQuotaIsCached(t *testing.T) {
	t.Parallel()

	// "No project_quotas row" is a legitimate result the trigger path treats
	// as "no per-project cap"; we must cache it just as eagerly as a real row.
	var calls atomic.Int64
	c := newQuotaCacheWithLoader(5*time.Second, &calls, nil, nil)
	ctx := context.Background()

	got, err := c.Get(ctx, "p1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Fatalf("Get returned %+v, want nil", got)
	}

	_, _ = c.Get(ctx, "p1")
	if calls.Load() != 1 {
		t.Fatalf("DB calls = %d, want 1 (nil quota cached)", calls.Load())
	}
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

	got, err := c.Get(context.Background(), "project-versioned")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.CacheVersion != 9 {
		t.Fatalf("Get() CacheVersion = %v, want 9", got)
	}

	raw, err := deps.Redis.Get(context.Background(), "strait:cache:"+quotaCacheNamespace+":project-versioned").Bytes()
	if err != nil {
		t.Fatalf("read redis entry: %v", err)
	}
	var envelope struct {
		Version int64 `json:"version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode redis entry: %v", err)
	}
	if envelope.Version != 9 {
		t.Fatalf("redis version = %d, want 9", envelope.Version)
	}
	if calls.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", calls.Load())
	}
}

func TestQuotaCache_Disabled(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	c := newQuotaCacheWithLoader(0, &calls, &store.ProjectQuota{ProjectID: "p1"}, nil)
	ctx := context.Background()

	_, _ = c.Get(ctx, "p1")
	_, _ = c.Get(ctx, "p1")
	_, _ = c.Get(ctx, "p1")
	if calls.Load() != 3 {
		t.Fatalf("DB calls = %d, want 3 (caching disabled)", calls.Load())
	}
}
