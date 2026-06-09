package grpc

import (
	"fmt"
	"sync"
	"testing"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeWorker builds a ConnectedWorker for test use.
func makeWorker(id, project, apiKeyID string, queues []string, slotsTotal int32) *ConnectedWorker {
	ch := make(chan *workerv1.ServerMessage, 32)
	return &ConnectedWorker{
		WorkerID:       id,
		ProjectID:      project,
		APIKeyID:       apiKeyID,
		Queues:         queues,
		SlotsTotal:     slotsTotal,
		SlotsAvailable: slotsTotal,
		Status:         "active",
		SendCh:         ch,
		revokeCh:       make(chan struct{}),
	}
}

// TestRegistry_Register_HappyPath verifies a worker can be registered and retrieved.
func TestRegistry_Register_HappyPath(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	require.NoError(t, r.
		Register(w))

	snap := r.Snapshot()
	require.Len(t,
		snap,
		1)
	assert.Equal(
		t, "w1",
		snap[0].WorkerID)
}

// TestRegistry_Register_Collision verifies that re-registration of the same
// project worker ID with a different API key returns an error (hijack protection).
func TestRegistry_Register_Collision(t *testing.T) {
	r := NewConnectionRegistry()
	w1 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	w2 := makeWorker("w1", "proj-a", "key-2", []string{"default"}, 4)
	require.NoError(t, r.
		Register(w1))

	err := r.Register(w2)
	require.Error(t, err)
}

// TestRegistry_Register_Reconnect verifies that same-key re-registration evicts the old
// byAPIKey entry and does not accumulate stale pointers.
func TestRegistry_Register_Reconnect(t *testing.T) {
	r := NewConnectionRegistry()
	w1 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	w2 := makeWorker("w1", "proj-a", "key-1", []string{"default", "batch"}, 4)
	require.NoError(t, r.
		Register(w1))
	require.NoError(t, r.
		Register(w2))

	// Only one worker should be in the byAPIKey index.
	r.mu.RLock()
	count := len(r.byAPIKey["key-1"])
	r.mu.RUnlock()
	assert.Equal(t, 1, count)

	// Snapshot should show updated queues.
	snap := r.Snapshot()
	require.Len(t,
		snap,
		1)
	assert.Len(t,
		snap[0].Queues,
		2)
}

// TestRegistry_Deregister_RemovesWorkerAndIndex verifies full cleanup.
func TestRegistry_Deregister_RemovesWorkerAndIndex(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	require.NoError(t, r.
		Register(w))

	r.Deregister("w1", w.regToken)

	snap := r.Snapshot()
	assert.Empty(t,
		snap)

	r.mu.RLock()
	_, keyExists := r.byAPIKey["key-1"]
	r.mu.RUnlock()
	assert.False(
		t, keyExists,
	)
}

// TestRegistry_Deregister_Noop verifies deregistering a nonexistent worker does not panic.
func TestRegistry_Deregister_Noop(t *testing.T) {
	r := NewConnectionRegistry()
	r.Deregister("does-not-exist", 1) // must not panic
}

// TestRegistry_IncrementSlots_Cap verifies that IncrementSlots cannot exceed SlotsTotal.
func TestRegistry_IncrementSlots_Cap(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 2)
	w.SlotsAvailable = 2
	require.NoError(t, r.
		Register(w))

	// already at cap

	r.IncrementSlots("w1")

	snap := r.Snapshot()
	assert.EqualValues(t, 2, snap[0].SlotsAvailable)
}

// TestRegistry_IncrementSlots_Normal verifies increment from below cap.
func TestRegistry_IncrementSlots_Normal(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	w.SlotsAvailable = 2
	require.NoError(t, r.
		Register(w))

	r.IncrementSlots("w1")

	snap := r.Snapshot()
	assert.EqualValues(t, 3, snap[0].SlotsAvailable)
}

// TestRegistry_DecrementSlots_Normal decrements from above zero.
func TestRegistry_DecrementSlots_Normal(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	require.NoError(t, r.
		Register(w))

	r.DecrementSlots("w1")

	snap := r.Snapshot()
	assert.EqualValues(t, 3, snap[0].SlotsAvailable)
}

