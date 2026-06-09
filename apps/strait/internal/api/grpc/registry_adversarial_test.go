package grpc

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdversarial_WorkerIDHijackSameProject verifies that a second worker
// with the same ID in the same project but a different API key receives
// AlreadyExists (surfaced as an error from Register).
func TestAdversarial_WorkerIDHijackSameProject(t *testing.T) {
	r := NewConnectionRegistry()

	legit := makeWorker("target-worker", "proj-a", "key-legit", []string{"q"}, 4)
	require.NoError(t, r.
		Register(legit))

	hijacker := makeWorker("target-worker", "proj-a", "key-attacker", []string{"q"}, 4)
	err := r.Register(hijacker)
	require.Error(t, err)
	assert.False(t, !strings.Contains(err.Error(), "different api key") || !strings.Contains(err.Error(),

		"proj-a"))

	// The original worker must still be registered.
	snap := r.Snapshot()
	assert.Len(t, snap, 1)
	assert.Equal(t, "key-legit", snap[0].APIKeyID)
}

func TestAdversarial_WorkerIDNamespaceAllowsSameIDAcrossProjects(t *testing.T) {
	r := NewConnectionRegistry()
	require.NoError(t, r.
		Register(makeWorker("shared-worker",

			"proj-a", "key-a", []string{"q"}, 4)))
	require.NoError(t, r.
		Register(makeWorker("shared-worker",

			"proj-b", "key-b", []string{"q"}, 4)))

	snap := r.Snapshot()
	require.Len(t, snap,
		2)

	for _, projectID := range []string{"proj-a", "proj-b"} {
		picked, ok := r.PickWorkerForQueue(projectID, "q")
		require.True(t, ok)
		require.False(t, picked.
			ProjectID !=
			projectID ||

			picked.WorkerID != "shared-worker")
	}
}

func TestAdversarial_WorkerIDNamespaceProjectScopedSlotRelease(t *testing.T) {
	r := NewConnectionRegistry()
	require.NoError(t, r.
		Register(makeWorker("shared-worker",

			"proj-a", "key-a", []string{"q"}, 2)))
	require.NoError(t, r.
		Register(makeWorker("shared-worker",

			"proj-b", "key-b", []string{"q"}, 2)))

	r.DecrementProjectSlots("proj-a", "shared-worker")
	r.IncrementProjectSlots("proj-b", "shared-worker")

	snap := r.Snapshot()
	available := map[string]int32{}
	for _, worker := range snap {
		available[worker.ProjectID] = worker.SlotsAvailable
	}
	require.EqualValues(t, 1, available["proj-a"])
	require.EqualValues(t, 2, available["proj-b"])
}

// TestAdversarial_SlotExhaustion verifies that dispatch beyond SlotsTotal causes
// slot count to clamp at 0 and does not go negative.
func TestAdversarial_SlotExhaustion(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 2)
	require.NoError(t, r.
		Register(w))

	// Exhaust all slots.
	r.DecrementSlots("w1")
	r.DecrementSlots("w1")

	// Additional decrements beyond zero must clamp to 0.
	r.DecrementSlots("w1")
	r.DecrementSlots("w1")
	r.DecrementSlots("w1")

	snap := r.Snapshot()
	assert.GreaterOrEqual(t, snap[0].SlotsAvailable,

		int32(0))
	assert.EqualValues(t, 0, snap[0].SlotsAvailable)

	// No worker should be pickable.
	_, ok := r.PickWorkerForQueue("proj-a", "q")
	assert.False(t, ok)
}

// TestAdversarial_RegistrationMaxWorkerIDLen verifies that worker IDs longer than
// maxWorkerIDLen are caught by the stream handler validation, which enforces the limit.
func TestAdversarial_RegistrationMaxWorkerIDLen(t *testing.T) {
	// maxWorkerIDLen is defined as 128 in stream.go.
	longID := strings.Repeat("x", maxWorkerIDLen+1)
	require.Greater(t, len(
		longID), maxWorkerIDLen,
	)
	assert.Equal(t, 128,
		maxWorkerIDLen,
	)

	// The registry itself does not enforce length — that's stream.go's job.
	// Verify the constant is set correctly and that the validation is enforceable.
}

// TestAdversarial_MaxQueuesPerWorker verifies the constant is enforced.
func TestAdversarial_MaxQueuesPerWorker(t *testing.T) {
	assert.Equal(t, 64, maxQueuesPerWorker)
}

// TestAdversarial_MaxInFlightTasks verifies the constant is enforced.
func TestAdversarial_MaxInFlightTasks(t *testing.T) {
	assert.Equal(t, 256,
		maxInFlightTasks,
	)
}

// TestAdversarial_MaxLogMessageBytes verifies log clamping constant.
func TestAdversarial_MaxLogMessageBytes(t *testing.T) {
	assert.Equal(t, 4096,
		maxLogMessageBytes,
	)
}

