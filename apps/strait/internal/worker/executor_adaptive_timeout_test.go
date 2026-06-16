package worker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	orcstore "strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestExecutor_AdaptiveTimeout_UsesP95WhenHigherThanStatic(t *testing.T) {
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
	store.getJobHealthStatsFn = func(context.Context, string, time.Time) (*orcstore.JobHealthStats, error) {
		return &orcstore.JobHealthStats{P95DurationSecs: 2.0}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	exec.adaptiveTimeoutEnabled = true
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusCompleted,

		calls[1].to)
	require.NotEqual(t, domain.
		StatusTimedOut,
		calls[1].to)
}

func TestExecutor_AdaptiveTimeout_FallsBackToStaticWhenP95Lower(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 3), nil
	}
	store.getJobHealthStatsFn = func(context.Context, string, time.Time) (*orcstore.JobHealthStats, error) {
		return &orcstore.JobHealthStats{P95DurationSecs: 0.5}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	exec.adaptiveTimeoutEnabled = true
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusCompleted,

		calls[1].to)
}

func TestExecutor_AdaptiveTimeout_FallsBackOnError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 2), nil
	}
	store.getJobHealthStatsFn = func(context.Context, string, time.Time) (*orcstore.JobHealthStats, error) {
		return nil, errors.New("health stats unavailable")
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	exec.adaptiveTimeoutEnabled = true
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusCompleted,

		calls[1].to)
}

func TestExecutor_AdaptiveTimeout_DisabledDoesNotLoadHealthStats(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 1), nil
	}
	store.getJobHealthStatsFn = func(context.Context, string, time.Time) (*orcstore.JobHealthStats, error) {
		require.Fail(t, "adaptive timeout stats should not load when disabled")
		return nil, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls, 2)
	require.Equal(t, domain.StatusCompleted, calls[1].to)
}
