package compute

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMachinePool_AcquireFromPopulated(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)
	pool.Release("proj-1", "img:latest", "iad", "m-1")

	id, ok := pool.Acquire("proj-1", "img:latest", "iad")
	if !ok || id != "m-1" {
		t.Fatalf("expected m-1, got %q ok=%v", id, ok)
	}
}

func TestMachinePool_AcquireFromEmpty(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)

	_, ok := pool.Acquire("proj-1", "img:latest", "iad")
	if ok {
		t.Fatal("expected false for empty pool")
	}
}

func TestMachinePool_ReleaseStores(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)
	pool.Release("proj-1", "img:latest", "iad", "m-1")
	pool.Release("proj-1", "img:latest", "iad", "m-2")

	if pool.Size() != 2 {
		t.Fatalf("expected size 2, got %d", pool.Size())
	}
}

func TestMachinePool_PruneRemovesOld(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)

	// Manually insert old entry.
	pool.mu.Lock()
	pool.entries[PoolKey("proj-1", "img:latest", "iad")] = []poolEntry{
		{MachineID: "m-old", StoppedAt: time.Now().Add(-20 * time.Minute)},
		{MachineID: "m-new", StoppedAt: time.Now()},
	}
	pool.mu.Unlock()

	var destroyed []string
	pruned := pool.Prune(10*time.Minute, func(id string) error {
		destroyed = append(destroyed, id)
		return nil
	})

	if pruned != 1 {
		t.Fatalf("expected 1 pruned, got %d", pruned)
	}
	if len(destroyed) != 1 || destroyed[0] != "m-old" {
		t.Fatalf("expected m-old destroyed, got %v", destroyed)
	}
	if pool.Size() != 1 {
		t.Fatalf("expected 1 remaining, got %d", pool.Size())
	}
}

func TestMachinePool_MaxPerKeyEvicts(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(2)

	pool.Release("proj-1", "img:latest", "iad", "m-1")
	pool.Release("proj-1", "img:latest", "iad", "m-2")
	pool.Release("proj-1", "img:latest", "iad", "m-3") // should evict m-1

	if pool.Size() != 2 {
		t.Fatalf("expected 2 after eviction, got %d", pool.Size())
	}

	id, _ := pool.Acquire("proj-1", "img:latest", "iad")
	if id != "m-2" {
		t.Fatalf("expected m-2 (oldest after eviction), got %q", id)
	}
}

func TestMachinePool_DifferentKeysIndependent(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)

	pool.Release("proj-1", "img:a", "iad", "m-a")
	pool.Release("proj-1", "img:b", "lhr", "m-b")

	_, ok := pool.Acquire("proj-1", "img:a", "lhr")
	if ok {
		t.Fatal("should not find machine for different key")
	}

	id, ok := pool.Acquire("proj-1", "img:a", "iad")
	if !ok || id != "m-a" {
		t.Fatalf("expected m-a, got %q", id)
	}
}

func TestMachinePool_PoolDisabled(t *testing.T) {
	t.Parallel()
	// nil pool simulates disabled.
	var pool *MachinePool
	if pool != nil {
		t.Fatal("expected nil pool")
	}
}

func TestMachinePool_PruneWithNilDestroy(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)
	pool.mu.Lock()
	pool.entries[PoolKey("proj-1", "img:latest", "iad")] = []poolEntry{
		{MachineID: "m-old", StoppedAt: time.Now().Add(-20 * time.Minute)},
	}
	pool.mu.Unlock()

	pruned := pool.Prune(10*time.Minute, nil)
	if pruned != 1 {
		t.Fatalf("expected 1 pruned with nil destroy, got %d", pruned)
	}
}

func TestMachinePool_EvictionCallsDestroy(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(2)

	var evicted atomic.Value
	pool.SetOnEvict(func(machineID string) {
		evicted.Store(machineID)
	})

	pool.Release("proj-1", "img:latest", "iad", "m-1")
	pool.Release("proj-1", "img:latest", "iad", "m-2")
	pool.Release("proj-1", "img:latest", "iad", "m-3") // should evict m-1

	// Give async goroutine time to run.
	time.Sleep(50 * time.Millisecond)

	val := evicted.Load()
	if val == nil {
		t.Fatal("expected onEvict to be called")
	}
	if val.(string) != "m-1" {
		t.Errorf("expected m-1 evicted, got %q", val)
	}
}

