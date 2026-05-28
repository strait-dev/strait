package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	orcstore "strait/internal/store"
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
	if len(retries) != 1 {
		t.Fatalf("scheduled retries = %d, want 1", len(retries))
	}
	if retries[0].attempt != 1 {
		t.Fatalf("scheduled retry attempt = %d, want same attempt 1", retries[0].attempt)
	}

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status updates = %d, want 1", len(calls))
	}
	if calls[0].to != domain.StatusQueued {
		t.Fatalf("status update to = %s, want queued", calls[0].to)
	}
	if calls[0].to == domain.StatusSystemFailed {
		t.Fatal("step visibility race must not system-fail the run")
	}
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
	if !errors.Is(err, orcstore.ErrWorkflowStepRunNotFound) {
		t.Fatalf("resolveExecutionPolicy error = %v, want ErrWorkflowStepRunNotFound", err)
	}
}
