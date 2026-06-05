package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/testutil"
)

func TestExecutor_Dispatch_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Run-ID") != "run-1" {
			t.Fatalf("X-Run-ID = %q, want %q", r.Header.Get("X-Run-ID"), "run-1")
		}
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
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}

	gotTransitions := []string{
		fmt.Sprintf("%s->%s", calls[0].from, calls[0].to),
		fmt.Sprintf("%s->%s", calls[1].from, calls[1].to),
	}
	testutil.AssertEqual(t, gotTransitions, []string{
		"dequeued->executing",
		"executing->completed",
	})

	gotResult, ok := calls[1].fields["result"].(json.RawMessage)
	if !ok {
		t.Fatalf("result field type = %T, want json.RawMessage", calls[1].fields["result"])
	}
	if string(gotResult) != `{"ok":true}` {
		t.Fatalf("result = %s, want %s", string(gotResult), `{"ok":true}`)
	}
	if run.Status != domain.StatusCompleted {
		t.Fatalf("run status = %s, want %s", run.Status, domain.StatusCompleted)
	}
}

func TestExecutor_Dispatch_IncludesSecretHeadersWhenEnabled(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Secret-API_KEY") != "super-secret" {
			t.Fatalf("X-Secret-API_KEY = %q, want %q", r.Header.Get("X-Secret-API_KEY"), "super-secret")
		}
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
		if jobID != "job-1" || environment != "env-secret" {
			t.Fatalf("unexpected args: %q %q", jobID, environment)
		}
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
	if err == nil {
		t.Fatal("dispatch error = nil, want EndpointError")
		return
	}

	var endpointErr *domain.EndpointError
	if !errors.As(err, &endpointErr) {
		t.Fatalf("dispatch error type = %T, want *domain.EndpointError", err)
	}
	if endpointErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("endpoint status = %d, want %d", endpointErr.StatusCode, http.StatusInternalServerError)
	}
	if endpointErr.Body != "upstream exploded" {
		t.Fatalf("endpoint body = %q, want %q", endpointErr.Body, "upstream exploded")
	}

	store.getJobFn = func(context.Context, string) (*domain.Job, error) {
		return job, nil
	}
	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
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
			if len(calls) != 2 {
				t.Fatalf("status update calls = %d, want 2", len(calls))
			}
			if calls[1].to != tt.wantStatus {
				t.Fatalf("final status = %s, want %s", calls[1].to, tt.wantStatus)
			}
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
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusQueued {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusQueued)
	}
	attempt, ok := calls[1].fields["attempt"].(int)
	if !ok {
		t.Fatalf("attempt field type = %T, want int", calls[1].fields["attempt"])
	}
	if attempt != 2 {
		t.Fatalf("attempt field = %d, want 2", attempt)
	}
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
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
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
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusQueued {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusQueued)
	}
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

	if fallbackCalled.Load() != 1 {
		t.Fatalf("fallback call count = %d, want 1", fallbackCalled.Load())
	}

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
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

	if fallbackCalled.Load() != 0 {
		t.Fatalf("fallback call count = %d, want 0", fallbackCalled.Load())
	}

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
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
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
	if run.Status != domain.StatusDeadLetter {
		t.Fatalf("run status = %s, want %s", run.Status, domain.StatusDeadLetter)
	}
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
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusDeadLetter {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusDeadLetter)
	}
	if run.Status != domain.StatusDeadLetter {
		t.Fatalf("run status = %s, want %s", run.Status, domain.StatusDeadLetter)
	}
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
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
	if _, ok := calls[1].fields["result"]; ok {
		t.Fatal("result field present for empty response, want absent")
	}
}
