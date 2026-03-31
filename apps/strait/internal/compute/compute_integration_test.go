//go:build integration

package compute_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/compute"
)

// --- Pool creation and sizing ---

func TestPool_CreateAndSize(t *testing.T) {
	pool := compute.NewMachinePool(3)

	if got := pool.Size(); got != 0 {
		t.Fatalf("empty pool Size() = %d, want 0", got)
	}

	pool.Release("proj-1", "image:latest", "iad", "machine-1")
	pool.Release("proj-1", "image:latest", "iad", "machine-2")

	if got := pool.Size(); got != 2 {
		t.Fatalf("Size() after 2 releases = %d, want 2", got)
	}
}

func TestPool_DefaultMaxPerKey(t *testing.T) {
	// Zero or negative maxPerKey should default to 3.
	pool := compute.NewMachinePool(0)

	pool.Release("proj-1", "image:v1", "iad", "m-1")
	pool.Release("proj-1", "image:v1", "iad", "m-2")
	pool.Release("proj-1", "image:v1", "iad", "m-3")
	pool.Release("proj-1", "image:v1", "iad", "m-4")

	// Only 3 should be kept (default cap), 4th release evicts the oldest.
	if got := pool.Size(); got != 3 {
		t.Fatalf("Size() with default cap = %d, want 3", got)
	}
}

