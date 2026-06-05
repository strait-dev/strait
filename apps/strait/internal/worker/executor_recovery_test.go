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

	"github.com/stretchr/testify/require"
)

func TestExecutor_HandleSystemFailure(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, nil)
	run := testRun(1)
	run.Status = domain.StatusExecuting

	exec.handleSystemFailure(context.Background(), run, "db unavailable")

	calls := store.statusUpdates()
	require.Len(t, calls,
		1)
	require.False(t,
		calls[0].
			from != domain.
			StatusExecuting ||
			calls[0].to !=
				domain.
					StatusSystemFailed,
	)

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
	require.Len(t, calls,
		1)
	require.Equal(t,
		domain.StatusSystemFailed,

		calls[0].to)

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
	require.Len(t, calls,
		1)

	select {
	case <-hit:
		require.Fail(t, "dispatch was called after transition failure")
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
		require.Nil(t, recover())

	}()
	require.Error(t,
		exec.dispatch(context.
			Background(), job, run,
		))

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
	require.Len(t, calls,
		1)
	require.Equal(t,
		domain.StatusSystemFailed,

		calls[0].to)

	errMsg, ok := calls[0].fields["error"].(string)
	require.True(t,
		ok)
	require.True(t,
		strings.Contains(errMsg,
			"panic:"))

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
	require.Len(t, calls,
		1)
	require.Equal(t,
		domain.StatusSystemFailed,

		calls[0].to)

}
