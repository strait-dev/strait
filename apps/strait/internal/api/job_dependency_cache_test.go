package api

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

func TestJobDependencyCache_PreservesMaxDependencyCacheVersionInRedis(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	defer cleanup()
	cache := newJobDependencyCache(time.Minute, deps)

	key := jobDepsCacheKey{JobID: "job-versioned", Limit: 1000}
	var loads atomic.Int64
	got, err := cache.List(context.Background(), key, func(context.Context, jobDepsCacheKey) (straitcache.Versioned[[]domain.JobDependency], error) {
		loads.Add(1)
		deps := []domain.JobDependency{
			{ID: "dep-low", JobID: key.JobID, DependsOnJobID: "job-a", Condition: "completed", CacheVersion: 4},
			{ID: "dep-high", JobID: key.JobID, DependsOnJobID: "job-b", Condition: "failed", CacheVersion: 12},
		}
		return straitcache.Versioned[[]domain.JobDependency]{Value: deps, Version: jobDependenciesCacheVersion(deps)}, nil
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() len = %d, want 2", len(got))
	}

	raw, err := deps.Redis.Get(context.Background(), "strait:cache:"+jobDependencyCacheNamespace+":"+jobDepsCacheKeyString(key)).Bytes()
	if err != nil {
		t.Fatalf("read redis entry: %v", err)
	}
	var envelope struct {
		Version int64 `json:"version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode redis entry: %v", err)
	}
	if envelope.Version != 12 {
		t.Fatalf("redis version = %d, want 12", envelope.Version)
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loads.Load())
	}
}

func TestJobDependencyCache_InvalidateJobClearsKnownPageShapes(t *testing.T) {
	t.Parallel()

	deps, cleanup := newTestRedisCacheDeps(t, nil)
	defer cleanup()
	cache := newJobDependencyCache(time.Minute, deps)

	var loads atomic.Int64
	loader := func(_ context.Context, key jobDepsCacheKey) (straitcache.Versioned[[]domain.JobDependency], error) {
		loads.Add(1)
		dependencies := []domain.JobDependency{{
			ID:             "dep",
			JobID:          key.JobID,
			DependsOnJobID: "job-parent",
			Condition:      "completed",
			CacheVersion:   int64(key.Limit),
		}}
		return straitcache.Versioned[[]domain.JobDependency]{Value: dependencies, Version: jobDependenciesCacheVersion(dependencies)}, nil
	}
	for _, limit := range jobDependencyCachedPageLimits {
		if _, err := cache.List(context.Background(), jobDepsCacheKey{JobID: "job-known", Limit: limit}, loader); err != nil {
			t.Fatalf("prime limit %d: %v", limit, err)
		}
	}

	cache.InvalidateJob(context.Background(), "job-known")

	for _, limit := range jobDependencyCachedPageLimits {
		if _, err := cache.List(context.Background(), jobDepsCacheKey{JobID: "job-known", Limit: limit}, loader); err != nil {
			t.Fatalf("reload limit %d: %v", limit, err)
		}
	}
	if loads.Load() != int64(len(jobDependencyCachedPageLimits)*2) {
		t.Fatalf("loader calls = %d, want %d", loads.Load(), len(jobDependencyCachedPageLimits)*2)
	}
}

func TestJobDependencyCache_RefreshJobWritesEmptyTombstone(t *testing.T) {
	t.Parallel()

	deps, cleanup := newTestRedisCacheDeps(t, nil)
	defer cleanup()
	cache := newJobDependencyCache(time.Minute, deps)

	cache.RefreshJob(context.Background(), "job-empty", func(context.Context, jobDepsCacheKey) (straitcache.Versioned[[]domain.JobDependency], error) {
		return straitcache.Versioned[[]domain.JobDependency]{Value: nil, Version: 9}, nil
	})

	var staleLoads atomic.Int64
	got, err := cache.List(context.Background(), jobDepsCacheKey{JobID: "job-empty", Limit: defaultPageLimit + 1}, func(context.Context, jobDepsCacheKey) (straitcache.Versioned[[]domain.JobDependency], error) {
		staleLoads.Add(1)
		return straitcache.Versioned[[]domain.JobDependency]{
			Value:   []domain.JobDependency{{ID: "stale", CacheVersion: 8}},
			Version: 8,
		}, nil
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List() len = %d, want empty tombstone", len(got))
	}
	if staleLoads.Load() != 0 {
		t.Fatalf("stale loader calls = %d, want 0", staleLoads.Load())
	}
}

func TestJobDependenciesCacheVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		deps []domain.JobDependency
		want int64
	}{
		{name: "empty", deps: nil, want: 0},
		{
			name: "uses max row version",
			deps: []domain.JobDependency{
				{ID: "dep-1", CacheVersion: 7},
				{ID: "dep-2", CacheVersion: 3},
				{ID: "dep-3", CacheVersion: 11},
			},
			want: 11,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jobDependenciesCacheVersion(tt.deps); got != tt.want {
				t.Fatalf("jobDependenciesCacheVersion() = %d, want %d", got, tt.want)
			}
		})
	}
}
