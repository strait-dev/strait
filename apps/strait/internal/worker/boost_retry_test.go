package worker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

func TestHandleFailure_RetryBoostsPriority(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 2
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 3

	exec.execute(context.Background(), run)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 5, gotPriority)
}

func TestHandleFailure_RetryPriorityCappedAt10(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 3
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 9

	exec.execute(context.Background(), run)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 10, gotPriority)
}

func TestHandleFailure_ZeroBoostNoChange(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 0
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 3

	exec.execute(context.Background(), run)

	requireRetryWithoutPriority(t, store.statusUpdates())
}

func TestHandleFailure_DefaultBoostIsOne(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 1 // default from DB
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 0

	exec.execute(context.Background(), run)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 1, gotPriority)
}

func TestHandleFailure_BoostFromMaxPriority(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 2
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 10

	exec.execute(context.Background(), run)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 10, gotPriority)
}

func TestHandleFailure_BoostExactlyToMax(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 2
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 8

	exec.execute(context.Background(), run)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 10, gotPriority)
}

func TestHandleFailure_LargeBoostValue(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 10
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 0

	exec.execute(context.Background(), run)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 10, gotPriority)
}

func TestHandleFailure_BoostOnHighAttempt(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 6, 5)
	job.RetryPriorityBoost = 1
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(4) // high attempt, still retryable
	run.Priority = 2

	exec.execute(context.Background(), run)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 3, gotPriority)
}

func TestHandleFailure_BoostNotAppliedWhenPoisonPill(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})
	threshold := 3
	errBody := "fail"
	endpointErr := &domain.EndpointError{StatusCode: 500, Body: errBody}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 3, Priority: 3, Metadata: map[string]string{
		"_error_hash":       errorHashForError(endpointErr),
		"_error_hash_count": "2",
	}}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, PoisonPillThreshold: &threshold}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, endpointErr, nil)

	last := requireLastStatusUpdateTo(t, store.statusUpdates(), domain.StatusDeadLetter)
	if _, ok := last.fields["priority"]; ok {
		assert.Fail(t,

			"expected no priority field when poison pill triggers")
	}
}

func TestHandleFailure_BoostAppliedWhenPoisonPillNotTriggered(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})
	// No PoisonPillThreshold set, so poison pill is disabled.
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 3, Priority: 3}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 5, gotPriority)
}

func TestHandleFailure_BoostNotAppliedOnLastAttempt(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 2
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(3) // last attempt
	run.Priority = 3

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	requireNoStatusUpdateTo(t, calls, domain.StatusQueued)
	requireStatusUpdateTo(t, calls, domain.StatusDeadLetter)
}

func TestHandleFailure_BoostWithNonRetryableError(t *testing.T) {
	t.Parallel()

	// 400 status code -> client error class -> non-retryable
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`bad request`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	job := testJob(server.URL, 3, 5)
	job.RetryPriorityBoost = 2
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	run.Priority = 3

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	requireNoStatusUpdateTo(t, calls, domain.StatusQueued)
	requireStatusUpdateTo(t, calls, domain.StatusDeadLetter)
}

// handleTimeout boost tests.

func TestHandleTimeout_RetryBoostsPriority(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	run.Priority = 3
	job := testJob("http://localhost", 3, 30)
	job.RetryPriorityBoost = 2
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 5, gotPriority)
}

func TestHandleTimeout_RetryPriorityCappedAt10(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	run.Priority = 9
	job := testJob("http://localhost", 3, 30)
	job.RetryPriorityBoost = 3
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 10, gotPriority)
}

func TestHandleTimeout_ZeroBoostNoChange(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	run.Priority = 3
	job := testJob("http://localhost", 3, 30)
	job.RetryPriorityBoost = 0
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

	requireRetryWithoutPriority(t, store.statusUpdates())
}

func TestHandleTimeout_BoostFromMaxPriority(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	run.Priority = 10
	job := testJob("http://localhost", 3, 30)
	job.RetryPriorityBoost = 1
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 10, gotPriority)
}

func TestHandleTimeout_BoostNotAppliedOnLastAttempt(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(3) // last attempt
	run.Status = domain.StatusExecuting
	run.Priority = 3
	job := testJob("http://localhost", 3, 30)
	job.RetryPriorityBoost = 2
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

	calls := store.statusUpdates()
	requireNoStatusUpdateTo(t, calls, domain.StatusQueued)
	requireStatusUpdateTo(t, calls, domain.StatusTimedOut)
}

// Cumulative boost simulation tests.

func TestHandleFailure_CumulativeBoostAcrossRetries(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, MaxAttempts: 6}
	policy := executionPolicy{maxAttempts: 6, timeoutSecs: 30}
	expectedPriorities := []int{2, 4, 6, 8, 10}

	priority := 0
	for i, expected := range expectedPriorities {
		store.mu.Lock()
		store.statusCalls = nil
		store.mu.Unlock()

		run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: i + 1, Priority: priority}
		exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

		gotPriority := requireRetryPriority(t, store.statusUpdates())
		require.Equal(t,
			expected,
			gotPriority,
		)

		priority = gotPriority
	}
}

func TestHandleFailure_CumulativeBoostWithBoostOne(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 1, MaxAttempts: 5}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	expectedPriorities := []int{1, 2, 3}

	priority := 0
	for i, expected := range expectedPriorities {
		store.mu.Lock()
		store.statusCalls = nil
		store.mu.Unlock()

		run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: i + 1, Priority: priority}
		exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

		gotPriority := requireRetryPriority(t, store.statusUpdates())
		require.Equal(t,
			expected,
			gotPriority,
		)

		priority = gotPriority
	}
}

