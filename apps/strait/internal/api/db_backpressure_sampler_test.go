package api

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakePoolStatter lets tests drive the sampler with deterministic counters.
type fakePoolStatter struct {
	mu              sync.Mutex
	acquired        int32
	maxConns        int32
	count           int64
	waitTotal       time.Duration
	emptyCountReads chan struct{}
}

func (f *fakePoolStatter) AcquiredConns() int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.acquired
}

func (f *fakePoolStatter) MaxConns() int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxConns
}

func (f *fakePoolStatter) EmptyAcquireCount() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.emptyCountReads != nil {
		select {
		case f.emptyCountReads <- struct{}{}:
		default:
		}
	}
	return f.count
}

func (f *fakePoolStatter) EmptyAcquireWaitTime() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.waitTotal
}

func (f *fakePoolStatter) set(count int64, waitTotal time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count, f.waitTotal = count, waitTotal
}

func (f *fakePoolStatter) setOccupancy(acquired, max int32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquired, f.maxConns = acquired, max
}

// Baseline tick (no delta) keeps shedding false.
func TestPoolBackpressureSampler_NoDeltaAdmits(t *testing.T) {
	ps := &fakePoolStatter{}
	s := newPoolBackpressureSampler(ps, time.Second, dbBackpressureAcquireWaitThreshold)
	s.lastCount = ps.EmptyAcquireCount()
	s.lastWait = ps.EmptyAcquireWaitTime()

	s.sampleOnce() // counters unchanged → no signal
	if s.Shedding() {
		t.Fatal("sampler reports shedding with no delta")
	}
}

// Avg wait below threshold keeps shedding false; at or above the threshold
// flips it to true.
func TestPoolBackpressureSampler_ThresholdGate(t *testing.T) {
	ps := &fakePoolStatter{}
	s := newPoolBackpressureSampler(ps, time.Second, 50*time.Millisecond)
	s.lastCount = 0
	s.lastWait = 0

	// 10 acquires, total wait 400ms → avg 40ms (below threshold).
	ps.set(10, 400*time.Millisecond)
	s.sampleOnce()
	if s.Shedding() {
		t.Fatal("sampler should admit at avg 40ms (< 50ms threshold)")
	}

	// 5 more acquires, +300ms wait → avg 60ms in window (≥ threshold).
	ps.set(15, 700*time.Millisecond)
	s.sampleOnce()
	if !s.Shedding() {
		t.Fatal("sampler should shed at avg 60ms (≥ 50ms threshold)")
	}

	// Wait subsides: next window adds counts but no extra wait → reset.
	ps.set(20, 700*time.Millisecond)
	s.sampleOnce()
	if s.Shedding() {
		t.Fatal("sampler should admit once wait stops growing")
	}
}

// All concurrent readers must observe the same verdict — this is the property
// the old delta-in-middleware design failed to provide.
func TestPoolBackpressureSampler_ConcurrentReadsAreConsistent(t *testing.T) {
	ps := &fakePoolStatter{}
	s := newPoolBackpressureSampler(ps, time.Second, 50*time.Millisecond)
	ps.set(0, 0)
	s.sampleOnce()

	// Force shedding state.
	ps.set(10, 1*time.Second) // avg = 100ms
	s.sampleOnce()
	if !s.Shedding() {
		t.Fatal("setup: expected shedding")
	}

	const fanout = 200
	var sheds atomic.Int32
	var wg sync.WaitGroup
	wg.Add(fanout)
	for range fanout {
		go func() {
			defer wg.Done()
			if s.Shedding() {
				sheds.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := sheds.Load(); got != fanout {
		t.Fatalf("concurrent readers saw shedding %d/%d times, want all %d", got, fanout, fanout)
	}
}

// Stop must release the goroutine and not block on a second call.
func TestPoolBackpressureSampler_StopIsIdempotent(t *testing.T) {
	ps := &fakePoolStatter{emptyCountReads: make(chan struct{}, 2)}
	s := newPoolBackpressureSampler(ps, 10*time.Millisecond, dbBackpressureAcquireWaitThreshold)
	s.Start()
	waitForEmptyAcquireCountReads(t, ps.emptyCountReads, 2)
	s.Stop()
	s.Stop() // second call must not block or panic
}

func waitForEmptyAcquireCountReads(t *testing.T, ch <-chan struct{}, want int) {
	t.Helper()
	timeout := time.After(time.Second)
	for got := 0; got < want; got++ {
		select {
		case <-ch:
		case <-timeout:
			t.Fatalf("timed out waiting for %d EmptyAcquireCount reads, got %d", want, got)
		}
	}
}

// Verifies that shouldApplyDBBackpressure reaches the same verdict regardless
// of how many concurrent callers fire — the bug N7 fixes is that the previous
// implementation admitted under load whenever any other caller had just
// updated the baseline within the same instant.
func TestShouldApplyDBBackpressure_AllConcurrentRequestsAgreeUnderPressure(t *testing.T) {
	ps := &fakePoolStatter{}
	ps.setOccupancy(0, 100) // occupancy clear, so the verdict is driven purely by the sampler
	srv := &Server{poolStatter: ps}
	srv.poolBackpressure = newPoolBackpressureSampler(ps, time.Second, 50*time.Millisecond)

	// Seed shedding=true via one synchronous sample tick.
	srv.poolBackpressure.lastCount = 0
	srv.poolBackpressure.lastWait = 0
	ps.set(10, 1*time.Second) // avg 100ms wait
	srv.poolBackpressure.sampleOnce()
	if !srv.poolBackpressure.Shedding() {
		t.Fatal("setup: expected sampler to be in shedding state")
	}

	const fanout = 200
	var admitted atomic.Int32
	var wg sync.WaitGroup
	wg.Add(fanout)
	for range fanout {
		go func() {
			defer wg.Done()
			if !srv.shouldApplyDBBackpressure() {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != 0 {
		t.Fatalf("admitted %d/%d requests while sampler was shedding; old delta-in-middleware would admit ~all-but-one", admitted.Load(), fanout)
	}
}

// Occupancy >90% should shed regardless of sampler verdict — the snapshot
// signal is an independent safety net.
func TestShouldApplyDBBackpressure_HighOccupancyShortCircuits(t *testing.T) {
	ps := &fakePoolStatter{}
	ps.setOccupancy(91, 100)
	srv := &Server{poolStatter: ps}
	srv.poolBackpressure = newPoolBackpressureSampler(ps, time.Second, 50*time.Millisecond)
	// Sampler is in admit state (no data) but occupancy alone should shed.
	if !srv.shouldApplyDBBackpressure() {
		t.Fatal("expected shedding when acquired conns > 90% of max")
	}
}
