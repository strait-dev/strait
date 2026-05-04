package grpc

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
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
	Status         string                         // active | draining
	SendCh         chan<- *workerv1.ServerMessage // populated by stream goroutine on Register
	// revokeCh is closed by the registry when the authenticating API key is
	// revoked or when a same-id reconnect supersedes this entry. The stream
	// goroutine selects on this channel to close itself immediately.
	revokeCh chan struct{}
	// regToken is the per-registration token assigned by the registry. It is
	// passed back to Deregister so a stale stream goroutine's deferred cleanup
	// cannot evict a live replacement that registered under the same WorkerID
	// after a reconnect race.
	regToken uint64
}

// ConnectionRegistry is an in-memory store of all active worker streams on
// this replica. It is the authoritative source for slot accounting.
// Workers are keyed by worker ID; project isolation is enforced at registration.
type ConnectionRegistry struct {
	mu       sync.RWMutex
	workers  map[string]*ConnectedWorker   // keyed by worker_id
	byAPIKey map[string][]*ConnectedWorker // keyed by api_key_id
	// nextToken issues monotonically increasing registration tokens. Any value
	// > 0 is valid; a zero token signals "unassigned" and is rejected on
	// Deregister to keep test fixtures and accidental zero-valued callers from
	// silently evicting live entries.
	nextToken atomic.Uint64
}

// NewConnectionRegistry creates an empty ConnectionRegistry.
func NewConnectionRegistry() *ConnectionRegistry {
	return &ConnectionRegistry{
		workers:  make(map[string]*ConnectedWorker),
		byAPIKey: make(map[string][]*ConnectedWorker),
	}
}

// Register adds or replaces a worker entry and indexes it by API key.
// If a worker with the same ID already exists under a different API key,
// the registration is rejected to prevent session hijacking via worker-id
// guessing.
//
// Same-key reconnect: the existing entry's revokeCh is closed (so the stale
// stream goroutine wakes up and exits) and a fresh registration token is
// assigned to the new entry. The old goroutine's deferred Deregister will
// no-op because its token no longer matches.
//
// Register populates w.regToken with the assigned token; callers must pass
// that token back to Deregister.
func (r *ConnectionRegistry) Register(w *ConnectedWorker) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.workers[w.WorkerID]; ok {
		if existing.APIKeyID != w.APIKeyID {
			return fmt.Errorf("worker_id %q already registered under a different api key", w.WorkerID)
		}
		// Same key reconnecting: signal the stale stream to exit, then evict
		// the old byAPIKey pointer.
		if existing.revokeCh != nil {
			select {
			case <-existing.revokeCh:
				// Already closed (e.g., concurrent CloseByAPIKey) — skip.
			default:
				close(existing.revokeCh)
			}
		}
		if existing.APIKeyID != "" {
			list := r.byAPIKey[existing.APIKeyID]
			filtered := list[:0]
			for _, e := range list {
				if e.WorkerID != existing.WorkerID {
					filtered = append(filtered, e)
				}
			}
			if len(filtered) == 0 {
				delete(r.byAPIKey, existing.APIKeyID)
			} else {
				r.byAPIKey[existing.APIKeyID] = filtered
			}
		}
	}
	w.regToken = r.nextToken.Add(1)
	r.workers[w.WorkerID] = w
	if w.APIKeyID != "" {
		r.byAPIKey[w.APIKeyID] = append(r.byAPIKey[w.APIKeyID], w)
	}
	return nil
}

// Deregister removes a worker from the registry and cleans up the API key
// index, but only if the stored entry's token matches the supplied token.
// This prevents a stale stream's deferred cleanup from evicting a live
// replacement that registered after a reconnect race. A token of 0 is always
// rejected (Register never assigns 0), making accidental zero-token calls
// safe no-ops.
func (r *ConnectionRegistry) Deregister(workerID string, token uint64) {
	if token == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workers[workerID]
	if !ok {
		return
	}
	if w.regToken != token {
		// The current registration belongs to a newer connection; the caller
		// is a stale goroutine cleaning up its own (already-superseded) entry.
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
// task completes or fails). Capped at SlotsTotal so a misbehaving worker
// cannot inflate its slot count and become preferred for every dispatch.
func (r *ConnectionRegistry) IncrementSlots(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.workers[workerID]; ok && w.SlotsAvailable < w.SlotsTotal {
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
	return slices.Contains(w.Queues, queue)
}
