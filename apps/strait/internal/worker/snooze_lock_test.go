package worker

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
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
	require.Len(t, st.
		statusUpdates(), 1)
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
	require.Len(t, st.
		statusUpdates(), 1)
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
	require.Len(t, st.
		statusUpdates(), 1)
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
	require.Len(t, st.
		statusUpdates(), 1)
}

func TestDeepSecSnoozeRun_ExecutingClaimTableRunUsesExecutingSource(t *testing.T) {
	t.Parallel()

	st := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, st, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	exec.snoozeRun(context.Background(), run, "endpoint circuit breaker open", nil)

	calls := st.statusUpdates()
	require.Len(t, calls,
		1)
	require.Equal(t,
		domain.StatusExecuting,

		calls[0].from)
}
