package scheduler

import (
	"context"
	"errors"
	"testing"
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
	if err != nil {
		t.Fatalf("runWithOptionalAdvisoryLock error = %v", err)
	}
	if !acquired || !ran {
		t.Fatalf("acquired=%v ran=%v, want acquired and ran", acquired, ran)
	}
	if runner.runCalls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.runCalls)
	}
	if runner.tryCalls != 0 || runner.releaseCalls != 0 {
		t.Fatalf("fallback lock calls used with runner: try=%d release=%d", runner.tryCalls, runner.releaseCalls)
	}
}

func TestRunWithOptionalAdvisoryLock_RunnerNotAcquiredSkipsWork(t *testing.T) {
	t.Parallel()

	runner := &testAdvisoryRunner{acquired: false}
	ran := false
	acquired, err := runWithOptionalAdvisoryLock(t.Context(), runner, 123, func(context.Context) error {
		ran = true
		return nil
	})
	if err != nil {
		t.Fatalf("runWithOptionalAdvisoryLock error = %v", err)
	}
	if acquired || ran {
		t.Fatalf("acquired=%v ran=%v, want skipped", acquired, ran)
	}
}

func TestRunWithOptionalAdvisoryLock_FallbackReleasesAfterWorkError(t *testing.T) {
	t.Parallel()

	workErr := errors.New("work failed")
	locker := &testAdvisoryLocker{acquired: true}
	acquired, err := runWithOptionalAdvisoryLock(t.Context(), locker, 123, func(context.Context) error {
		return workErr
	})
	if !acquired {
		t.Fatal("acquired = false, want true")
	}
	if !errors.Is(err, workErr) {
		t.Fatalf("error = %v, want work error", err)
	}
	if locker.tryCalls != 1 || locker.releaseCalls != 1 {
		t.Fatalf("try/release calls = %d/%d, want 1/1", locker.tryCalls, locker.releaseCalls)
	}
}

func TestRunWithOptionalAdvisoryLock_FallbackSurfacesReleaseError(t *testing.T) {
	t.Parallel()

	releaseErr := errors.New("release failed")
	locker := &testAdvisoryLocker{acquired: true, releaseErr: releaseErr}
	acquired, err := runWithOptionalAdvisoryLock(t.Context(), locker, 123, func(context.Context) error {
		return nil
	})
	if !acquired {
		t.Fatal("acquired = false, want true")
	}
	if !errors.Is(err, releaseErr) {
		t.Fatalf("error = %v, want release error", err)
	}
}

func TestRunWithOptionalAdvisoryLock_FallbackReleasesAfterPanic(t *testing.T) {
	t.Parallel()

	locker := &testAdvisoryLocker{acquired: true}
	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("expected panic to propagate")
		}
		if locker.tryCalls != 1 || locker.releaseCalls != 1 {
			t.Fatalf("try/release calls = %d/%d, want 1/1", locker.tryCalls, locker.releaseCalls)
		}
	}()

	_, _ = runWithOptionalAdvisoryLock(t.Context(), locker, 123, func(context.Context) error {
		panic("locked section failed")
	})
}
