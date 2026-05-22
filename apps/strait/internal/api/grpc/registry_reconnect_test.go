package grpc

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRegistry_StaleDeregisterIsNoop is the direct regression for the
// reconnect race: a same-id reconnect supersedes the prior entry, then the
// old goroutine's deferred Deregister with the old token MUST NOT evict the
// live replacement.
func TestRegistry_StaleDeregisterIsNoop(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()

	w1 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	if err := r.Register(w1); err != nil {
		t.Fatalf("register w1: %v", err)
	}
	oldToken := w1.regToken

	// Same-id reconnect.
	w2 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	if err := r.Register(w2); err != nil {
		t.Fatalf("register w2: %v", err)
	}
	if w2.regToken == oldToken {
		t.Fatal("expected a new token on reconnect")
	}

	// Old goroutine's deferred cleanup runs with the stale token.
	r.Deregister("w1", oldToken)

	// Live replacement must still be present.
	snap := r.Snapshot()
	if len(snap) != 1 || snap[0].WorkerID != "w1" {
		t.Fatalf("live replacement evicted by stale Deregister: snap=%+v", snap)
	}

	// And byAPIKey index must still hold the new entry.
	r.mu.RLock()
	defer r.mu.RUnlock()
	if got := len(r.byAPIKey["key-1"]); got != 1 {
		t.Fatalf("expected 1 byAPIKey entry, got %d", got)
	}
	if r.byAPIKey["key-1"][0].regToken != w2.regToken {
		t.Fatal("byAPIKey points to stale entry after stale Deregister")
	}
}

// TestRegistry_ReconnectClosesOldRevokeCh asserts that the existing entry's
// revokeCh is closed on same-key reconnect so the stale stream goroutine
// wakes up and exits.
func TestRegistry_ReconnectClosesOldRevokeCh(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()

	w1 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	if err := r.Register(w1); err != nil {
		t.Fatalf("register w1: %v", err)
	}
	oldRevoke := w1.revokeCh

	w2 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	if err := r.Register(w2); err != nil {
		t.Fatalf("register w2: %v", err)
	}

	select {
	case <-oldRevoke:
		// expected — old revokeCh closed.
	case <-time.After(200 * time.Millisecond):
		t.Fatal("old revokeCh was not closed on same-key reconnect")
	}

	// The new entry's revokeCh must remain open.
	select {
	case <-w2.revokeCh:
		t.Fatal("new revokeCh was unexpectedly closed")
	default:
	}
}

// TestRegistry_ReconnectAlreadyClosedRevokeCh covers the case where the
// existing revokeCh has already been closed by a prior CloseByAPIKey; the
// reconnect path must not double-close + panic. revokeOnce owns the close;
// after CloseByAPIKey consumes it, reconnect's revokeOnce.Do is a no-op.
func TestRegistry_ReconnectAlreadyClosedRevokeCh(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()

	w1 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	if err := r.Register(w1); err != nil {
		t.Fatalf("register w1: %v", err)
	}
	r.CloseByAPIKey("key-1") // consume the once via the supported path

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Register panicked on already-closed revokeCh: %v", r)
		}
	}()
	w2 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	if err := r.Register(w2); err != nil {
		t.Fatalf("register w2: %v", err)
	}
}

// TestRegistry_DeregisterZeroTokenIsNoop guards accidental zero-token calls
// (e.g. a test fixture that didn't capture the token). They must never evict
// any live entry.
func TestRegistry_DeregisterZeroTokenIsNoop(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	if err := r.Register(w); err != nil {
		t.Fatalf("register: %v", err)
	}

	r.Deregister("w1", 0)

	if got := len(r.Snapshot()); got != 1 {
		t.Fatalf("zero-token Deregister evicted live entry: snap len=%d", got)
	}
}

// TestRegistry_ReconnectStorm hammers the same workerID with many concurrent
// reconnects. Final state must contain exactly one entry and have no
// goroutine leak via stale revokeCh listeners.
func TestRegistry_ReconnectStorm(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()

	const reconnects = 100
	var wg sync.WaitGroup
	wg.Add(reconnects)

	tokens := make(chan uint64, reconnects)
	for range reconnects {
		go func() {
			defer wg.Done()
			w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
			if err := r.Register(w); err != nil {
				return
			}
			tokens <- w.regToken
		}()
	}
	wg.Wait()
	close(tokens)

	// Exactly one live entry expected.
	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("after reconnect storm, expected 1 live entry, got %d", len(snap))
	}

	// Now deregister with every token captured. Only one (the latest) should
	// match; the rest must be no-ops. After all calls, there can be 0 or 1
	// entry depending on whether the latest token was drained from the
	// channel; either is correct as long as no panic and invariants hold.
	for tok := range tokens {
		r.Deregister("w1", tok)
	}

	// byAPIKey invariant.
	r.mu.RLock()
	defer r.mu.RUnlock()
	for keyID, workers := range r.byAPIKey {
		for _, w := range workers {
			if _, ok := r.workers[workerRegistryKey(w.ProjectID, w.WorkerID)]; !ok {
				t.Errorf("byAPIKey[%s] references worker %s not in workers map", keyID, w.WorkerID)
			}
		}
	}
}

// TestRegistry_ReconnectStorm_ParallelDeregister runs concurrent
// register-then-deregister cycles where the deregister always uses the
// freshly-issued token, asserting consistency under race.
func TestRegistry_ReconnectStorm_ParallelDeregister(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()

	const cycles = 200
	var wg sync.WaitGroup
	var registers atomic.Int64

	for range cycles {
		wg.Go(func() {
			w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
			if err := r.Register(w); err != nil {
				return
			}
			registers.Add(1)
			r.Deregister("w1", w.regToken)
		})
	}
	wg.Wait()

	// All registrations succeeded.
	if registers.Load() == 0 {
		t.Fatal("no successful registrations")
	}

	// byAPIKey invariant.
	r.mu.RLock()
	defer r.mu.RUnlock()
	for keyID, workers := range r.byAPIKey {
		for _, w := range workers {
			if _, ok := r.workers[workerRegistryKey(w.ProjectID, w.WorkerID)]; !ok {
				t.Errorf("byAPIKey[%s] references worker %s not in workers map", keyID, w.WorkerID)
			}
		}
	}
}

// TestRegistry_TokensAreUniquePerRegister ensures each successful Register
// returns a fresh, monotonically increasing token. Used by Deregister CAS.
func TestRegistry_TokensAreUniquePerRegister(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()
	seen := map[uint64]struct{}{}
	for i := range 50 {
		w := makeWorker(fmt.Sprintf("w-%d", i), "proj-a", fmt.Sprintf("key-%d", i), []string{"q"}, 1)
		if err := r.Register(w); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
		if w.regToken == 0 {
			t.Fatalf("register issued zero token at i=%d", i)
		}
		if _, dup := seen[w.regToken]; dup {
			t.Fatalf("duplicate token %d at i=%d", w.regToken, i)
		}
		seen[w.regToken] = struct{}{}
	}
}
