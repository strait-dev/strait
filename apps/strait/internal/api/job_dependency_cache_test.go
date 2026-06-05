package api

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestJobDependencyCache_PreservesMaxDependencyCacheVersionInRedis(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	defer cleanup()
	cache := newJobDependencyCache(time.Minute, deps)

	key := jobDepsCacheKey{JobID: "job-versioned", Limit: 1000}
	var loads atomic.Int64
	loader := func(
		context.Context,
		jobDepsCacheKey,
	) (straitcache.Versioned[[]domain.JobDependency], error) {
		loads.Add(1)
		deps := []domain.JobDependency{
			{ID: "dep-low", JobID: key.JobID, DependsOnJobID: "job-a", Condition: "completed", CacheVersion: 4},
			{ID: "dep-high", JobID: key.JobID, DependsOnJobID: "job-b", Condition: "failed", CacheVersion: 12},
		}
		return straitcache.Versioned[[]domain.JobDependency]{Value: deps, Version: jobDependenciesCacheVersion(deps)}, nil
	}
	got, err := cache.List(t.Context(), key, loader)
	require.NoError(t, err)
	require.Len(t,
		got, 2)

	redisKey := "strait:cache:" + jobDependencyCacheNamespace + ":" + jobDepsCacheKeyString(key)
	raw, err := deps.Redis.Get(t.Context(), redisKey).Bytes()
	require.NoError(t, err)

	var envelope struct {
		Version int64 `json:"version"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.EqualValues(t, 12, envelope.
		Version)
	require.EqualValues(t, 1, loads.Load())
}

func TestJobDependencyCache_InvalidateJobClearsKnownPageShapes(t *testing.T) {
	t.Parallel()

	deps, cleanup := newTestRedisCacheDeps(t, nil)
	defer cleanup()
	cache := newJobDependencyCache(time.Minute, deps)

	var loads atomic.Int64
	var versionBase atomic.Int64
	loader := func(
		_ context.Context,
		key jobDepsCacheKey,
	) (straitcache.Versioned[[]domain.JobDependency], error) {
		loads.Add(1)
		dependencies := []domain.JobDependency{{
			ID:             "dep",
			JobID:          key.JobID,
			DependsOnJobID: "job-parent",
			Condition:      "completed",
			CacheVersion:   versionBase.Load() + int64(key.Limit),
		}}
		return straitcache.Versioned[[]domain.JobDependency]{
			Value:   dependencies,
			Version: jobDependenciesCacheVersion(dependencies),
		}, nil
	}
	for _, limit := range jobDependencyCachedPageLimits {
		if _, err := cache.List(t.Context(), jobDepsCacheKey{JobID: "job-known", Limit: limit}, loader); err != nil {
			require.Failf(t, "test failure",

				"prime limit %d: %v", limit, err)
		}
	}

	cache.InvalidateJobWithVersion(t.Context(), "job-known", 2000)
	versionBase.Store(3000)

	for _, limit := range jobDependencyCachedPageLimits {
		if _, err := cache.List(t.Context(), jobDepsCacheKey{JobID: "job-known", Limit: limit}, loader); err != nil {
			require.Failf(t, "test failure",

				"reload limit %d: %v", limit, err)
		}
	}
	require.Equal(t, int64(len(jobDependencyCachedPageLimits)*2),
		loads.
			Load())
}

func TestJobDependencyCache_RefreshJobWritesEmptyTombstone(t *testing.T) {
	t.Parallel()

	deps, cleanup := newTestRedisCacheDeps(t, nil)
	defer cleanup()
	cache := newJobDependencyCache(time.Minute, deps)

	cache.RefreshJob(t.Context(), "job-empty", func(
		context.Context,
		jobDepsCacheKey,
	) (straitcache.Versioned[[]domain.JobDependency], error) {
		return straitcache.Versioned[[]domain.JobDependency]{Value: nil, Version: 9}, nil
	})

	var staleLoads atomic.Int64
	key := jobDepsCacheKey{JobID: "job-empty", Limit: defaultPageLimit + 1}
	loader := func(
		context.Context,
		jobDepsCacheKey,
	) (straitcache.Versioned[[]domain.JobDependency], error) {
		staleLoads.Add(1)
		return straitcache.Versioned[[]domain.JobDependency]{
			Value:   []domain.JobDependency{{ID: "stale", CacheVersion: 8}},
			Version: 8,
		}, nil
	}
	got, err := cache.List(t.Context(), key, loader)
	require.NoError(t, err)
	require.Empty(t,
		got)
	require.EqualValues(t, 0, staleLoads.
		Load())
}

func TestJobDependencyCache_StrongBarrierRejectsStaleListFill(t *testing.T) {
	t.Parallel()

	deps, cleanup := newTestRedisCacheDeps(t, nil)
	defer cleanup()
	cache := newJobDependencyCache(time.Minute, deps)

	key := jobDepsCacheKey{JobID: "job-stale", Limit: defaultPageLimit + 1}
	cache.InvalidateJobWithVersion(t.Context(), key.JobID, 10)

	loader := func(
		context.Context,
		jobDepsCacheKey,
	) (straitcache.Versioned[[]domain.JobDependency], error) {
		return straitcache.Versioned[[]domain.JobDependency]{
			Value:   []domain.JobDependency{{ID: "stale", JobID: key.JobID, CacheVersion: 9}},
			Version: 9,
		}, nil
	}
	_, err := cache.List(t.Context(), key, loader)
	require.Error(t, err)
}

func TestJobDependencyCache_StrongBarrierAllowsEqualVersionEmptyList(t *testing.T) {
	t.Parallel()

	deps, cleanup := newTestRedisCacheDeps(t, nil)
	defer cleanup()
	cache := newJobDependencyCache(time.Minute, deps)

	key := jobDepsCacheKey{JobID: "job-empty-equal", Limit: defaultPageLimit + 1}
	cache.InvalidateJobWithVersion(t.Context(), key.JobID, 12)
	loader := func(
		context.Context,
		jobDepsCacheKey,
	) (straitcache.Versioned[[]domain.JobDependency], error) {
		return straitcache.Versioned[[]domain.JobDependency]{Value: nil, Version: 12}, nil
	}
	got, err := cache.List(t.Context(), key, loader)
	require.NoError(t, err)
	require.Empty(t,
		got)
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
			require.Equal(t, tt.want, jobDependenciesCacheVersion(tt.
				deps))
		})
	}
}
