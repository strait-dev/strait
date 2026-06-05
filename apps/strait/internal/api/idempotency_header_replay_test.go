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

// TestCompleteCapturesHandlerHeaders pins the contract that the handler
// headers visible at WriteHeader time are persisted alongside the body.
// Pre-fix CompleteIdempotencyKey only saw status + body, so replays
// silently dropped Content-Type, Location, Set-Cookie, ETag, and any
// custom header the handler set.
func TestCompleteCapturesHandlerHeaders(t *testing.T) {
	t.Parallel()

	var (
		mu          sync.Mutex
		gotHeaders  http.Header
		gotStatus   int
		gotBody     []byte
		completeHit bool
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, status int, headers http.Header, body []byte) error {
			mu.Lock()
			defer mu.Unlock()
			completeHit = true
			gotStatus = status
			gotHeaders = headers
			gotBody = body
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Header().Set("Location", "/v1/jobs/abc")
		w.Header().Set("X-Custom", "alpha")
		w.Header().Add("Set-Cookie", "session=1")
		w.Header().Add("Set-Cookie", "csrf=2")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("<ok/>"))
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "k1")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	mu.Lock()
	defer mu.Unlock()
	require.True(
		t, completeHit,
	)
	require.Equal(t, http.StatusCreated,
		gotStatus,
	)
	require.Equal(t, "<ok/>",
		string(gotBody))
	require.Equal(t, "application/xml",
		gotHeaders.
			Get("Content-Type"))
	require.Equal(t, "/v1/jobs/abc",
		gotHeaders.Get("Location"))
	require.Equal(t, "alpha",
		gotHeaders.Get("X-Custom"))

	if got := gotHeaders.Values("Set-Cookie"); len(got) != 2 || got[0] != "session=1" || got[1] != "csrf=2" {
		require.Failf(t, "test failure",

			"Set-Cookie = %v, want [session=1 csrf=2]", got)
	}
}

// TestCompleteSnapshotsHeadersAtWriteHeader regresses the .Clone()
// invariant: a handler that mutates the response header map after
// WriteHeader (e.g. middleware adding trailers, or an accidental late
// header set) must NOT corrupt the cached snapshot. The cache must
// reflect the exact headers committed to the wire.
func TestCompleteSnapshotsHeadersAtWriteHeader(t *testing.T) {
	t.Parallel()

	var (
		mu         sync.Mutex
		gotHeaders http.Header
	)
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ int, headers http.Header, _ []byte) error {
			mu.Lock()
			defer mu.Unlock()
			gotHeaders = headers
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Snapshot-State", "before")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		// Mutate AFTER WriteHeader; must not leak into cache.
		w.Header().Set("X-Snapshot-State", "after")
		w.Header().Set("X-Late", "leaked")
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "snapshot")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	wrapped.ServeHTTP(httptest.NewRecorder(), r)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "before",
		gotHeaders.Get("X-Snapshot-State"))
	require.Equal(t, "", gotHeaders.
		Get("X-Late"))

}

// TestReplayWritesCachedHeadersVerbatim verifies that a "completed"
// status from the store causes the middleware to write the cached
// headers to the response untouched, with Idempotency-Replayed: true
// added. Multi-valued headers (Set-Cookie) must replay all values.
func TestReplayWritesCachedHeadersVerbatim(t *testing.T) {
	t.Parallel()

	cached := http.Header{
		"Content-Type":    []string{"application/xml"},
		"Location":        []string{"/v1/runs/xyz"},
		"Etag":            []string{`"v1"`},
		"X-Custom-Header": []string{"hello"},
		"Set-Cookie":      []string{"session=abc", "csrf=def"},
	}
	cachedBody := []byte("<replay/>")

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "completed", http.StatusCreated, cached, cachedBody, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		require.Fail(t,

			"handler must not run on replay")
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "replay-1")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code)
	require.Equal(t, "application/xml",
		w.Header().
			Get("Content-Type"))
	require.Equal(t, "/v1/runs/xyz",
		w.Header().Get("Location"))
	require.Equal(t, `"v1"`,
		w.Header().Get("Etag"))
	require.Equal(t, "hello",
		w.Header().Get("X-Custom-Header"))

	if got := w.Header().Values("Set-Cookie"); len(got) != 2 || got[0] != "session=abc" || got[1] != "csrf=def" {
		require.Failf(t, "test failure",

			"Set-Cookie = %v, want [session=abc csrf=def]", got)
	}
	require.Equal(t, "true",
		w.Header().Get("Idempotency-Replayed"))
	require.Equal(t, "<replay/>",
		w.Body.String())

}

// TestReplayLegacyRowFallsBackToJSON regresses the migration safety
// path: pre-migration rows have NULL response_headers, which the store
// surfaces as a nil http.Header. Replays must still succeed by
// defaulting to application/json so older cached rows do not return
// header-less responses.
func TestReplayLegacyRowFallsBackToJSON(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "completed", http.StatusOK, nil, []byte(`{"legacy":true}`), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	wrapped := srv.idempotencyMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		require.Fail(t,

			"handler must not run on replay")
	}))

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "legacy")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, "application/json",
		w.Header().Get("Content-Type"))
	require.Equal(t, "true",
		w.Header().Get("Idempotency-Replayed"))
	require.Equal(t, `{"legacy":true}`,
		w.Body.String())

}

// TestReplayDoesNotEmitContentTypeWhenCachedHasNone is the adversarial
// guard: if the cached header set explicitly has NO Content-Type (e.g.
// a 204 response or a handler that intentionally omitted it), the
// replay must not fabricate one. Only the legacy nil path falls back
// to JSON.
func TestReplayDoesNotEmitContentTypeWhenCachedHasNone(t *testing.T) {
	t.Parallel()

	cached := http.Header{
		"X-Custom": []string{"yes"},
	}
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "completed", http.StatusNoContent, cached, nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	wrapped := srv.idempotencyMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		require.Fail(t,

			"handler must not run on replay")
	}))

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "no-ct")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)
	require.Equal(t, http.StatusNoContent,
		w.Code)
	require.Equal(t, "", w.Header().Get("Content-Type"))
	require.Equal(t, "yes", w.
		Header().Get("X-Custom"))
	require.Equal(t, "true",
		w.Header().Get("Idempotency-Replayed"))

}
