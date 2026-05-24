package worker

import (
	"log/slog"
	"testing"
	"time"
)

func TestCheckMemoryPressure_DisabledThreshold(t *testing.T) {
	t.Parallel()
	e := &Executor{logger: slog.Default()}
	if e.checkMemoryPressure() {
		t.Fatal("checkMemoryPressure() = true with threshold <= 0, want false")
	}
}

func TestCheckMemoryPressure_ReusesRecentVerdict(t *testing.T) {
	t.Parallel()
	e := &Executor{logger: slog.Default(), memoryPressureThreshold: 90}

	// Seed a recent sample whose cached verdict is "pressured". A call within
	// the sample interval must return the cached verdict without re-sampling.
	e.memStatsLastSample.Store(time.Now().UnixNano())
	e.memStatsPressure.Store(true)
	if !e.checkMemoryPressure() {
		t.Fatal("checkMemoryPressure() = false, want cached true verdict")
	}

	// Flip the cached verdict; a fresh call still inside the interval must
	// reflect the cache rather than performing a new ReadMemStats.
	e.memStatsPressure.Store(false)
	if e.checkMemoryPressure() {
		t.Fatal("checkMemoryPressure() = true, want cached false verdict")
	}
}

func TestCheckMemoryPressure_ResamplesAfterInterval(t *testing.T) {
	t.Parallel()
	e := &Executor{logger: slog.Default(), memoryPressureThreshold: 99.9}

	// A stale sample (older than the interval) forces a real ReadMemStats. With
	// a near-100% threshold the live heap is almost certainly under it, so the
	// stale cached "true" must be overwritten with a freshly sampled "false".
	e.memStatsLastSample.Store(time.Now().Add(-2 * memStatsSampleInterval).UnixNano())
	e.memStatsPressure.Store(true)
	if e.checkMemoryPressure() {
		t.Fatal("checkMemoryPressure() = true after resample under a 99.9% threshold, want false")
	}
	if e.memStatsLastSample.Load() == 0 {
		t.Fatal("expected memStatsLastSample to be updated after resample")
	}
}
