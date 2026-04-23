package build

import (
	"context"
	"sync"
	"testing"

	"github.com/sourcegraph/conc"

	"strait/internal/domain"
)

func TestAddressPool_RoundRobin(t *testing.T) {
	// When extras is provided, only extras entries are used (primary is overridden).
	pool := NewAddressPool("ignored", "addr0,addr1,addr2")

	if pool.Len() != 3 {
		t.Fatalf("expected pool size 3, got %d", pool.Len())
	}

	counts := make(map[string]int)
	const calls = 9
	for range calls {
		counts[pool.Next()]++
	}

	if len(counts) != 3 {
		t.Errorf("expected all 3 addresses to be used, got %d", len(counts))
	}
	for addr, count := range counts {
		if count != calls/3 {
			t.Errorf("address %s called %d times, expected %d", addr, count, calls/3)
		}
	}
}

func TestAddressPool_FallsBackToSingleAddress(t *testing.T) {
	pool := NewAddressPool("solo:1234", "")

	if pool.Len() != 1 {
		t.Fatalf("expected pool size 1, got %d", pool.Len())
	}
	for range 5 {
		if got := pool.Next(); got != "solo:1234" {
			t.Errorf("expected solo:1234, got %q", got)
		}
	}
}

func TestAddressPool_IgnoresBlankEntries(t *testing.T) {
	// Extras ",  ,addr2,  " has 1 non-blank entry; primary is overridden.
	pool := NewAddressPool("primary", ",  ,addr2,  ")

	if pool.Len() != 1 {
		t.Fatalf("expected 1 address (blank entries dropped), got %d", pool.Len())
	}
	if got := pool.Next(); got != "addr2" {
		t.Errorf("expected addr2, got %q", got)
	}
}

func TestAddressPool_ConcurrentNext(t *testing.T) {
	// When extras is "b,c", primary "a" is overridden.
	pool := NewAddressPool("a", "b,c")

	var wg conc.WaitGroup
	const goroutines = 50
	results := make([]string, goroutines)
	for i := range goroutines {
		wg.Go(func() {
			results[i] = pool.Next()
		})
	}
	wg.Wait()

	for i, r := range results {
		if r != "a" && r != "b" && r != "c" {
			t.Errorf("goroutine %d got unexpected address %q", i, r)
		}
	}
}

func TestOrchestrator_DispatchesAcrossAddressPool(t *testing.T) {
	const concurrency = 3

	var mu sync.Mutex
	claimed := 0

	ms := &mockOrchestratorStore{
		claimBuildingFn: func(_ context.Context, _ string) (*domain.CodeDeployment, error) {
			mu.Lock()
			defer mu.Unlock()
			if claimed >= concurrency {
				return nil, nil
			}
			claimed++
			return &domain.CodeDeployment{
				ID:        "deploy-pool-" + string(rune('0'+claimed)),
				JobID:     "job",
				ProjectID: "proj",
				Status:    domain.DeploymentStatusBuilding,
			}, nil
		},
		updateStatusFn: func(_ context.Context, _ string, _ domain.DeploymentBuildStatus, _ map[string]any) error {
			return nil
		},
	}

	// extras overrides primary: pool is [addr0, addr1, addr2] (3 addresses).
	pool := NewAddressPool("ignored", "addr0,addr1,addr2")
	o := NewOrchestrator(ms, nil, WithConcurrency(concurrency), WithAddressPool(pool))

	if o.addrPool == nil {
		t.Fatal("expected addrPool to be set")
	}
	if o.addrPool.Len() != 3 {
		t.Errorf("expected pool size 3, got %d", o.addrPool.Len())
	}

	// Verify Next() produces diverse addresses.
	seen := map[string]bool{}
	for range 9 {
		seen[o.addrPool.Next()] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected all 3 pool addresses to be used, got %d", len(seen))
	}
}
