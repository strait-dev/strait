package scheduler

import (
	"context"
	"errors"
	"github.com/sourcegraph/conc"
	"testing"
	"time"
)

type fakePartitionStore struct {
	err      error
	panicRun bool
	calls    int
}

func (f *fakePartitionStore) EnsureJobRunsPartitions(_ context.Context, _ int) error {
	f.calls++
	if f.panicRun {
		panic("partition store panic")
	}
	return f.err
}

func (f *fakePartitionStore) EnsureOutboxHistoryPartitions(_ context.Context, _ int) error {
	return f.err
}

func TestPartitionEnsurer_Defaults(t *testing.T) {
	p := NewPartitionEnsurer(&fakePartitionStore{}, PartitionEnsurerConfig{})
	if p.interval != 24*time.Hour {
		t.Errorf("interval = %v", p.interval)
	}
	if p.monthsAhead != 2 {
		t.Errorf("months = %d", p.monthsAhead)
	}
}

func TestPartitionEnsurer_RunOnceHappyPath(t *testing.T) {
	s := &fakePartitionStore{}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{MonthsAhead: 3})
	if err := p.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if s.calls != 1 {
		t.Errorf("calls = %d", s.calls)
	}
	if p.Iterations() != 1 {
		t.Errorf("iterations = %d", p.Iterations())
	}
	if p.Errors() != 0 {
		t.Errorf("errors = %d", p.Errors())
	}
}

func TestPartitionEnsurer_StoreErrorAccumulates(t *testing.T) {
	s := &fakePartitionStore{err: errors.New("create partition failed")}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{})
	_ = p.runOnce(context.Background())
	_ = p.runOnce(context.Background())
	if p.Errors() != 2 {
		t.Errorf("errors = %d", p.Errors())
	}
}

func TestPartitionEnsurer_PanicReturnsError(t *testing.T) {
	s := &fakePartitionStore{panicRun: true}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{})
	if err := p.runOnce(context.Background()); err == nil {
		t.Fatal("runOnce error = nil, want recovered panic error")
	}
	if p.Errors() != 1 {
		t.Fatalf("errors = %d, want 1", p.Errors())
	}
}

func TestPartitionEnsurer_RunOnceForTestPropagatesRecoveredPanic(t *testing.T) {
	s := &fakePartitionStore{panicRun: true}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{})
	if err := p.RunOnceForTest(context.Background()); err == nil {
		t.Fatal("RunOnceForTest error = nil, want recovered panic error")
	}
	if p.Iterations() != 1 {
		t.Fatalf("iterations = %d, want 1", p.Iterations())
	}
	if p.Errors() != 1 {
		t.Fatalf("errors = %d, want 1", p.Errors())
	}
}

func TestPartitionEnsurer_LockNotAcquired(t *testing.T) {
	s := &fakePartitionStore{}
	locker := &fakeLocker{acquireOK: false}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{}).WithAdvisoryLocker(locker)
	_ = p.runOnce(context.Background())
	if s.calls != 0 {
		t.Errorf("store called despite missing lock: %d", s.calls)
	}
}

func TestPartitionEnsurer_LockAcquiredAndReleased(t *testing.T) {
	s := &fakePartitionStore{}
	locker := &fakeLocker{acquireOK: true}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{}).WithAdvisoryLocker(locker)
	_ = p.runOnce(context.Background())
	if !locker.acquired || !locker.released {
		t.Errorf("lock not used: acquired=%v released=%v", locker.acquired, locker.released)
	}
}

func TestPartitionEnsurer_RunExitsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	s := &fakePartitionStore{}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		p.Run(ctx)
		close(done)
	})
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("did not exit on cancel")
	}
	if p.Iterations() < 2 {
		t.Errorf("iterations = %d", p.Iterations())
	}
}
