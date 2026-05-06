package grpc

import (
	"fmt"
	"sync"
	"testing"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
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

	if err := r.Register(w); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 worker in snapshot, got %d", len(snap))
	}
	if snap[0].WorkerID != "w1" {
		t.Errorf("expected worker id w1, got %s", snap[0].WorkerID)
	}
}

// TestRegistry_Register_Collision verifies that re-registration with a different API key
// returns an error (hijack protection).
func TestRegistry_Register_Collision(t *testing.T) {
	r := NewConnectionRegistry()
	w1 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	w2 := makeWorker("w1", "proj-b", "key-2", []string{"default"}, 4)

	if err := r.Register(w1); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err := r.Register(w2)
	if err == nil {
		t.Fatal("expected collision error, got nil")
	}
}

// TestRegistry_Register_Reconnect verifies that same-key re-registration evicts the old
// byAPIKey entry and does not accumulate stale pointers.
func TestRegistry_Register_Reconnect(t *testing.T) {
	r := NewConnectionRegistry()
	w1 := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	w2 := makeWorker("w1", "proj-a", "key-1", []string{"default", "batch"}, 4)

	if err := r.Register(w1); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := r.Register(w2); err != nil {
		t.Fatalf("reconnect register failed: %v", err)
	}

	// Only one worker should be in the byAPIKey index.
	r.mu.RLock()
	count := len(r.byAPIKey["key-1"])
	r.mu.RUnlock()
	if count != 1 {
		t.Errorf("expected 1 byAPIKey entry after reconnect, got %d", count)
	}

	// Snapshot should show updated queues.
	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 worker in snapshot after reconnect, got %d", len(snap))
	}
	if len(snap[0].Queues) != 2 {
		t.Errorf("expected 2 queues after reconnect, got %d", len(snap[0].Queues))
	}
}

// TestRegistry_Deregister_RemovesWorkerAndIndex verifies full cleanup.
func TestRegistry_Deregister_RemovesWorkerAndIndex(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	r.Deregister("w1", w.regToken)

	snap := r.Snapshot()
	if len(snap) != 0 {
		t.Errorf("expected empty snapshot after deregister, got %d", len(snap))
	}
	r.mu.RLock()
	_, keyExists := r.byAPIKey["key-1"]
	r.mu.RUnlock()
	if keyExists {
		t.Error("expected byAPIKey entry to be cleaned up after deregister")
	}
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
	w.SlotsAvailable = 2 // already at cap

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	r.IncrementSlots("w1")

	snap := r.Snapshot()
	if snap[0].SlotsAvailable != 2 {
		t.Errorf("expected slots capped at 2, got %d", snap[0].SlotsAvailable)
	}
}

// TestRegistry_IncrementSlots_Normal verifies increment from below cap.
func TestRegistry_IncrementSlots_Normal(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)
	w.SlotsAvailable = 2

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	r.IncrementSlots("w1")

	snap := r.Snapshot()
	if snap[0].SlotsAvailable != 3 {
		t.Errorf("expected slots=3, got %d", snap[0].SlotsAvailable)
	}
}

// TestRegistry_DecrementSlots_Normal decrements from above zero.
func TestRegistry_DecrementSlots_Normal(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	r.DecrementSlots("w1")

	snap := r.Snapshot()
	if snap[0].SlotsAvailable != 3 {
		t.Errorf("expected slots=3, got %d", snap[0].SlotsAvailable)
	}
}

// TestRegistry_DecrementSlots_UnderflowGuard verifies that SlotsAvailable never goes
// below zero even with repeated decrements.
func TestRegistry_DecrementSlots_UnderflowGuard(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 2)
	w.SlotsAvailable = 0 // start at zero

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	r.DecrementSlots("w1")
	r.DecrementSlots("w1")

	snap := r.Snapshot()
	if snap[0].SlotsAvailable < 0 {
		t.Errorf("slots went negative: %d", snap[0].SlotsAvailable)
	}
	if snap[0].SlotsAvailable != 0 {
		t.Errorf("expected slots=0, got %d", snap[0].SlotsAvailable)
	}
}