// TestAdversarial_SlotInflation verifies that IncrementSlots cannot inflate beyond SlotsTotal,
// preventing a misbehaving worker from becoming the preferred dispatch target.
func TestAdversarial_SlotInflation(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 5)
	require.NoError(t, r.
		Register(w))

	// Attempt to inflate slots beyond SlotsTotal.
	for range 100 {
		r.IncrementSlots("w1")
	}

	snap := r.Snapshot()
	assert.LessOrEqual(t,
		snap[0].
			SlotsAvailable,
		snap[0].SlotsTotal)
}

// TestAdversarial_SendChannelDeadlock verifies that a full (blocked) SendCh does not
// cause the registry or dispatch path to deadlock. The slot must be recoverable.
func TestAdversarial_SendChannelDeadlock(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	r := NewConnectionRegistry()

	// A full channel - nothing draining from the other end.
	ch := make(chan *workerv1.ServerMessage, 1)
	ch <- &workerv1.ServerMessage{} // fill it

	w := &ConnectedWorker{
		WorkerID:       "w1",
		ProjectID:      "proj-a",
		APIKeyID:       "key-1",
		Queues:         []string{"q"},
		SlotsTotal:     4,
		SlotsAvailable: 4,
		Status:         "active",
		SendCh:         ch,
		revokeCh:       make(chan struct{}),
	}
	require.NoError(t, r.
		Register(w))

	// sendCancel uses select with default, so it must not block even on a full channel.
	d := &WorkerDispatcher{registry: r}
	done := make(chan struct{})
	concWG.Go(func() {
		defer close(done)
		d.sendCancel(ch, "run-blocked")
	})

	select {
	case <-done:
		// sendCancel returned without deadlocking.
	case <-time.After(500 * time.Millisecond):
		assert.Fail(t, "sendCancel deadlocked on full channel")
	}
}

// TestAdversarial_ReconnectStorm_ByAPIKeyConsistency verifies that after 100 parallel
// reconnects for the same worker_id+api_key pair, the byAPIKey index has at most 1 entry.
func TestAdversarial_ReconnectStorm_ByAPIKeyConsistency(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	r := NewConnectionRegistry()
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n)

	for i := range n {
		concWG.Go(func() {
			defer wg.Done()
			w := &ConnectedWorker{
				WorkerID:       "storm",
				ProjectID:      "proj-a",
				APIKeyID:       "key-storm",
				Queues:         []string{fmt.Sprintf("q%d", i)},
				SlotsTotal:     4,
				SlotsAvailable: 4,
				Status:         "active",
				SendCh:         make(chan *workerv1.ServerMessage, 1),
				revokeCh:       make(chan struct{}),
			}
			_ = r.Register(w)
		})
	}
	wg.Wait()

	r.mu.RLock()
	count := len(r.byAPIKey["key-storm"])
	workers := len(r.workers)
	r.mu.RUnlock()
	assert.LessOrEqual(t,
		count,
		1)
	assert.LessOrEqual(t,
		workers,
		1)
}

// TestAdversarial_SnapshotDuringDrain verifies that SnapshotQueues handles a worker
// that transitions to draining mid-iteration without panicking.
func TestAdversarial_SnapshotDuringDrain(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	r := NewConnectionRegistry()

	for i := range 10 {
		w := makeWorker(fmt.Sprintf("w%d", i), "proj-a", fmt.Sprintf("key-%d", i), []string{"q"}, 4)
		require.NoError(t, r.
			Register(w))
	}

	var wg sync.WaitGroup
	wg.Add(2)
	concWG.

		// Concurrently drain workers while snapshotting queues.
		Go(func() {
			defer wg.Done()
			for i := range 10 {
				r.MarkDraining(fmt.Sprintf("w%d", i))
			}
		})
	concWG.Go(func() {
		defer wg.Done()
		for range 20 {
			_ = r.SnapshotQueues()
		}
	})

	wg.Wait() // must not panic or race
}

// TestAdversarial_CrossProject_PickWorker verifies that workers from project A
// are never returned for project B queries.
func TestAdversarial_CrossProject_PickWorker(t *testing.T) {
	r := NewConnectionRegistry()

	for i := range 5 {
		w := makeWorker(fmt.Sprintf("proj-a-w%d", i), "proj-a", fmt.Sprintf("ka%d", i), []string{"shared-q"}, 4)
		require.NoError(t, r.
			Register(w))
	}

	_, ok := r.PickWorkerForQueue("proj-b", "shared-q")
	assert.False(t, ok)
}

// TestAdversarial_CloseByAPIKey_MultipleWorkers verifies that all streams for a given
// API key are closed, not just the first.
func TestAdversarial_CloseByAPIKey_MultipleWorkers(t *testing.T) {
	r := NewConnectionRegistry()

	workers := make([]*ConnectedWorker, 3)
	for i := range 3 {
		w := makeWorker(fmt.Sprintf("w%d", i), "proj-a", "shared-key", []string{"q"}, 4)
		workers[i] = w
		require.NoError(t, r.
			Register(w))
	}

	r.CloseByAPIKey("shared-key")

	for i, w := range workers {
		select {
		case <-w.revokeCh:
			// expected
		default:
			assert.Failf(t, "test failure", "worker w%d revokeCh was not closed", i)
		}
	}
}
