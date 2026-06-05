package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestExecutor_CircuitOpen_RequeuesBeforeExecuting(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	retryAt := time.Now().UTC().Add(45 * time.Second)
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}
	store.canDispatchFn = func(context.Context, string, time.Time) (bool, *time.Time, error) {
		return false, &retryAt, nil
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(2),
		Queue:             &mockExecQueue{},
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
		HTTPClient:        server.Client(),
	})
	t.Cleanup(func() { _ = exec.pool.Shutdown(context.Background()) })

	run := testRun(1)
	exec.execute(context.Background(), run)

	if called.Load() != 0 {
		t.Fatalf("dispatch called %d times, want 0", called.Load())
	}

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].from != domain.StatusDequeued || calls[0].to != domain.StatusQueued {
		t.Fatalf("transition = %s->%s, want %s->%s", calls[0].from, calls[0].to, domain.StatusDequeued, domain.StatusQueued)
	}
	if _, ok := calls[0].fields["next_retry_at"]; ok {
		t.Fatalf("next_retry_at must not be in job_runs UPDATE fields; lives in job_retries side table now")
	}
	scheduled := store.scheduleRetries()
	if len(scheduled) != 1 {
		t.Fatalf("ScheduleRetry calls = %d, want 1", len(scheduled))
	}
	if !scheduled[0].at.Equal(retryAt) {
		t.Fatalf("ScheduleRetry at = %v, want %v", scheduled[0].at, retryAt)
	}
}

func TestExecutor_CircuitBreaker_RecordsFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("temporary failure"))
	}))
	defer server.Close()

	var failureCalled atomic.Int32
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}
	store.recordFailureFn = func(context.Context, string, time.Time, int, time.Duration) error {
		failureCalled.Add(1)
		return nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	if failureCalled.Load() != 1 {
		t.Fatalf("record failure called = %d, want 1", failureCalled.Load())
	}
}

func TestExecutor_CircuitBreakerFailureUsesProjectScopedEndpointKey(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("temporary failure"))
	}))
	defer server.Close()

	var precheckKey string
	var failureKey string
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}
	store.canDispatchFn = func(_ context.Context, endpointURL string, _ time.Time) (bool, *time.Time, error) {
		precheckKey = endpointURL
		return true, nil, nil
	}
	store.recordFailureFn = func(_ context.Context, endpointURL string, _ time.Time, _ int, _ time.Duration) error {
		failureKey = endpointURL
		return nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	want := endpointStateKey("proj-1", server.URL)
	if precheckKey != want {
		t.Fatalf("precheck endpoint key = %q, want %q", precheckKey, want)
	}
	if failureKey != want {
		t.Fatalf("failure endpoint key = %q, want %q", failureKey, want)
	}
	healthKeys := store.healthResults()
	if len(healthKeys) == 0 {
		t.Fatal("expected health failure to be recorded")
	}
	if healthKeys[len(healthKeys)-1] != want {
		t.Fatalf("health endpoint key = %q, want %q", healthKeys[len(healthKeys)-1], want)
	}
}

func TestExecutor_TimeoutFailureUsesProjectScopedEndpointKey(t *testing.T) {
	t.Parallel()

	var failureKey string
	store := &mockExecutorStore{}
	store.recordFailureFn = func(_ context.Context, endpointURL string, _ time.Time, _ int, _ time.Duration) error {
		failureKey = endpointURL
		return nil
	}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("http://timeout.test", 3, 30)
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

	want := endpointStateKey("proj-1", job.EndpointURL)
	if failureKey != want {
		t.Fatalf("timeout failure endpoint key = %q, want %q", failureKey, want)
	}
	healthKeys := store.healthResults()
	if len(healthKeys) == 0 {
		t.Fatal("expected timeout health result to be recorded")
	}
	if healthKeys[len(healthKeys)-1] != want {
		t.Fatalf("timeout health endpoint key = %q, want %q", healthKeys[len(healthKeys)-1], want)
	}
}

func TestExecutor_CircuitBreaker_RecordsSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	var successCalled atomic.Int32
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}
	store.recordSuccessFn = func(context.Context, string) error {
		successCalled.Add(1)
		return nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	if successCalled.Load() != 1 {
		t.Fatalf("record success called = %d, want 1", successCalled.Load())
	}
}
