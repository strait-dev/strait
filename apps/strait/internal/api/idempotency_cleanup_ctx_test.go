package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestPanicCleanupRunsAfterContextCancel pins the panic-cleanup
// path: when the inner handler cancels its own request context and then
// panics, DeleteIdempotencyKey must still be invoked with a context that
// is NOT canceled. Otherwise the pending row is stuck for the full TTL
// and the caller cannot retry.
func TestPanicCleanupRunsAfterContextCancel(t *testing.T) {
	t.Parallel()

	var (
		mu                sync.Mutex
		deleteCalled      bool
		deleteCtxErr      error
		deleteCtxDeadline bool
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, []byte, error) {
			return "acquired", 0, nil, nil
		},
		DeleteIdempotencyKeyFunc: func(ctx context.Context, _, _ string) (int64, error) {
			mu.Lock()
			defer mu.Unlock()
			deleteCalled = true
			deleteCtxErr = ctx.Err()
			_, deleteCtxDeadline = ctx.Deadline()
			return 1, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// Cancel the inbound request context, then panic. The middleware
		// must not pass r.Context() to DeleteIdempotencyKey because it
		// would already be canceled.
		if cancel, ok := r.Context().Value(testCancelKey{}).(context.CancelFunc); ok {
			cancel()
		}
		panic("handler exploded")
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "panic-cleanup-key")
	ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-1")
	cancellable, cancel := context.WithCancel(ctx)
	cancellable = context.WithValue(cancellable, testCancelKey{}, cancel)
	r = r.WithContext(cancellable)
	w := httptest.NewRecorder()

	defer func() {
		// Recover from the deliberate panic so the test continues.
		_ = recover()
	}()
	func() {
		defer func() { _ = recover() }()
		wrapped.ServeHTTP(w, r)
	}()

	mu.Lock()
	defer mu.Unlock()
	if !deleteCalled {
		t.Fatal("expected DeleteIdempotencyKey to run on panic-cleanup path")
	}
	if deleteCtxErr != nil {
		t.Fatalf("DeleteIdempotencyKey received canceled context: ctx.Err() = %v", deleteCtxErr)
	}
	if !deleteCtxDeadline {
		t.Fatal("DeleteIdempotencyKey context must carry a deadline so cleanup cannot block forever")
	}
}

// TestNonSuccessCleanupSurvivesTimeout verifies the non-2xx
// cleanup branch: when the handler returns a 500 after the request
// context has been canceled (e.g. timeout middleware fired), the
// DeleteIdempotencyKey call must still execute against a live ctx.
func TestNonSuccessCleanupSurvivesTimeout(t *testing.T) {
	t.Parallel()

	var (
		mu           sync.Mutex
		deleteCalled bool
		deleteCtxErr error
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, []byte, error) {
			return "acquired", 0, nil, nil
		},
		DeleteIdempotencyKeyFunc: func(ctx context.Context, _, _ string) (int64, error) {
			mu.Lock()
			defer mu.Unlock()
			deleteCalled = true
			deleteCtxErr = ctx.Err()
			return 1, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cancel, ok := r.Context().Value(testCancelKey{}).(context.CancelFunc); ok {
			cancel()
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "non-success-cleanup")
	ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-1")
	cancellable, cancel := context.WithCancel(ctx)
	cancellable = context.WithValue(cancellable, testCancelKey{}, cancel)
	r = r.WithContext(cancellable)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	mu.Lock()
	defer mu.Unlock()
	if !deleteCalled {
		t.Fatal("expected DeleteIdempotencyKey to run on non-2xx cleanup path")
	}
	if deleteCtxErr != nil {
		t.Fatalf("DeleteIdempotencyKey received canceled context: ctx.Err() = %v", deleteCtxErr)
	}
}

// TestCleanupBoundsCleanupDuration is the adversarial guard:
// DeleteIdempotencyKey must complete within ~5s even when the store
// blocks indefinitely. The cleanup timeout protects shutdown ordering
// and prevents leaking goroutines on a wedged store.
func TestCleanupBoundsCleanupDuration(t *testing.T) {
	t.Parallel()

	deleteCh := make(chan struct{})
	deadlineCh := make(chan time.Duration, 1)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, []byte, error) {
			return "acquired", 0, nil, nil
		},
		DeleteIdempotencyKeyFunc: func(ctx context.Context, _, _ string) (int64, error) {
			deadline, ok := ctx.Deadline()
			if ok {
				deadlineCh <- time.Until(deadline)
			} else {
				deadlineCh <- 0
			}
			<-ctx.Done()
			close(deleteCh)
			return 0, ctx.Err()
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "blocking-cleanup")
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	start := time.Now()
	wrapped.ServeHTTP(w, r)
	select {
	case <-deleteCh:
	case <-time.After(6 * time.Second):
		t.Fatalf("DeleteIdempotencyKey did not return within 6s; cleanup timeout missing")
	}
	elapsed := time.Since(start)
	if elapsed > 6*time.Second {
		t.Fatalf("middleware blocked %v on cleanup; expected <= ~5s", elapsed)
	}

	select {
	case d := <-deadlineCh:
		if d <= 0 || d > 6*time.Second {
			t.Fatalf("cleanup deadline = %v, want a finite positive value <= 5s", d)
		}
	default:
		t.Fatal("cleanup ctx had no deadline")
	}
}

type testCancelKey struct{}