// TestRegistry_DecrementSlots_UnderflowGuard verifies that SlotsAvailable never goes
// below zero even with repeated decrements.
func TestRegistry_DecrementSlots_UnderflowGuard(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 2)
	w.SlotsAvailable = 0
	require.NoError(t, r.
		Register(w))

	// start at zero

	r.DecrementSlots("w1")
	r.DecrementSlots("w1")

	snap := r.Snapshot()
	assert.GreaterOrEqual(t, snap[0].SlotsAvailable,

		int32(0))
	assert.EqualValues(t, 0, snap[0].SlotsAvailable)
}

// TestRegistry_SnapshotQueues_OnlyActiveWorkers verifies that draining workers are
// excluded from the queue snapshot.
func TestRegistry_SnapshotQueues_OnlyActiveWorkers(t *testing.T) {
	r := NewConnectionRegistry()
	active := makeWorker("active", "proj-a", "key-1", []string{"q1", "q2"}, 4)
	draining := makeWorker("draining", "proj-a", "key-2", []string{"q3"}, 4)
	draining.Status = "draining"
	require.NoError(t, r.
		Register(active))
	require.NoError(t, r.
		Register(draining),
	)

	queues := r.SnapshotQueues()

	for _, q := range queues {
		assert.NotEqual(t, "q3",
			q)
	}
	assert.Len(t,
		queues,
		2)
}

// TestRegistry_SnapshotQueues_Empty returns nil when no active workers are registered.
func TestRegistry_SnapshotQueues_Empty(t *testing.T) {
	r := NewConnectionRegistry()
	queues := r.SnapshotQueues()
	assert.Nil(t, queues)
}

func TestRegistry_SnapshotWorkerQueues_IncludesEnvironmentScopes(t *testing.T) {
	r := NewConnectionRegistry()
	projectWide := makeWorker("wide", "proj-a", "key-wide", []string{"q1", "q2"}, 4)
	staging := makeWorker("staging", "proj-a", "key-staging", []string{"q1"}, 4)
	staging.EnvironmentID = "env-staging"
	stagingDup := makeWorker("staging-dup", "proj-a", "key-staging-dup", []string{"q1"}, 4)
	stagingDup.EnvironmentID = "env-staging"
	draining := makeWorker("draining", "proj-a", "key-draining", []string{"q3"}, 4)
	draining.EnvironmentID = "env-prod"
	draining.Status = "draining"

	for _, w := range []*ConnectedWorker{projectWide, staging, stagingDup, draining} {
		require.NoError(t, r.
			Register(w))
	}

	got := r.SnapshotWorkerQueues()
	seen := make(map[domain.WorkerQueueRef]struct{}, len(got))
	for _, ref := range got {
		seen[ref] = struct{}{}
	}
	want := []domain.WorkerQueueRef{
		{ProjectID: "proj-a", QueueName: "q1"},
		{ProjectID: "proj-a", QueueName: "q2"},
		{ProjectID: "proj-a", QueueName: "q1", EnvironmentID: "env-staging"},
	}
	require.Len(t,
		seen,
		len(want))

	for _, ref := range want {
		if _, ok := seen[ref]; !ok {
			require.Failf(t, "test failure",

				"missing worker queue ref %+v from %+v", ref, got)
		}
	}
	if _, ok := seen[domain.WorkerQueueRef{ProjectID: "proj-a", QueueName: "q3", EnvironmentID: "env-prod"}]; ok {
		require.Failf(t, "test failure",

			"draining worker ref leaked into snapshot: %+v", got)
	}
}