func TestPool_MaxPerKeyEnforced(t *testing.T) {
	var mu sync.Mutex
	var evicted []string
	pool := compute.NewMachinePool(2)
	pool.SetOnEvict(func(machineID string) {
		mu.Lock()
		evicted = append(evicted, machineID)
		mu.Unlock()
	})

	pool.Release("proj-1", "image:v1", "iad", "m-1")
	pool.Release("proj-1", "image:v1", "iad", "m-2")
	pool.Release("proj-1", "image:v1", "iad", "m-3")

	// Allow eviction goroutine to run.
	time.Sleep(50 * time.Millisecond)

	if got := pool.Size(); got != 2 {
		t.Fatalf("Size() after eviction = %d, want 2", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(evicted) != 1 || evicted[0] != "m-1" {
		t.Errorf("evicted = %v, want [m-1]", evicted)
	}
}

// --- Machine slot allocation/deallocation ---

func TestPool_AcquireReturnsOldest(t *testing.T) {
	pool := compute.NewMachinePool(5)

	pool.Release("proj-1", "image:v1", "iad", "m-old")
	pool.Release("proj-1", "image:v1", "iad", "m-new")

	got, ok := pool.Acquire("proj-1", "image:v1", "iad")
	if !ok {
		t.Fatal("Acquire() returned false, want true")
	}
	if got != "m-old" {
		t.Errorf("Acquire() = %q, want %q (oldest first)", got, "m-old")
	}

	if size := pool.Size(); size != 1 {
		t.Errorf("Size() after Acquire = %d, want 1", size)
	}
}

func TestPool_AcquireEmptyPool(t *testing.T) {
	pool := compute.NewMachinePool(3)

	_, ok := pool.Acquire("proj-1", "image:v1", "iad")
	if ok {
		t.Error("Acquire() on empty pool returned true, want false")
	}
}

func TestPool_AcquireKeyIsolation(t *testing.T) {
	pool := compute.NewMachinePool(3)

	pool.Release("proj-1", "image:v1", "iad", "m-1")
	pool.Release("proj-2", "image:v1", "iad", "m-2")

	// Acquire from proj-1 should only get m-1.
	got, ok := pool.Acquire("proj-1", "image:v1", "iad")
	if !ok {
		t.Fatal("Acquire(proj-1) returned false")
	}
	if got != "m-1" {
		t.Errorf("Acquire(proj-1) = %q, want %q", got, "m-1")
	}

	// proj-1 pool is now empty.
	_, ok = pool.Acquire("proj-1", "image:v1", "iad")
	if ok {
		t.Error("Acquire(proj-1) returned true on empty key")
	}

	// proj-2 should still have its machine.
	got, ok = pool.Acquire("proj-2", "image:v1", "iad")
	if !ok {
		t.Fatal("Acquire(proj-2) returned false")
	}
	if got != "m-2" {
		t.Errorf("Acquire(proj-2) = %q, want %q", got, "m-2")
	}
}

func TestPool_ReleaseEmptyMachineID(t *testing.T) {
	pool := compute.NewMachinePool(3)

	// Releasing an empty machine ID should be a no-op.
	pool.Release("proj-1", "image:v1", "iad", "")

	if got := pool.Size(); got != 0 {
		t.Fatalf("Size() after releasing empty ID = %d, want 0", got)
	}
}

// --- Pool pruning ---

func TestPool_PruneByAge(t *testing.T) {
	pool := compute.NewMachinePool(10)

	pool.Release("proj-1", "image:v1", "iad", "m-old")

	// Wait to create age difference.
	time.Sleep(100 * time.Millisecond)

	pool.Release("proj-1", "image:v1", "iad", "m-new")

	var destroyed []string
	pruned := pool.Prune(80*time.Millisecond, func(machineID string) error {
		destroyed = append(destroyed, machineID)
		return nil
	})

	if pruned != 1 {
		t.Errorf("Prune() = %d, want 1", pruned)
	}
	if len(destroyed) != 1 || destroyed[0] != "m-old" {
		t.Errorf("destroyed = %v, want [m-old]", destroyed)
	}

	// Only the newer machine should remain.
	if got := pool.Size(); got != 1 {
		t.Errorf("Size() after prune = %d, want 1", got)
	}

	got, ok := pool.Acquire("proj-1", "image:v1", "iad")
	if !ok {
		t.Fatal("Acquire() after prune returned false")
	}
	if got != "m-new" {
		t.Errorf("Acquire() after prune = %q, want %q", got, "m-new")
	}
}

func TestPool_PruneNothingToRemove(t *testing.T) {
	pool := compute.NewMachinePool(3)

	pool.Release("proj-1", "image:v1", "iad", "m-1")

	pruned := pool.Prune(1*time.Hour, nil)
	if pruned != 0 {
		t.Errorf("Prune() = %d, want 0 (nothing expired)", pruned)
	}

	if got := pool.Size(); got != 1 {
		t.Errorf("Size() after no-op prune = %d, want 1", got)
	}
}

func TestPool_PruneAll(t *testing.T) {
	pool := compute.NewMachinePool(5)

	pool.Release("proj-1", "image:v1", "iad", "m-1")
	pool.Release("proj-1", "image:v1", "iad", "m-2")
	pool.Release("proj-2", "image:v2", "lax", "m-3")

	// Wait so all entries are older than the threshold.
	time.Sleep(100 * time.Millisecond)

	pruned := pool.Prune(50*time.Millisecond, func(machineID string) error {
		return nil
	})

	if pruned != 3 {
		t.Errorf("Prune() = %d, want 3", pruned)
	}

	if got := pool.Size(); got != 0 {
		t.Errorf("Size() after full prune = %d, want 0", got)
	}
}

// --- Concurrent pool access ---

func TestPool_ConcurrentAcquireRelease(t *testing.T) {
	pool := compute.NewMachinePool(50)

	// Pre-fill the pool.
	for i := range 20 {
		pool.Release("proj-1", "image:v1", "iad", fmt.Sprintf("m-%d", i))
	}

	var acquired atomic.Int64

	var wg sync.WaitGroup
	// Concurrent acquirers.
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := pool.Acquire("proj-1", "image:v1", "iad"); ok {
				acquired.Add(1)
			}
		}()
	}

	// Concurrent releasers.
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Release("proj-1", "image:v1", "iad", fmt.Sprintf("m-new-%d", i))
		}()
	}

	wg.Wait()

	// At least the 20 pre-filled machines should have been acquired.
	// Plus potentially some of the 10 newly released ones.
	if got := acquired.Load(); got < 20 {
		t.Errorf("concurrent acquired = %d, want >= 20", got)
	}
}

func TestPool_ConcurrentPrune(t *testing.T) {
	pool := compute.NewMachinePool(50)

	for i := range 30 {
		pool.Release("proj-1", "image:v1", "iad", fmt.Sprintf("m-%d", i))
	}

	// Wait so entries are old enough.
	time.Sleep(100 * time.Millisecond)

	var totalPruned atomic.Int64
	var wg sync.WaitGroup

	// Run concurrent prune and acquire operations.
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := pool.Prune(50*time.Millisecond, func(machineID string) error {
				return nil
			})
			totalPruned.Add(int64(n))
		}()
	}

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Acquire("proj-1", "image:v1", "iad")
		}()
	}

	wg.Wait()

	// Pool should be empty after all operations.
	if got := pool.Size(); got != 0 {
		t.Errorf("Size() after concurrent prune+acquire = %d, want 0", got)
	}
}

// --- Cost calculation with real preset data ---

func TestCalculateCost_AllPresets(t *testing.T) {
	// Verify cost calculations work for all known presets.
	for _, name := range compute.PresetOrder {
		preset, err := compute.PresetFromName(name)
		if err != nil {
			t.Fatalf("PresetFromName(%q) error = %v", name, err)
		}

		cost, err := compute.CalculateCost(name, 60.0)
		if err != nil {
			t.Fatalf("CalculateCost(%q, 60) error = %v", name, err)
		}

		wantCost := preset.CostPerSecond * 60
		if cost != wantCost {
			t.Errorf("CalculateCost(%q, 60) = %d, want %d", name, cost, wantCost)
		}
	}
}

