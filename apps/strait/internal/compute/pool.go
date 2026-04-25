package compute

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"
)

// MachinePool manages a pool of stopped machines for reuse, reducing cold starts.
type MachinePool struct {
	mu       sync.Mutex
	entries  map[string][]poolEntry // Key: "{imageURI}:{region}"
	maxPer   int
	onEvict  func(machineID string) // Called asynchronously when a machine is evicted.
	evictSem chan struct{}          // Bounds concurrent eviction callbacks.
}

type poolEntry struct {
	MachineID string
	StoppedAt time.Time
	LastRunID string
}

// NewMachinePool creates a new machine pool with the given max entries per key.
func NewMachinePool(maxPerKey int) *MachinePool {
	if maxPerKey <= 0 {
		maxPerKey = 3
	}
	return &MachinePool{
		entries:  make(map[string][]poolEntry),
		maxPer:   maxPerKey,
		evictSem: make(chan struct{}, 10),
	}
}

// SetOnEvict sets the callback invoked (asynchronously) when a machine is evicted.
func (p *MachinePool) SetOnEvict(fn func(machineID string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onEvict = fn
}

// PoolKey returns a collision-resistant cache key for a given project, image,
// and region. Uses SHA-256 hash to eliminate any risk of separator collisions
// regardless of input content.
func PoolKey(projectID, imageURI, region string) string {
	h := sha256.New()
	h.Write([]byte(projectID))
	h.Write([]byte{0})
	h.Write([]byte(imageURI))
	h.Write([]byte{0})
	h.Write([]byte(region))
	return hex.EncodeToString(h.Sum(nil))
}

// AcquireResult holds the machine ID and metadata from a pool Acquire.
type AcquireResult struct {
	MachineID string
	LastRunID string
}

// Acquire removes and returns the oldest machine from the pool for the given key.
// Returns empty string and false if no machine is available.
func (p *MachinePool) Acquire(projectID, imageURI, region string) (string, bool) {
	key := PoolKey(projectID, imageURI, region)

	p.mu.Lock()
	defer p.mu.Unlock()

	entries := p.entries[key]
	if len(entries) == 0 {
		return "", false
	}

	// Pop oldest (first).
	entry := entries[0]
	p.entries[key] = entries[1:]
	return entry.MachineID, true
}

// Release returns a stopped machine to the pool. If the pool is at capacity
// for this key, the oldest entry is evicted.
func (p *MachinePool) Release(projectID, imageURI, region, machineID string) {
	if machineID == "" {
		return
	}

	key := PoolKey(projectID, imageURI, region)

	p.mu.Lock()
	defer p.mu.Unlock()

	entries := p.entries[key]

	// Evict oldest if at capacity.
	if len(entries) >= p.maxPer {
		evicted := entries[0]
		p.entries[key] = entries[1:]
		entries = p.entries[key]
		if p.onEvict != nil {
			fn := p.onEvict
			select {
			case p.evictSem <- struct{}{}:
				go func() {
					defer func() {
						recover() //nolint:errcheck // Prevent eviction panic from crashing the process.
						<-p.evictSem
					}()
					fn(evicted.MachineID)
				}()
			default:
				// Eviction semaphore full -- drop evicted machine instead of
				// blocking inline (which would hold p.mu across network I/O).
				// The periodic GC will clean up any leaked machines.
				slog.Warn("machine pool eviction semaphore full, dropping evicted machine",
					"machine_id", evicted.MachineID,
				)
			}
		}
	}

	p.entries[key] = append(entries, poolEntry{
		MachineID: machineID,
		StoppedAt: time.Now(),
	})
}

// Prune removes machines older than maxAge and calls destroyFn for each.
// The destroy callbacks run outside the pool lock so that slow network I/O
// (e.g. K8s API calls) does not block other pool operations.
func (p *MachinePool) Prune(maxAge time.Duration, destroyFn func(machineID string) error) int {
	cutoff := time.Now().Add(-maxAge)

	// Step 1: collect machines to prune under the lock.
	var toPrune []string

	p.mu.Lock()
	for key, entries := range p.entries {
		var kept []poolEntry
		for _, e := range entries {
			if e.StoppedAt.Before(cutoff) {
				toPrune = append(toPrune, e.MachineID)
			} else {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			delete(p.entries, key)
		} else {
			p.entries[key] = kept
		}
	}
	p.mu.Unlock()

	// Step 2: destroy outside the lock so slow I/O doesn't block pool ops.
	if destroyFn != nil {
		for _, id := range toPrune {
			_ = destroyFn(id)
		}
	}

	return len(toPrune)
}

// Size returns the total number of machines in the pool.
func (p *MachinePool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	total := 0
	for _, entries := range p.entries {
		total += len(entries)
	}
	return total
}
