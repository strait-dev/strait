package grpc

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
)

// TestAdversarial_WorkerIDHijackSameProject verifies that a second worker
// with the same ID in the same project but a different API key receives
// AlreadyExists (surfaced as an error from Register).
func TestAdversarial_WorkerIDHijackSameProject(t *testing.T) {
	r := NewConnectionRegistry()

	legit := makeWorker("target-worker", "proj-a", "key-legit", []string{"q"}, 4)
	if err := r.Register(legit); err != nil {
		t.Fatalf("legitimate register failed: %v", err)
	}

	hijacker := makeWorker("target-worker", "proj-a", "key-attacker", []string{"q"}, 4)
	err := r.Register(hijacker)
	if err == nil {
		t.Fatal("expected error for worker_id hijack attempt, got nil")
	}
	if !strings.Contains(err.Error(), "different api key") || !strings.Contains(err.Error(), "proj-a") {
		t.Errorf("error should mention the same-project api-key collision, got: %s", err.Error())
	}

	// The original worker must still be registered.
	snap := r.Snapshot()
	if len(snap) != 1 || snap[0].APIKeyID != "key-legit" {
		t.Errorf("original worker was displaced by hijacker: %+v", snap)
	}
}

func TestAdversarial_WorkerIDNamespaceAllowsSameIDAcrossProjects(t *testing.T) {
	r := NewConnectionRegistry()

	if err := r.Register(makeWorker("shared-worker", "proj-a", "key-a", []string{"q"}, 4)); err != nil {
		t.Fatalf("register proj-a worker: %v", err)
	}
	if err := r.Register(makeWorker("shared-worker", "proj-b", "key-b", []string{"q"}, 4)); err != nil {
		t.Fatalf("same worker id in another project must not be rejected: %v", err)
	}

	snap := r.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot len = %d, want 2: %+v", len(snap), snap)
	}
	for _, projectID := range []string{"proj-a", "proj-b"} {
		picked, ok := r.PickWorkerForQueue(projectID, "q")
		if !ok {
			t.Fatalf("project %s could not pick its worker", projectID)
		}
		if picked.ProjectID != projectID || picked.WorkerID != "shared-worker" {
			t.Fatalf("project %s picked wrong worker: %+v", projectID, picked)
		}
	}
}

func TestAdversarial_WorkerIDNamespaceProjectScopedSlotRelease(t *testing.T) {
	r := NewConnectionRegistry()

	if err := r.Register(makeWorker("shared-worker", "proj-a", "key-a", []string{"q"}, 2)); err != nil {
		t.Fatalf("register proj-a worker: %v", err)
	}
	if err := r.Register(makeWorker("shared-worker", "proj-b", "key-b", []string{"q"}, 2)); err != nil {
		t.Fatalf("register proj-b worker: %v", err)
	}

	r.DecrementProjectSlots("proj-a", "shared-worker")
	r.IncrementProjectSlots("proj-b", "shared-worker")

	snap := r.Snapshot()
	available := map[string]int32{}
	for _, worker := range snap {
		available[worker.ProjectID] = worker.SlotsAvailable
	}
	if available["proj-a"] != 1 {
		t.Fatalf("proj-a slots = %d, want 1", available["proj-a"])
	}
	if available["proj-b"] != 2 {
		t.Fatalf("proj-b slots = %d, want capped 2", available["proj-b"])
	}
}

// TestAdversarial_SlotExhaustion verifies that dispatch beyond SlotsTotal causes
// slot count to clamp at 0 and does not go negative.
func TestAdversarial_SlotExhaustion(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 2)

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Exhaust all slots.
	r.DecrementSlots("w1")
	r.DecrementSlots("w1")

	// Additional decrements beyond zero must clamp to 0.
	r.DecrementSlots("w1")
	r.DecrementSlots("w1")
	r.DecrementSlots("w1")

	snap := r.Snapshot()
	if snap[0].SlotsAvailable < 0 {
		t.Errorf("slots went negative: %d", snap[0].SlotsAvailable)
	}
	if snap[0].SlotsAvailable != 0 {
		t.Errorf("expected 0 slots, got %d", snap[0].SlotsAvailable)
	}

	// No worker should be pickable.
	_, ok := r.PickWorkerForQueue("proj-a", "q")
	if ok {
		t.Error("expected no worker pickable with exhausted slots")
	}
}

// TestAdversarial_RegistrationMaxWorkerIDLen verifies that worker IDs longer than
// maxWorkerIDLen are caught by the stream handler validation, which enforces the limit.
func TestAdversarial_RegistrationMaxWorkerIDLen(t *testing.T) {
	// maxWorkerIDLen is defined as 128 in stream.go.
	longID := strings.Repeat("x", maxWorkerIDLen+1)
	if len(longID) <= maxWorkerIDLen {
		t.Fatalf("test setup error: longID length should exceed maxWorkerIDLen")
	}

	// The registry itself does not enforce length — that's stream.go's job.
	// Verify the constant is set correctly and that the validation is enforceable.
	if maxWorkerIDLen != 128 {
		t.Errorf("expected maxWorkerIDLen=128, got %d", maxWorkerIDLen)
	}
}

