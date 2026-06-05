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

	"github.com/stretchr/testify/require"
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
			require.Fail(t,

				"Subscribe should not be reached when conn cap is exhausted")
			return nil, nil
		},
	}
	srv := newChunkStreamServer(t, executingRunStore(), pub, 1)
	require.True(
		t, srv.acquireSSEConn(
			"proj-1"))

	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/stream/chunks", "")
	req.Header.Set("Accept", "text/event-stream")
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusServiceUnavailable,

		w.Code)
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	counter := srv.projectSSECounter("proj-1")
	require.EqualValues(t, 0, counter.
		Load())
	require.True(
		t, srv.acquireSSEConn(
			"proj-1"))

	// A second acquire on the same project must succeed now that the handler released its slot.
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.EqualValues(t, 1, subscribeCalls.
		Load())

	counter := srv.projectSSECounter("proj-1")
	require.EqualValues(t, 0, counter.
		Load())
}
