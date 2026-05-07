package scheduler

import (
	"context"
	"testing"
	"time"
)

func TestWithReaperAdvisoryLocker_WiresReaper(t *testing.T) {
	t.Parallel()

	reaper := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, true, nil)
	sched := &Scheduler{reaper: reaper}
	locker := &mockAdvisoryLocker{}

	WithReaperAdvisoryLocker(locker)(sched)

	if sched.reaper.advisoryLocker != locker {
		t.Fatal("reaper advisory locker was not wired")
	}
}

func TestReaperRun_UsesPinnedAdvisoryLockRunner(t *testing.T) {
	store := &mockReaperStore{}
	runner := &mockAdvisoryLockRunner{
		acquired: true,
		called:   make(chan struct{}, 1),
	}
	r := NewReaper(store, time.Millisecond, 30*time.Second, 0, 0, false, nil).WithAdvisoryLocker(runner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.cancel = cancel

	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	select {
	case <-runner.called:
	case <-time.After(time.Second):
		t.Fatal("reaper did not use advisory lock runner")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reaper did not stop after context cancellation")
	}

	if runner.lockID != reaperAdvisoryLockID {
		t.Fatalf("lockID = %d, want %d", runner.lockID, reaperAdvisoryLockID)
	}
	if runner.fnCalls == 0 {
		t.Fatal("fnCalls = 0, want at least one advisory-locked cycle")
	}
}

func TestReaperRun_SkipsWhenPinnedRunnerDoesNotAcquire(t *testing.T) {
	store := &mockReaperStore{}
	runner := &mockAdvisoryLockRunner{
		acquired: false,
		called:   make(chan struct{}, 1),
	}
	r := NewReaper(store, time.Millisecond, 30*time.Second, 0, 0, false, nil).WithAdvisoryLocker(runner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.cancel = cancel

	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	select {
	case <-runner.called:
	case <-time.After(time.Second):
		t.Fatal("reaper did not call advisory lock runner")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reaper did not stop after context cancellation")
	}

	if runner.fnCalls != 0 {
		t.Fatalf("fnCalls = %d, want 0 when lock is not acquired", runner.fnCalls)
	}
}

type mockAdvisoryLockRunner struct {
	acquired bool
	called   chan struct{}
	cancel   context.CancelFunc
	lockID   int64
	fnCalls  int
}

func (m *mockAdvisoryLockRunner) TryAdvisoryLock(context.Context, int64) (bool, error) {
	panic("TryAdvisoryLock should not be used when RunWithAdvisoryLock is available")
}

func (m *mockAdvisoryLockRunner) ReleaseAdvisoryLock(context.Context, int64) error {
	panic("ReleaseAdvisoryLock should not be used when RunWithAdvisoryLock is available")
}

func (m *mockAdvisoryLockRunner) RunWithAdvisoryLock(ctx context.Context, lockID int64, fn func(context.Context) error) (bool, error) {
	m.lockID = lockID
	if m.acquired {
		m.fnCalls++
		if err := fn(ctx); err != nil {
			return true, err
		}
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.called <- struct{}{}
	return m.acquired, nil
}