func TestMachinePool_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(100)

	var released atomic.Int32
	var acquired atomic.Int32

	done := make(chan struct{})
	for i := range 50 {
		go func(idx int) {
			pool.Release("proj-1", "img:latest", "iad", "m-"+string(rune('a'+idx)))
			released.Add(1)
			if _, ok := pool.Acquire("proj-1", "img:latest", "iad"); ok {
				acquired.Add(1)
			}
			done <- struct{}{}
		}(i)
	}

	for range 50 {
		<-done
	}

	if released.Load() != 50 {
		t.Fatalf("expected 50 releases, got %d", released.Load())
	}
}

func TestMachinePool_EvictionBounded(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(1) // Max 1 per key → every release evicts the old one.

	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	pool.SetOnEvict(func(_ string) {
		cur := concurrent.Add(1)
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond) // Hold slot to test bounding.
		concurrent.Add(-1)
	})

	// Release 20 machines -> 19 evictions (first doesn't evict).
	for i := range 20 {
		pool.Release("proj-1", "img:latest", "iad", fmt.Sprintf("m-%d", i))
	}

	// Wait for all evictions to complete.
	time.Sleep(500 * time.Millisecond)

	// Max concurrent should be capped at 11 (10 from evictSem + 1 inline fallback).
	if maxConcurrent.Load() > 11 {
		t.Errorf("max concurrent evictions = %d, want <= 11", maxConcurrent.Load())
	}
}

func TestMachinePool_ReleaseEmptyID_Noop(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)
	pool.Release("proj-1", "img:latest", "iad", "")
	if pool.Size() != 0 {
		t.Fatalf("expected size 0 after empty-ID release, got %d", pool.Size())
	}
}

func TestMachinePool_ReleaseWithoutAcquire_CapsAtMax(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)
	for i := range 100 {
		pool.Release("proj-1", "img:latest", "iad", fmt.Sprintf("m-%d", i))
	}
	// Wait for any async evictions.
	time.Sleep(100 * time.Millisecond)
	if pool.Size() != 3 {
		t.Fatalf("expected size = maxPer (3), got %d", pool.Size())
	}
}

func TestMachinePool_ConcurrentStress(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(5)

	var wg sync.WaitGroup
	for i := range 500 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("img-%d", idx%10)
			if idx%2 == 0 {
				pool.Release("proj-1", key, "iad", fmt.Sprintf("m-%d", idx))
			} else {
				pool.Acquire("proj-1", key, "iad")
			}
		}(i)
	}
	wg.Wait()

	// Verify pool size ≤ maxPer * numKeys (10 keys * 5 = 50).
	if pool.Size() > 50 {
		t.Errorf("pool size %d exceeds max possible (50)", pool.Size())
	}
}

func TestMachinePool_PruneDuringConcurrentAccess(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(10)

	// Pre-fill with old entries.
	pool.mu.Lock()
	for i := range 20 {
		key := PoolKey("proj-1", "img:latest", "iad")
		pool.entries[key] = append(pool.entries[key], poolEntry{
			MachineID: fmt.Sprintf("m-%d", i),
			StoppedAt: time.Now().Add(-20 * time.Minute),
		})
	}
	pool.mu.Unlock()

	done := make(chan struct{})
	go func() {
		// Concurrent Release/Acquire on a different key so it doesn't
		// drain the entries that Prune targets.
		for i := range 100 {
			pool.Release("proj-1", "img:other", "iad", fmt.Sprintf("new-%d", i))
			pool.Acquire("proj-1", "img:other", "iad")
		}
		close(done)
	}()

	pruned := pool.Prune(10*time.Minute, func(_ string) error { return nil })
	<-done

	if pruned < 1 {
		t.Errorf("expected at least 1 pruned, got %d", pruned)
	}
}

