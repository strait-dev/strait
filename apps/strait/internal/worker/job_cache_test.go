package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/eko/gocache/lib/v4/cache"
	gocachestore "github.com/eko/gocache/store/go_cache/v4"
	gocache "github.com/patrickmn/go-cache"
)

func newTestJobCache(ttl time.Duration) *cache.Cache[*domain.Job] {
	gc := gocache.New(ttl, 2*ttl)
	return cache.New[*domain.Job](gocachestore.NewGoCache(gc))
}

func TestJobCache_HitAvoidsDatabaseLookup(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(5 * time.Second)
	ctx := context.Background()

	job := &domain.Job{
		ID:      "job-1",
		Version: 1,
		Name:    "test-job",
	}

	// Seed cache.
	if err := jobCache.Set(ctx, "job-1", job); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Get should return the cached job.
	cached, err := jobCache.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cached.ID != "job-1" || cached.Version != 1 {
		t.Fatalf("cached job = %+v, want ID=job-1 Version=1", cached)
	}
}

func TestJobCache_MissReturnsError(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(5 * time.Second)
	ctx := context.Background()

	_, err := jobCache.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error on cache miss, got nil")
	}
}

func TestJobCache_ExpiresAfterTTL(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(1 * time.Millisecond)
	ctx := context.Background()

	job := &domain.Job{ID: "job-ttl", Version: 1}
	if err := jobCache.Set(ctx, "job-ttl", job); err != nil {
		t.Fatalf("Set: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	_, err := jobCache.Get(ctx, "job-ttl")
	if err == nil {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestJobCache_OverwriteUpdatesValue(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(5 * time.Second)
	ctx := context.Background()

	v1 := &domain.Job{ID: "job-ow", Version: 1, Name: "old-name"}
	v2 := &domain.Job{ID: "job-ow", Version: 2, Name: "new-name"}

	_ = jobCache.Set(ctx, "job-ow", v1)
	_ = jobCache.Set(ctx, "job-ow", v2)

	cached, err := jobCache.Get(ctx, "job-ow")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cached.Version != 2 || cached.Name != "new-name" {
		t.Fatalf("expected v2, got %+v", cached)
	}
}

func TestJobCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(5 * time.Second)
	ctx := context.Background()

	const goroutines = 50
	var wg sync.WaitGroup

	// Writers.
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			job := &domain.Job{ID: "job-conc", Version: i}
			_ = jobCache.Set(ctx, "job-conc", job)
		}(i)
	}

	// Readers.
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_, _ = jobCache.Get(ctx, "job-conc")
		}()
	}

	wg.Wait()

	// Should not panic or race. Final value should be one of the written versions.
	cached, err := jobCache.Get(ctx, "job-conc")
	if err != nil {
		t.Fatalf("Get after concurrent writes: %v", err)
	}
	if cached.ID != "job-conc" {
		t.Fatalf("unexpected job ID: %s", cached.ID)
	}
}

func TestJobCache_Delete(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(5 * time.Second)
	ctx := context.Background()

	job := &domain.Job{ID: "job-del", Version: 1}
	_ = jobCache.Set(ctx, "job-del", job)

	if err := jobCache.Delete(ctx, "job-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := jobCache.Get(ctx, "job-del")
	if err == nil {
		t.Fatal("expected miss after delete")
	}
}

func TestJobCache_MultipleKeys(t *testing.T) {
	t.Parallel()

	jobCache := newTestJobCache(5 * time.Second)
	ctx := context.Background()

	for i := range 100 {
		job := &domain.Job{ID: "job-multi", Version: i}
		_ = jobCache.Set(ctx, job.ID+string(rune('a'+i)), job)
	}

	// Verify a sample.
	for _, i := range []int{0, 50, 99} {
		key := "job-multi" + string(rune('a'+i))
		cached, err := jobCache.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get(%s): %v", key, err)
		}
		if cached.Version != i {
			t.Fatalf("Version for key %s = %d, want %d", key, cached.Version, i)
		}
	}
}

func TestJobCache_NilCacheDisablesLookup(t *testing.T) {
	t.Parallel()

	// When JobCacheTTL is 0, the executor should not create a cache.
	// Verify the executor handles nil jobCache gracefully.
	var dbCalls atomic.Int64
	mockStore := &mockExecutorStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			dbCalls.Add(1)
			return &domain.Job{ID: id, Version: 1}, nil
		},
	}

	e := &Executor{
		store:    mockStore,
		jobCache: nil, // disabled
	}

	ctx := context.Background()
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	job, err := e.resolveJobForRun(ctx, run)
	if err != nil {
		t.Fatalf("resolveJobForRun: %v", err)
	}
	if job.ID != "job-1" {
		t.Fatalf("job ID = %q, want job-1", job.ID)
	}
	if dbCalls.Load() != 1 {
		t.Fatalf("DB calls = %d, want 1", dbCalls.Load())
	}

	// Second call should also hit DB since cache is nil.
	_, _ = e.resolveJobForRun(ctx, run)
	if dbCalls.Load() != 2 {
		t.Fatalf("DB calls = %d, want 2 (no cache)", dbCalls.Load())
	}
}

func TestJobCache_ResolveJobForRun_CacheHit(t *testing.T) {
	t.Parallel()

	var dbCalls atomic.Int64
	mockStore := &mockExecutorStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			dbCalls.Add(1)
			return &domain.Job{ID: id, Version: 1}, nil
		},
	}

	e := &Executor{
		store:    mockStore,
		jobCache: newTestJobCache(5 * time.Second),
	}

	ctx := context.Background()
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	// First call: cache miss, hits DB.
	job, err := e.resolveJobForRun(ctx, run)
	if err != nil {
		t.Fatalf("resolveJobForRun: %v", err)
	}
	if job.ID != "job-1" {
		t.Fatalf("job ID = %q, want job-1", job.ID)
	}
	if dbCalls.Load() != 1 {
		t.Fatalf("DB calls = %d, want 1", dbCalls.Load())
	}

	// Second call: cache hit, no DB call.
	_, err = e.resolveJobForRun(ctx, run)
	if err != nil {
		t.Fatalf("resolveJobForRun: %v", err)
	}
	if dbCalls.Load() != 1 {
		t.Fatalf("DB calls = %d, want 1 (cache hit)", dbCalls.Load())
	}
}

func TestJobCache_ResolveJobForRun_CacheExpiry(t *testing.T) {
	t.Parallel()

	var dbCalls atomic.Int64
	mockStore := &mockExecutorStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			dbCalls.Add(1)
			return &domain.Job{ID: id, Version: 1}, nil
		},
	}

	e := &Executor{
		store:    mockStore,
		jobCache: newTestJobCache(1 * time.Millisecond),
	}

	ctx := context.Background()
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1}

	// First call: populates cache.
	_, _ = e.resolveJobForRun(ctx, run)
	if dbCalls.Load() != 1 {
		t.Fatalf("DB calls = %d, want 1", dbCalls.Load())
	}

	time.Sleep(5 * time.Millisecond)

	// After TTL: cache miss, hits DB again.
	_, _ = e.resolveJobForRun(ctx, run)
	if dbCalls.Load() != 2 {
		t.Fatalf("DB calls = %d, want 2 (cache expired)", dbCalls.Load())
	}
}
