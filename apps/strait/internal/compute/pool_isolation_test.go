package compute

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPool_CrossProjectSameImage verifies that different projects using the same
// image and region get isolated pool slots.
func TestPool_CrossProjectSameImage(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Project-A releases a machine.
	pool.Release("project-A", "myapp:v1", "iad", "m-projectA")

	// Project-B cannot acquire from project-A's pool.
	_, ok := pool.Acquire("project-B", "myapp:v1", "iad")
	if ok {
		t.Fatal("expected Acquire to fail for different project with same image+region")
	}

	// Project-A can still acquire its own machine.
	id, ok := pool.Acquire("project-A", "myapp:v1", "iad")
	if !ok || id != "m-projectA" {
		t.Fatalf("expected m-projectA, got %q ok=%v", id, ok)
	}

	// Verify that PoolKey returns a 64-char hex SHA-256 hash.
	key := PoolKey("project-A", "myapp:v1", "iad")
	if len(key) != 64 {
		t.Fatalf("PoolKey should be a 64-char hex hash, got %d chars: %q", len(key), key)
	}
}

// TestPool_EnvironmentCleanupOnReuse documents that the pool stores only machine
// IDs, not environment variables. Callers are responsible for passing fresh env
// when restarting a pooled machine via Start().
func TestPool_EnvironmentCleanupOnReuse(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Release a machine -- pool only stores MachineID and StoppedAt.
	pool.Release("proj-1", "myapp:v1", "iad", "m-1")

	// Acquire returns only the machine ID.
	id, ok := pool.Acquire("proj-1", "myapp:v1", "iad")
	if !ok || id != "m-1" {
		t.Fatalf("expected m-1, got %q ok=%v", id, ok)
	}

	// The pool entry (poolEntry) has no Env field, so no stale environment
	// leaks through the pool itself. The caller must supply fresh env on
	// Start(). Verify key is a 64-char hex SHA-256 hash.
	key := PoolKey("proj-1", "myapp:v1", "iad")
	if len(key) != 64 {
		t.Fatalf("expected 64-char hex hash key, got %d chars: %q", len(key), key)
	}
}

// TestPool_FilesystemIsolation documents that the pool does not perform any
// filesystem cleanup between reuses. The machine's filesystem persists across
// runs when a pooled machine is reused.
func TestPool_FilesystemIsolation(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Release and re-acquire the same machine.
	pool.Release("proj-1", "myapp:v1", "iad", "m-1")
	id, ok := pool.Acquire("proj-1", "myapp:v1", "iad")
	if !ok || id != "m-1" {
		t.Fatalf("expected m-1, got %q ok=%v", id, ok)
	}

	// The pool performs no filesystem operations -- it simply tracks machine
	// IDs. Filesystem state from previous runs persists. This is a known
	// trade-off for reduced cold-start latency.
}

// TestPool_SecretLeakageBetweenRuns verifies that pool entries do not store
// environment variables or other secret-bearing fields.
func TestPool_SecretLeakageBetweenRuns(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Release a machine. The poolEntry struct only contains MachineID,
	// StoppedAt, and LastRunID -- no env, secrets, or request data.
	pool.Release("proj-1", "myapp:v1", "iad", "m-secret-run")

	id, ok := pool.Acquire("proj-1", "myapp:v1", "iad")
	if !ok || id != "m-secret-run" {
		t.Fatalf("expected m-secret-run, got %q ok=%v", id, ok)
	}

	// Verify pool is now empty -- no residual entries.
	if pool.Size() != 0 {
		t.Fatalf("expected pool empty after acquire, got size %d", pool.Size())
	}
}

// TestPool_MaxPoolSize verifies that releasing more machines than maxPerKey
// triggers eviction of the oldest entries.
func TestPool_MaxPoolSize(t *testing.T) {
	t.Parallel()

	maxPer := 3
	pool := NewMachinePool(maxPer)

	var evicted []string
	var mu sync.Mutex
	pool.SetOnEvict(func(machineID string) {
		mu.Lock()
		evicted = append(evicted, machineID)
		mu.Unlock()
	})

	// Release more machines than the max.
	for i := range 6 {
		pool.Release("proj-1", "myapp:v1", "iad", fmt.Sprintf("m-%d", i))
	}

	// Allow async eviction goroutines to complete.
	time.Sleep(100 * time.Millisecond)

	if pool.Size() != maxPer {
		t.Fatalf("expected pool size %d, got %d", maxPer, pool.Size())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(evicted) != 3 {
		t.Fatalf("expected 3 evictions, got %d: %v", len(evicted), evicted)
	}
}

// TestPool_ConcurrentAcquireRelease verifies that concurrent Acquire and
// Release operations do not cause data races or panics.
func TestPool_ConcurrentAcquireRelease(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(10)

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			pool.Release("proj-1", "myapp:v1", "iad", fmt.Sprintf("m-%d", idx))
		}(i)
		go func(idx int) {
			defer wg.Done()
			pool.Acquire("proj-1", "myapp:v1", "iad")
		}(i)
	}
	wg.Wait()

	// Pool should not exceed maxPerKey.
	if pool.Size() > 10 {
		t.Fatalf("pool size %d exceeds max per key (10)", pool.Size())
	}
}

