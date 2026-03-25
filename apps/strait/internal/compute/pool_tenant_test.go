package compute

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestPoolKey_IncludesProjectID verifies that different projects produce different keys.
func TestPoolKey_IncludesProjectID(t *testing.T) {
	t.Parallel()

	keyA := PoolKey("project-A", "myapp:v1", "iad")
	keyB := PoolKey("project-B", "myapp:v1", "iad")

	if keyA == keyB {
		t.Fatalf("expected different keys for different projects, both got %q", keyA)
	}
}

// TestPoolKey_SameProjectSameKey verifies that identical inputs produce the same key.
func TestPoolKey_SameProjectSameKey(t *testing.T) {
	t.Parallel()

	k1 := PoolKey("proj-1", "img:latest", "iad")
	k2 := PoolKey("proj-1", "img:latest", "iad")

	if k1 != k2 {
		t.Fatalf("expected same key for same inputs, got %q and %q", k1, k2)
	}
}

// TestPoolKey_NullByteSeparator verifies that colons in URIs do not collide
// with the separator. Using null bytes prevents ambiguity that a colon
// separator would introduce.
func TestPoolKey_NullByteSeparator(t *testing.T) {
	t.Parallel()

	// These would collide with a colon separator but not with null byte.
	k1 := PoolKey("proj-1", "img:latest", "iad")
	k2 := PoolKey("proj-1", "img", "latest:iad")
	k3 := PoolKey("proj-1:img", "latest", "iad")

	keys := map[string]string{
		"k1": k1,
		"k2": k2,
		"k3": k3,
	}

	for nameA, a := range keys {
		for nameB, b := range keys {
			if nameA != nameB && a == b {
				t.Fatalf("collision between %s and %s: both %q", nameA, nameB, a)
			}
		}
	}

	// Verify the key is a hex-encoded SHA-256 hash (64 chars).
	if len(k1) != 64 {
		t.Fatalf("expected 64-char hex hash key, got %d chars: %q", len(k1), k1)
	}
}

// TestPool_CrossProjectIsolation verifies that a machine released by project A
// cannot be acquired by project B.
func TestPool_CrossProjectIsolation(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Project A releases a machine.
	pool.Release("project-A", "myapp:v1", "iad", "m-from-A")

	// Project B tries to acquire -- should miss.
	_, ok := pool.Acquire("project-B", "myapp:v1", "iad")
	if ok {
		t.Fatal("project-B should not acquire a machine released by project-A")
	}

	// Project A can still acquire its own machine.
	id, ok := pool.Acquire("project-A", "myapp:v1", "iad")
	if !ok {
		t.Fatal("project-A should be able to acquire its own machine")
	}
	if id != "m-from-A" {
		t.Fatalf("expected m-from-A, got %q", id)
	}
}

// TestPool_SameProjectReuse verifies that a machine released by a project
// can be acquired by the same project.
func TestPool_SameProjectReuse(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	pool.Release("proj-1", "myapp:v1", "iad", "m-reuse")

	id, ok := pool.Acquire("proj-1", "myapp:v1", "iad")
	if !ok {
		t.Fatal("expected pool hit for same project")
	}
	if id != "m-reuse" {
		t.Fatalf("expected m-reuse, got %q", id)
	}
}

// TestPool_ConcurrentCrossProject verifies that concurrent operations from
// different projects do not leak machines across tenant boundaries.
func TestPool_ConcurrentCrossProject(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(10)
	projects := []string{"proj-A", "proj-B", "proj-C", "proj-D"}

	// Each project releases machines concurrently.
	var wg sync.WaitGroup
	for _, proj := range projects {
		for i := range 5 {
			wg.Add(1)
			go func(p string, idx int) {
				defer wg.Done()
				pool.Release(p, "shared-img:v1", "iad", fmt.Sprintf("m-%s-%d", p, idx))
			}(proj, i)
		}
	}
	wg.Wait()

	// Each project should only acquire its own machines.
	for _, proj := range projects {
		acquired := 0
		for {
			id, ok := pool.Acquire(proj, "shared-img:v1", "iad")
			if !ok {
				break
			}
			acquired++
			prefix := fmt.Sprintf("m-%s-", proj)
			if !strings.HasPrefix(id, prefix) {
				t.Errorf("project %s acquired machine %q which belongs to another project", proj, id)
			}
		}
		if acquired != 5 {
			t.Errorf("project %s acquired %d machines, expected 5", proj, acquired)
		}
	}

	// Pool should now be empty.
	if pool.Size() != 0 {
		t.Fatalf("expected empty pool after draining all projects, got size %d", pool.Size())
	}
}

// FuzzPoolKeyCollision fuzzes PoolKey to ensure different inputs never produce
// the same key (no false collisions).
func FuzzPoolKeyCollision(f *testing.F) {
	f.Add("proj-A", "img:v1", "iad", "proj-B", "img:v1", "iad")
	f.Add("proj", "img:v1", "iad", "proj\x00img", "v1", "iad")
	f.Add("a", "b", "c", "a\x00b", "c", "")
	f.Add("", "", "", "", "", "a")

	f.Fuzz(func(t *testing.T, proj1, img1, reg1, proj2, img2, reg2 string) {
		k1 := PoolKey(proj1, img1, reg1)
		k2 := PoolKey(proj2, img2, reg2)

		inputsSame := proj1 == proj2 && img1 == img2 && reg1 == reg2
		if k1 == k2 && !inputsSame {
			t.Errorf("collision: PoolKey(%q, %q, %q) == PoolKey(%q, %q, %q) == %q",
				proj1, img1, reg1, proj2, img2, reg2, k1)
		}
		if inputsSame && k1 != k2 {
			t.Errorf("same inputs produced different keys: %q vs %q", k1, k2)
		}
	})
}
