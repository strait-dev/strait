package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/goleak"

	"strait/internal/pubsub"
)

// TestProjectActivityStream_FanoutDrains locks in that the SSE handler waits for
// its fanout goroutines to drain before returning, and cancels their context
// first so the wait cannot deadlock. The subscription channels here never close,
// so a fanout goroutine can only exit via context cancellation: a Wait() that
// relied solely on the deferred cancel (which runs after the wait) would hang,
// and a handler that never waited at all would leak these goroutines past its
// own return and tear down the subscriptions while they were still in use.
func TestProjectActivityStream_FanoutDrains(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	ignoreCacheCleanup := goleak.IgnoreTopFunction("github.com/maypok86/otter/v2.(*cache[...]).periodicCleanUp")

	subscribed := make(chan struct{}, 8)
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			ch := make(chan []byte) // never closed; exit only via ctx cancel
			sub := pubsub.NewSubscription(ch, func() {})
			subscribed <- struct{}{}
			return sub, nil
		},
	}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, pub)

	w := httptest.NewRecorder()
	reqCtx, cancelReq := context.WithCancel(context.Background())
	defer cancelReq()
	r := authedRequest(http.MethodGet, "/v1/projects/proj-1/activity/stream/", "")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "proj-1")
	ctx := context.WithValue(reqCtx, chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	r = r.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		srv.handleProjectActivityStream(w, r)
		close(done)
	}()

	// Wait until every CDC channel has an active fanout goroutine.
	for range 3 {
		select {
		case <-subscribed:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for activity stream to subscribe")
		}
	}

	// Client disconnects.
	cancelReq()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after client disconnect: fanout barrier missing or deadlocked")
	}
	srv.Close()

	// All fanout goroutines must be gone now that the handler has returned.
	goleak.VerifyNone(t, baseline, ignoreCacheCleanup)
}
