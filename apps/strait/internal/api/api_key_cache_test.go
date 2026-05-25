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
	if err != nil {
		t.Fatalf("marshal invalidate: %v", err)
	}
	registry.Handle(t.Context(), data)
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

	first, err := cache.Get(context.Background(), "hash-1", loader)
	if err != nil {
		t.Fatalf("Get() first error = %v", err)
	}
	first.Scopes[0] = domain.ScopeRunsWrite

	second, err := cache.Get(context.Background(), "hash-1", loader)
	if err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loads.Load())
	}
	if second.Scopes[0] != domain.ScopeRunsRead {
		t.Fatalf("cached scopes were mutated: %+v", second.Scopes)
	}
	if len(second.RotationWebhookSecret) != 0 {
		t.Fatalf("cached key includes rotation webhook secret: %q", second.RotationWebhookSecret)
	}
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
		key, err := cache.Get(context.Background(), "missing-hash", loader)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if key != nil {
			t.Fatalf("Get() key = %+v, want nil", key)
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loads.Load())
	}
}

func TestAPIKeyCache_InvalidateForcesReload(t *testing.T) {
	t.Parallel()

	cache := newAPIKeyCache(time.Minute)
	var loads atomic.Int64
	loader := func(_ context.Context, _ string) (*domain.APIKey, error) {
		loads.Add(1)
		return &domain.APIKey{ID: "key", KeyHash: "hash-1"}, nil
	}

	if _, err := cache.Get(context.Background(), "hash-1", loader); err != nil {
		t.Fatalf("Get() first error = %v", err)
	}
	cache.Invalidate(context.Background(), "hash-1")
	if _, err := cache.Get(context.Background(), "hash-1", loader); err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if loads.Load() != 2 {
		t.Fatalf("loader calls = %d, want 2", loads.Load())
	}
}

func TestAPIKeyCache_RedisL2BackfillAndCachebusInvalidate(t *testing.T) {
	t.Parallel()

	registryA := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	depsA, cleanupA := newTestRedisCacheDeps(t, registryA)
	defer cleanupA()
	cacheA := newAPIKeyCache(time.Minute, depsA)
	cacheA.Set(context.Background(), &domain.APIKey{
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
	got, err := cacheB.Get(context.Background(), "hash-1", func(context.Context, string) (*domain.APIKey, error) {
		loads.Add(1)
		return nil, store.ErrAPIKeyNotFound
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.ID != "key-1" {
		t.Fatalf("Get() = %+v, want key-1 from Redis L2", got)
	}
	if loads.Load() != 0 {
		t.Fatalf("loader calls = %d, want 0 on L2 hit", loads.Load())
	}

	publishTestInvalidate(t, registryB, apiKeyAuthCacheNamespace, "hash-1")
	got, err = cacheB.Get(context.Background(), "hash-1", func(context.Context, string) (*domain.APIKey, error) {
		loads.Add(1)
		return nil, store.ErrAPIKeyNotFound
	})
	if err != nil {
		t.Fatalf("Get() after invalidate error = %v", err)
	}
	if got != nil {
		t.Fatalf("Get() after invalidate = %+v, want nil from DB loader", got)
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls after invalidate = %d, want 1", loads.Load())
	}
}

func TestAPIKeyCache_PreservesStoreCacheVersionInRedis(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	defer cleanup()
	cache := newAPIKeyCache(time.Minute, deps)

	got, err := cache.Get(context.Background(), "hash-versioned", func(context.Context, string) (*domain.APIKey, error) {
		return &domain.APIKey{
			ID:           "key-versioned",
			ProjectID:    "proj-versioned",
			KeyHash:      "hash-versioned",
			Scopes:       []string{domain.ScopeRunsRead},
			CacheVersion: 7,
		}, nil
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.CacheVersion != 7 {
		t.Fatalf("Get() CacheVersion = %v, want 7", got)
	}

	raw, err := deps.Redis.Get(context.Background(), "strait:cache:"+apiKeyAuthCacheNamespace+":hash-versioned").Bytes()
	if err != nil {
		t.Fatalf("read redis entry: %v", err)
	}
	var envelope struct {
		Version int64 `json:"version"`
		Value   struct {
			RotationWebhookSecret []byte `json:"-"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode redis entry: %v", err)
	}
	if envelope.Version != 7 {
		t.Fatalf("redis version = %d, want 7", envelope.Version)
	}
}

func TestAPIKeyCache_StrongModeFallsBackToDBWhenRedisEntryMissing(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	defer cleanup()
	cache := newAPIKeyCache(time.Minute, deps)
	cache.Set(context.Background(), &domain.APIKey{ID: "key-1", ProjectID: "proj-1", KeyHash: "hash-1"})

	if _, err := cache.Get(context.Background(), "hash-1", func(context.Context, string) (*domain.APIKey, error) {
		t.Fatal("loader should not run while Redis L2 is warm")
		return nil, nil
	}); err != nil {
		t.Fatalf("Get() warm error = %v", err)
	}

	if err := deps.Redis.Del(context.Background(), "strait:cache:"+apiKeyAuthCacheNamespace+":hash-1").Err(); err != nil {
		t.Fatalf("delete redis cache entry: %v", err)
	}
	var loads atomic.Int64
	got, err := cache.Get(context.Background(), "hash-1", func(context.Context, string) (*domain.APIKey, error) {
		loads.Add(1)
		return nil, store.ErrAPIKeyNotFound
	})
	if err != nil {
		t.Fatalf("Get() after Redis delete error = %v", err)
	}
	if got != nil {
		t.Fatalf("Get() after Redis delete = %+v, want DB-confirmed nil", got)
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loads.Load())
	}
}
