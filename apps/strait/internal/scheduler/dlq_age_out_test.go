package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDLQAgeOutStore struct {
	masked int64
	err    error
	calls  int
}

func (f *fakeDLQAgeOutStore) MaskOldDLQRows(_ context.Context, _ time.Duration, _ int) (int64, error) {
	f.calls++
	return f.masked, f.err
}

func TestDLQAgeOut_Defaults(t *testing.T) {
	a := NewDLQAgeOut(&fakeDLQAgeOutStore{}, DLQAgeOutConfig{})
	if a.interval != 24*time.Hour {
		t.Errorf("interval = %v", a.interval)
	}
	if a.retention != 30*24*time.Hour {
		t.Errorf("retention = %v", a.retention)
	}
	if a.batchLimit != 1000 {
		t.Errorf("batchLimit = %d", a.batchLimit)
	}
}

func TestDLQAgeOut_RunOnceMasksRows(t *testing.T) {
	s := &fakeDLQAgeOutStore{masked: 7}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{Retention: time.Hour, BatchLimit: 100})
	if err := a.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if s.calls != 1 {
		t.Errorf("calls = %d", s.calls)
	}
	if a.TotalMasked() != 7 {
		t.Errorf("masked = %d", a.TotalMasked())
	}
}

func TestDLQAgeOut_AccumulatesAcrossCycles(t *testing.T) {
	s := &fakeDLQAgeOutStore{masked: 3}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{})
	_ = a.runOnce(context.Background())
	_ = a.runOnce(context.Background())
	if a.TotalMasked() != 6 {
		t.Errorf("total = %d, want 6", a.TotalMasked())
	}
}

func TestDLQAgeOut_StoreErrorPropagates(t *testing.T) {
	s := &fakeDLQAgeOutStore{err: errors.New("locked")}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{})
	if err := a.runOnce(context.Background()); err == nil {
		t.Error("expected error")
	}
}

func TestDLQAgeOut_LockNotAcquired(t *testing.T) {
	s := &fakeDLQAgeOutStore{masked: 5}
	locker := &fakeLocker{acquireOK: false}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{}).WithAdvisoryLocker(locker)
	_ = a.runOnce(context.Background())
	if s.calls != 0 {
		t.Errorf("should not call store without lock, got %d", s.calls)
	}
}

func TestDLQAgeOut_LockAcquiredAndReleased(t *testing.T) {
	s := &fakeDLQAgeOutStore{masked: 5}
	locker := &fakeLocker{acquireOK: true}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{}).WithAdvisoryLocker(locker)
	_ = a.runOnce(context.Background())
	if !locker.acquired || !locker.released {
		t.Errorf("lock workflow broken")
	}
}

// scanningDLQStore implements DLQAgeOutStore and DLQPartitionScanner to
// exercise the parallel-scan path. It tracks peak concurrent scans and
// verifies that ScanDLQPartitionCandidates runs before MaskOldDLQRows.
type scanningDLQStore struct {
	partitions []string
	masked     int64
	scanDelay  time.Duration

	mu                sync.Mutex
	scannedPartitions []string
	maskCalls         int
	scansFinishedAt   time.Time
	maskCalledAt      time.Time
	active            atomic.Int32
	peak              atomic.Int32
}

func (s *scanningDLQStore) MaskOldDLQRows(_ context.Context, _ time.Duration, _ int) (int64, error) {
	s.mu.Lock()
	s.maskCalls++
	s.maskCalledAt = time.Now()
	s.mu.Unlock()
	return s.masked, nil
}

func (s *scanningDLQStore) ListDLQPartitions(_ context.Context) ([]string, error) {
	return s.partitions, nil
}

func (s *scanningDLQStore) ScanDLQPartitionCandidates(_ context.Context, partition string, _ time.Duration, _ int) (int64, error) {
	n := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		peak := s.peak.Load()
		if n <= peak || s.peak.CompareAndSwap(peak, n) {
			break
		}
	}
	time.Sleep(s.scanDelay)
	s.mu.Lock()
	s.scannedPartitions = append(s.scannedPartitions, partition)
	s.scansFinishedAt = time.Now()
	s.mu.Unlock()
	return 1, nil
}

func TestDLQAgeOut_ParallelPartitionScan(t *testing.T) {
	parts := []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8"}
	s := &scanningDLQStore{partitions: parts, masked: 3, scanDelay: 10 * time.Millisecond}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{})
	start := time.Now()
	if err := a.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	elapsed := time.Since(start)
	// Serial = 80ms, parallel (pool 4) ~= 20ms + slack.
	if elapsed > 70*time.Millisecond {
		t.Errorf("expected parallel scans, elapsed=%v", elapsed)
	}
	if peak := s.peak.Load(); peak < 2 {
		t.Errorf("expected concurrent scans, peak=%d", peak)
	}
	if peak := s.peak.Load(); peak > dlqAgeOutScanPoolSize {
		t.Errorf("peak=%d exceeds pool %d", peak, dlqAgeOutScanPoolSize)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.scannedPartitions) != len(parts) {
		t.Errorf("scanned %d partitions, want %d", len(s.scannedPartitions), len(parts))
	}
	if s.maskCalls != 1 {
		t.Errorf("mask calls = %d, want 1", s.maskCalls)
	}
	// Scans should finish before the serial mask.
	if !s.maskCalledAt.After(s.scansFinishedAt) && !s.maskCalledAt.Equal(s.scansFinishedAt) {
		t.Errorf("mask (%v) ran before scans finished (%v)", s.maskCalledAt, s.scansFinishedAt)
	}
	if a.TotalMasked() != 3 {
		t.Errorf("masked = %d, want 3", a.TotalMasked())
	}
}

func TestDLQAgeOut_RunExitsOnCancel(t *testing.T) {
	s := &fakeDLQAgeOutStore{}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.Run(ctx)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("did not exit")
	}
}
