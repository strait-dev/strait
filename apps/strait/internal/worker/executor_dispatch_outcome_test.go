package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutor_Dispatch_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t,
			"run-1", r.Header.
				Get("X-Run-ID"),
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)

	gotTransitions := []string{
		fmt.Sprintf("%s->%s", calls[0].from, calls[0].to),
		fmt.Sprintf("%s->%s", calls[1].from, calls[1].to),
	}
	testutil.AssertEqual(t, gotTransitions, []string{
		"dequeued->executing",
		"executing->completed",
	})

	gotResult, ok := calls[1].fields["result"].(json.RawMessage)
	require.True(t,
		ok)
	require.Equal(t,
		`{"ok":true}`, string(gotResult))
	require.Equal(t,
		domain.StatusCompleted,
		run.
			Status,
	)
}

func TestExecutor_Dispatch_IncludesSecretHeadersWhenEnabled(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t,
			"super-secret",
			r.Header.Get("X-Secret-API_KEY"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		job := testJob(server.URL, 1, 5)
		job.EnvironmentID = "env-secret"
		return job, nil
	}
	store.listSecretsFn = func(_ context.Context, jobID, environment string) ([]domain.JobSecret, error) {
		require.False(t,
			jobID != "job-1" ||
				environment !=
					"env-secret",
		)

		return []domain.JobSecret{{SecretKey: "API_KEY", EncryptedValue: "super-secret"}}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)
}

func TestExecutor_Dispatch_NonOKStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream exploded"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	job := testJob(server.URL, 1, 5)
	run := testRun(1)

	err := exec.dispatch(context.Background(), job, run)
	require.Error(t,
		err)

	var endpointErr *domain.EndpointError
	require.ErrorAs(t,
		err, &endpointErr)
	require.Equal(t,
		http.StatusInternalServerError,

		endpointErr.
			StatusCode,
	)
	require.Equal(t,
		"upstream exploded",
		endpointErr.
			Body)

	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusDeadLetter,
		calls[1].
			to)
}

func TestExecutor_Dispatch_Timeout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		attempt     int
		maxAttempts int
		wantStatus  domain.RunStatus
	}{
		{name: "retry queued", attempt: 1, maxAttempts: 2, wantStatus: domain.StatusQueued},
		{name: "final timed out", attempt: 2, maxAttempts: 2, wantStatus: domain.StatusTimedOut},
	}

	timeoutTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	httpClient := &http.Client{Transport: timeoutTransport}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockExecutorStore{}
			store.getJobFn = func(context.Context, string) (*domain.Job, error) {
				return testJob("http://timeout.test", tt.maxAttempts, 1), nil
			}

			exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, httpClient)
			run := testRun(tt.attempt)

			exec.execute(context.Background(), run)

			calls := store.statusUpdates()
			require.Len(t, calls,
				2)
			require.Equal(t,
				tt.wantStatus, calls[1].to,
			)
		})
	}
}

func TestExecutor_Dispatch_RetryOnFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("temporary failure"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusQueued,
		calls[1].to)

	attempt, ok := calls[1].fields["attempt"].(int)
	require.True(t,
		ok)
	require.Equal(t, 2, attempt)
}

func TestExecutor_SmartRetry_ClientErrorSkipsRetry(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid payload"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusDeadLetter,
		calls[1].
			to)
}

func TestExecutor_SmartRetry_ServerErrorRetries(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("temporary"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 3, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusQueued,
		calls[1].to)
}

func TestExecutor_Fallback_TransientErrorUsesFallbackEndpoint(t *testing.T) {
	t.Parallel()
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer primary.Close()

	fallbackCalled := atomic.Int32{}
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackCalled.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"source":"fallback"}`))
	}))
	defer fallback.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		job := testJob(primary.URL, 2, 5)
		job.FallbackEndpointURL = fallback.URL
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, primary.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)
	require.EqualValues(t, 1, fallbackCalled.
		Load())

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusCompleted,
		calls[1].
			to)
}

func TestExecutor_Fallback_ClientErrorDoesNotUseFallback(t *testing.T) {
	t.Parallel()
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer primary.Close()

	fallbackCalled := atomic.Int32{}
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackCalled.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"source":"fallback"}`))
	}))
	defer fallback.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		job := testJob(primary.URL, 1, 5)
		job.FallbackEndpointURL = fallback.URL
		return job, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, primary.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)
	require.EqualValues(t, 0, fallbackCalled.
		Load())

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusDeadLetter,
		calls[1].
			to)
}

func TestExecutor_Dispatch_FinalFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("hard failure"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 2, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(2)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusDeadLetter,
		calls[1].
			to)
	require.Equal(t,
		domain.StatusDeadLetter,
		run.
			Status,
	)
}

func TestExecutor_DLQ_TransitionsToDeadLetterOnExhaustedRetries(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("hard failure"))
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())

	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusDeadLetter,
		calls[1].
			to)
	require.Equal(t,
		domain.StatusDeadLetter,
		run.
			Status,
	)
}

func TestExecutor_Dispatch_EmptyResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())
	run := testRun(1)
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusCompleted,
		calls[1].
			to)

	if _, ok := calls[1].fields["result"]; ok {
		require.Fail(t,

			"result field present for empty response, want absent")
	}
}