func TestHandleTimeout_CumulativeBoostAcrossRetries(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 3, MaxAttempts: 5}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	expectedPriorities := []int{3, 6, 9, 10}

	priority := 0
	for i, expected := range expectedPriorities {
		store.mu.Lock()
		store.statusCalls = nil
		store.mu.Unlock()

		run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: i + 1, Priority: priority, Status: domain.StatusExecuting}
		exec.handleTimeout(context.Background(), run, job, policy, nil)

		gotPriority := requireRetryPriority(t, store.statusUpdates())
		require.Equal(t,
			expected,
			gotPriority,
		)

		priority = gotPriority
	}
}

// Adversarial and edge case tests.

func TestHandleFailure_BoostWithMaxIntPriority(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: math.MaxInt}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 1, MaxAttempts: 3}
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, 10, gotPriority)
}

func TestHandleFailure_BoostDoesNotMutateOriginalRun(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: 3}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 5, MaxAttempts: 3}
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)
	require.Equal(t, 3, run.Priority)

	// The in-memory run struct should NOT be mutated
}

func TestHandleTimeout_BoostFieldsMapIsolation(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, MaxAttempts: 5}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}

	// Two consecutive timeout retries should each have their own fields map
	run1 := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: 0, Status: domain.StatusExecuting}
	exec.handleTimeout(context.Background(), run1, job, policy, nil)

	run2 := &domain.JobRun{ID: "run-2", JobID: "job-1", Attempt: 1, Priority: 5, Status: domain.StatusExecuting}
	exec.handleTimeout(context.Background(), run2, job, policy, nil)

	calls := store.statusUpdates()
	var priorities []int
	for _, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			if p, ok := c.fields["priority"].(int); ok {
				priorities = append(priorities, p)
			}
		}
	}
	require.Len(t, priorities,

		2)
	require.Equal(t, 2, priorities[0])
	require.Equal(t, 7, priorities[1])
}

func TestHandleFailure_NegativePriorityWithBoost(t *testing.T) {
	t.Parallel()

	// If a run somehow has a negative priority, boost should still work correctly.
	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: -5}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 3, MaxAttempts: 3}
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

	gotPriority := requireRetryPriority(t, store.statusUpdates())
	require.Equal(t, -2, gotPriority)

	// -5 + 3 = -2, which is < 10 so min returns -2.
}

func TestHandleFailure_BoostWithStoreError(t *testing.T) {
	t.Parallel()

	// Verify that a store error during retry doesn't panic or corrupt state.
	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return errors.New("database connection lost")
		},
	}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: 3}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, MaxAttempts: 3}
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	// Should not panic even when store fails.
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)
	require.Equal(t, 3, run.Priority)

	// Verify the original run struct is not mutated despite error.
}

func TestHandleTimeout_BoostWithStoreError(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return errors.New("database connection lost")
		},
	}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Priority: 3, Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, MaxAttempts: 3}
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	// Should not panic.
	exec.handleTimeout(context.Background(), run, job, policy, nil)
	require.Equal(t, 3, run.Priority)
}

func TestHandleFailure_BoostConsistencyBetweenFailureAndTimeout(t *testing.T) {
	t.Parallel()

	// Verify that the same inputs produce the same priority boost
	// whether the retry comes from failure or timeout.
	failureStore := &mockExecutorStore{}
	timeoutStore := &mockExecutorStore{}

	failureExec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: failureStore, PollInterval: time.Hour,
	})
	timeoutExec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: timeoutStore, PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 3, MaxAttempts: 5}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}

	failureRun := &domain.JobRun{ID: "run-f", JobID: "job-1", Attempt: 2, Priority: 4}
	timeoutRun := &domain.JobRun{ID: "run-t", JobID: "job-1", Attempt: 2, Priority: 4, Status: domain.StatusExecuting}

	failureExec.handleFailure(context.Background(), failureRun, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)
	timeoutExec.handleTimeout(context.Background(), timeoutRun, job, policy, nil)

	var failurePriority, timeoutPriority int
	for _, c := range failureStore.statusUpdates() {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			failurePriority = c.fields["priority"].(int)
			break
		}
	}
	for _, c := range timeoutStore.statusUpdates() {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			timeoutPriority = c.fields["priority"].(int)
			break
		}
	}
	require.Equal(t,
		timeoutPriority,

		failurePriority,
	)
	require.Equal(t, 7, failurePriority)
}

func TestHandleFailure_RapidSequentialRetriesNoDataRace(t *testing.T) {
	t.Parallel()

	// Run many retries concurrently to check for data races.
	// This test is meaningful when run with -race flag.
	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", RetryPriorityBoost: 2, MaxAttempts: 10}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}

	var wg conc.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			run := &domain.JobRun{
				ID: fmt.Sprintf("run-%d", i), JobID: "job-1",
				Attempt: 1, Priority: i % 10,
			}
			exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)
		})
	}
	wg.Wait()

	calls := store.statusUpdates()
	retryCount := 0
	for _, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			retryCount++
			priority := c.fields["priority"].(int)
			require.LessOrEqual(t, priority,

				10)
		}
	}
	require.Equal(t, 20, retryCount)
}
