package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCompleteIdempotencyKeyUsesDetachedContext is the regression guard
// for the bug we just fixed: when r.Context() is canceled (timeout
// middleware, client disconnect) right as the handler returns 200, the
// CompleteIdempotencyKey call must still receive a live context so the
// cache write actually lands. Otherwise the next retry re-executes the
// non-idempotent operation that already succeeded.
func TestCompleteIdempotencyKeyUsesDetachedContext(t *testing.T) {
	t.Parallel()

	var (
		mu               sync.Mutex
		completeCalled   bool
		completeCtxErr   error
		completeDeadline bool
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(ctx context.Context, _, _ string, _ int, _ http.Header, _ []byte) error {
			mu.Lock()
			defer mu.Unlock()
			completeCalled = true
			completeCtxErr = ctx.Err()
			_, completeDeadline = ctx.Deadline()
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Cancel the inbound request context BEFORE the middleware runs
		// CompleteIdempotencyKey. This mirrors the timeout-middleware /
		// client-disconnect race a real deployment sees on slow stores.
		if cancel, ok := r.Context().Value(testCancelKey{}).(context.CancelFunc); ok {
			cancel()
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "complete-detached-ctx")
	ctx := idempotencyTestCtx(r.Context(), "proj-1")
	cancellable, cancel := context.WithCancel(ctx)
	cancellable = context.WithValue(cancellable, testCancelKey{}, cancel)
	r = r.WithContext(cancellable)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	mu.Lock()
	defer mu.Unlock()
	require.True(
		t, completeCalled,
	)
	require.Nil(t, completeCtxErr)
	require.True(
		t, completeDeadline,
	)

}

// TestCompleteIdempotencyKeyTimeoutBudgetEnforced is the adversarial
// guard: the cache write must abort within the configured cleanup
// timeout when the store hangs, instead of pinning the request goroutine
// for the full TTL.
func TestCompleteIdempotencyKeyTimeoutBudgetEnforced(t *testing.T) {
	t.Parallel()

	deadlineCh := make(chan time.Duration, 1)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(ctx context.Context, _, _ string, _ int, _ http.Header, _ []byte) error {
			if d, ok := ctx.Deadline(); ok {
				deadlineCh <- time.Until(d)
			} else {
				deadlineCh <- 0
			}
			<-ctx.Done()
			return ctx.Err()
		},
		// The middleware's cleanup() path runs after Complete fails;
		// stub Delete so it does not influence the test.
		DeleteIdempotencyKeyFunc: func(_ context.Context, _, _ string) (int64, error) {
			return 1, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "complete-timeout-budget")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()

	start := time.Now()
	wrapped.ServeHTTP(w, r)
	elapsed := time.Since(start)
	require.LessOrEqual(t, elapsed,
		10*time.Second,
	)

	select {
	case d := <-deadlineCh:
		if d <= 0 || d > 6*time.Second {
			require.Failf(t, "test failure",

				"Complete deadline = %v, want a finite positive value ≤ 5s", d)
		}
	default:
		require.Fail(t, "Complete ctx had no deadline")
	}
}

// TestCompleteIdempotencyKeyHappyPathStillCommits regresses the
// uncanceled-ctx path so the detach refactor does not silently break
// normal completion.
func TestCompleteIdempotencyKeyHappyPathStillCommits(t *testing.T) {
	t.Parallel()

	var (
		mu             sync.Mutex
		completeStatus int
		completeBody   []byte
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, status int, _ http.Header, body []byte) error {
			mu.Lock()
			defer mu.Unlock()
			completeStatus = status
			completeBody = append([]byte(nil), body...)
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created":true}`))
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "complete-happy-path")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, http.StatusCreated,
		completeStatus,
	)
	require.Equal(t, `{"created":true}`,
		string(completeBody))

}
