package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeIdempotencyGCStore struct {
	deleted int64
	err     error
	calls   int
}

func (f *fakeIdempotencyGCStore) DeleteExpiredIdempotencyEntries(_ context.Context, _ int) (int64, error) {
	f.calls++
	return f.deleted, f.err
}

func TestIdempotencyGC_Defaults(t *testing.T) {
	g := NewIdempotencyGC(&fakeIdempotencyGCStore{}, IdempotencyGCConfig{})
	if g.interval != time.Hour {
		t.Errorf("interval = %v", g.interval)
	}
	if g.batchLimit != 10000 {
		t.Errorf("batchLimit = %d", g.batchLimit)
	}
}

func TestIdempotencyGC_RunOnceAccumulates(t *testing.T) {
	s := &fakeIdempotencyGCStore{deleted: 23}
	g := NewIdempotencyGC(s, IdempotencyGCConfig{})
	_ = g.runOnce(context.Background())
	if g.TotalDeleted() != 23 {
		t.Errorf("total = %d, want 23", g.TotalDeleted())
	}
	if s.calls != 1 {
		t.Errorf("calls = %d", s.calls)
	}
}

func TestIdempotencyGC_LockNotAcquired(t *testing.T) {
	s := &fakeIdempotencyGCStore{deleted: 9}
	locker := &fakeLocker{acquireOK: false}
	g := NewIdempotencyGC(s, IdempotencyGCConfig{}).WithAdvisoryLocker(locker)
	_ = g.runOnce(context.Background())
	if s.calls != 0 {
		t.Errorf("store should not be called when lock not acquired, got %d", s.calls)
	}
}

func TestIdempotencyGC_LockAcquiredAndReleased(t *testing.T) {
	s := &fakeIdempotencyGCStore{deleted: 4}
	locker := &fakeLocker{acquireOK: true}
	g := NewIdempotencyGC(s, IdempotencyGCConfig{}).WithAdvisoryLocker(locker)
	_ = g.runOnce(context.Background())
	if s.calls != 1 || !locker.acquired || !locker.released {
		t.Errorf("lock workflow broken: calls=%d acquired=%v released=%v",
			s.calls, locker.acquired, locker.released)
	}
}

func TestIdempotencyGC_DeleteError(t *testing.T) {
	s := &fakeIdempotencyGCStore{err: errors.New("boom")}
	g := NewIdempotencyGC(s, IdempotencyGCConfig{})
	if err := g.runOnce(context.Background()); err == nil {
		t.Error("expected error propagation")
	}
}

func TestIdempotencyGC_RunExitsOnCancel(t *testing.T) {
	s := &fakeIdempotencyGCStore{}
	g := NewIdempotencyGC(s, IdempotencyGCConfig{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		g.Run(ctx)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("did not exit on cancel")
	}
	if g.Iterations() < 2 {
		t.Errorf("iterations = %d", g.Iterations())
	}
}
