package worker

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

func newSingletonTestExecutor(t *testing.T, store *mockExecutorStore) *Executor {
	t.Helper()
	pool := NewPool(2)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	return NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             &mockExecQueue{},
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
	})
}

// TestReleaseSingletonLock_CalledWhenRunHeldKey: a terminal run that resolved a
// singleton key triggers the release+promote fast-path.
func TestReleaseSingletonLock_CalledWhenRunHeldKey(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		releaseSingletonAndPromoteFn: func(_ context.Context, _ string) (bool, string, error) {
			return true, "promoted-1", nil
		},
	}
	exec := newSingletonTestExecutor(t, store)

	run := &domain.JobRun{ID: "run-1", SingletonKey: "k"}
	exec.releaseSingletonLock(context.Background(), run)

	calls := store.releaseSingletonCalls()
	if len(calls) != 1 || calls[0] != "run-1" {
		t.Fatalf("expected one release for run-1, got %v", calls)
	}
}

// TestReleaseSingletonLock_SkippedWhenRunHeldNoKey: a run that never resolved a
// singleton key cannot hold a lock, so the fast-path must not touch the store.
// This covers the common non-singleton terminal path and the system-failure
// path (no job in scope) identically.
func TestReleaseSingletonLock_SkippedWhenRunHeldNoKey(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSingletonTestExecutor(t, store)

	run := &domain.JobRun{ID: "run-1"} // no SingletonKey
	exec.releaseSingletonLock(context.Background(), run)

	if calls := store.releaseSingletonCalls(); len(calls) != 0 {
		t.Fatalf("expected no release for run without a singleton key, got %v", calls)
	}
}

// TestReleaseSingletonLock_ReleasesEvenWithoutJob: the system-failure path has no
// job in scope; a run that held a key is still released because the gate is the
// run's recorded key, not the live job config.
func TestReleaseSingletonLock_ReleasesEvenWithoutJob(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSingletonTestExecutor(t, store)

	run := &domain.JobRun{ID: "run-1", SingletonKey: "k"}
	exec.releaseSingletonLock(context.Background(), run)

	if calls := store.releaseSingletonCalls(); len(calls) != 1 {
		t.Fatalf("expected release for run with key and no job, got %v", calls)
	}
}

// TestReleaseSingletonLock_NoopForEmptyRun: guards against a nil/empty run.
func TestReleaseSingletonLock_NoopForEmptyRun(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSingletonTestExecutor(t, store)

	exec.releaseSingletonLock(context.Background(), nil)
	exec.releaseSingletonLock(context.Background(), &domain.JobRun{})
	exec.releaseSingletonLock(context.Background(), &domain.JobRun{ID: "run-1"}) // ID but no key

	if calls := store.releaseSingletonCalls(); len(calls) != 0 {
		t.Fatalf("expected no release for empty run, got %v", calls)
	}
}
