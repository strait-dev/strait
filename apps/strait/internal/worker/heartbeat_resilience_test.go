//go:build integration

package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
)

// failableHeartbeatStore implements HeartbeatStore with a configurable batch function.
type failableHeartbeatStore struct {
	batchFn func(ctx context.Context, ids []string) error
}

func (m *failableHeartbeatStore) UpdateHeartbeat(context.Context, string) error { return nil }

func (m *failableHeartbeatStore) BatchUpdateHeartbeat(ctx context.Context, ids []string) error {
	if m.batchFn != nil {
		return m.batchFn(ctx, ids)
	}
	return nil
}

func TestHeartbeat_FlushTimeout_DoesNotBlockTicker(t *testing.T) {
	store := &failableHeartbeatStore{
		batchFn: func(ctx context.Context, _ []string) error {
			// Sleep for 10s, but respect context cancellation.
			select {
			case <-time.After(10 * time.Second):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}

	hm := NewHeartbeatManager(store, 30*time.Second)
	hm.Register("run-1")
	hm.Register("run-2")
	hm.Register("run-3")

	// Parent context with 100ms timeout — shorter than the 5s internal flush timeout.
	parentCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	hm.flush(parentCtx)
	elapsed := time.Since(start)

	// flush creates its own 5s timeout derived from parentCtx.
	// Since parentCtx expires in 100ms, the effective deadline is ~100ms.
	// Must complete well under 6s (the assignment threshold).
	if elapsed >= 6*time.Second {
		t.Fatalf("flush blocked for %v; expected < 6s", elapsed)
	}

	// Verify the next flush still works (ticker not permanently stuck).
	store.batchFn = func(context.Context, []string) error { return nil }

	freshCtx, freshCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer freshCancel()

	start = time.Now()
	hm.flush(freshCtx)
	elapsed = time.Since(start)

	if elapsed >= 2*time.Second {
		t.Fatalf("second flush blocked for %v; expected near-instant", elapsed)
	}
}

func TestHeartbeat_ConsecutiveFailures_ResetsOnSuccess(t *testing.T) {
	callCount := 0
	store := &failableHeartbeatStore{
		batchFn: func(context.Context, []string) error {
			callCount++
			if callCount <= 3 {
				return fmt.Errorf("simulated failure #%d", callCount)
			}
			return nil
		},
	}

	hm := NewHeartbeatManager(store, 30*time.Second)
	hm.Register("run-1")

	ctx := context.Background()

	// 3 failing flushes.
	for i := 1; i <= 3; i++ {
		hm.flush(ctx)
		if hm.consecutiveFailures != i {
			t.Fatalf("after failure %d: consecutiveFailures = %d, want %d",
				i, hm.consecutiveFailures, i)
		}
	}

	// 1 successful flush resets the counter.
	hm.flush(ctx)
	if hm.consecutiveFailures != 0 {
		t.Fatalf("after success: consecutiveFailures = %d, want 0", hm.consecutiveFailures)
	}
}

func TestHeartbeat_FlushWithEmptyActiveSet(t *testing.T) {
	called := false
	store := &failableHeartbeatStore{
		batchFn: func(context.Context, []string) error {
			called = true
			return nil
		},
	}

	hm := NewHeartbeatManager(store, 30*time.Second)

	hm.flush(context.Background())

	if called {
		t.Fatal("BatchUpdateHeartbeat called with no active runs; expected no-op")
	}
}

func TestHeartbeat_CollectActiveIDs_ConcurrentSafety(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	store := &failableHeartbeatStore{}
	hm := NewHeartbeatManager(store, 30*time.Second)

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(goroutines + 1)

	// 10 goroutines registering and deregistering.
	for g := range goroutines {
		{
			id := g
			concWG.Go(func() {
				defer wg.Done()
				for i := range ops {
					runID := fmt.Sprintf("run-%d-%d", id, i)
					hm.Register(runID)
					hm.Deregister(runID)
				}
			})
		}
	}
	concWG.

		// 1 goroutine continuously calling collectActiveIDs.
		Go(func() {
			defer wg.Done()
			for range ops * goroutines {
				_ = hm.collectActiveIDs()
			}
		})

	// If we reach here without panic, the test passes.
	wg.Wait()
}
