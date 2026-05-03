package grpc

import (
	"sync"
)

// ConnectedWorker holds in-memory state for a single connected worker stream.
type ConnectedWorker struct {
	WorkerID       string
	ProjectID      string
	Name           string
	Hostname       string
	SDKVersion     string
	SDKLanguage    string
	Queues         []string
	SlotsTotal     int32
	SlotsAvailable int32
	Status         string // active | draining
	SendCh         chan<- interface{} // typed in Phase 6.4
}

// ConnectionRegistry is an in-memory store of all active worker streams on
// this replica. It is the authoritative source for slot accounting.
// Workers are keyed by worker ID; project isolation is enforced at registration.
type ConnectionRegistry struct {
	mu      sync.RWMutex
	workers map[string]*ConnectedWorker // keyed by worker_id
}

// NewConnectionRegistry creates an empty ConnectionRegistry.
func NewConnectionRegistry() *ConnectionRegistry {
	return &ConnectionRegistry{
		workers: make(map[string]*ConnectedWorker),
	}
}

// Register adds or replaces a worker entry.
func (r *ConnectionRegistry) Register(w *ConnectedWorker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workers[w.WorkerID] = w
}

// Deregister removes a worker from the registry.
func (r *ConnectionRegistry) Deregister(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.workers, workerID)
}

// MarkDraining transitions a worker to draining status.
func (r *ConnectionRegistry) MarkDraining(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.workers[workerID]; ok {
		w.Status = "draining"
	}
}

// PickWorkerForQueue returns the least-loaded active worker for the given
// queue that belongs to the given project, or (nil, false) if none found.
func (r *ConnectionRegistry) PickWorkerForQueue(projectID, queue string) (*ConnectedWorker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var best *ConnectedWorker
	for _, w := range r.workers {
		if w.ProjectID != projectID || w.Status != "active" || w.SlotsAvailable <= 0 {
			continue
		}
		if !workerHasQueue(w, queue) {
			continue
		}
		if best == nil || w.SlotsAvailable > best.SlotsAvailable {
			best = w
		}
	}
	if best == nil {
		return nil, false
	}
	return best, true
}

// IncrementSlots increases a worker's available slots by one (called when a
// task completes or fails).
func (r *ConnectionRegistry) IncrementSlots(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.workers[workerID]; ok {
		w.SlotsAvailable++
	}
}

// DecrementSlots decreases a worker's available slots by one (called when a
// task is assigned).
func (r *ConnectionRegistry) DecrementSlots(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.workers[workerID]; ok && w.SlotsAvailable > 0 {
		w.SlotsAvailable--
	}
}

// Snapshot returns a copy of all connected workers for read-only iteration
// (e.g. DB sync loop, metrics).
func (r *ConnectionRegistry) Snapshot() []ConnectedWorker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ConnectedWorker, 0, len(r.workers))
	for _, w := range r.workers {
		out = append(out, *w)
	}
	return out
}

// SnapshotQueues returns the deduplicated set of queue names across all active
// workers on this replica. Used by the dequeue tick to filter worker-mode runs.
func (r *ConnectionRegistry) SnapshotQueues() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]struct{})
	for _, w := range r.workers {
		if w.Status != "active" {
			continue
		}
		for _, q := range w.Queues {
			seen[q] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for q := range seen {
		out = append(out, q)
	}
	return out
}

// workerHasQueue returns true if the worker is registered for the given queue.
func workerHasQueue(w *ConnectedWorker, queue string) bool {
	for _, q := range w.Queues {
		if q == queue {
			return true
		}
	}
	return false
}
