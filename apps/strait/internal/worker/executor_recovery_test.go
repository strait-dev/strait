package worker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestExecutor_HandleSystemFailure(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, nil)
	run := testRun(1)
	run.Status = domain.StatusExecuting

	exec.handleSystemFailure(context.Background(), run, "db unavailable")

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].from != domain.StatusExecuting || calls[0].to != domain.StatusSystemFailed {
		t.Fatalf("transition = %s->%s, want %s->%s", calls[0].from, calls[0].to, domain.StatusExecuting, domain.StatusSystemFailed)
	}
}

func TestExecutor_Execute_JobLookupFails(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return nil, errors.New("job lookup failed")
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, nil)
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].to != domain.StatusSystemFailed {
		t.Fatalf("final status = %s, want %s", calls[0].to, domain.StatusSystemFailed)
	}
}

func TestExecutor_Execute_StatusTransitionFails(t *testing.T) {
	t.Parallel()
	hit := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit <- struct{}{}
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}
	store.updateRunStatusFn = func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
		if from == domain.StatusDequeued && to == domain.StatusExecuting {
			return errors.New("write conflict")
		}
		return nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}

	select {
	case <-hit:
		t.Fatal("dispatch was called after transition failure")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestExecutor_NilMetrics(t *testing.T) {
	t.Parallel()
	errTransport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})

	exec := NewExecutor(ExecutorConfig{
		Pool:              NewPool(1),
		Queue:             &mockExecQueue{},
		Store:             &mockExecutorStore{},
		HTTPClient:        &http.Client{Transport: errTransport},
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
	})
	t.Cleanup(func() { _ = exec.pool.Shutdown(context.Background()) })

	job := testJob("http://example.invalid", 1, 5)
	run := testRun(1)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("dispatch panicked with nil metrics: %v", r)
		}
	}()

	if err := exec.dispatch(context.Background(), job, run); err == nil {
		t.Fatal("dispatch error = nil, want non-nil")
		return
	}
}

func TestExecutor_PanicRecovery(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		panic("simulated crash in job lookup")
	}

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			return []domain.JobRun{*testRun(1)}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
	})

	exec.poll(context.Background())
	_ = pool.Shutdown(context.Background())

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].to != domain.StatusSystemFailed {
		t.Fatalf("status = %s, want %s", calls[0].to, domain.StatusSystemFailed)
	}
	errMsg, ok := calls[0].fields["error"].(string)
	if !ok {
		t.Fatal("expected error field in status update")
	}
	if !strings.Contains(errMsg, "panic:") {
		t.Fatalf("error = %q, want to contain 'panic:'", errMsg)
	}
}

func TestExecutor_PanicRecovery_ErrorValue(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer func() { _ = pool.Shutdown(context.Background()) }()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		panic(errors.New("runtime error: index out of range"))
	}

	q := &mockExecQueue{
		dequeueNFn: func(_ context.Context, _ int) ([]domain.JobRun, error) {
			return []domain.JobRun{*testRun(1)}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             q,
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
	})

	exec.poll(context.Background())
	_ = pool.Shutdown(context.Background())

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("status update calls = %d, want 1", len(calls))
	}
	if calls[0].to != domain.StatusSystemFailed {
		t.Fatalf("status = %s, want %s", calls[0].to, domain.StatusSystemFailed)
	}
}
