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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermissionCache_GetSet(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()

	// Miss on empty cache.
	_, ok := c.Get("proj", "user")
	require.False(t, ok)

	// Set and hit.
	c.Set("proj", "user", []string{"jobs:read", "jobs:write"})
	perms, ok := c.Get("proj", "user")
	require.True(
		t, ok)
	require.Len(t,
		perms, 2)

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
			require.Fail(t, "timed out waiting for cache entry to expire")
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
	require.False(t, ok)

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
	require.False(t, !ok || len(perms) !=
		1 || perms[0] != "jobs:read",
	)

	publishTestInvalidate(t, registryB, permissionCacheNamespace, cacheB.key("proj", "user"))
	if _, ok := cacheB.Get("proj", "user"); ok {
		require.Fail(t,

			"expected peer cache miss after cachebus invalidation")
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
	require.NoError(t, err)

	var envelope struct {
		Version int64 `json:"version"`
	}
	require.NoError(t, json.Unmarshal(raw,
		&envelope,
	))
	require.EqualValues(t, 14, envelope.
		Version,
	)

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
		require.Failf(t, "test failure",

			"Get() = %v, true; want barrier to reject stale update", got)
	}
	cache.SetWithVersion("proj", "user", []string{domain.ScopeJobsWrite}, 5)
	got, ok := cache.Get("proj", "user")
	require.False(t, !ok || !slices.
		Contains(got, domain.
			ScopeJobsWrite,
		))

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
	require.NoError(t, err)
	require.Len(t,
		members, 2)

	cache.InvalidateProject(t.Context(), "proj", time.Now().UnixNano())

	fresh := newPermissionCache(time.Minute, deps)
	if _, ok := fresh.Get("proj", "user-a"); ok {
		require.Fail(t,

			"user-a permission survived project invalidation")
	}
	if _, ok := fresh.Get("proj", "user-b"); ok {
		require.Fail(t,

			"user-b permission survived project invalidation")
	}
	require.EqualValues(t, 0, deps.Redis.
		Exists(t.Context(), indexKey).Val())

}

func TestPermissionCache_IsolatesProjects(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()
	c.Set("proj-a", "user", []string{"jobs:read"})
	c.Set("proj-b", "user", []string{"*"})

	permsA, ok := c.Get("proj-a", "user")
	require.True(
		t, ok)
	require.Len(t,
		permsA, 1)

	permsB, ok := c.Get("proj-b", "user")
	require.True(
		t, ok)
	require.False(t, len(permsB) !=
		1 ||
		permsB[0] !=
			"*")

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
			require.Fail(t, "timed out waiting for cache entry to expire")
		case <-ticker.C:
			if _, ok := c.Get("proj", "user"); !ok {
				goto expired
			}
		}
	}
expired:

	// A second Get should still miss (entry was evicted, not just stale).
	_, ok := c.Get("proj", "user")
	require.False(t, ok)

}

func TestPermissionCache_SetOverwritesExisting(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()
	c.Set("proj", "user", []string{"jobs:read"})
	c.Set("proj", "user", []string{"*", "runs:write"})

	perms, ok := c.Get("proj", "user")
	require.True(
		t, ok)
	require.False(t, len(perms) !=
		2 || perms[0] !=
		"*")

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
	require.True(
		t, ok)
	require.NotNil(t, perms)
	require.Len(t,
		perms, 0)

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
		require.True(
			t, ok)

	}
}

func TestPermissionCache_ZeroTTL(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(0)
	defer c.Stop()
	c.Set("proj", "user", []string{"*"})

	_, ok := c.Get("proj", "user")
	require.False(t, ok)

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

	for _, ok := range results {
		assert.True(t,
			ok)

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
			require.Fail(t, "timed out waiting for cache entry to expire")
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
			assert.False(
				t, ok)

		})
	}
	wg.Wait()

	_, ok := c.Get("proj", "user")
	require.False(t, ok)

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
		require.Fail(t, "timed out waiting for first read")
	}

	for i := range 100 {
		c.Set("proj", "user", []string{fmt.Sprintf("perm-%d", i)})
	}

	cancel()
	wg.Wait()
	require.NotEqual(t, 0, readCount.
		Load())

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
	require.True(
		t, ok)
	require.False(t, len(perms) !=
		1 || perms[0] !=
		"refreshed")

}

func TestPermissionCache_KeySeparatorCollision(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	defer c.Stop()

	// These should NOT collide because we use \x00 as separator.
	c.Set("a", "b", []string{"perm-ab"})
	c.Set("a\x00b", "", []string{"perm-collision"})

	permsAB, ok := c.Get("a", "b")
	require.True(
		t, ok)
	require.False(t, len(permsAB) !=
		1 ||
		permsAB[0] != "perm-ab")

	permsCollision, ok := c.Get("a\x00b", "")
	require.True(
		t, ok)
	require.False(t, len(permsCollision) !=
		1 || permsCollision[0] !=
		"perm-collision",
	)

}
