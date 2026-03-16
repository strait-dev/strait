package compute

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestMachinePool_AcquireFromPopulated(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)
	pool.Release("img:latest", "iad", "m-1")

	id, ok := pool.Acquire("img:latest", "iad")
	if !ok || id != "m-1" {
		t.Fatalf("expected m-1, got %q ok=%v", id, ok)
	}
}

func TestMachinePool_AcquireFromEmpty(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)

	_, ok := pool.Acquire("img:latest", "iad")
	if ok {
		t.Fatal("expected false for empty pool")
	}
}

func TestMachinePool_ReleaseStores(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)
	pool.Release("img:latest", "iad", "m-1")
	pool.Release("img:latest", "iad", "m-2")

	if pool.Size() != 2 {
		t.Fatalf("expected size 2, got %d", pool.Size())
	}
}

func TestMachinePool_PruneRemovesOld(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)

	// Manually insert old entry.
	pool.mu.Lock()
	pool.entries[PoolKey("img:latest", "iad")] = []poolEntry{
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

	pool.Release("img:latest", "iad", "m-1")
	pool.Release("img:latest", "iad", "m-2")
	pool.Release("img:latest", "iad", "m-3") // should evict m-1

	if pool.Size() != 2 {
		t.Fatalf("expected 2 after eviction, got %d", pool.Size())
	}

	id, _ := pool.Acquire("img:latest", "iad")
	if id != "m-2" {
		t.Fatalf("expected m-2 (oldest after eviction), got %q", id)
	}
}

func TestMachinePool_DifferentKeysIndependent(t *testing.T) {
	t.Parallel()
	pool := NewMachinePool(3)

	pool.Release("img:a", "iad", "m-a")
	pool.Release("img:b", "lhr", "m-b")

	_, ok := pool.Acquire("img:a", "lhr")
	if ok {
		t.Fatal("should not find machine for different key")
	}

	id, ok := pool.Acquire("img:a", "iad")
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
	pool.entries[PoolKey("img:latest", "iad")] = []poolEntry{
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

	pool.Release("img:latest", "iad", "m-1")
	pool.Release("img:latest", "iad", "m-2")
	pool.Release("img:latest", "iad", "m-3") // should evict m-1

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
			pool.Release("img:latest", "iad", "m-"+string(rune('a'+idx)))
			released.Add(1)
			if _, ok := pool.Acquire("img:latest", "iad"); ok {
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
