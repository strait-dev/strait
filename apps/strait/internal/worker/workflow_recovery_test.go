package worker

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	orcstore "strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestExecutor_WorkflowStepVisibilityRaceRequeues(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobFn: func(context.Context, string) (*domain.Job, error) {
			return testJob("https://example.com", 3, 30), nil
		},
		getWorkflowStepRunFn: func(context.Context, string) (*domain.WorkflowStepRun, error) {
			return nil, orcstore.ErrWorkflowStepRunNotFound
		},
	}
	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, nil)
	run := testRun(1)
	run.WorkflowStepRunID = "wsr-missing"

	exec.execute(context.Background(), run)

	retries := store.scheduleRetries()
	require.Len(t, retries,
		1)
	require.Equal(t, 1, retries[0].attempt)

	calls := store.statusUpdates()
	require.Len(t, calls,
		1)
	require.Equal(t,
		domain.StatusQueued,

		calls[0].
			to)
	require.NotEqual(t, domain.
		StatusSystemFailed,

		calls[0].to)
}

func TestResolveExecutionPolicy_WrapsMissingStepRunAsTransient(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getWorkflowStepRunFn: func(context.Context, string) (*domain.WorkflowStepRun, error) {
			return nil, nil
		},
	}
	exec := &Executor{store: store, logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: "wsr-missing"}

	_, err := exec.resolveExecutionPolicy(context.Background(), run, executionPolicy{maxAttempts: 3})
	require.ErrorIs(t,
		err, orcstore.
			ErrWorkflowStepRunNotFound)
}
