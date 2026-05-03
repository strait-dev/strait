package grpc

import (
	"sync"
)

// ConnectedWorker holds in-memory state for a single connected worker stream.
type ConnectedWorker struct {
	WorkerID       string
	ProjectID      string
	APIKeyID       string // ID of the API key that authenticated this stream
	Name           string
	Hostname       string
	SDKVersion     string
	SDKLanguage    string
	Queues         []string
	SlotsTotal     int32
	SlotsAvailable int32
	Status         string            // active | draining
	SendCh         chan<- interface{} // typed in Phase 6.4
	// revokeCh is closed by the registry when the authenticating API key is revoked.
	// The stream goroutine selects on this channel to close itself immediately.
	revokeCh chan struct{}
}

// ConnectionRegistry is an in-memory store of all active worker streams on
// this replica. It is the authoritative source for slot accounting.
// Workers are keyed by worker ID; project isolation is enforced at registration.
type ConnectionRegistry struct {
	mu       sync.RWMutex
	workers  map[string]*ConnectedWorker   // keyed by worker_id
	byAPIKey map[string][]*ConnectedWorker // keyed by api_key_id
}

// NewConnectionRegistry creates an empty ConnectionRegistry.
func NewConnectionRegistry() *ConnectionRegistry {
	return &ConnectionRegistry{
		workers:  make(map[string]*ConnectedWorker),
		byAPIKey: make(map[string][]*ConnectedWorker),
	}
}

// Register adds or replaces a worker entry and indexes it by API key.
func (r *ConnectionRegistry) Register(w *ConnectedWorker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workers[w.WorkerID] = w
	if w.APIKeyID != "" {
		r.byAPIKey[w.APIKeyID] = append(r.byAPIKey[w.APIKeyID], w)
	}
}

// Deregister removes a worker from the registry and cleans up the API key index.
func (r *ConnectionRegistry) Deregister(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workers[workerID]
	if !ok {
		return
	}
	delete(r.workers, workerID)
	if w.APIKeyID != "" {
		list := r.byAPIKey[w.APIKeyID]
		filtered := list[:0]
		for _, entry := range list {
			if entry.WorkerID != workerID {
				filtered = append(filtered, entry)
			}
		}
		if len(filtered) == 0 {
			delete(r.byAPIKey, w.APIKeyID)
		} else {
			r.byAPIKey[w.APIKeyID] = filtered
		}
	}
}

// CloseByAPIKey signals all streams authenticated with the given API key to
// close by closing their revokeCh. This implements the cross-replica revocation
// path: the subscriber in stream.go reacts to the Redis pub/sub signal and calls
// this method so every local stream for that key is closed immediately.
func (r *ConnectionRegistry) CloseByAPIKey(apiKeyID string) {
	r.mu.RLock()
	workers := make([]*ConnectedWorker, len(r.byAPIKey[apiKeyID]))
	copy(workers, r.byAPIKey[apiKeyID])
	r.mu.RUnlock()

	for _, w := range workers {
		select {
		case <-w.revokeCh:
			// Already closed — skip.
		default:
			close(w.revokeCh)
		}
	}
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
