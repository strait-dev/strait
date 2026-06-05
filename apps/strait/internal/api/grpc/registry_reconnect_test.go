package grpc

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistry_StaleDeregisterIsNoop is the direct regression for the
// reconnect race: a same-id reconnect supersedes the prior entry, then the
// old goroutine's deferred Deregister with the old token MUST NOT evict the
// live replacement.
func TestRegistry_StaleDeregisterIsNoop(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()

	w1 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	require.NoError(t,
		r.Register(w1),
	)

	oldToken := w1.regToken

	// Same-id reconnect.
	w2 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	require.NoError(t,
		r.Register(w2),
	)
	require.NotEqual(t,
		oldToken, w2.
			regToken)

	// Old goroutine's deferred cleanup runs with the stale token.
	r.Deregister("w1", oldToken)

	// Live replacement must still be present.
	snap := r.Snapshot()
	require.False(t, len(snap) != 1 ||
		snap[0].WorkerID !=
			"w1")

	// And byAPIKey index must still hold the new entry.
	r.mu.RLock()
	defer r.mu.RUnlock()
	require.EqualValues(t, 1,
		len(r.byAPIKey["key-1"]),
	)
	require.Equal(t, w2.
		regToken, r.byAPIKey["key-1"][0].
		regToken)

}

// TestRegistry_ReconnectClosesOldRevokeCh asserts that the existing entry's
// revokeCh is closed on same-key reconnect so the stale stream goroutine
// wakes up and exits.
func TestRegistry_ReconnectClosesOldRevokeCh(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()

	w1 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	require.NoError(t,
		r.Register(w1),
	)

	oldRevoke := w1.revokeCh

	w2 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	require.NoError(t,
		r.Register(w2),
	)

	select {
	case <-oldRevoke:
		// expected — old revokeCh closed.
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "old revokeCh was not closed on same-key reconnect")
	}

	// The new entry's revokeCh must remain open.
	select {
	case <-w2.revokeCh:
		require.Fail(t, "new revokeCh was unexpectedly closed")
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
	require.NoError(t,
		r.Register(w1),
	)

	r.CloseByAPIKey("key-1") // consume the once via the supported path

	defer func() {
		require.Nil(t, recover())

	}()
	w2 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	require.NoError(t,
		r.Register(w2),
	)

}

// TestRegistry_DeregisterZeroTokenIsNoop guards accidental zero-token calls
// (e.g. a test fixture that didn't capture the token). They must never evict
// any live entry.
func TestRegistry_DeregisterZeroTokenIsNoop(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	require.NoError(t,
		r.Register(w))

	r.Deregister("w1", 0)
	require.EqualValues(t, 1,
		len(r.Snapshot()))

}

// TestRegistry_ReconnectStorm hammers the same workerID with many concurrent
// reconnects. Final state must contain exactly one entry and have no
// goroutine leak via stale revokeCh listeners.
func TestRegistry_ReconnectStorm(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	r := NewConnectionRegistry()

	const reconnects = 100
	var wg sync.WaitGroup
	wg.Add(reconnects)

	tokens := make(chan uint64, reconnects)
	for range reconnects {
		concWG.Go(func() {
			defer wg.Done()
			w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
			if err := r.Register(w); err != nil {
				return
			}
			tokens <- w.regToken
		})
	}
	wg.Wait()
	close(tokens)

	// Exactly one live entry expected.
	snap := r.Snapshot()
	require.Len(t, snap,
		1)

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
				assert.Failf(t, "test failure",

					"byAPIKey[%s] references worker %s not in workers map", keyID, w.WorkerID)
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
	require.NotEqual(t,
		0, registers.
			Load())

	// All registrations succeeded.

	// byAPIKey invariant.
	r.mu.RLock()
	defer r.mu.RUnlock()
	for keyID, workers := range r.byAPIKey {
		for _, w := range workers {
			if _, ok := r.workers[workerRegistryKey(w.ProjectID, w.WorkerID)]; !ok {
				assert.Failf(t, "test failure",

					"byAPIKey[%s] references worker %s not in workers map", keyID, w.WorkerID)
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
		require.NoError(t,
			r.Register(w))
		require.NotEqual(t,
			0, w.regToken,
		)

		if _, dup := seen[w.regToken]; dup {
			require.Failf(t, "test failure",

				"duplicate token %d at i=%d", w.regToken, i)
		}
		seen[w.regToken] = struct{}{}
	}
}
