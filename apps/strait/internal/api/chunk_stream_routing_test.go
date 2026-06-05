package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
	"strait/internal/pubsub"
)

// TestChunkStreamReturnsSSEHeadersThroughRouter exercises
// /v1/runs/{runID}/stream/chunks through the full chi router (rather than
// dispatching the handler directly). Before the fix this route lived inside
// the /v1 group, where the JSON Accept gate plus the rlsTxMiddleware-wrapped
// non-flushable response writer combined to fail the SSE handshake. After the
// fix the route is mounted alongside the other run SSE handlers.
func TestChunkStreamReturnsSSEHeadersThroughRouter(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
	}

	dataCh := make(chan []byte, 1)
	_, subCancel := context.WithCancel(context.Background())
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, channel string) (*pubsub.Subscription, error) {
			if !strings.HasPrefix(channel, "run_stream:") {
				t.Fatalf("unexpected pubsub channel: %s", channel)
			}
			return pubsub.NewSubscription(dataCh, subCancel), nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, pub)

	close(dataCh)

	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/stream/chunks", "")
	req.Header.Set("Accept", "text/event-stream")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 through router, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type=text/event-stream, got %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("expected Cache-Control=no-cache, got %q", cc)
	}
}

// TestChunkStreamRouteAcceptsSSEAcceptHeader pins the regression where
// the /v1 group's requireJSONAccept middleware rejected text/event-stream
// callers with 406. Mounting outside /v1 must allow this Accept value.
func TestChunkStreamRouteAcceptsSSEAcceptHeader(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/stream/chunks", "")
	req.Header.Set("Accept", "text/event-stream")
	srv.ServeHTTP(w, req)

	if w.Code == http.StatusNotAcceptable {
		t.Fatalf("got 406 for text/event-stream Accept; route is still wrapped by JSON Accept gate")
	}
	// pubsub is nil so handler emits a 200 with an SSE error body. The
	// important assertion is that we cleared the JSON Accept gate.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (handler-level SSE error), got %d", w.Code)
	}
}

// TestChunkStreamPreservesTerminalGuard confirms the run-state guard at
// the top of handleRunChunkStream still fires after the route move.
func TestChunkStreamPreservesTerminalGuard(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusCompleted, Attempt: 1,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/stream/chunks", "")
	req.Header.Set("Accept", "text/event-stream")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone for terminal run, got %d", w.Code)
	}
}

// TestChunkStreamRequiresAuth ensures the moved route still rejects
// unauthenticated callers (defense-in-depth: the new mount sits next to other
// SSE routes, all of which require apiKeyOrSecretAuth).
func TestChunkStreamRequiresAuth(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/run-1/stream/chunks", nil)
	req.Header.Set("Accept", "text/event-stream")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", w.Code)
	}
}
