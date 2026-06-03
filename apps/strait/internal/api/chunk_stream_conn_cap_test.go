package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
)

func newChunkStreamServer(t *testing.T, ms *APIStoreMock, pub *mockPublisher, maxPerProject int64) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:        "test-secret-value",
		MaxBulkTriggerItems:   500,
		JWTSigningKey:         testJWTSigningKey,
		SSEMaxConns:           5000,
		SSEMaxConnsPerProject: maxPerProject,
	}
	var p pubsub.Publisher
	if pub != nil {
		p = pub
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   ms,
		Queue:   &mockQueue{},
		PubSub:  p,
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func executingRunStore() *APIStoreMock {
	return &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
	}
}

// TestChunkStreamRespectsPerProjectCap pins the new acquire-before-stream
// guard. With the per-project cap saturated by an out-of-band acquire, a fresh
// run chunk stream request must be rejected with 503 instead of consuming pubsub
// resources.
func TestChunkStreamRespectsPerProjectCap(t *testing.T) {
	t.Parallel()

	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			t.Fatal("Subscribe should not be reached when conn cap is exhausted")
			return nil, nil
		},
	}
	srv := newChunkStreamServer(t, executingRunStore(), pub, 1)

	if !srv.acquireSSEConn("proj-1") {
		t.Fatal("baseline acquire should succeed")
	}

	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/stream/chunks", "")
	req.Header.Set("Accept", "text/event-stream")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when per-project cap is exhausted, got %d: %s", w.Code, w.Body.String())
	}
}

// TestChunkStreamReleasesOnHandlerReturn proves the handler defers the
// release: a successful (pubsub-less) request must leave the project counter
// back at zero so subsequent streams can connect.
func TestChunkStreamReleasesOnHandlerReturn(t *testing.T) {
	t.Parallel()

	srv := newChunkStreamServer(t, executingRunStore(), nil, 1)

	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/stream/chunks", "")
	req.Header.Set("Accept", "text/event-stream")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with SSE error body, got %d", w.Code)
	}

	counter := srv.projectSSECounter("proj-1")
	if got := counter.Load(); got != 0 {
		t.Fatalf("per-project counter not released: got %d, want 0", got)
	}

	// A second acquire on the same project must succeed now that the handler released its slot.
	if !srv.acquireSSEConn("proj-1") {
		t.Fatal("acquire after handler return should succeed")
	}
}

// TestChunkStreamReleasesOnEarlyError ensures the slot is released when
// pubsub.Subscribe returns an error mid-handshake.
func TestChunkStreamReleasesOnEarlyError(t *testing.T) {
	t.Parallel()

	var subscribeCalls atomic.Int64
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			subscribeCalls.Add(1)
			return nil, errors.New("subscribe failed")
		},
	}
	srv := newChunkStreamServer(t, executingRunStore(), pub, 1)

	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/stream/chunks", "")
	req.Header.Set("Accept", "text/event-stream")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with SSE error body, got %d", w.Code)
	}
	if subscribeCalls.Load() != 1 {
		t.Fatalf("expected Subscribe to be called once, got %d", subscribeCalls.Load())
	}

	counter := srv.projectSSECounter("proj-1")
	if got := counter.Load(); got != 0 {
		t.Fatalf("per-project counter not released after subscribe error: got %d, want 0", got)
	}
}
