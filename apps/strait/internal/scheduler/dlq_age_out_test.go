package scheduler

import (
	"context"
	"errors"
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
