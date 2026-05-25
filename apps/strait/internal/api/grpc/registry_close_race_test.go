package grpc

import (
	"github.com/sourcegraph/conc"
	"sync"
	"testing"
)

// TestCloseByAPIKey_NoDoubleClosePanic verifies the Phase I contract: the
// revokeCh close is guarded by sync.Once, so racing CloseByAPIKey calls (or
// CloseByAPIKey racing with a same-key reconnect) cannot panic with
// "close of closed channel".
//
// The previous select-default-close pattern was insufficient: between the
// select branch and the close, another goroutine could pass the same select
// and try to close again, panicking. With sync.Once, only the first closer
// runs the close.
func TestCloseByAPIKey_NoDoubleClosePanic(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	const racers = 50
	const workers = 5
	r := NewConnectionRegistry()

	for i := range workers {
		w := makeWorker(workerID(i), "proj-a", "shared-key", []string{"q"}, 4)
		if err := r.Register(w); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(racers)
	start := make(chan struct{})
	for range racers {
		concWG.Go(func() {
			defer wg.Done()
			<-start
			r.CloseByAPIKey("shared-key")
		})
	}
	close(start)
	wg.Wait()

	// All revokeCh must be closed.
	snap := r.Snapshot()
	if len(snap) != workers {
		t.Fatalf("workers count = %d, want %d", len(snap), workers)
	}
	for i := range workers {
		w := lookupWorker(t, r, workerID(i))
		select {
		case <-w.revokeCh:
			// closed, good
		default:
			t.Errorf("worker %s revokeCh not closed", w.WorkerID)
		}
	}
}

// TestRegister_SameKeyReconnect_RacesCloseByAPIKey verifies that interleaving
// a same-key reconnect with CloseByAPIKey does not panic. The reconnect path
// closes the existing entry's revokeCh; CloseByAPIKey closes the same
// channel. Both must go through the once.
func TestRegister_SameKeyReconnect_RacesCloseByAPIKey(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	const racers = 50
	r := NewConnectionRegistry()

	w0 := makeWorker("w-race", "proj-a", "key-race", []string{"q"}, 4)
	if err := r.Register(w0); err != nil {
		t.Fatalf("seed register: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2 * racers)
	start := make(chan struct{})

	// Half the racers reconnect with the same WorkerID/APIKeyID.
	for range racers {
		concWG.Go(func() {
			defer wg.Done()
			<-start
			rw := makeWorker("w-race", "proj-a", "key-race", []string{"q"}, 4)
			_ = r.Register(rw)
		})
	}
	// Other half close by api key.
	for range racers {
		concWG.Go(func() {
			defer wg.Done()
			<-start
			r.CloseByAPIKey("key-race")
		})
	}
	close(start)
	wg.Wait() // must not panic
}

func workerID(i int) string {
	return "w-" + string(rune('a'+i))
}

func lookupWorker(t *testing.T, r *ConnectionRegistry, id string) *ConnectedWorker {
	t.Helper()
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.workers[workerRegistryKey("proj-a", id)]
	if !ok {
		t.Fatalf("worker %s not found", id)
	}
	return w
}