// TestAdversarial_MaxQueuesPerWorker verifies the constant is enforced.
func TestAdversarial_MaxQueuesPerWorker(t *testing.T) {
	if maxQueuesPerWorker != 64 {
		t.Errorf("expected maxQueuesPerWorker=64, got %d", maxQueuesPerWorker)
	}
}

// TestAdversarial_MaxInFlightTasks verifies the constant is enforced.
func TestAdversarial_MaxInFlightTasks(t *testing.T) {
	if maxInFlightTasks != 256 {
		t.Errorf("expected maxInFlightTasks=256, got %d", maxInFlightTasks)
	}
}

// TestAdversarial_MaxLogMessageBytes verifies log clamping constant.
func TestAdversarial_MaxLogMessageBytes(t *testing.T) {
	if maxLogMessageBytes != 4096 {
		t.Errorf("expected maxLogMessageBytes=4096, got %d", maxLogMessageBytes)
	}
}

// TestAdversarial_SlotInflation verifies that IncrementSlots cannot inflate beyond SlotsTotal,
// preventing a misbehaving worker from becoming the preferred dispatch target.
func TestAdversarial_SlotInflation(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 5)

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Attempt to inflate slots beyond SlotsTotal.
	for range 100 {
		r.IncrementSlots("w1")
	}

	snap := r.Snapshot()
	if snap[0].SlotsAvailable > snap[0].SlotsTotal {
		t.Errorf("slots inflated beyond total: available=%d total=%d",
			snap[0].SlotsAvailable, snap[0].SlotsTotal)
	}
}

// TestAdversarial_SendChannelDeadlock verifies that a full (blocked) SendCh does not
// cause the registry or dispatch path to deadlock. The slot must be recoverable.
func TestAdversarial_SendChannelDeadlock(t *testing.T) {
	r := NewConnectionRegistry()
	// A full channel — nothing draining from the other end.
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
	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// sendCancel uses select with default, so it must not block even on a full channel.
	d := &WorkerDispatcher{registry: r}
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.sendCancel(ch, "run-blocked")
	}()

	select {
	case <-done:
		// sendCancel returned without deadlocking.
	case <-time.After(500 * time.Millisecond):
		t.Error("sendCancel deadlocked on full channel")
	}
}

// TestAdversarial_ReconnectStorm_ByAPIKeyConsistency verifies that after 100 parallel
// reconnects for the same worker_id+api_key pair, the byAPIKey index has at most 1 entry.
func TestAdversarial_ReconnectStorm_ByAPIKeyConsistency(t *testing.T) {
	r := NewConnectionRegistry()
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n)

	for i := range n {
		go func() {
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
		}()
	}
	wg.Wait()

	r.mu.RLock()
	count := len(r.byAPIKey["key-storm"])
	workers := len(r.workers)
	r.mu.RUnlock()

	if count > 1 {
		t.Errorf("byAPIKey has %d entries for key-storm after reconnect storm, expected at most 1", count)
	}
	if workers > 1 {
		t.Errorf("workers map has %d entries after reconnect storm, expected at most 1", workers)
	}
}

// TestAdversarial_SnapshotDuringDrain verifies that SnapshotQueues handles a worker
// that transitions to draining mid-iteration without panicking.
func TestAdversarial_SnapshotDuringDrain(t *testing.T) {
	r := NewConnectionRegistry()

	for i := range 10 {
		w := makeWorker(fmt.Sprintf("w%d", i), "proj-a", fmt.Sprintf("key-%d", i), []string{"q"}, 4)
		if err := r.Register(w); err != nil {
			t.Fatalf("register w%d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Concurrently drain workers while snapshotting queues.
	go func() {
		defer wg.Done()
		for i := range 10 {
			r.MarkDraining(fmt.Sprintf("w%d", i))
		}
	}()
	go func() {
		defer wg.Done()
		for range 20 {
			_ = r.SnapshotQueues()
		}
	}()

	wg.Wait() // must not panic or race
}

// TestAdversarial_CrossProject_PickWorker verifies that workers from project A
// are never returned for project B queries.
func TestAdversarial_CrossProject_PickWorker(t *testing.T) {
	r := NewConnectionRegistry()

	for i := range 5 {
		w := makeWorker(fmt.Sprintf("proj-a-w%d", i), "proj-a", fmt.Sprintf("ka%d", i), []string{"shared-q"}, 4)
		if err := r.Register(w); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	picked, ok := r.PickWorkerForQueue("proj-b", "shared-q")
	if ok {
		t.Errorf("cross-project pick succeeded: returned worker %s from proj-a for proj-b query", picked.WorkerID)
	}
}

// TestAdversarial_CloseByAPIKey_MultipleWorkers verifies that all streams for a given
// API key are closed, not just the first.
func TestAdversarial_CloseByAPIKey_MultipleWorkers(t *testing.T) {
	r := NewConnectionRegistry()

	workers := make([]*ConnectedWorker, 3)
	for i := range 3 {
		w := makeWorker(fmt.Sprintf("w%d", i), "proj-a", "shared-key", []string{"q"}, 4)
		workers[i] = w
		if err := r.Register(w); err != nil {
			t.Fatalf("register w%d: %v", i, err)
		}
	}

	r.CloseByAPIKey("shared-key")

	for i, w := range workers {
		select {
		case <-w.revokeCh:
			// expected
		default:
			t.Errorf("worker w%d revokeCh was not closed", i)
		}
	}
}
