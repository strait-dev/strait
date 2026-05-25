package grpc

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sourcegraph/conc"
)

// TestReserveWorkerForQueue_AtomicDecrement verifies that picking + slot
// decrement happens under a single critical section. With N concurrent
// reservers racing on a 1-slot worker, exactly one wins; the others see
// SlotsAvailable=0 and miss. The non-atomic Pick+Decrement form would let
// multiple reservers see SlotsAvailable=1 and all decrement, driving the
// counter negative (or to 0 with N tasks routed to a 1-slot worker).
func TestReserveWorkerForQueue_AtomicDecrement(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	const racers = 100
	r := NewConnectionRegistry()
	w := makeWorker("solo", "proj-a", "key", []string{"q"}, 1)
	if err := r.Register(w); err != nil {
		t.Fatalf("register: %v", err)
	}

	var wins atomic.Int32
	var wg sync.WaitGroup
	wg.Add(racers)
	start := make(chan struct{})
	for range racers {
		concWG.Go(func() {
			defer wg.Done()
			<-start
			id, sendCh, ok := r.ReserveWorkerForQueue("proj-a", "q", "")
			if ok {
				wins.Add(1)
				if id != "solo" {
					t.Errorf("unexpected workerID: %q", id)
				}
				if sendCh == nil {
					t.Error("sendCh nil for ok reservation")
				}
			}
		})
	}
	close(start)
	wg.Wait()

	if got := wins.Load(); got != 1 {
		t.Fatalf("expected exactly 1 winner with 1 slot and %d racers, got %d", racers, got)
	}

	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(snap))
	}
	if snap[0].SlotsAvailable != 0 {
		t.Fatalf("SlotsAvailable=%d, want 0 (must not go negative)", snap[0].SlotsAvailable)
	}
}

// TestReserveWorkerForQueue_NoneAvailable verifies the negative path: no
// matching worker → ok=false, no slot mutation.
func TestReserveWorkerForQueue_NoneAvailable(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()
	w := makeWorker("a", "proj-a", "key", []string{"other"}, 4)
	if err := r.Register(w); err != nil {
		t.Fatalf("register: %v", err)
	}

	id, sendCh, ok := r.ReserveWorkerForQueue("proj-a", "q-none", "")
	if ok {
		t.Fatalf("expected ok=false, got id=%q sendCh=%v", id, sendCh)
	}
	if id != "" || sendCh != nil {
		t.Fatalf("non-zero return on miss: id=%q sendCh=%v", id, sendCh)
	}

	// Different project: also a miss.
	if _, _, ok := r.ReserveWorkerForQueue("proj-other", "other", ""); ok {
		t.Fatal("expected ok=false for cross-project pick")
	}
}

// TestReserveWorkerForQueue_PicksLeastLoaded verifies that the reserver
// picks the worker with the most available slots (least loaded). With two
// workers offering 2 and 4 slots respectively, the 4-slot worker is picked.
func TestReserveWorkerForQueue_PicksLeastLoaded(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()
	if err := r.Register(makeWorker("loaded", "proj-a", "key1", []string{"q"}, 2)); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Register(makeWorker("idle", "proj-a", "key2", []string{"q"}, 4)); err != nil {
		t.Fatalf("register: %v", err)
	}

	id, _, ok := r.ReserveWorkerForQueue("proj-a", "q", "")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if id != "idle" {
		t.Fatalf("expected idle worker, got %q", id)
	}
}

// TestReserveWorkerForQueue_DrainingExcluded verifies that draining workers
// are not picked, even when they have slots available.
func TestReserveWorkerForQueue_DrainingExcluded(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()
	if err := r.Register(makeWorker("draining", "proj-a", "key", []string{"q"}, 4)); err != nil {
		t.Fatalf("register: %v", err)
	}
	r.MarkDraining("draining")

	if _, _, ok := r.ReserveWorkerForQueue("proj-a", "q", ""); ok {
		t.Fatal("expected draining worker to be excluded from reservations")
	}
}

func TestReserveWorkerForQueue_EnvironmentScopedWorkerOnlyMatchesSameEnvironment(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	prod := makeWorker("prod", "proj-a", "key-prod", []string{"q"}, 2)
	prod.EnvironmentID = "env-prod"
	staging := makeWorker("staging", "proj-a", "key-staging", []string{"q"}, 2)
	staging.EnvironmentID = "env-staging"
	projectWide := makeWorker("wide", "proj-a", "key-wide", []string{"q"}, 2)
	if err := r.Register(prod); err != nil {
		t.Fatalf("register prod: %v", err)
	}
	if err := r.Register(staging); err != nil {
		t.Fatalf("register staging: %v", err)
	}
	if err := r.Register(projectWide); err != nil {
		t.Fatalf("register wide: %v", err)
	}

	id, _, ok := r.ReserveWorkerForQueue("proj-a", "q", "env-prod")
	if !ok {
		t.Fatal("expected env-prod reservation")
	}
	if id == "staging" {
		t.Fatal("env-staging worker must not receive env-prod job")
	}

	id, _, ok = r.ReserveWorkerForQueue("proj-a", "q", "env-dev")
	if !ok {
		t.Fatal("expected project-wide worker to cover unmatched environment")
	}
	if id != "wide" {
		t.Fatalf("expected project-wide worker for unmatched environment, got %q", id)
	}
}