func TestCalculateCost_ZeroDuration(t *testing.T) {
	cost, err := compute.CalculateCost("micro", 0)
	if err != nil {
		t.Fatalf("CalculateCost(micro, 0) error = %v", err)
	}
	if cost != 0 {
		t.Errorf("CalculateCost(micro, 0) = %d, want 0", cost)
	}
}

func TestCalculateCost_NegativeDuration(t *testing.T) {
	cost, err := compute.CalculateCost("micro", -10)
	if err != nil {
		t.Fatalf("CalculateCost(micro, -10) error = %v", err)
	}
	if cost != 0 {
		t.Errorf("CalculateCost(micro, -10) = %d, want 0", cost)
	}
}

func TestCalculateCost_UnknownPreset(t *testing.T) {
	_, err := compute.CalculateCost("nonexistent", 60)
	if err == nil {
		t.Error("CalculateCost(nonexistent) error = nil, want error")
	}
}

func TestEstimateCost_MatchesCalculateCost(t *testing.T) {
	for _, name := range compute.PresetOrder {
		timeoutSecs := 300

		estimate, err := compute.EstimateCost(name, timeoutSecs)
		if err != nil {
			t.Fatalf("EstimateCost(%q, %d) error = %v", name, timeoutSecs, err)
		}

		calculated, err := compute.CalculateCost(name, float64(timeoutSecs))
		if err != nil {
			t.Fatalf("CalculateCost(%q, %d) error = %v", name, timeoutSecs, err)
		}

		if estimate != calculated {
			t.Errorf("EstimateCost(%q) = %d, CalculateCost(%q) = %d, want equal",
				name, estimate, name, calculated)
		}
	}
}

func TestCalculateCost_Ordering(t *testing.T) {
	// Larger presets should cost more per second.
	durationSecs := 100.0
	var prevCost int64

	for _, name := range compute.PresetOrder {
		cost, err := compute.CalculateCost(name, durationSecs)
		if err != nil {
			t.Fatalf("CalculateCost(%q) error = %v", name, err)
		}
		if cost <= prevCost && prevCost > 0 {
			t.Errorf("preset %q cost %d is not greater than previous cost %d", name, cost, prevCost)
		}
		prevCost = cost
	}
}

// --- PoolKey determinism and collision resistance ---

func TestPoolKey_Deterministic(t *testing.T) {
	k1 := compute.PoolKey("proj-1", "image:v1", "iad")
	k2 := compute.PoolKey("proj-1", "image:v1", "iad")
	if k1 != k2 {
		t.Errorf("PoolKey not deterministic: %q != %q", k1, k2)
	}
}

func TestPoolKey_DifferentInputs(t *testing.T) {
	keys := map[string]bool{}

	inputs := [][3]string{
		{"proj-1", "image:v1", "iad"},
		{"proj-2", "image:v1", "iad"},
		{"proj-1", "image:v2", "iad"},
		{"proj-1", "image:v1", "lax"},
		{"proj-1", "image:v1:iad", ""},
	}

	for _, in := range inputs {
		k := compute.PoolKey(in[0], in[1], in[2])
		if keys[k] {
			t.Errorf("PoolKey collision for inputs %v", in)
		}
		keys[k] = true
	}
}

// --- Eviction callback ---

func TestPool_EvictionCallback(t *testing.T) {
	var evicted []string
	var mu sync.Mutex

	pool := compute.NewMachinePool(2)
	pool.SetOnEvict(func(machineID string) {
		mu.Lock()
		defer mu.Unlock()
		evicted = append(evicted, machineID)
	})

	pool.Release("proj-1", "image:v1", "iad", "m-1")
	pool.Release("proj-1", "image:v1", "iad", "m-2")
	// This should evict m-1 (oldest).
	pool.Release("proj-1", "image:v1", "iad", "m-3")
	// This should evict m-2.
	pool.Release("proj-1", "image:v1", "iad", "m-4")

	// Wait for async eviction goroutines to complete.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(evicted) != 2 {
		t.Fatalf("evicted count = %d, want 2", len(evicted))
	}
	if evicted[0] != "m-1" {
		t.Errorf("evicted[0] = %q, want %q", evicted[0], "m-1")
	}
	if evicted[1] != "m-2" {
		t.Errorf("evicted[1] = %q, want %q", evicted[1], "m-2")
	}
}
