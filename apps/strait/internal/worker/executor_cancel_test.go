package worker

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/pubsub"
)

func cancelTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type cancelNoopPublisher struct{}

func (cancelNoopPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (cancelNoopPublisher) Subscribe(_ context.Context, _ string) (*pubsub.Subscription, error) {
	return nil, nil
}
func (cancelNoopPublisher) Close() error { return nil }

func TestDispatchCancel_Success(t *testing.T) {
	t.Parallel()
	var received atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received.Store(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exec := &Executor{
		httpClient: srv.Client(),
		logger:     cancelTestLogger(),
	}
	job := &domain.Job{ID: "job-1", Slug: "my-job", CancelEndpointURL: srv.URL}
	run := &domain.JobRun{ID: "run-1"}

	exec.dispatchCancel(job, run)

	raw, ok := received.Load().([]byte)
	if !ok || raw == nil {
		t.Fatal("cancel endpoint was not called")
	}

	var payload cancelWebhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload.RunID != "run-1" {
		t.Errorf("expected run_id=run-1, got %s", payload.RunID)
	}
	if payload.JobID != "job-1" {
		t.Errorf("expected job_id=job-1, got %s", payload.JobID)
	}
	if payload.JobSlug != "my-job" {
		t.Errorf("expected job_slug=my-job, got %s", payload.JobSlug)
	}
}

func TestDispatchCancel_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	exec := &Executor{
		httpClient: srv.Client(),
		logger:     cancelTestLogger(),
	}
	job := &domain.Job{ID: "job-1", CancelEndpointURL: srv.URL}
	run := &domain.JobRun{ID: "run-1"}

	// Should not panic — best effort
	exec.dispatchCancel(job, run)
}

func TestDispatchCancel_Timeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exec := &Executor{
		httpClient: &http.Client{Timeout: 50 * time.Millisecond},
		logger:     cancelTestLogger(),
	}
	job := &domain.Job{ID: "job-1", CancelEndpointURL: srv.URL}
	run := &domain.JobRun{ID: "run-1"}

	// Should return quickly despite slow server
	exec.dispatchCancel(job, run)
}

func TestDispatchCancel_EmptyURL(t *testing.T) {
	t.Parallel()
	exec := &Executor{logger: cancelTestLogger()}
	job := &domain.Job{ID: "job-1", CancelEndpointURL: ""}
	run := &domain.JobRun{ID: "run-1"}

	// Should be a no-op
	exec.dispatchCancel(job, run)
}

func TestDispatchCancel_InvalidURL(t *testing.T) {
	t.Parallel()
	exec := &Executor{
		httpClient: http.DefaultClient,
		logger:     cancelTestLogger(),
	}
	job := &domain.Job{ID: "job-1", CancelEndpointURL: "://invalid"}
	run := &domain.JobRun{ID: "run-1"}

	// Should log error but not panic
	exec.dispatchCancel(job, run)
}

func TestDispatchCancel_RequestBody(t *testing.T) {
	t.Parallel()
	var method, contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		contentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exec := &Executor{
		httpClient: srv.Client(),
		logger:     cancelTestLogger(),
	}
	job := &domain.Job{ID: "job-1", CancelEndpointURL: srv.URL}
	run := &domain.JobRun{ID: "run-1"}

	exec.dispatchCancel(job, run)

	if method != http.MethodPost {
		t.Errorf("expected POST, got %s", method)
	}
	if contentType != "application/json" {
		t.Errorf("expected application/json, got %s", contentType)
	}
}

func TestTransitionToCanceling_WithCancelEndpoint(t *testing.T) {
	t.Parallel()
	var cancelCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cancelCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transitions := make([]string, 0)
	ms := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			transitions = append(transitions, string(from)+"→"+string(to))
			return nil
		},
	}

	exec := &Executor{
		store:      ms,
		httpClient: srv.Client(),
		logger:     cancelTestLogger(),
		metrics:    nil,
		publisher:  &cancelNoopPublisher{},
	}
	job := &domain.Job{ID: "job-1", CancelEndpointURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}

	exec.transitionToCanceling(context.Background(), job, run)

	if !cancelCalled.Load() {
		t.Error("cancel endpoint was not called")
	}
	if len(transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %d: %v", len(transitions), transitions)
	}
	if transitions[0] != "executing→canceling" {
		t.Errorf("expected executing→canceling, got %s", transitions[0])
	}
	if transitions[1] != "canceling→canceled" {
		t.Errorf("expected canceling→canceled, got %s", transitions[1])
	}
}

func TestTransitionToCanceling_WithoutCancelEndpoint(t *testing.T) {
	t.Parallel()
	transitions := make([]string, 0)
	ms := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			transitions = append(transitions, string(from)+"→"+string(to))
			return nil
		},
	}

	exec := &Executor{
		store:     ms,
		logger:    cancelTestLogger(),
		metrics:   nil,
		publisher: &cancelNoopPublisher{},
	}
	job := &domain.Job{ID: "job-1", CancelEndpointURL: ""}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}

	exec.transitionToCanceling(context.Background(), job, run)

	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d: %v", len(transitions), transitions)
	}
	if transitions[0] != "executing→canceled" {
		t.Errorf("expected executing→canceled, got %s", transitions[0])
	}
}

func TestTransitionToCanceling_CancelingTransitionFails_FallsBack(t *testing.T) {
	t.Parallel()
	callCount := 0
	transitions := make([]string, 0)
	ms := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			callCount++
			if callCount == 1 {
				// First call: executing → canceling fails
				return context.DeadlineExceeded
			}
			transitions = append(transitions, string(from)+"→"+string(to))
			return nil
		},
	}

	exec := &Executor{
		store:     ms,
		logger:    cancelTestLogger(),
		metrics:   nil,
		publisher: &cancelNoopPublisher{},
	}
	// Has cancel endpoint, but canceling transition fails → falls back to direct cancel
	job := &domain.Job{ID: "job-1", CancelEndpointURL: "http://example.com/cancel"}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}

	exec.transitionToCanceling(context.Background(), job, run)

	if len(transitions) != 1 {
		t.Fatalf("expected 1 fallback transition, got %d: %v", len(transitions), transitions)
	}
	if transitions[0] != "executing→canceled" {
		t.Errorf("expected executing→canceled fallback, got %s", transitions[0])
	}
}