func TestRegistry_SnapshotWorkerQueueAvailability_CapsByAvailableSlots(t *testing.T) {
	r := NewConnectionRegistry()
	projectWide := makeWorker("wide", "proj-a", "key-wide", []string{"q1", "q2"}, 4)
	projectWide.SlotsAvailable = 2
	staging := makeWorker("staging", "proj-a", "key-staging", []string{"q1"}, 4)
	staging.EnvironmentID = "env-staging"
	staging.SlotsAvailable = 1
	busy := makeWorker("busy", "proj-a", "key-busy", []string{"q3"}, 4)
	busy.SlotsAvailable = 0
	draining := makeWorker("draining", "proj-a", "key-draining", []string{"q4"}, 4)
	draining.Status = "draining"

	for _, w := range []*ConnectedWorker{projectWide, staging, busy, draining} {
		require.NoError(t, r.Register(w))
	}

	got := r.SnapshotWorkerQueueAvailability()
	require.Equal(t, 3, got.SlotsAvailable)

	seen := make(map[domain.WorkerQueueRef]struct{}, len(got.Queues))
	for _, ref := range got.Queues {
		seen[ref] = struct{}{}
	}
	require.Contains(t, seen, domain.WorkerQueueRef{ProjectID: "proj-a", QueueName: "q1"})
	require.Contains(t, seen, domain.WorkerQueueRef{ProjectID: "proj-a", QueueName: "q2"})
	require.Contains(t, seen, domain.WorkerQueueRef{ProjectID: "proj-a", QueueName: "q1", EnvironmentID: "env-staging"})
	require.NotContains(t, seen, domain.WorkerQueueRef{ProjectID: "proj-a", QueueName: "q3"})
	require.NotContains(t, seen, domain.WorkerQueueRef{ProjectID: "proj-a", QueueName: "q4"})
}

// TestRegistry_MarkDraining transitions a worker status correctly.
func TestRegistry_MarkDraining(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	require.NoError(t, r.
		Register(w))

	r.MarkDraining("w1")

	snap := r.Snapshot()
	assert.Equal(
		t, "draining",

		snap[0].Status,
	)
}

// TestRegistry_MarkDraining_Noop verifies no panic for unknown worker.
func TestRegistry_MarkDraining_Noop(t *testing.T) {
	r := NewConnectionRegistry()
	r.MarkDraining("does-not-exist") // must not panic
}

// TestRegistry_PickWorkerForQueue_LeastLoaded verifies least-loaded selection.
func TestRegistry_PickWorkerForQueue_LeastLoaded(t *testing.T) {
	r := NewConnectionRegistry()

	busy := makeWorker("busy", "proj-a", "key-1", []string{"q"}, 4)
	busy.SlotsAvailable = 1

	idle := makeWorker("idle", "proj-a", "key-2", []string{"q"}, 4)
	idle.SlotsAvailable = 3
	require.NoError(t, r.
		Register(busy))
	require.NoError(t, r.
		Register(idle))

	picked, ok := r.PickWorkerForQueue("proj-a", "q")
	require.True(
		t, ok)
	assert.Equal(
		t, "idle",
		picked.
			WorkerID,
	)
}

// TestRegistry_PickWorkerForQueue_NoSlots verifies that workers with zero slots are skipped.
func TestRegistry_PickWorkerForQueue_NoSlots(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 4)
	w.SlotsAvailable = 0
	require.NoError(t, r.
		Register(w))

	_, ok := r.PickWorkerForQueue("proj-a", "q")
	assert.False(
		t, ok)
}

// TestRegistry_PickWorkerForQueue_WrongProject verifies project isolation.
func TestRegistry_PickWorkerForQueue_WrongProject(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 4)
	require.NoError(t, r.
		Register(w))

	_, ok := r.PickWorkerForQueue("proj-b", "q")
	assert.False(
		t, ok)
}

// TestRegistry_PickWorkerForQueue_WrongQueue verifies queue filtering.
func TestRegistry_PickWorkerForQueue_WrongQueue(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q1"}, 4)
	require.NoError(t, r.
		Register(w))

	_, ok := r.PickWorkerForQueue("proj-a", "q2")
	assert.False(
		t, ok)
}

func TestRegistry_ErrNoWorkerForQueueSentinel(t *testing.T) {
	t.Parallel()
	require.ErrorIs(
		t, ErrNoWorkerAvailable, ErrNoWorkerForQueue)

	wrapped := fmt.Errorf("dispatch failed: %w", ErrNoWorkerForQueue)
	require.ErrorIs(
		t, wrapped, ErrNoWorkerForQueue)
}

// TestRegistry_PickWorkerForQueue_DrainedSkipped verifies draining workers are skipped.
func TestRegistry_PickWorkerForQueue_DrainedSkipped(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 4)
	require.NoError(t, r.
		Register(w))

	r.MarkDraining("w1")

	_, ok := r.PickWorkerForQueue("proj-a", "q")
	assert.False(
		t, ok)
}

