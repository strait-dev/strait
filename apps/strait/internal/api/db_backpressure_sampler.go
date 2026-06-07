package api

import (
	"sync/atomic"
	"time"
)

// poolBackpressureSampler decouples the connection-pool acquire-wait signal
// from the per-request admission decision so all concurrent requests observe a
// consistent shed/admit verdict.
//
// The earlier implementation computed deltaCount = currentCount - lastCount
// inside the middleware under a mutex and then immediately updated lastCount.
// Under N concurrent requests, only the first request sees the real delta; all
// others (microseconds later) read a near-zero delta against the just-updated
// baseline and admit themselves — even when the underlying pool is in
// active wait. That defeated the whole purpose of acquire-wait-aware shedding
// during the steady-state benchmark.
//
// The sampler fixes this by running a single goroutine that polls the pool's
// cumulative acquire counters on a fixed interval, computes the average wait
// over the elapsed window, and publishes a boolean verdict via an atomic. The
// middleware just reads the atomic — so 1 VU and 1,000 VUs see the same answer
// at the same instant.
type poolBackpressureSampler struct {
	poolStatter PoolStatter
	interval    time.Duration
	threshold   time.Duration

	// shedding is the published admission verdict. Read by the middleware
	// (one atomic load per request); written exclusively by run() on each
	// tick.
	shedding atomic.Bool

	// Last sampled cumulative counter / wait time. Read and written only by
	// run(); no synchronization needed because there is exactly one writer.
	lastCount int64
	lastWait  time.Duration

	stop chan struct{}
	done chan struct{}
}

const (
	// defaultPoolBackpressureSampleInterval is short enough to react to a
	// load spike within ~one tick yet sparse enough that the sampler itself
	// adds no measurable load.
	defaultPoolBackpressureSampleInterval = 100 * time.Millisecond

	// dbBackpressureAcquireWaitThreshold is the per-acquire average wait
	// above which we 503 incoming requests. 50ms is well below the request
	// timeout but a clear signal that the pool is contended.
	dbBackpressureAcquireWaitThreshold = 50 * time.Millisecond
)

func newPoolBackpressureSampler(ps PoolStatter, interval, threshold time.Duration) *poolBackpressureSampler {
	if interval <= 0 {
		interval = defaultPoolBackpressureSampleInterval
	}
	if threshold <= 0 {
		threshold = dbBackpressureAcquireWaitThreshold
	}
	return &poolBackpressureSampler{
		poolStatter: ps,
		interval:    interval,
		threshold:   threshold,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
}

// Start begins sampling. Must be called exactly once. The sampler runs until
// Stop is invoked or the process exits.
func (s *poolBackpressureSampler) Start() {
	go s.run()
}

// Stop halts the sampler and blocks until the goroutine has exited. Safe to
// call multiple times.
func (s *poolBackpressureSampler) Stop() {
	select {
	case <-s.stop:
		// Already stopped.
	default:
		close(s.stop)
	}
	<-s.done
}

// Shedding returns true when the most recent sample window showed acquire-wait
// pressure exceeding the threshold. False until the first non-trivial sample
// completes (initial window establishes the baseline).
func (s *poolBackpressureSampler) Shedding() bool {
	return s.shedding.Load()
}

func (s *poolBackpressureSampler) run() {
	defer close(s.done)

	// Establish the baseline. The first window after Start runs from now
	// until the first tick; we don't have meaningful delta data yet so we
	// stay in admit mode.
	stats := poolBackpressureStats(s.poolStatter)
	s.lastCount = stats.EmptyAcquireCount
	s.lastWait = stats.EmptyAcquireWaitTime

	t := time.NewTicker(s.interval)
	defer t.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.sampleOnce()
		}
	}
}

// sampleOnce reads the current cumulative counters, computes the average wait
// over the window since the last sample, and publishes the shed/admit verdict.
// Pulled out so unit tests can drive the sampler synchronously without timing
// dependencies.
func (s *poolBackpressureSampler) sampleOnce() {
	stats := poolBackpressureStats(s.poolStatter)
	count := stats.EmptyAcquireCount
	wait := stats.EmptyAcquireWaitTime
	deltaCount := count - s.lastCount
	deltaWait := wait - s.lastWait
	s.lastCount = count
	s.lastWait = wait

	if deltaCount <= 0 || deltaWait <= 0 {
		s.shedding.Store(false)
		return
	}
	avgWait := deltaWait / time.Duration(deltaCount)
	s.shedding.Store(avgWait >= s.threshold)
}