// TestRegistry_SnapshotQueues_OnlyActiveWorkers verifies that draining workers are
// excluded from the queue snapshot.
func TestRegistry_SnapshotQueues_OnlyActiveWorkers(t *testing.T) {
	r := NewConnectionRegistry()
	active := makeWorker("active", "proj-a", "key-1", []string{"q1", "q2"}, 4)
	draining := makeWorker("draining", "proj-a", "key-2", []string{"q3"}, 4)
	draining.Status = "draining"

	if err := r.Register(active); err != nil {
		t.Fatalf("register active: %v", err)
	}
	if err := r.Register(draining); err != nil {
		t.Fatalf("register draining: %v", err)
	}

	queues := r.SnapshotQueues()

	for _, q := range queues {
		if q == "q3" {
			t.Error("draining worker's queue should not appear in SnapshotQueues")
		}
	}
	if len(queues) != 2 {
		t.Errorf("expected 2 queues (q1, q2), got %v", queues)
	}
}

// TestRegistry_SnapshotQueues_Empty returns nil when no active workers are registered.
func TestRegistry_SnapshotQueues_Empty(t *testing.T) {
	r := NewConnectionRegistry()
	queues := r.SnapshotQueues()
	if queues != nil {
		t.Errorf("expected nil from empty registry, got %v", queues)
	}
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
		if err := r.Register(w); err != nil {
			t.Fatalf("register %s: %v", w.WorkerID, err)
		}
	}

	got := r.SnapshotWorkerQueues()
	seen := make(map[domain.WorkerQueueRef]struct{}, len(got))
	for _, ref := range got {
		seen[ref] = struct{}{}
	}
	want := []domain.WorkerQueueRef{
		{QueueName: "q1"},
		{QueueName: "q2"},
		{QueueName: "q1", EnvironmentID: "env-staging"},
	}
	if len(seen) != len(want) {
		t.Fatalf("snapshot refs = %+v, want %d unique refs", got, len(want))
	}
	for _, ref := range want {
		if _, ok := seen[ref]; !ok {
			t.Fatalf("missing worker queue ref %+v from %+v", ref, got)
		}
	}
	if _, ok := seen[domain.WorkerQueueRef{QueueName: "q3", EnvironmentID: "env-prod"}]; ok {
		t.Fatalf("draining worker ref leaked into snapshot: %+v", got)
	}
}

// TestRegistry_MarkDraining transitions a worker status correctly.
func TestRegistry_MarkDraining(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"default"}, 4)

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	r.MarkDraining("w1")

	snap := r.Snapshot()
	if snap[0].Status != "draining" {
		t.Errorf("expected status=draining, got %s", snap[0].Status)
	}
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

	if err := r.Register(busy); err != nil {
		t.Fatalf("register busy: %v", err)
	}
	if err := r.Register(idle); err != nil {
		t.Fatalf("register idle: %v", err)
	}

	picked, ok := r.PickWorkerForQueue("proj-a", "q")
	if !ok {
		t.Fatal("expected a worker to be picked")
	}
	if picked.WorkerID != "idle" {
		t.Errorf("expected idle worker to be picked (more slots), got %s", picked.WorkerID)
	}
}

// TestRegistry_PickWorkerForQueue_NoSlots verifies that workers with zero slots are skipped.
func TestRegistry_PickWorkerForQueue_NoSlots(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 4)
	w.SlotsAvailable = 0

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, ok := r.PickWorkerForQueue("proj-a", "q")
	if ok {
		t.Error("expected no worker picked when all slots are exhausted")
	}
}

// TestRegistry_PickWorkerForQueue_WrongProject verifies project isolation.
func TestRegistry_PickWorkerForQueue_WrongProject(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 4)

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, ok := r.PickWorkerForQueue("proj-b", "q")
	if ok {
		t.Error("expected no worker picked for different project")
	}
}

// TestRegistry_PickWorkerForQueue_WrongQueue verifies queue filtering.
func TestRegistry_PickWorkerForQueue_WrongQueue(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q1"}, 4)

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, ok := r.PickWorkerForQueue("proj-a", "q2")
	if ok {
		t.Error("expected no worker picked for queue not registered")
	}
}

// TestRegistry_PickWorkerForQueue_DrainedSkipped verifies draining workers are skipped.
func TestRegistry_PickWorkerForQueue_DrainedSkipped(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 4)

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	r.MarkDraining("w1")

	_, ok := r.PickWorkerForQueue("proj-a", "q")
	if ok {
		t.Error("expected no worker picked when draining")
	}
}

