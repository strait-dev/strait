package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/store"
)

func TestHandleRunStream_RunNotFound(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/missing-run/stream", ""))

	// store.ErrRunNotFound does NOT match pgx.ErrNoRows, so handler returns 500
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunStream_StoreError(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-123/stream", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunStream_TerminalRun(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusCompleted,
				Attempt:   1,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-done/stream", ""))

	if w.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunStream_NoPubSub(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}
	// nil pubsub
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-123/stream", ""))

	// When pubsub is nil, handler writes SSE headers then error event
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (SSE response), got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Fatalf("expected error event in body, got: %s", body)
	}
}

func TestHandleRunStream_SubscribeError(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return nil, errors.New("subscribe failed")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-123/stream", ""))

	// Handler writes SSE headers then error event for subscribe failure
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (SSE), got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Fatalf("expected error event for subscribe failure, got: %s", body)
	}
}

func TestHandleRunStream_ReceivesMessage(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}

	// Create a channel that sends one message then closes
	ch := make(chan []byte, 1)
	ch <- []byte(`{"status":"completed"}`)
	close(ch)

	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			_, cancel := context.WithCancel(context.Background())
			return pubsub.NewSubscription(ch, cancel), nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, pub)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-123/stream", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `data: {"status":"completed"}`) {
		t.Fatalf("expected data event with message, got: %s", body)
	}
}

func TestHandleRunStream_ClientDisconnect(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}

	// Channel that blocks forever — the context cancellation should stop the handler
	ch := make(chan []byte)
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			_, cancel := context.WithCancel(context.Background())
			return pubsub.NewSubscription(ch, cancel), nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, pub)
	w := httptest.NewRecorder()

	// Create a request with a cancelled context to simulate client disconnect
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel
	r := authedRequest(http.MethodGet, "/v1/runs/run-123/stream", "")
	r = r.WithContext(ctx)

	srv.ServeHTTP(w, r)

	// Handler should return after seeing cancelled context
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (SSE), got %d", w.Code)
	}
}

func TestHandleRunStream_TerminalStatuses(t *testing.T) {
	t.Parallel()

	terminalStatuses := []domain.RunStatus{
		domain.StatusCompleted,
		domain.StatusFailed,
		domain.StatusTimedOut,
		domain.StatusCanceled,
		domain.StatusExpired,
	}

	for _, status := range terminalStatuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			ms := &mockAPIStore{
				getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
					return &domain.JobRun{
						ID:        id,
						JobID:     "job-1",
						ProjectID: "proj-1",
						Status:    status,
						Attempt:   1,
					}, nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/stream", ""))

			if w.Code != http.StatusGone {
				t.Fatalf("status %s: expected 410, got %d", status, w.Code)
			}
		})
	}
}

func TestHandleRunStream_SSEHeaders(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}

	ch := make(chan []byte)
	close(ch)
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			_, cancel := context.WithCancel(context.Background())
			return pubsub.NewSubscription(ch, cancel), nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, pub)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-123/stream", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", cc)
	}
	if conn := w.Header().Get("Connection"); conn != "keep-alive" {
		t.Fatalf("Connection = %q, want keep-alive", conn)
	}
	if xab := w.Header().Get("X-Accel-Buffering"); xab != "no" {
		t.Fatalf("X-Accel-Buffering = %q, want no", xab)
	}
}

func TestHandleRunStream_NonTerminalStatuses(t *testing.T) {
	t.Parallel()

	// These are non-terminal statuses that should allow streaming
	nonTerminalStatuses := []domain.RunStatus{
		domain.StatusQueued,
		domain.StatusDequeued,
		domain.StatusExecuting,
		domain.StatusDelayed,
	}

	for _, status := range nonTerminalStatuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			ms := &mockAPIStore{
				getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
					return &domain.JobRun{
						ID:        id,
						JobID:     "job-1",
						ProjectID: "proj-1",
						Status:    status,
						Attempt:   1,
					}, nil
				},
			}

			ch := make(chan []byte)
			close(ch) // close immediately so handler exits
			pub := &mockPublisher{
				subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
					_, cancel := context.WithCancel(context.Background())
					return pubsub.NewSubscription(ch, cancel), nil
				},
			}

			srv := newTestServer(t, ms, &mockQueue{}, pub)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/stream", ""))

			// Should NOT return 410 for non-terminal statuses
			if w.Code == http.StatusGone {
				t.Fatalf("status %s: got 410, should allow streaming", status)
			}
		})
	}
}

// Suppress unused import warnings.
var _ = time.Second
