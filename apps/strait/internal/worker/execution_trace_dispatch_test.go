package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestExecutor_ExecutionTracing_Enabled_CapturesTrace(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	exec.executionTraceMode = executionTraceFull
	run := testRun(1)
	run.CreatedAt = time.Now().Add(-50 * time.Millisecond)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	traceValue, ok := calls[1].fields["execution_trace"]
	if !ok {
		t.Fatal("execution_trace missing from terminal update fields")
	}
	trace, ok := traceValue.(*domain.ExecutionTrace)
	if !ok {
		t.Fatalf("execution_trace type = %T, want *domain.ExecutionTrace", traceValue)
	}
	if trace.QueueWaitMs <= 0 {
		t.Fatalf("QueueWaitMs = %d, want > 0", trace.QueueWaitMs)
	}
	if trace.DispatchMs <= 0 {
		t.Fatalf("DispatchMs = %d, want > 0", trace.DispatchMs)
	}
	if trace.TotalMs <= 0 {
		t.Fatalf("TotalMs = %d, want > 0", trace.TotalMs)
	}
}

func TestExecutor_ExecutionTracing_OnFailure_CapturesTrace(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream failed"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	exec.executionTraceMode = executionTraceFull
	run := testRun(1)
	run.CreatedAt = time.Now().Add(-50 * time.Millisecond)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
	traceValue, ok := calls[1].fields["execution_trace"]
	if !ok {
		t.Fatal("execution_trace missing from failure terminal update")
	}
	trace, ok := traceValue.(*domain.ExecutionTrace)
	if !ok {
		t.Fatalf("execution_trace type = %T, want *domain.ExecutionTrace", traceValue)
	}
	if trace.QueueWaitMs <= 0 {
		t.Fatalf("QueueWaitMs = %d, want > 0", trace.QueueWaitMs)
	}
	if trace.DispatchMs <= 0 {
		t.Fatalf("DispatchMs = %d, want > 0", trace.DispatchMs)
	}
	if trace.TotalMs <= 0 {
		t.Fatalf("TotalMs = %d, want > 0", trace.TotalMs)
	}
}

func TestExecutor_ExecutionTracing_OnTimeout_CapturesTrace(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 1), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	exec.executionTraceMode = executionTraceFull
	run := testRun(1)
	run.CreatedAt = time.Now().Add(-50 * time.Millisecond)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusTimedOut {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusTimedOut)
	}
	traceValue, ok := calls[1].fields["execution_trace"]
	if !ok {
		t.Fatal("execution_trace missing from timeout terminal update")
	}
	trace, ok := traceValue.(*domain.ExecutionTrace)
	if !ok {
		t.Fatalf("execution_trace type = %T, want *domain.ExecutionTrace", traceValue)
	}
	if trace.QueueWaitMs <= 0 {
		t.Fatalf("QueueWaitMs = %d, want > 0", trace.QueueWaitMs)
	}
	if trace.TotalMs <= 0 {
		t.Fatalf("TotalMs = %d, want > 0", trace.TotalMs)
	}
}