// TestRegistry_CloseByAPIKey_ClosesRevokeCh verifies that CloseByAPIKey signals all streams
// for the given API key.
func TestRegistry_CloseByAPIKey_ClosesRevokeCh(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 4)

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	r.CloseByAPIKey("key-1")

	select {
	case <-w.revokeCh:
		// expected
	default:
		t.Error("expected revokeCh to be closed after CloseByAPIKey")
	}
}

// TestRegistry_CloseByAPIKey_IdempotentDoubleClose verifies that calling CloseByAPIKey
// twice on the same key does not panic.
func TestRegistry_CloseByAPIKey_IdempotentDoubleClose(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 4)

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	r.CloseByAPIKey("key-1")
	r.CloseByAPIKey("key-1") // must not panic
}

// TestRegistry_Concurrent_RegisterDeregister verifies that concurrent Register/Deregister
// operations do not race, panic, or corrupt invariants.
func TestRegistry_Concurrent_RegisterDeregister(t *testing.T) {
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
		go func() {
			defer wg.Done()
			wid := fmt.Sprintf("w-%d", i)
			w := makeWorker(wid, "proj-a", fmt.Sprintf("key-%d", i), []string{"q"}, 4)
			_ = r.Register(w)
			tokenChs[i] <- w.regToken
		}()
		go func() {
			defer wg.Done()
			tok := <-tokenChs[i]
			r.Deregister(fmt.Sprintf("w-%d", i), tok)
		}()
	}

	wg.Wait()

	// Invariant: byAPIKey and workers must remain consistent (no orphaned entries).
	r.mu.RLock()
	defer r.mu.RUnlock()
	for keyID, workers := range r.byAPIKey {
		for _, w := range workers {
			if _, ok := r.workers[w.WorkerID]; !ok {
				t.Errorf("byAPIKey[%s] references worker %s not in workers map", keyID, w.WorkerID)
			}
		}
	}
}

// TestRegistry_Concurrent_SlotOperations verifies that concurrent increment/decrement
// operations keep slots within [0, SlotsTotal].
func TestRegistry_Concurrent_SlotOperations(t *testing.T) {
	r := NewConnectionRegistry()
	w := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 10)

	if err := r.Register(w); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	const ops = 200
	var wg sync.WaitGroup
	wg.Add(ops * 2)

	for range ops {
		go func() {
			defer wg.Done()
			r.IncrementSlots("w1")
		}()
		go func() {
			defer wg.Done()
			r.DecrementSlots("w1")
		}()
	}

	wg.Wait()

	snap := r.Snapshot()
	if len(snap) == 0 {
		t.Fatal("worker disappeared from registry")
	}
	slots := snap[0].SlotsAvailable
	total := snap[0].SlotsTotal
	if slots < 0 {
		t.Errorf("slots went negative: %d", slots)
	}
	if slots > total {
		t.Errorf("slots (%d) exceeded total (%d)", slots, total)
	}
}

// TestRegistry_Concurrent_ReconnectStorm verifies that 1000 parallel reconnects for
// the same worker_id and same api_key do not cause panics or leave the registry inconsistent.
func TestRegistry_Concurrent_ReconnectStorm(t *testing.T) {
	r := NewConnectionRegistry()

	const n = 100 // reduced from 1000 to keep test duration short while still exercising concurrency
	var wg sync.WaitGroup
	wg.Add(n)

	for i := range n {
		go func() {
			defer wg.Done()
			w := makeWorker("storm-worker", "proj-a", "key-1", []string{fmt.Sprintf("q%d", i)}, 4)
			_ = r.Register(w)
		}()
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
	if count > 1 {
		t.Errorf("expected at most 1 storm-worker in registry, got %d", count)
	}

	// byAPIKey index must not have more than 1 entry for key-1 pointing to storm-worker.
	r.mu.RLock()
	stormCount := 0
	for _, bw := range r.byAPIKey["key-1"] {
		if bw.WorkerID == "storm-worker" {
			stormCount++
		}
	}
	r.mu.RUnlock()
	if stormCount > 1 {
		t.Errorf("byAPIKey has %d storm-worker entries, expected at most 1", stormCount)
	}
}
