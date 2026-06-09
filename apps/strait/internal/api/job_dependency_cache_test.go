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

func TestJobDependencyCache_CacheEnabled(t *testing.T) {
	t.Parallel()

	enabled := newJobDependencyCache(time.Minute)
	defer enabled.Stop()

	tests := []struct {
		name  string
		cache *jobDependencyCache
		want  bool
	}{
		{
			name:  "nil cache",
			cache: nil,
			want:  false,
		},
		{
			name:  "missing tier",
			cache: &jobDependencyCache{},
			want:  false,
		},
		{
			name:  "enabled cache",
			cache: enabled,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, tt.cache.cacheEnabled())
		})
	}
}

func TestJobDepsCacheKeyString(t *testing.T) {
	t.Parallel()

	got := jobDepsCacheKeyString(jobDepsCacheKey{JobID: "job-1", Limit: 101, Cursor: "cursor-1"})
	require.Equal(t, "job-1\x00101\x00cursor-1", got)
}

func TestParseJobDepsCacheKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want jobDepsCacheKey
		ok   bool
	}{
		{
			name: "valid empty cursor",
			raw:  "job-1\x00101\x00",
			want: jobDepsCacheKey{JobID: "job-1", Limit: 101},
			ok:   true,
		},
		{
			name: "valid cursor",
			raw:  "job-1\x00101\x00cursor-1",
			want: jobDepsCacheKey{JobID: "job-1", Limit: 101, Cursor: "cursor-1"},
			ok:   true,
		},
		{name: "missing separator", raw: "job-1", ok: false},
		{name: "too many separators", raw: "job-1\x00101\x00cursor\x00extra", ok: false},
		{name: "invalid limit", raw: "job-1\x00bad\x00cursor", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseJobDepsCacheKey(tt.raw)
			require.Equal(t, tt.ok, ok)
			if tt.ok {
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func BenchmarkJobDepsCacheKeyString(b *testing.B) {
	b.Run("empty_cursor", func(b *testing.B) {
		key := jobDepsCacheKey{JobID: "job-dependency-cache-key", Limit: 1001}

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			out := jobDepsCacheKeyString(key)
			if out == "" {
				b.Fatal("jobDepsCacheKeyString() returned empty key")
			}
		}
	})

	b.Run("with_cursor", func(b *testing.B) {
		key := jobDepsCacheKey{JobID: "job-dependency-cache-key", Limit: 1001, Cursor: "cursor-page-token"}

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			out := jobDepsCacheKeyString(key)
			if out == "" {
				b.Fatal("jobDepsCacheKeyString() returned empty key")
			}
		}
	})
}

func BenchmarkParseJobDepsCacheKey(b *testing.B) {
	benchmarks := []struct {
		name string
		raw  string
		ok   bool
	}{
		{name: "empty_cursor", raw: "job-dependency-cache-key\x001001\x00", ok: true},
		{name: "with_cursor", raw: "job-dependency-cache-key\x001001\x00cursor-page-token", ok: true},
		{name: "invalid_limit", raw: "job-dependency-cache-key\x00bad\x00cursor-page-token", ok: false},
		{name: "too_many_parts", raw: "job-dependency-cache-key\x001001\x00cursor-page-token\x00extra", ok: false},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				_, ok := parseJobDepsCacheKey(bm.raw)
				if ok != bm.ok {
					b.Fatalf("parseJobDepsCacheKey() ok = %v, want %v", ok, bm.ok)
				}
			}
		})
	}
}

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
	require.Len(t, got, 2)

	redisKey := "strait:cache:" + jobDependencyCacheNamespace + ":" + jobDepsCacheKeyString(key)
	raw, err := deps.Redis.Get(t.Context(), redisKey).Bytes()
	require.NoError(t, err)

	var envelope struct {
		Version int64 `json:"version"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.EqualValues(t, 12, envelope.Version)
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
	require.Equal(t, int64(len(jobDependencyCachedPageLimits)*2), loads.Load())
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
	require.Empty(t, got)
	require.EqualValues(t, 0, staleLoads.Load())
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
	require.Empty(t, got)
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
			require.Equal(t, tt.want, jobDependenciesCacheVersion(tt.deps))
		})
	}
}
