package worker

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestSnoozeRun_LockedRowSkipsCleanly(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{
		snoozeRunWithLockFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return store.ErrRunLocked
		},
	}
	exec := newSnoozeTestExecutor(t, st, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued

	// Must not panic, must not propagate the locked error to the caller. The
	// only observable effect is the appended statusUpdate call from the mock.
	exec.snoozeRun(context.Background(), run, "raced with reaper", nil)

	if got := len(st.statusUpdates()); got != 1 {
		t.Fatalf("expected 1 SnoozeRunWithLock call, got %d", got)
	}
}

func TestSnoozeRun_ConflictRowSkipsCleanly(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{
		snoozeRunWithLockFn: func(_ context.Context, _ string, from, _ domain.RunStatus, _ map[string]any) error {
			return errors.Join(store.ErrRunConflict, errors.New("status moved on from "+string(from)))
		},
	}
	exec := newSnoozeTestExecutor(t, st, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	exec.snoozeRun(context.Background(), run, "raced with completion", nil)

	if got := len(st.statusUpdates()); got != 1 {
		t.Fatalf("expected 1 SnoozeRunWithLock call, got %d", got)
	}
}

func TestSnoozeRunFromExecuting_LockedRowSkipsCleanly(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{
		snoozeRunWithLockFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return store.ErrRunLocked
		},
	}
	exec := newSnoozeTestExecutor(t, st, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	exec.snoozeRunFromExecuting(context.Background(), run, "watchdog tick", nil)

	if got := len(st.statusUpdates()); got != 1 {
		t.Fatalf("expected 1 SnoozeRunWithLock call, got %d", got)
	}
}

func TestSnoozeRun_GenuineErrorStillLogged(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{
		snoozeRunWithLockFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return errors.New("db connection refused")
		},
	}
	exec := newSnoozeTestExecutor(t, st, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	// Must not panic. The path logs an error and returns; no emit.
	exec.snoozeRun(context.Background(), run, "snooze under failure", nil)

	if got := len(st.statusUpdates()); got != 1 {
		t.Fatalf("expected 1 SnoozeRunWithLock call, got %d", got)
	}
}
