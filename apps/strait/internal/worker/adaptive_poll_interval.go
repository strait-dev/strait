package worker

import (
	"sync"
	"sync/atomic"
	"time"
)

// Adaptive poll interval.
//
// The fixed poll interval is fine in steady state but wastes DB calls
// when the queue is empty and blocks latency from dropping when the
// queue is deep. AdaptivePollInterval adjusts the next tick based on
// the observed queue depth and consecutive-empty count, bounded by
// MinInterval and MaxInterval.
//
// Formula:
//   interval = clamp(base * 2^emptyCount / (1 + depth/100), min, max)
//
// - emptyCount increments every tick that returns zero rows, resetting
//   on any successful claim.
// - depth is the most recent observed queue depth (fed externally by
//   the health sampler gauge or by the worker's own claim result).
//
// The adapter is goroutine-safe: workers may call Observe from their
// poll loop and Next from a separate ticker goroutine.

// AdaptivePollInterval produces poll intervals that respond to queue load.
type AdaptivePollInterval struct {
	mu          sync.Mutex
	base        time.Duration
	minInterval time.Duration
	maxInterval time.Duration
	emptyCount  int
	depth       int64
	enabled     atomic.Bool
}

// NewAdaptivePollInterval builds an adapter. Zero values fall back to
// sensible defaults suitable for the default PollerInterval=5s.
func NewAdaptivePollInterval(base, minI, maxI time.Duration) *AdaptivePollInterval {
	if base <= 0 {
		base = 5 * time.Second
	}
	if minI <= 0 {
		minI = 200 * time.Millisecond
	}
	if maxI <= 0 {
		maxI = 30 * time.Second
	}
	if minI > maxI {
		minI = maxI
	}
	a := &AdaptivePollInterval{
		base:        base,
		minInterval: minI,
		maxInterval: maxI,
	}
	a.enabled.Store(true)
	return a
}

// Enable / Disable let callers opt out at runtime (kill switch).
func (a *AdaptivePollInterval) Enable()  { a.enabled.Store(true) }
func (a *AdaptivePollInterval) Disable() { a.enabled.Store(false) }

// ObserveClaim records a non-zero claim: resets the empty counter.
func (a *AdaptivePollInterval) ObserveClaim(count int) {
	if count <= 0 {
		a.ObserveEmpty()
		return
	}
	a.mu.Lock()
	a.emptyCount = 0
	a.mu.Unlock()
}

// ObserveEmpty records a tick that returned no rows.
func (a *AdaptivePollInterval) ObserveEmpty() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.emptyCount < 16 { // cap to avoid overflow
		a.emptyCount++
	}
}

// ObserveDepth updates the most recent queue depth estimate.
func (a *AdaptivePollInterval) ObserveDepth(depth int64) {
	atomic.StoreInt64(&a.depth, depth)
}

// Next returns the next poll interval. Thread-safe.
func (a *AdaptivePollInterval) Next() time.Duration {
	if !a.enabled.Load() {
		return a.base
	}
	a.mu.Lock()
	empty := a.emptyCount
	a.mu.Unlock()
	depth := atomic.LoadInt64(&a.depth)

	// Start from base; extend for emptiness; shrink for depth.
	d := float64(a.base)
	for range empty {
		d *= 2
		if d >= float64(a.maxInterval) {
			d = float64(a.maxInterval)
			break
		}
	}
	if depth > 0 {
		d /= 1 + float64(depth)/100
	}
	return min(max(time.Duration(d), a.minInterval), a.maxInterval)
}
