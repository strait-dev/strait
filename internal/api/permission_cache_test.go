package api

import (
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
