package grpc

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, r.Register(w))

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
				assert.Equal(t, "solo", id)
				assert.NotNil(t, sendCh)

			}
		})
	}
	close(start)
	wg.Wait()
	require.EqualValues(t, 1, wins.Load())

	snap := r.Snapshot()
	require.Len(t, snap, 1)
	require.EqualValues(t, 0, snap[0].SlotsAvailable)

}

// TestReserveWorkerForQueue_NoneAvailable verifies the negative path: no
// matching worker → ok=false, no slot mutation.
func TestReserveWorkerForQueue_NoneAvailable(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()
	w := makeWorker("a", "proj-a", "key", []string{"other"}, 4)
	require.NoError(t, r.Register(w))

	id, sendCh, ok := r.ReserveWorkerForQueue("proj-a", "q-none", "")
	require.False(t, ok)
	require.False(t, id != "" ||
		sendCh !=

			nil)

	// Different project: also a miss.
	if _, _, ok := r.ReserveWorkerForQueue("proj-other", "other", ""); ok {
		require.Fail(t,

			"expected ok=false for cross-project pick")
	}
}

// TestReserveWorkerForQueue_PicksLeastLoaded verifies that the reserver
// picks the worker with the most available slots (least loaded). With two
// workers offering 2 and 4 slots respectively, the 4-slot worker is picked.
func TestReserveWorkerForQueue_PicksLeastLoaded(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()
	require.NoError(t, r.Register(makeWorker("loaded", "proj-a", "key1",
		[]string{"q"}, 2)))
	require.NoError(t, r.Register(makeWorker("idle", "proj-a", "key2", []string{"q"}, 4)))

	id, _, ok := r.ReserveWorkerForQueue("proj-a", "q", "")
	require.True(t, ok)
	require.Equal(t, "idle", id)

}

// TestReserveWorkerForQueue_DrainingExcluded verifies that draining workers
// are not picked, even when they have slots available.
func TestReserveWorkerForQueue_DrainingExcluded(t *testing.T) {
	t.Parallel()
	r := NewConnectionRegistry()
	require.NoError(t, r.Register(makeWorker("draining", "proj-a", "key",
		[]string{"q"}, 4)))

	r.MarkDraining("draining")

	if _, _, ok := r.ReserveWorkerForQueue("proj-a", "q", ""); ok {
		require.Fail(t,

			"expected draining worker to be excluded from reservations")
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
	require.NoError(t, r.Register(prod))
	require.NoError(t, r.Register(staging))
	require.NoError(t, r.Register(projectWide))

	id, _, ok := r.ReserveWorkerForQueue("proj-a", "q", "env-prod")
	require.True(t, ok)
	require.NotEqual(t, "staging",
		id)

	id, _, ok = r.ReserveWorkerForQueue("proj-a", "q", "env-dev")
	require.True(t, ok)
	require.Equal(t, "wide", id)

}
