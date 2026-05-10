package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
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
	if !completeHit {
		t.Fatal("CompleteIdempotencyKey was not invoked")
	}
	if gotStatus != http.StatusCreated {
		t.Fatalf("status = %d, want 201", gotStatus)
	}
	if string(gotBody) != "<ok/>" {
		t.Fatalf("body = %q, want <ok/>", gotBody)
	}
	if got := gotHeaders.Get("Content-Type"); got != "application/xml" {
		t.Fatalf("Content-Type = %q, want application/xml", got)
	}
	if got := gotHeaders.Get("Location"); got != "/v1/jobs/abc" {
		t.Fatalf("Location = %q, want /v1/jobs/abc", got)
	}
	if got := gotHeaders.Get("X-Custom"); got != "alpha" {
		t.Fatalf("X-Custom = %q, want alpha", got)
	}
	if got := gotHeaders.Values("Set-Cookie"); len(got) != 2 || got[0] != "session=1" || got[1] != "csrf=2" {
		t.Fatalf("Set-Cookie = %v, want [session=1 csrf=2]", got)
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
		w.Header().Set("X-Phase", "before")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		// Mutate AFTER WriteHeader; must not leak into cache.
		w.Header().Set("X-Phase", "after")
		w.Header().Set("X-Late", "leaked")
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "snapshot")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	wrapped.ServeHTTP(httptest.NewRecorder(), r)

	mu.Lock()
	defer mu.Unlock()
	if got := gotHeaders.Get("X-Phase"); got != "before" {
		t.Fatalf("X-Phase = %q, want before (snapshot must be taken at WriteHeader)", got)
	}
	if got := gotHeaders.Get("X-Late"); got != "" {
		t.Fatalf("X-Late = %q, want empty (post-WriteHeader mutation must not leak)", got)
	}
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
		t.Fatal("handler must not run on replay")
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "replay-1")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/xml" {
		t.Fatalf("Content-Type = %q, want application/xml", got)
	}
	if got := w.Header().Get("Location"); got != "/v1/runs/xyz" {
		t.Fatalf("Location = %q, want /v1/runs/xyz", got)
	}
	if got := w.Header().Get("Etag"); got != `"v1"` {
		t.Fatalf("Etag = %q, want \"v1\"", got)
	}
	if got := w.Header().Get("X-Custom-Header"); got != "hello" {
		t.Fatalf("X-Custom-Header = %q, want hello", got)
	}
	if got := w.Header().Values("Set-Cookie"); len(got) != 2 || got[0] != "session=abc" || got[1] != "csrf=def" {
		t.Fatalf("Set-Cookie = %v, want [session=abc csrf=def]", got)
	}
	if got := w.Header().Get("Idempotency-Replayed"); got != "true" {
		t.Fatalf("Idempotency-Replayed = %q, want true", got)
	}
	if got := w.Body.String(); got != "<replay/>" {
		t.Fatalf("body = %q, want <replay/>", got)
	}
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
		t.Fatal("handler must not run on replay")
	}))

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "legacy")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json (legacy fallback)", got)
	}
	if got := w.Header().Get("Idempotency-Replayed"); got != "true" {
		t.Fatalf("Idempotency-Replayed = %q, want true", got)
	}
	if got := w.Body.String(); got != `{"legacy":true}` {
		t.Fatalf("body = %q, want legacy JSON", got)
	}
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
		t.Fatal("handler must not run on replay")
	}))

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "no-ct")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty (cache had none, must not fabricate)", got)
	}
	if got := w.Header().Get("X-Custom"); got != "yes" {
		t.Fatalf("X-Custom = %q, want yes", got)
	}
	if got := w.Header().Get("Idempotency-Replayed"); got != "true" {
		t.Fatalf("Idempotency-Replayed = %q, want true", got)
	}
}
