package api

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestRedisCacheDeps(t *testing.T, registry *straitcache.Registry) (apiCacheDeps, func()) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return apiCacheDeps{Redis: rdb, Registry: registry}, func() {
		_ = rdb.Close()
		mr.Close()
	}
}

func publishTestInvalidate(t *testing.T, registry *straitcache.Registry, namespace, key string) {
	t.Helper()
	data, err := json.Marshal(straitcache.BusMessage{
		Action:    straitcache.BusActionInvalidate,
		Namespace: namespace,
		Key:       key,
		Version:   time.Now().UnixNano(),
		Origin:    "peer",
		SentAt:    time.Now().UTC(),
	})
	require.NoError(t, err)

	registry.Handle(t.Context(), data)
}

func TestAPIKeyCache_CacheEnabled(t *testing.T) {
	t.Parallel()

	enabled := newAPIKeyCache(time.Minute)
	defer enabled.Stop()

	tests := []struct {
		name  string
		cache *apiKeyCache
		want  bool
	}{
		{
			name:  "nil cache",
			cache: nil,
			want:  false,
		},
		{
			name:  "missing tier",
			cache: &apiKeyCache{},
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

func TestAPIKeyCache_ServesValidKeyAndSanitizesSecrets(t *testing.T) {
	t.Parallel()

	cache := newAPIKeyCache(time.Minute)
	var loads atomic.Int64
	key := &domain.APIKey{
		ID:                    "key-1",
		ProjectID:             "proj-1",
		KeyHash:               "hash-1",
		KeyPrefix:             "strait_abcde",
		Scopes:                []string{domain.ScopeRunsRead},
		RotationWebhookSecret: []byte("secret-ciphertext"),
	}
	loader := func(_ context.Context, _ string) (*domain.APIKey, error) {
		loads.Add(1)
		return key, nil
	}

	first, err := cache.Get(t.Context(), "hash-1", loader)
	require.NoError(t, err)

	first.Scopes[0] = domain.ScopeRunsWrite

	second, err := cache.Get(t.Context(), "hash-1", loader)
	require.NoError(t, err)
	require.EqualValues(t, 1, loads.Load())
	require.Equal(t, domain.ScopeRunsRead,
		second.
			Scopes[0])
	require.Empty(t,
		second.RotationWebhookSecret)
}

func TestAPIKeyCache_NegativeCachesInvalidKey(t *testing.T) {
	t.Parallel()

	cache := newAPIKeyCache(time.Minute)
	var loads atomic.Int64
	loader := func(_ context.Context, _ string) (*domain.APIKey, error) {
		loads.Add(1)
		return nil, store.ErrAPIKeyNotFound
	}

	for range 2 {
		key, err := cache.Get(t.Context(), "missing-hash", loader)
		require.NoError(t, err)
		require.Nil(t, key)
	}
	require.EqualValues(t, 1, loads.Load())
}

func TestAPIKeyCache_InvalidateForcesReload(t *testing.T) {
	t.Parallel()

	cache := newAPIKeyCache(time.Minute)
	var loads atomic.Int64
	loader := func(_ context.Context, _ string) (*domain.APIKey, error) {
		loads.Add(1)
		return &domain.APIKey{ID: "key", KeyHash: "hash-1"}, nil
	}

	if _, err := cache.Get(t.Context(), "hash-1", loader); err != nil {
		require.Failf(t, "test failure",

			"Get() first error = %v", err)
	}
	cache.InvalidateWithVersion(t.Context(), "hash-1", 2)
	loader = func(_ context.Context, _ string) (*domain.APIKey, error) {
		loads.Add(1)
		return &domain.APIKey{ID: "key", KeyHash: "hash-1", CacheVersion: 3}, nil
	}
	if _, err := cache.Get(t.Context(), "hash-1", loader); err != nil {
		require.Failf(t, "test failure",

			"Get() second error = %v", err)
	}
	require.EqualValues(t, 2, loads.Load())
}

func TestStrongAPIKeyCache_BarrierAllowsNegativeDBConfirmation(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	defer cleanup()
	cache := newAPIKeyCache(time.Minute, deps)

	cache.Set(t.Context(), &domain.APIKey{ID: "key-1", ProjectID: "proj-1", KeyHash: "hash-1", CacheVersion: 4})
	cache.InvalidateWithVersion(t.Context(), "hash-1", 5)

	var loads atomic.Int64
	got, err := cache.Get(t.Context(), "hash-1", func(context.Context, string) (*domain.APIKey, error) {
		loads.Add(1)
		return nil, store.ErrAPIKeyNotFound
	})
	require.NoError(t, err)
	require.Nil(t, got)
	require.EqualValues(t, 1, loads.Load())
}

func TestAPIKeyCache_RedisL2BackfillAndCachebusInvalidate(t *testing.T) {
	t.Parallel()

	registryA := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	depsA, cleanupA := newTestRedisCacheDeps(t, registryA)
	defer cleanupA()
	cacheA := newAPIKeyCache(time.Minute, depsA)
	cacheA.Set(t.Context(), &domain.APIKey{
		ID:        "key-1",
		ProjectID: "proj-1",
		KeyHash:   "hash-1",
		Scopes:    []string{domain.ScopeRunsRead},
	})

	registryB := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-b"})
	depsB := depsA
	depsB.Registry = registryB
	cacheB := newAPIKeyCache(time.Minute, depsB)
	var loads atomic.Int64
	got, err := cacheB.Get(t.Context(), "hash-1", func(context.Context, string) (*domain.APIKey, error) {
		loads.Add(1)
		return nil, store.ErrAPIKeyNotFound
	})
	require.NoError(t, err)
	require.False(t, got == nil ||
		got.ID != "key-1",
	)
	require.EqualValues(t, 0, loads.Load())

	publishTestInvalidate(t, registryB, apiKeyAuthCacheNamespace, "hash-1")
	got, err = cacheB.Get(t.Context(), "hash-1", func(context.Context, string) (*domain.APIKey, error) {
		loads.Add(1)
		return nil, store.ErrAPIKeyNotFound
	})
	require.NoError(t, err)
	require.Nil(t, got)
	require.EqualValues(t, 1, loads.Load())
}

func TestAPIKeyCache_PreservesStoreCacheVersionInRedis(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	defer cleanup()
	cache := newAPIKeyCache(time.Minute, deps)

	got, err := cache.Get(t.Context(), "hash-versioned", func(context.Context, string) (*domain.APIKey, error) {
		return &domain.APIKey{
			ID:           "key-versioned",
			ProjectID:    "proj-versioned",
			KeyHash:      "hash-versioned",
			Scopes:       []string{domain.ScopeRunsRead},
			CacheVersion: 7,
		}, nil
	})
	require.NoError(t, err)
	require.False(t, got == nil ||
		got.CacheVersion !=
			7)

	raw, err := deps.Redis.Get(t.Context(), "strait:cache:"+apiKeyAuthCacheNamespace+":hash-versioned").Bytes()
	require.NoError(t, err)

	var envelope struct {
		Version int64 `json:"version"`
		Value   struct {
			RotationWebhookSecret []byte `json:"-"`
		} `json:"value"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.EqualValues(t, 7, envelope.Version)
}

func TestAPIKeyCache_StrongModeFallsBackToDBWhenRedisEntryMissing(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	defer cleanup()
	cache := newAPIKeyCache(time.Minute, deps)
	cache.Set(t.Context(), &domain.APIKey{ID: "key-1", ProjectID: "proj-1", KeyHash: "hash-1"})

	if _, err := cache.Get(t.Context(), "hash-1", func(context.Context, string) (*domain.APIKey, error) {
		require.Fail(t,

			"loader should not run while Redis L2 is warm")
		return nil, nil
	}); err != nil {
		require.Failf(t, "test failure",

			"Get() warm error = %v", err)
	}
	require.NoError(t, deps.Redis.
		Del(t.Context(),
			"strait:cache:"+
				apiKeyAuthCacheNamespace+
				":hash-1").Err())

	var loads atomic.Int64
	got, err := cache.Get(t.Context(), "hash-1", func(context.Context, string) (*domain.APIKey, error) {
		loads.Add(1)
		return nil, store.ErrAPIKeyNotFound
	})
	require.NoError(t, err)
	require.Nil(t, got)
	require.EqualValues(t, 1, loads.Load())
}
