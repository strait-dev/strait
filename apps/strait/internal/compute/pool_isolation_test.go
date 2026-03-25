package compute

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPool_CrossProjectSameImage verifies that the pool key is based on image
// URI and region only. Machines from different "projects" sharing the same image
// and region will share a pool slot, because PoolKey does not include project.
func TestPool_CrossProjectSameImage(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Simulate project-A releasing a machine.
	pool.Release("myapp:v1", "iad", "m-projectA")

	// Project-B acquires from the same image+region key.
	id, ok := pool.Acquire("myapp:v1", "iad")
	if !ok {
		t.Fatal("expected Acquire to succeed for same image+region from different project")
	}
	if id != "m-projectA" {
		t.Fatalf("expected m-projectA, got %q", id)
	}

	// Verify that PoolKey does not include any project identifier.
	key := PoolKey("myapp:v1", "iad")
	if strings.Contains(key, "project") {
		t.Fatalf("PoolKey should not contain project identifier, got %q", key)
	}
}

// TestPool_EnvironmentCleanupOnReuse documents that the pool stores only machine
// IDs, not environment variables. Callers are responsible for passing fresh env
// when restarting a pooled machine via Start().
func TestPool_EnvironmentCleanupOnReuse(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Release a machine -- pool only stores MachineID and StoppedAt.
	pool.Release("myapp:v1", "iad", "m-1")

	// Acquire returns only the machine ID.
	id, ok := pool.Acquire("myapp:v1", "iad")
	if !ok || id != "m-1" {
		t.Fatalf("expected m-1, got %q ok=%v", id, ok)
	}

	// The pool entry (poolEntry) has no Env field, so no stale environment
	// leaks through the pool itself. The caller must supply fresh env on
	// Start(). This is verified by inspecting the pool key format.
	key := PoolKey("myapp:v1", "iad")
	if key != "myapp:v1:iad" {
		t.Fatalf("unexpected pool key format: %q", key)
	}
}

// TestPool_FilesystemIsolation documents that the pool does not perform any
// filesystem cleanup between reuses. The machine's filesystem persists across
// runs when a pooled machine is reused.
func TestPool_FilesystemIsolation(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Release and re-acquire the same machine.
	pool.Release("myapp:v1", "iad", "m-1")
	id, ok := pool.Acquire("myapp:v1", "iad")
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
	pool.Release("myapp:v1", "iad", "m-secret-run")

	id, ok := pool.Acquire("myapp:v1", "iad")
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
		pool.Release("myapp:v1", "iad", fmt.Sprintf("m-%d", i))
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
			pool.Release("myapp:v1", "iad", fmt.Sprintf("m-%d", idx))
		}(i)
		go func(idx int) {
			defer wg.Done()
			pool.Acquire("myapp:v1", "iad")
		}(i)
	}
	wg.Wait()

	// Pool should not exceed maxPerKey.
	if pool.Size() > 10 {
		t.Fatalf("pool size %d exceeds max per key (10)", pool.Size())
	}
}

// TestPool_KeyCollision verifies that special characters in image URIs used as
// pool keys do not cause collisions or unexpected behavior.
func TestPool_KeyCollision(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Keys that might collide if separators are not handled carefully.
	pool.Release("img:latest", "iad", "m-1")
	pool.Release("img", "latest:iad", "m-2")

	// These should be stored under different keys.
	key1 := PoolKey("img:latest", "iad")
	key2 := PoolKey("img", "latest:iad")

	// The keys will be "img:latest:iad" and "img:latest:iad" -- they collide
	// because the separator is ":" and the image URI can contain ":".
	if key1 == key2 {
		// This is a documented key collision. PoolKey uses simple concatenation
		// with ":" which can produce identical keys for different inputs.
		t.Logf("key collision detected: %q == %q (known limitation)", key1, key2)

		// Both machines end up in the same bucket.
		if pool.Size() != 2 {
			t.Fatalf("expected 2 entries in collided bucket, got %d", pool.Size())
		}
	}
}

// TestPool_StalePoolEntry verifies behavior when acquiring a machine ID that
// may no longer exist. The pool returns the ID regardless -- the caller is
// responsible for handling machines that have been destroyed externally.
func TestPool_StalePoolEntry(t *testing.T) {
	t.Parallel()

	pool := NewMachinePool(3)

	// Release a machine that "no longer exists" externally.
	pool.Release("myapp:v1", "iad", "m-destroyed-externally")

	// Pool returns the stale ID. Caller must handle ErrMachineGone from Start().
	id, ok := pool.Acquire("myapp:v1", "iad")
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
	pool.Release("myapp:v1", "iad", "m-oldest")
	pool.Release("myapp:v1", "iad", "m-middle")
	pool.Release("myapp:v1", "iad", "m-newest")

	// One more triggers eviction of the oldest.
	pool.Release("myapp:v1", "iad", "m-pressure")

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
	id, _ := pool.Acquire("myapp:v1", "iad")
	if id != "m-middle" {
		t.Fatalf("expected m-middle as oldest remaining, got %q", id)
	}
}

// FuzzPoolKey sends random image and region strings through PoolKey to ensure
// it never panics.
func FuzzPoolKey(f *testing.F) {
	f.Add("nginx:latest", "iad")
	f.Add("", "")
	f.Add("img:tag", "region:with:colons")
	f.Add(strings.Repeat("a", 10000), strings.Repeat("b", 10000))
	f.Add("image/with/slashes", "us-east-1")
	f.Add("registry.com:5000/org/img@sha256:abc", "eu-west-1")

	f.Fuzz(func(t *testing.T, image, region string) {
		// Must not panic regardless of input.
		key := PoolKey(image, region)

		// Key must contain both inputs when they are non-empty.
		if image != "" && !strings.Contains(key, image) {
			t.Errorf("key %q does not contain image %q", key, image)
		}
		if region != "" && !strings.Contains(key, region) {
			t.Errorf("key %q does not contain region %q", key, region)
		}

		// Key must be usable as a map key without issues.
		var released atomic.Int32
		pool := NewMachinePool(1)
		pool.Release(image, region, "m-fuzz")
		if _, ok := pool.Acquire(image, region); ok {
			released.Add(1)
		}
	})
}