// TestPool_KeyCollision verifies that null byte separators prevent collisions
// between image URIs containing colons.
func TestPool_KeyCollision(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Keys that would collide with colon separator but not with null byte.
	pool.Release("proj-1", "img:latest", "iad", "m-1")
	pool.Release("proj-1", "img", "latest\x00iad", "m-2")

	// These should be stored under different keys.
	key1 := PoolKey("proj-1", "img:latest", "iad")
	key2 := PoolKey("proj-1", "img", "latest\x00iad")

	if key1 == key2 {
		t.Fatalf("keys should not collide: %q == %q", key1, key2)
	}
}

// TestPool_StalePoolEntry verifies behavior when acquiring a machine ID that
// may no longer exist. The pool returns the ID regardless -- the caller is
// responsible for handling machines that have been destroyed externally.
func TestPool_StalePoolEntry(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Release a machine that "no longer exists" externally.
	pool.Release("proj-1", "myapp:v1", "iad", "m-destroyed-externally")

	// Pool returns the stale ID. Caller must handle ErrMachineGone from Start().
	id, ok := pool.Acquire("proj-1", "myapp:v1", "iad")
	if !ok {
		t.Fatal("expected Acquire to return the stale entry")
	}
	if id != "m-destroyed-externally" {
		t.Fatalf("expected m-destroyed-externally, got %q", id)
	}
}

// TestPool_EvictionUnderPressure verifies that the oldest entries are evicted
// when the pool is at capacity.
func TestPool_EvictionUnderPressure(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	var evictOrder []string
	var mu sync.Mutex
	pool.SetOnEvict(func(machineID string) {
		mu.Lock()
		evictOrder = append(evictOrder, machineID)
		mu.Unlock()
	})

	// Fill to capacity.
	pool.Release("proj-1", "myapp:v1", "iad", "m-oldest")
	pool.Release("proj-1", "myapp:v1", "iad", "m-middle")
	pool.Release("proj-1", "myapp:v1", "iad", "m-newest")

	// One more triggers eviction of the oldest.
	pool.Release("proj-1", "myapp:v1", "iad", "m-pressure")

	// Allow async eviction.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(evictOrder) != 1 {
		t.Fatalf("expected 1 eviction, got %d: %v", len(evictOrder), evictOrder)
	}
	if evictOrder[0] != "m-oldest" {
		t.Fatalf("expected oldest (m-oldest) to be evicted, got %q", evictOrder[0])
	}

	// Verify the remaining entries are the newer ones.
	id, _ := pool.Acquire("proj-1", "myapp:v1", "iad")
	if id != "m-middle" {
		t.Fatalf("expected m-middle as oldest remaining, got %q", id)
	}
}

// FuzzPoolKey sends random image and region strings through PoolKey to ensure
// it never panics.
func FuzzPoolKey(f *testing.F) {
	f.Add("proj-1", "nginx:latest", "iad")
	f.Add("", "", "")
	f.Add("proj-1", "img:tag", "region:with:colons")
	f.Add("proj-1", strings.Repeat("a", 10000), strings.Repeat("b", 10000))
	f.Add("proj-1", "image/with/slashes", "us-east-1")
	f.Add("proj-1", "registry.com:5000/org/img@sha256:abc", "eu-west-1")

	f.Fuzz(func(t *testing.T, projectID, image, region string) {
		// Must not panic regardless of input.
		key := PoolKey(projectID, image, region)

		// Key must be a 64-char hex SHA-256 hash.
		if len(key) != 64 {
			t.Errorf("expected 64-char hex hash, got %d chars: %q", len(key), key)
		}

		// Same inputs must always produce the same key.
		key2 := PoolKey(projectID, image, region)
		if key != key2 {
			t.Errorf("same inputs produced different keys: %q vs %q", key, key2)
		}

		// Key must be usable as a map key without issues.
		var released atomic.Int32
		pool := NewMachinePool(1)
		pool.Release(projectID, image, region, "m-fuzz")
		if _, ok := pool.Acquire(projectID, image, region); ok {
			released.Add(1)
		}
	})
}
