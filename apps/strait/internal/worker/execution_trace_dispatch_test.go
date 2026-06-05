package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.Len(t, calls,
		2)

	traceValue, ok := calls[1].fields["execution_trace"]
	require.True(t,
		ok)

	trace, ok := traceValue.(*domain.ExecutionTrace)
	require.True(t,
		ok)
	require.False(t,
		trace.QueueWaitMs <=
			0)
	require.False(t,
		trace.DispatchMs <=
			0)
	require.False(t,
		trace.TotalMs <=
			0)

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
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusDeadLetter,
		calls[1].to)

	traceValue, ok := calls[1].fields["execution_trace"]
	require.True(t,
		ok)

	trace, ok := traceValue.(*domain.ExecutionTrace)
	require.True(t,
		ok)
	require.False(t,
		trace.QueueWaitMs <=
			0)
	require.False(t,
		trace.DispatchMs <=
			0)
	require.False(t,
		trace.TotalMs <=
			0)

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
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusTimedOut,
		calls[1].
			to)

	traceValue, ok := calls[1].fields["execution_trace"]
	require.True(t,
		ok)

	trace, ok := traceValue.(*domain.ExecutionTrace)
	require.True(t,
		ok)
	require.False(t,
		trace.QueueWaitMs <=
			0)
	require.False(t,
		trace.TotalMs <=
			0)

}
