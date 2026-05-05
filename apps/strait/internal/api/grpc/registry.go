package grpc

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
)

const (
	defaultMaxWorkerStreamsPerProject = 1000
	defaultMaxWorkerStreamsPerAPIKey  = 100
)

var ErrWorkerStreamQuotaExceeded = errors.New("worker stream quota exceeded")

// ConnectedWorker holds in-memory state for a single connected worker stream.
type ConnectedWorker struct {
	WorkerID       string
	ProjectID      string
	EnvironmentID  string
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
	// revokeOnce guards the close(revokeCh). Both Register's same-key reconnect
	// path and CloseByAPIKey can race to close the channel — without this
	// once, the second closer panics with "close of closed channel". The
	// previous select-default-close pattern was insufficient: between the
	// select branch and the close, a concurrent closer can pass the same
	// select and double-close.
	//
	// Pointer-typed so the surrounding struct stays copyable (Snapshot copies
	// ConnectedWorker by value); sync.Once carries a noCopy guard.
	revokeOnce *sync.Once
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
	mu                   sync.RWMutex
	workers              map[string]*ConnectedWorker   // keyed by worker_id
	byAPIKey             map[string][]*ConnectedWorker // keyed by api_key_id
	maxStreamsPerProject int
	maxStreamsPerAPIKey  int
	// nextToken issues monotonically increasing registration tokens. Any value
	// > 0 is valid; a zero token signals "unassigned" and is rejected on
	// Deregister to keep test fixtures and accidental zero-valued callers from
	// silently evicting live entries.
	nextToken atomic.Uint64
}

// NewConnectionRegistry creates an empty ConnectionRegistry.
func NewConnectionRegistry() *ConnectionRegistry {
	return &ConnectionRegistry{
		workers:              make(map[string]*ConnectedWorker),
		byAPIKey:             make(map[string][]*ConnectedWorker),
		maxStreamsPerProject: defaultMaxWorkerStreamsPerProject,
		maxStreamsPerAPIKey:  defaultMaxWorkerStreamsPerAPIKey,
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
	if w.revokeOnce == nil {
		w.revokeOnce = &sync.Once{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.workers[w.WorkerID]; ok {
		if existing.APIKeyID != w.APIKeyID {
			return fmt.Errorf("worker_id %q already registered under a different api key", w.WorkerID)
		}
		// Same key reconnecting: signal the stale stream to exit, then evict
		// the old byAPIKey pointer. The once guards against a concurrent
		// CloseByAPIKey racing to close the same channel.
		if existing.revokeCh != nil {
			existing.revokeOnce.Do(func() { close(existing.revokeCh) })
		}
		delete(r.workers, existing.WorkerID)
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
	if r.maxStreamsPerProject > 0 && r.countProjectLocked(w.ProjectID) >= r.maxStreamsPerProject {
		return fmt.Errorf("%w: project %s has reached %d active streams", ErrWorkerStreamQuotaExceeded, w.ProjectID, r.maxStreamsPerProject)
	}
	if w.APIKeyID != "" && r.maxStreamsPerAPIKey > 0 && len(r.byAPIKey[w.APIKeyID]) >= r.maxStreamsPerAPIKey {
		return fmt.Errorf("%w: api key %s has reached %d active streams", ErrWorkerStreamQuotaExceeded, w.APIKeyID, r.maxStreamsPerAPIKey)
	}
	w.regToken = r.nextToken.Add(1)
	r.workers[w.WorkerID] = w
	if w.APIKeyID != "" {
		r.byAPIKey[w.APIKeyID] = append(r.byAPIKey[w.APIKeyID], w)
	}
	return nil
}

func (r *ConnectionRegistry) countProjectLocked(projectID string) int {
	count := 0
	for _, worker := range r.workers {
		if worker.ProjectID == projectID {
			count++
		}
	}
	return count
}

// Deregister removes a worker from the registry and cleans up the API key
// index, but only if the stored entry's token matches the supplied token.
// This prevents a stale stream's deferred cleanup from evicting a live
// replacement that registered after a reconnect race. A token of 0 is always
// rejected (Register never assigns 0), making accidental zero-token calls
// safe no-ops. It returns true only when this call removed the current live
// registration.
func (r *ConnectionRegistry) Deregister(workerID string, token uint64) bool {
	if token == 0 {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workers[workerID]
	if !ok {
		return false
	}
	if w.regToken != token {
		// The current registration belongs to a newer connection; the caller
		// is a stale goroutine cleaning up its own (already-superseded) entry.
		return false
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
	return true
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
		w.revokeOnce.Do(func() { close(w.revokeCh) })
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
//
// NOTE: callers that intend to dispatch a task should use
// ReserveWorkerForQueue instead, which combines pick + decrement under a
// single critical section. PickWorkerForQueue is retained for read-only
// inspection (tests, metrics, debugging). The returned pointer aliases a
// live registry entry — do not mutate it and do not retain it across calls.
func (r *ConnectionRegistry) PickWorkerForQueue(projectID, queue string) (*ConnectedWorker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	best := r.pickLocked(projectID, queue, "")
	if best == nil {
		return nil, false
	}
	return best, true
}

// ReserveWorkerForQueue atomically picks the least-loaded active worker for
// the queue and decrements its available slots in a single critical section.
// This eliminates the TOCTOU race where N concurrent dispatchers each see
// the same SlotsAvailable, all "win" the pick, and oversubscribe a worker
// that only had one slot.
//
// Returns the worker's ID and SendCh so callers can route a task message
// without keeping a pointer to the registry entry. The SendCh is the same
// channel used by the stream goroutine; the caller must select on
// ctx.Done() when sending so it gives up if the worker disconnects (the
// stream's send loop exits on ctx.Done(), so receiving stops). On any
// failure to actually deliver the work, the caller must call
// IncrementSlots(workerID) to release the reservation.
func (r *ConnectionRegistry) ReserveWorkerForQueue(projectID, queue, environmentID string) (workerID string, sendCh chan<- *workerv1.ServerMessage, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	best := r.pickLocked(projectID, queue, environmentID)
	if best == nil {
		return "", nil, false
	}
	best.SlotsAvailable--
	return best.WorkerID, best.SendCh, true
}

// pickLocked returns the least-loaded active worker for the queue, or nil if
// none qualify. Caller must hold r.mu (read or write).
func (r *ConnectionRegistry) pickLocked(projectID, queue, environmentID string) *ConnectedWorker {
	var best *ConnectedWorker
	for _, w := range r.workers {
		if w.ProjectID != projectID || w.Status != "active" || w.SlotsAvailable <= 0 {
			continue
		}
		if w.EnvironmentID != "" && w.EnvironmentID != environmentID {
			continue
		}
		if !workerHasQueue(w, queue) {
			continue
		}
		if best == nil || w.SlotsAvailable > best.SlotsAvailable {
			best = w
		}
	}
	return best
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
