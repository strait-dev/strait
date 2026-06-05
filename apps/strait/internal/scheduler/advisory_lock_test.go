package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type testAdvisoryLocker struct {
	acquired     bool
	tryCalls     int
	releaseCalls int
	releaseErr   error
}

func (l *testAdvisoryLocker) TryAdvisoryLock(context.Context, int64) (bool, error) {
	l.tryCalls++
	return l.acquired, nil
}

func (l *testAdvisoryLocker) ReleaseAdvisoryLock(context.Context, int64) error {
	l.releaseCalls++
	return l.releaseErr
}

type testAdvisoryRunner struct {
	testAdvisoryLocker
	acquired bool
	runCalls int
}

func (r *testAdvisoryRunner) RunWithAdvisoryLock(ctx context.Context, lockID int64, fn func(context.Context) error) (bool, error) {
	r.runCalls++
	if !r.acquired {
		return false, nil
	}
	return true, fn(ctx)
}

func TestRunWithOptionalAdvisoryLock_PrefersPinnedRunner(t *testing.T) {
	t.Parallel()

	runner := &testAdvisoryRunner{acquired: true}
	ran := false
	acquired, err := runWithOptionalAdvisoryLock(t.Context(), runner, 123, func(context.Context) error {
		ran = true
		return nil
	})
	require.NoError(t,
		err)
	require.False(t, !acquired ||
		!ran)
	require.Equal(t, 1,
		runner.runCalls,
	)
	require.False(t, runner.
		tryCalls !=
		0 ||
		runner.
			releaseCalls != 0)
}

func TestRunWithOptionalAdvisoryLock_RunnerNotAcquiredSkipsWork(t *testing.T) {
	t.Parallel()

	runner := &testAdvisoryRunner{acquired: false}
	ran := false
	acquired, err := runWithOptionalAdvisoryLock(t.Context(), runner, 123, func(context.Context) error {
		ran = true
		return nil
	})
	require.NoError(t,
		err)
	require.False(t, acquired ||
		ran)
}

func TestRunWithOptionalAdvisoryLock_FallbackReleasesAfterWorkError(t *testing.T) {
	t.Parallel()

	workErr := errors.New("work failed")
	locker := &testAdvisoryLocker{acquired: true}
	acquired, err := runWithOptionalAdvisoryLock(t.Context(), locker, 123, func(context.Context) error {
		return workErr
	})
	require.True(t, acquired)
	require.ErrorIs(t, err, workErr)
	require.False(t, locker.
		tryCalls !=
		1 ||
		locker.
			releaseCalls != 1)
}

func TestRunWithOptionalAdvisoryLock_FallbackSurfacesReleaseError(t *testing.T) {
	t.Parallel()

	releaseErr := errors.New("release failed")
	locker := &testAdvisoryLocker{acquired: true, releaseErr: releaseErr}
	acquired, err := runWithOptionalAdvisoryLock(t.Context(), locker, 123, func(context.Context) error {
		return nil
	})
	require.True(t, acquired)
	require.ErrorIs(t, err, releaseErr)
}

func TestRunWithOptionalAdvisoryLock_FallbackReleasesAfterPanic(t *testing.T) {
	t.Parallel()

	locker := &testAdvisoryLocker{acquired: true}
	defer func() {
		require.NotNil(t,
			recover())
		require.False(t, locker.
			tryCalls !=
			1 ||
			locker.
				releaseCalls != 1)
	}()

	_, _ = runWithOptionalAdvisoryLock(t.Context(), locker, 123, func(context.Context) error {
		panic("locked section failed")
	})
}
