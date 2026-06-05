package api

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
)

func TestPermissionCache_GetSet(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()

	// Miss on empty cache.
	_, ok := c.Get("proj", "user")
	if ok {
		t.Fatal("expected cache miss")
	}

	// Set and hit.
	c.Set("proj", "user", []string{"jobs:read", "jobs:write"})
	perms, ok := c.Get("proj", "user")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(perms) != 2 {
		t.Fatalf("len(perms) = %d, want 2", len(perms))
	}
}

func TestPermissionCache_Expiry(t *testing.T) {
	t.Parallel()

	// Otter uses a timer wheel with ~1s granularity for expiration.
	c := newPermissionCache(1 * time.Second)
	defer c.Stop()
	c.Set("proj", "user", []string{"*"})

	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for cache entry to expire")
		case <-ticker.C:
			if _, ok := c.Get("proj", "user"); !ok {
				return // entry expired as expected
			}
		}
	}
}

func TestPermissionCache_Invalidate(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()
	c.Set("proj", "user", []string{"*"})

	c.Invalidate("proj", "user")

	_, ok := c.Get("proj", "user")
	if ok {
		t.Fatal("expected cache miss after invalidate")
	}
}

func TestPermissionCache_RedisL2BackfillAndCachebusInvalidate(t *testing.T) {
	t.Parallel()

	registryA := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	depsA, cleanupA := newTestRedisCacheDeps(t, registryA)
	defer cleanupA()
	cacheA := newPermissionCache(time.Minute, depsA)
	cacheA.Set("proj", "user", []string{"jobs:read"})

	registryB := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-b"})
	depsB := depsA
	depsB.Registry = registryB
	cacheB := newPermissionCache(time.Minute, depsB)
	perms, ok := cacheB.Get("proj", "user")
	if !ok || len(perms) != 1 || perms[0] != "jobs:read" {
		t.Fatalf("permissions from L2 = %v, %v; want [jobs:read],true", perms, ok)
	}

	publishTestInvalidate(t, registryB, permissionCacheNamespace, cacheB.key("proj", "user"))
	if _, ok := cacheB.Get("proj", "user"); ok {
		t.Fatal("expected peer cache miss after cachebus invalidation")
	}
}

