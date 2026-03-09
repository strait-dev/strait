package api

import (
	"sync"
	"testing"
	"time"
)

func TestPermissionCache_GetSet(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)

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

	c := newPermissionCache(1 * time.Millisecond)
	c.Set("proj", "user", []string{"*"})

	time.Sleep(5 * time.Millisecond)

	_, ok := c.Get("proj", "user")
	if ok {
		t.Fatal("expected cache miss after expiry")
	}
}

func TestPermissionCache_Invalidate(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	c.Set("proj", "user", []string{"*"})

	c.Invalidate("proj", "user")

	_, ok := c.Get("proj", "user")
	if ok {
		t.Fatal("expected cache miss after invalidate")
	}
}

func TestPermissionCache_IsolatesProjects(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
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

	c := newPermissionCache(1 * time.Millisecond)
	c.Set("proj", "user", []string{"*"})

	time.Sleep(5 * time.Millisecond)

	// Get should evict the expired entry.
	_, ok := c.Get("proj", "user")
	if ok {
		t.Fatal("expected cache miss")
	}

	// Verify internal map is cleaned.
	c.mu.RLock()
	_, exists := c.entries[c.key("proj", "user")]
	c.mu.RUnlock()
	if exists {
		t.Fatal("expired entry should have been evicted from internal map")
	}
}

func TestPermissionCache_SetOverwritesExisting(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
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
	// Should not panic.
	c.Invalidate("proj", "nonexistent")
	c.Invalidate("", "")
}

func TestPermissionCache_EmptyPermissionsSlice(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)

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

	var wg sync.WaitGroup
	const goroutines = 50

	// Readers.
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			for range 100 {
				c.Get("proj", "user")
				c.Get("proj", "other")
			}
			_ = i
		}(i)
	}

	// Writers.
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			for range 100 {
				c.Set("proj", "user", []string{"jobs:read"})
				c.Set("proj", "other", []string{"*"})
			}
			_ = i
		}(i)
	}

	// Invalidators.
	wg.Add(10)
	for range 10 {
		go func() {
			defer wg.Done()
			for range 100 {
				c.Invalidate("proj", "user")
			}
		}()
	}

	wg.Wait()
	// If we get here without panics or races, the test passes.
}

func TestPermissionCache_ManyEntries(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)
	const n = 10000

	for i := range n {
		c.Set("proj", "user-"+string(rune(i)), []string{"*"})
	}

	// Verify a sample are retrievable.
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
	c.Set("proj", "user", []string{"*"})

	// With zero TTL, everything should expire immediately.
	// time.Since(cachedAt) > 0 should be true.
	_, ok := c.Get("proj", "user")
	if ok {
		t.Fatal("expected cache miss with zero TTL")
	}
}

func TestPermissionCache_KeySeparatorCollision(t *testing.T) {
	t.Parallel()

	c := newPermissionCache(5 * time.Second)

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
