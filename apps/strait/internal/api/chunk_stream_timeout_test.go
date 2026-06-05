package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func newChunkStreamServerWithDuration(t *testing.T, ms *APIStoreMock, pub *mockPublisher, maxDuration time.Duration) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:        "test-secret-value",
		MaxBulkTriggerItems:   500,
		JWTSigningKey:         testJWTSigningKey,
		SSEMaxConns:           5000,
		SSEMaxConnsPerProject: 100,
		SSEMaxConnDuration:    maxDuration,
		SSEKeepaliveInterval:  10 * time.Second,
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

// TestChunkStreamClosesAfterMaxDuration confirms the handler tears down
// once SSEMaxConnDuration elapses, even with a still-open pubsub subscription.
// Without the timeout the goroutine would block indefinitely on a healthy
// subscription, holding the SSE conn slot.
func TestChunkStreamClosesAfterMaxDuration(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	dataCh := make(chan []byte) // never closed, never written
	_, subCancel := context.WithCancel(context.Background())
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return pubsub.NewSubscription(dataCh, subCancel), nil
		},
	}
	srv := newChunkStreamServerWithDuration(t, executingRunStore(), pub, 100*time.Millisecond)

	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/stream/chunks", "")
	req.Header.Set("Accept", "text/event-stream")

	done := make(chan struct{})
	start := time.Now()
	concWG.Go(func() {
		defer close(done)
		srv.ServeHTTP(w, req)
	})

	select {
	case <-done:
		// Allow generous slack for slow CI; the point is "doesn't hang forever".
		if d := time.Since(start); d > 5*time.Second {
			require.Failf(t, "test failure",

				"handler took %s to honor SSEMaxConnDuration=100ms", d)
		}
	case <-time.After(5 * time.Second):
		require.Fail(t, "handler did not return within 5s; SSEMaxConnDuration not enforced")
	}
	require.Equal(t, http.StatusOK,
		w.Code)

}

// TestChunkStreamFallsBackToDefaultDurationWhenZero pins the
// "zero-config means 30 minutes" fallback. We capture the context that
// reaches pubsub.Subscribe and assert its deadline is at least 25 minutes
// out, which only happens when the handler applied the default.
func TestChunkStreamFallsBackToDefaultDurationWhenZero(t *testing.T) {
	t.Parallel()

	var (
		mu        sync.Mutex
		deadline  time.Time
		hasDdl    bool
		subCalled bool
	)
	dataCh := make(chan []byte, 1)
	_, subCancel := context.WithCancel(context.Background())
	pub := &mockPublisher{
		subscribeFn: func(ctx context.Context, _ string) (*pubsub.Subscription, error) {
			mu.Lock()
			subCalled = true
			deadline, hasDdl = ctx.Deadline()
			mu.Unlock()
			return pubsub.NewSubscription(dataCh, subCancel), nil
		},
	}
	srv := newChunkStreamServerWithDuration(t, executingRunStore(), pub, 0)

	close(dataCh) // unblock the handler immediately

	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/stream/chunks", "")
	req.Header.Set("Accept", "text/event-stream")
	srv.ServeHTTP(w, req)

	mu.Lock()
	defer mu.Unlock()
	require.True(
		t, subCalled,
	)
	require.True(
		t, hasDdl)
	require.GreaterOrEqual(t,
		time.Until(deadline),
		25*time.
			Minute)

}