func TestPermissionCache_SetWithVersionPreservesVersionInRedis(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	defer cleanup()
	cache := newPermissionCache(time.Minute, deps)

	cache.SetWithVersion("proj", "user", []string{"jobs:read"}, 14)

	raw, err := deps.Redis.Get(t.Context(), "strait:cache:"+permissionCacheNamespace+":"+cache.key("proj", "user")).Bytes()
	if err != nil {
		t.Fatalf("read redis entry: %v", err)
	}
	var envelope struct {
		Version int64 `json:"version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode redis entry: %v", err)
	}
	if envelope.Version != 14 {
		t.Fatalf("redis version = %d, want 14", envelope.Version)
	}
}

func TestStrongPermissionCache_BarrierRejectsStaleUpdate(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	defer cleanup()
	cache := newPermissionCache(time.Minute, deps)

	cache.SetWithVersion("proj", "user", []string{domain.ScopeJobsRead}, 4)
	cache.InvalidateWithVersion("proj", "user", 5)
	cache.SetWithVersion("proj", "user", []string{domain.ScopeJobsWrite}, 4)

	if got, ok := cache.Get("proj", "user"); ok {
		t.Fatalf("Get() = %v, true; want barrier to reject stale update", got)
	}
	cache.SetWithVersion("proj", "user", []string{domain.ScopeJobsWrite}, 5)
	got, ok := cache.Get("proj", "user")
	if !ok || !slices.Contains(got, domain.ScopeJobsWrite) {
		t.Fatalf("Get() = %v, %v; want equal-version replacement", got, ok)
	}
}

func TestPermissionCache_ProjectInvalidationClearsRedisL2(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "node-a"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	defer cleanup()
	cache := newPermissionCache(time.Minute, deps)
	cache.Set("proj", "user-a", []string{"jobs:read"})
	cache.Set("proj", "user-b", []string{"jobs:write"})
	indexKey := cache.redisProjectIndexKey("proj")

	members, err := deps.Redis.SMembers(t.Context(), indexKey).Result()
	if err != nil {
		t.Fatalf("read permission project index: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("permission project index size = %d, want 2", len(members))
	}

	cache.InvalidateProject(t.Context(), "proj", time.Now().UnixNano())

	fresh := newPermissionCache(time.Minute, deps)
	if _, ok := fresh.Get("proj", "user-a"); ok {
		t.Fatal("user-a permission survived project invalidation")
	}
	if _, ok := fresh.Get("proj", "user-b"); ok {
		t.Fatal("user-b permission survived project invalidation")
	}
	if exists := deps.Redis.Exists(t.Context(), indexKey).Val(); exists != 0 {
		t.Fatalf("permission project index exists = %d, want 0", exists)
	}
}

func TestPermissionCache_IsolatesProjects(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()
	c.Set("proj-a", "user", []string{"jobs:read"})
	c.Set("proj-b", "user", []string{"*"})

	permsA, ok := c.Get("proj-a", "user")
	if !ok {
		t.Fatal("expected hit for proj-a")
	}
	if len(permsA) != 1 {
		t.Fatalf("proj-a perms = %d, want 1", len(permsA))
	}

	permsB, ok := c.Get("proj-b", "user")
	if !ok {
		t.Fatal("expected hit for proj-b")
	}
	if len(permsB) != 1 || permsB[0] != "*" {
		t.Fatalf("proj-b perms = %v, want [*]", permsB)
	}
}

func TestPermissionCache_EvictsOnExpiredRead(t *testing.T) {
	t.Parallel()

	// Otter uses a timer wheel with ~1s granularity for expiration.
	c := newPermissionCache(1 * time.Second)
	defer c.Stop()
	c.Set("proj", "user", []string{"*"})

	// Poll until the entry expires.
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for cache entry to expire")
		case <-ticker.C:
			if _, ok := c.Get("proj", "user"); !ok {
				goto expired
			}
		}
	}
expired:

	// A second Get should still miss (entry was evicted, not just stale).
	_, ok := c.Get("proj", "user")
	if ok {
		t.Fatal("expected cache miss on second read after expiry")
	}
}

func TestPermissionCache_SetOverwritesExisting(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()
	c.Set("proj", "user", []string{"jobs:read"})
	c.Set("proj", "user", []string{"*", "runs:write"})

	perms, ok := c.Get("proj", "user")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(perms) != 2 || perms[0] != "*" {
		t.Fatalf("perms = %v, want [* runs:write]", perms)
	}
}

func TestPermissionCache_InvalidateNonexistent(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()
	// Should not panic.
	c.Invalidate("proj", "nonexistent")
	c.Invalidate("", "")
}

func TestPermissionCache_EmptyPermissionsSlice(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()

	// Set empty (non-nil) permissions — should be distinguishable from cache miss.
	c.Set("proj", "user", []string{})
	perms, ok := c.Get("proj", "user")
	if !ok {
		t.Fatal("expected cache hit for empty permissions slice")
	}
	if perms == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(perms) != 0 {
		t.Fatalf("expected empty slice, got %v", perms)
	}
}

func TestPermissionCache_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(50 * time.Millisecond)
	defer c.Stop()

	var wg conc.WaitGroup
	const goroutines = 50

	for range goroutines {
		wg.Go(func() {
			for range 100 {
				c.Get("proj", "user")
				c.Get("proj", "other")
			}
		})
	}

	for range goroutines {
		wg.Go(func() {
			for range 100 {
				c.Set("proj", "user", []string{"jobs:read"})
				c.Set("proj", "other", []string{"*"})
			}
		})
	}

	for range 10 {
		wg.Go(func() {
			for range 100 {
				c.Invalidate("proj", "user")
			}
		})
	}

	wg.Wait()
}

func TestPermissionCache_ManyEntries(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()
	const n = 10000

	for i := range n {
		c.Set("proj", "user-"+string(rune(i)), []string{"*"})
	}

	for _, i := range []int{0, 100, 5000, 9999} {
		_, ok := c.Get("proj", "user-"+string(rune(i)))
		if !ok {
			t.Fatalf("expected cache hit for user-%d", i)
		}
	}
}

func TestPermissionCache_ZeroTTL(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(0)
	defer c.Stop()
	c.Set("proj", "user", []string{"*"})

	_, ok := c.Get("proj", "user")
	if ok {
		t.Fatal("expected cache miss with zero TTL")
	}
}

func TestPermissionCache_RLockAllowsConcurrentReads(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()

	c.Set("proj", "user", []string{"read"})

	var wg conc.WaitGroup
	const readers = 100
	results := make([]bool, readers)
	for i := range readers {
		wg.Go(func() {
			_, ok := c.Get("proj", "user")
			results[i] = ok
		})
	}
	wg.Wait()

	for i, ok := range results {
		if !ok {
			t.Errorf("reader %d got cache miss, expected hit", i)
		}
	}
}

func TestPermissionCache_EvictRaceOnExpiry(t *testing.T) {
	t.Parallel()

	// Otter uses a timer wheel with ~1s granularity for expiration.
	c := newPermissionCache(1 * time.Second)
	defer c.Stop()

	c.Set("proj", "user", []string{"*"})

	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for cache entry to expire")
		case <-ticker.C:
			if _, ok := c.Get("proj", "user"); !ok {
				goto expired
			}
		}
	}
expired:

	var wg conc.WaitGroup
	const goroutines = 50
	for range goroutines {
		wg.Go(func() {
			_, ok := c.Get("proj", "user")
			if ok {
				t.Error("expected miss for expired entry")
			}
		})
	}
	wg.Wait()

	_, ok := c.Get("proj", "user")
	if ok {
		t.Fatal("expired entry should have been evicted")
	}
}

func TestPermissionCache_GetDoesNotBlockSet(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()

	c.Set("proj", "user", []string{"old"})

	firstRead := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	var readCount atomic.Int64
	var wg conc.WaitGroup
	wg.Go(func() {
		for ctx.Err() == nil {
			c.Get("proj", "user")
			if readCount.Add(1) == 1 {
				close(firstRead)
			}
		}
	})

	select {
	case <-firstRead:
	case <-ctx.Done():
		t.Fatal("timed out waiting for first read")
	}

	for i := range 100 {
		c.Set("proj", "user", []string{fmt.Sprintf("perm-%d", i)})
	}

	cancel()
	wg.Wait()

	if readCount.Load() == 0 {
		t.Fatal("no reads completed, possible deadlock")
	}
}

func TestPermissionCache_RefreshedBetweenRLockAndLock(t *testing.T) {
	c := newPermissionCache(50 * time.Millisecond)
	defer c.Stop()

	c.Set("proj", "user", []string{"original"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := c.Get("proj", "user"); !ok {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// Simulate: one goroutine refreshes between another's RLock and Lock.
	// We can't perfectly control timing, but we can verify correctness
	// by refreshing and then checking.
	c.Set("proj", "user", []string{"refreshed"})

	perms, ok := c.Get("proj", "user")
	if !ok {
		t.Fatal("expected hit after refresh")
	}
	if len(perms) != 1 || perms[0] != "refreshed" {
		t.Fatalf("perms = %v, want [refreshed]", perms)
	}
}

func TestPermissionCache_KeySeparatorCollision(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()

	// These should NOT collide because we use \x00 as separator.
	c.Set("a", "b", []string{"perm-ab"})
	c.Set("a\x00b", "", []string{"perm-collision"})

	permsAB, ok := c.Get("a", "b")
	if !ok {
		t.Fatal("expected hit for a/b")
	}
	if len(permsAB) != 1 || permsAB[0] != "perm-ab" {
		t.Fatalf("a/b perms = %v, want [perm-ab]", permsAB)
	}

	permsCollision, ok := c.Get("a\x00b", "")
	if !ok {
		t.Fatal("expected hit for a\\x00b/empty")
	}
	if len(permsCollision) != 1 || permsCollision[0] != "perm-collision" {
		t.Fatalf("collision perms = %v, want [perm-collision]", permsCollision)
	}
}