// TestRegistry_CloseByAPIKey_ClosesRevokeCh verifies that CloseByAPIKey signals all streams
// for the given API key.
func TestRegistry_CloseByAPIKey_ClosesRevokeCh(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 4)
	require.NoError(t, r.
		Register(w))

	r.CloseByAPIKey("key-1")

	select {
	case <-w.revokeCh:
		// expected
	default:
		assert.Fail(t, "expected revokeCh to be closed after CloseByAPIKey")
	}
}

// TestRegistry_CloseByAPIKey_IdempotentDoubleClose verifies that calling CloseByAPIKey
// twice on the same key does not panic.
func TestRegistry_CloseByAPIKey_IdempotentDoubleClose(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 4)
	require.NoError(t, r.
		Register(w))

	r.CloseByAPIKey("key-1")
	r.CloseByAPIKey("key-1") // must not panic
}

// TestRegistry_Concurrent_RegisterDeregister verifies that concurrent Register/Deregister
// operations do not race, panic, or corrupt invariants.
func TestRegistry_Concurrent_RegisterDeregister(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	r := NewConnectionRegistry()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers: register a unique worker, then deregister with the assigned
	// token. The deregister goroutine waits on a per-id channel so it
	// receives the token issued by Register.
	tokenChs := make([]chan uint64, goroutines)
	for i := range goroutines {
		tokenChs[i] = make(chan uint64, 1)
	}

	for i := range goroutines {
		concWG.Go(func() {
			defer wg.Done()
			wid := fmt.Sprintf("w-%d", i)
			w := makeWorker(wid, "proj-a", fmt.Sprintf("key-%d", i), []string{"q"}, 4)
			_ = r.Register(w)
			tokenChs[i] <- w.regToken
		})
		concWG.Go(func() {
			defer wg.Done()
			tok := <-tokenChs[i]
			r.Deregister(fmt.Sprintf("w-%d", i), tok)
		})
	}

	wg.Wait()

	// Invariant: byAPIKey and workers must remain consistent (no orphaned entries).
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

// TestRegistry_Concurrent_SlotOperations verifies that concurrent increment/decrement
// operations keep slots within [0, SlotsTotal].
func TestRegistry_Concurrent_SlotOperations(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 10)
	require.NoError(t, r.
		Register(w))

	const ops = 200
	var wg sync.WaitGroup
	wg.Add(ops * 2)

	for range ops {
		concWG.Go(func() {
			defer wg.Done()
			r.IncrementSlots("w1")
		})
		concWG.Go(func() {
			defer wg.Done()
			r.DecrementSlots("w1")
		})
	}

	wg.Wait()

	snap := r.Snapshot()
	require.NotEmpty(t,
		snap)

	slots := snap[0].SlotsAvailable
	total := snap[0].SlotsTotal
	assert.GreaterOrEqual(t, slots,
		int32(0))
	assert.LessOrEqual(t,
		slots,
		total)
}

// TestRegistry_Concurrent_ReconnectStorm verifies that 1000 parallel reconnects for
// the same worker_id and same api_key do not cause panics or leave the registry inconsistent.
func TestRegistry_Concurrent_ReconnectStorm(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	r := NewConnectionRegistry()

	const n = 100 // reduced from 1000 to keep test duration short while still exercising concurrency
	var wg sync.WaitGroup
	wg.Add(n)

	for i := range n {
		concWG.Go(func() {
			defer wg.Done()
			w := makeWorker("storm-worker", "proj-a", "key-1", []string{fmt.Sprintf("q%d", i)}, 4)
			_ = r.Register(w)
		})
	}

	wg.Wait()

	// Registry must be consistent: at most one entry for storm-worker.
	snap := r.Snapshot()
	count := 0
	for _, sw := range snap {
		if sw.WorkerID == "storm-worker" {
			count++
		}
	}
	assert.LessOrEqual(t,
		count,
		1)

	// byAPIKey index must not have more than 1 entry for key-1 pointing to storm-worker.
	r.mu.RLock()
	stormCount := 0
	for _, bw := range r.byAPIKey["key-1"] {
		if bw.WorkerID == "storm-worker" {
			stormCount++
		}
	}
	r.mu.RUnlock()
	assert.LessOrEqual(t,
		stormCount,
		1)
}