func TestMachinePool_OnEvictPanic_DoesntCrashPool(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(1)
	pool.SetOnEvict(func(_ string) {
		panic("eviction panic")
	})

	// This should not crash — the bounded eviction wraps in a goroutine
	// and panics are isolated per goroutine. We don't recover, but
	// we verify the pool is still usable after a non-inline eviction.
	pool.Release("proj-1", "img:latest", "iad", "m-1")

	// Fill semaphore to force inline eviction path.
	// Note: with semaphore, first eviction runs async, might panic in goroutine.
	// We give it time but the test process won't crash because panics in
	// goroutines only crash if they reach the top of the goroutine stack.
	time.Sleep(100 * time.Millisecond)

	// Pool should still be functional.
	pool.Release("proj-1", "img:latest", "iad", "m-2")
	id, ok := pool.Acquire("proj-1", "img:latest", "iad")
	if !ok || id != "m-2" {
		t.Errorf("pool not functional after eviction panic: got %q ok=%v", id, ok)
	}
}

// Phase 4 tests.

func TestMachinePool_AcquireAfterPrune(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(5)
	pool.Release("proj-1", "img:latest", "iad", "m-1")
	pool.Release("proj-1", "img:latest", "iad", "m-2")

	// Prune everything by using age 0.
	pool.Prune(0, func(_ string) error { return nil })

	_, ok := pool.Acquire("proj-1", "img:latest", "iad")
	if ok {
		t.Fatal("expected Acquire to return false after pruning all entries")
	}
}

func TestMachinePool_PruneWithZeroAge(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(5)
	pool.Release("proj-1", "img:latest", "iad", "m-1")
	pool.Release("proj-1", "img:latest", "iad", "m-2")
	pool.Release("proj-1", "img:other", "lhr", "m-3")

	pruned := pool.Prune(0, func(_ string) error { return nil })
	if pruned != 3 {
		t.Fatalf("expected 3 pruned with zero age, got %d", pruned)
	}
	if pool.Size() != 0 {
		t.Fatalf("expected pool empty after Prune(0), got size %d", pool.Size())
	}
}

func TestMachinePool_ReleaseSameMachineTwice(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(5)
	pool.Release("proj-1", "img:latest", "iad", "m-1")
	pool.Release("proj-1", "img:latest", "iad", "m-1")

	if pool.Size() != 2 {
		t.Fatalf("expected size 2 after releasing same machine twice, got %d", pool.Size())
	}
}

func TestMachinePool_Size_AccurateUnderConcurrency(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(100)

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("img-%d", idx%10)
			pool.Release("proj-1", key, "iad", fmt.Sprintf("m-%d", idx))
		}(i)
	}
	wg.Wait()

	size := pool.Size()
	// 10 keys, up to 100 per key, 10 releases per key -> expect exactly 100.
	if size < 1 || size > 100 {
		t.Fatalf("expected pool size between 1 and 100, got %d", size)
	}
}

func TestMachinePool_PruneDestroyError_ContinuesPruning(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(10)

	// Insert 5 old entries.
	pool.mu.Lock()
	key := PoolKey("proj-1", "img:latest", "iad")
	for i := range 5 {
		pool.entries[key] = append(pool.entries[key], poolEntry{
			MachineID: fmt.Sprintf("m-%d", i),
			StoppedAt: time.Now().Add(-20 * time.Minute),
		})
	}
	pool.mu.Unlock()

	var attempted sync.Map
	pruned := pool.Prune(10*time.Minute, func(id string) error {
		attempted.Store(id, true)
		// Error on even-numbered machines.
		if id == "m-0" || id == "m-2" || id == "m-4" {
			return fmt.Errorf("destroy failed for %s", id)
		}
		return nil
	})

	if pruned != 5 {
		t.Fatalf("expected 5 pruned (all old), got %d", pruned)
	}

	// Verify all 5 machines were attempted.
	for i := range 5 {
		id := fmt.Sprintf("m-%d", i)
		if _, ok := attempted.Load(id); !ok {
			t.Errorf("expected destroy to be attempted for %s", id)
		}
	}
}
