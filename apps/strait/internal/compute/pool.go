package compute

import (
	"sync"
	"time"
)

// MachinePool manages a pool of stopped machines for reuse, reducing cold starts.
type MachinePool struct {
	mu      sync.Mutex
	entries map[string][]poolEntry // Key: "{imageURI}:{region}"
	maxPer  int
	onEvict func(machineID string) // Called asynchronously when a machine is evicted.
}

type poolEntry struct {
	MachineID string
	StoppedAt time.Time
}

// NewMachinePool creates a new machine pool with the given max entries per key.
func NewMachinePool(maxPerKey int) *MachinePool {
	if maxPerKey <= 0 {
		maxPerKey = 3
	}
	return &MachinePool{
		entries: make(map[string][]poolEntry),
		maxPer:  maxPerKey,
	}
}

// SetOnEvict sets the callback invoked (asynchronously) when a machine is evicted.
func (p *MachinePool) SetOnEvict(fn func(machineID string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onEvict = fn
}

// PoolKey returns the cache key for a given image and region.
func PoolKey(imageURI, region string) string {
	return imageURI + ":" + region
}

// Acquire removes and returns the oldest machine from the pool for the given key.
// Returns empty string and false if no machine is available.
func (p *MachinePool) Acquire(imageURI, region string) (string, bool) {
	key := PoolKey(imageURI, region)

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
func (p *MachinePool) Release(imageURI, region, machineID string) {
	key := PoolKey(imageURI, region)

	p.mu.Lock()
	defer p.mu.Unlock()

	entries := p.entries[key]

	// Evict oldest if at capacity.
	if len(entries) >= p.maxPer {
		evicted := entries[0]
		p.entries[key] = entries[1:]
		entries = p.entries[key]
		if p.onEvict != nil {
			go p.onEvict(evicted.MachineID)
		}
	}

	p.entries[key] = append(entries, poolEntry{
		MachineID: machineID,
		StoppedAt: time.Now(),
	})
}

// Prune removes machines older than maxAge and calls destroyFn for each.
func (p *MachinePool) Prune(maxAge time.Duration, destroyFn func(machineID string) error) int {
	cutoff := time.Now().Add(-maxAge)
	var pruned int

	p.mu.Lock()
	defer p.mu.Unlock()

	for key, entries := range p.entries {
		var kept []poolEntry
		for _, e := range entries {
			if e.StoppedAt.Before(cutoff) {
				pruned++
				if destroyFn != nil {
					_ = destroyFn(e.MachineID)
				}
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

	return pruned
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
