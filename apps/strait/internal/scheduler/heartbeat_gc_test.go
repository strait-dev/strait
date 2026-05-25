package scheduler

import (
	"context"
	"errors"
	"github.com/sourcegraph/conc"
	"testing"
	"time"
)

type fakeGCStore struct {
	deleted  int64
	err      error
	panicRun bool
	calls    int
}

func (f *fakeGCStore) DeleteOrphanedHeartbeats(_ context.Context, _ int) (int64, error) {
	f.calls++
	if f.panicRun {
		panic("heartbeat store panic")
	}
	return f.deleted, f.err
}

func TestHeartbeatGC_Defaults(t *testing.T) {
	g := NewHeartbeatGC(&fakeGCStore{}, HeartbeatGCConfig{})
	if g.interval != time.Hour {
		t.Errorf("interval = %v", g.interval)
	}
	if g.batchLimit != 10000 {
		t.Errorf("batchLimit = %d", g.batchLimit)
	}
}

func TestHeartbeatGC_RunOnceAccumulates(t *testing.T) {
	s := &fakeGCStore{deleted: 17}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	_ = g.runOnce(context.Background())
	if g.TotalDeleted() != 17 {
		t.Errorf("total = %d, want 17", g.TotalDeleted())
	}
	if s.calls != 1 {
		t.Errorf("calls = %d", s.calls)
	}
}

func TestHeartbeatGC_LockNotAcquired(t *testing.T) {
	s := &fakeGCStore{deleted: 5}
	locker := &fakeLocker{acquireOK: false}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{}).WithAdvisoryLocker(locker)
	_ = g.runOnce(context.Background())
	if s.calls != 0 {
		t.Errorf("store should not be called when lock not acquired, got %d", s.calls)
	}
}

func TestHeartbeatGC_LockAcquired(t *testing.T) {
	s := &fakeGCStore{deleted: 5}
	locker := &fakeLocker{acquireOK: true}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{}).WithAdvisoryLocker(locker)
	_ = g.runOnce(context.Background())
	if s.calls != 1 || !locker.acquired || !locker.released {
		t.Errorf("lock workflow broken: calls=%d acquired=%v released=%v",
			s.calls, locker.acquired, locker.released)
	}
}

func TestHeartbeatGC_DeleteError(t *testing.T) {
	s := &fakeGCStore{err: errors.New("oops")}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	if err := g.runOnce(context.Background()); err == nil {
		t.Error("expected error propagation")
	}
}

func TestHeartbeatGC_PanicReturnsError(t *testing.T) {
	s := &fakeGCStore{panicRun: true}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	if err := g.runOnce(context.Background()); err == nil {
		t.Fatal("runOnce error = nil, want recovered panic error")
	}
	if g.Iterations() != 1 {
		t.Fatalf("iterations = %d, want 1", g.Iterations())
	}
}

func TestHeartbeatGC_RunExitsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	s := &fakeGCStore{}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		g.Run(ctx)
		close(done)
	})
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
