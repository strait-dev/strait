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

// TestReleaseSingletonLock_CalledForSingletonJob: a terminal run whose job
// carries a singleton policy triggers the release+promote fast-path.
func TestReleaseSingletonLock_CalledForSingletonJob(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		releaseSingletonAndPromoteFn: func(_ context.Context, _ string) (bool, string, error) {
			return true, "promoted-1", nil
		},
	}
	exec := newSingletonTestExecutor(t, store)

	run := &domain.JobRun{ID: "run-1", SingletonKey: "k"}
	job := &domain.Job{ID: "job-1", SingletonOnConflict: domain.SingletonOnConflictQueue}
	exec.releaseSingletonLock(context.Background(), run, job)

	calls := store.releaseSingletonCalls()
	if len(calls) != 1 || calls[0] != "run-1" {
		t.Fatalf("expected one release for run-1, got %v", calls)
	}
}

// TestReleaseSingletonLock_SkippedForNonSingletonJob: when the job carries no
// singleton config the fast-path must not touch the store at all.
func TestReleaseSingletonLock_SkippedForNonSingletonJob(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSingletonTestExecutor(t, store)

	run := &domain.JobRun{ID: "run-1"}
	job := &domain.Job{ID: "job-1"} // no SingletonOnConflict
	exec.releaseSingletonLock(context.Background(), run, job)

	if calls := store.releaseSingletonCalls(); len(calls) != 0 {
		t.Fatalf("expected no release for non-singleton job, got %v", calls)
	}
}

// TestReleaseSingletonLock_NilJobForcesLookup: the system-failure path passes
// job=nil; the fast-path must still attempt the indexed holder lookup.
func TestReleaseSingletonLock_NilJobForcesLookup(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSingletonTestExecutor(t, store)

	run := &domain.JobRun{ID: "run-1"}
	exec.releaseSingletonLock(context.Background(), run, nil)

	if calls := store.releaseSingletonCalls(); len(calls) != 1 {
		t.Fatalf("expected forced lookup for nil job, got %v", calls)
	}
}

// TestReleaseSingletonLock_NoopForEmptyRun: guards against a nil/empty run.
func TestReleaseSingletonLock_NoopForEmptyRun(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSingletonTestExecutor(t, store)

	exec.releaseSingletonLock(context.Background(), nil, nil)
	exec.releaseSingletonLock(context.Background(), &domain.JobRun{}, nil)

	if calls := store.releaseSingletonCalls(); len(calls) != 0 {
		t.Fatalf("expected no release for empty run, got %v", calls)
	}
}
