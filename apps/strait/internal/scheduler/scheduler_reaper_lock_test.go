package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestWithReaperAdvisoryLocker_WiresReaper(t *testing.T) {
	t.Parallel()

	reaper := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, true, nil)
	sched := &Scheduler{reaper: reaper}
	locker := &mockAdvisoryLocker{}

	WithReaperAdvisoryLocker(locker)(sched)
	require.Equal(t, locker,
		sched.
			reaper.
			advisoryLocker,
	)
}

func TestReaperRun_UsesPinnedAdvisoryLockRunner(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		r.Run(ctx)
		close(done)
	})

	select {
	case <-runner.called:
	case <-time.After(time.Second):
		require.Fail(t, "reaper did not use advisory lock runner")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.Fail(t, "reaper did not stop after context cancellation")
	}
	require.Equal(t, reaperAdvisoryLockID,

		runner.lockID)
	require.NotEqual(t,
		0, runner.
			fnCalls)
}

func TestReaperRun_SkipsWhenPinnedRunnerDoesNotAcquire(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		r.Run(ctx)
		close(done)
	})

	select {
	case <-runner.called:
	case <-time.After(time.Second):
		require.Fail(t, "reaper did not call advisory lock runner")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "reaper did not stop after context cancellation")
	}
	require.Equal(t, 0,
		runner.fnCalls,
	)
}

func TestReaperRun_LogsFallbackAdvisoryLockCheckError(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	locker := &reaperFallbackAdvisoryLocker{
		tryErr:        errors.New("lock check failed"),
		cancel:        cancel,
		tryCalled:     make(chan struct{}, 1),
		releaseSignal: make(chan struct{}, 1),
	}
	r := NewReaper(&mockReaperStore{}, time.Millisecond, 30*time.Second, 0, 0, false, nil).
		WithAdvisoryLocker(locker)

	done := make(chan struct{})
	concWG.Go(func() {
		r.Run(ctx)
		close(done)
	})

	select {
	case <-locker.tryCalled:
	case <-time.After(time.Second):
		require.Fail(t, "reaper did not attempt fallback advisory lock")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "reaper did not stop after fallback lock error")
	}
	require.Equal(t, reaperAdvisoryLockID, locker.tryLockID)
	require.False(t, locker.releaseCalled)
}

func TestReaperRun_LogsFallbackAdvisoryLockReleaseError(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	locker := &reaperFallbackAdvisoryLocker{
		acquired:      true,
		releaseErr:    errors.New("release failed"),
		cancel:        cancel,
		tryCalled:     make(chan struct{}, 1),
		releaseSignal: make(chan struct{}, 1),
	}
	r := NewReaper(&mockReaperStore{}, time.Millisecond, 30*time.Second, 0, 0, false, nil).
		WithAdvisoryLocker(locker)

	done := make(chan struct{})
	concWG.Go(func() {
		r.Run(ctx)
		close(done)
	})

	select {
	case <-locker.releaseSignal:
	case <-time.After(time.Second):
		require.Fail(t, "reaper did not release fallback advisory lock")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "reaper did not stop after fallback release")
	}
	require.Equal(t, reaperAdvisoryLockID, locker.releaseLockID)
	require.True(t, locker.releaseCalled)
}

type reaperFallbackAdvisoryLocker struct {
	acquired      bool
	tryErr        error
	releaseErr    error
	cancel        context.CancelFunc
	tryCalled     chan struct{}
	releaseSignal chan struct{}
	tryLockID     int64
	releaseLockID int64
	releaseCalled bool
}

func (m *reaperFallbackAdvisoryLocker) TryAdvisoryLock(_ context.Context, lockID int64) (bool, error) {
	if m.tryCalled == nil {
		m.tryCalled = make(chan struct{}, 1)
	}
	if m.releaseSignal == nil {
		m.releaseSignal = make(chan struct{}, 1)
	}
	m.tryLockID = lockID
	m.tryCalled <- struct{}{}
	if m.tryErr != nil && m.cancel != nil {
		m.cancel()
	}
	return m.acquired, m.tryErr
}

func (m *reaperFallbackAdvisoryLocker) ReleaseAdvisoryLock(_ context.Context, lockID int64) error {
	if m.releaseSignal == nil {
		m.releaseSignal = make(chan struct{}, 1)
	}
	m.releaseLockID = lockID
	m.releaseCalled = true
	m.releaseSignal <- struct{}{}
	if m.cancel != nil {
		m.cancel()
	}
	return m.releaseErr
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
