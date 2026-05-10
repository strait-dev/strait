package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestCanonicalizeIdempotencyPath is the unit-level table for the
// canonicalization rules: same logical resource regardless of trailing
// slash, double slashes, dot segments, or case.
func TestCanonicalizeIdempotencyPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "/"},
		{"root", "/", "/"},
		{"plain", "/v1/jobs", "/v1/jobs"},
		{"trailing_slash", "/v1/jobs/", "/v1/jobs"},
		{"double_slash", "/v1//jobs", "/v1/jobs"},
		{"triple_slash", "/v1///jobs", "/v1/jobs"},
		{"dot_segment", "/v1/./jobs", "/v1/jobs"},
		{"parent_segment", "/v1/foo/../jobs", "/v1/jobs"},
		{"uppercase", "/V1/Jobs", "/v1/jobs"},
		{"mixed_case_with_slash", "/V1/JOBS/", "/v1/jobs"},
		{"already_clean", "/v1/jobs/abc", "/v1/jobs/abc"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := canonicalizeIdempotencyPath(tc.in); got != tc.want {
				t.Fatalf("canonicalizeIdempotencyPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestIdempotencyKeySurvivesPathCosmetics is the middleware-level
// regression: two requests that differ only in trailing slash, double
// slashes, or case must hash to the SAME composite key, so the second
// call replays the cached response instead of re-executing.
func TestIdempotencyKeySurvivesPathCosmetics(t *testing.T) {
	t.Parallel()

	cases := [][2]string{
		{"/v1/jobs", "/v1/jobs/"},
		{"/v1/jobs", "/v1//jobs"},
		{"/v1/jobs", "/V1/Jobs"},
		{"/v1/jobs/abc", "/v1/jobs/./abc"},
	}

	for _, pair := range cases {
		t.Run(pair[0]+"_vs_"+pair[1], func(t *testing.T) {
			t.Parallel()

			var (
				mu          sync.Mutex
				acquireKeys []string
			)
			ms := &APIStoreMock{
				TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, http.Header, []byte, error) {
					mu.Lock()
					defer mu.Unlock()
					acquireKeys = append(acquireKeys, key)
					return "acquired", 0, nil, nil, nil
				},
				CompleteIdempotencyKeyFunc: func(context.Context, string, string, int, http.Header, []byte) error {
					return nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			wrapped := srv.idempotencyMiddleware(handler)

			for _, p := range pair {
				r := httptest.NewRequest(http.MethodPost, p, nil)
				r.Header.Set("Idempotency-Key", "shared")
				ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-1")
				ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:alice")
				r = r.WithContext(ctx)
				wrapped.ServeHTTP(httptest.NewRecorder(), r)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(acquireKeys) != 2 {
				t.Fatalf("acquire calls = %d, want 2", len(acquireKeys))
			}
			if acquireKeys[0] != acquireKeys[1] {
				t.Fatalf("paths %q and %q must canonicalize to the same composite key, got %q vs %q",
					pair[0], pair[1], acquireKeys[0], acquireKeys[1])
			}
		})
	}
}

// TestIdempotencyKeyDistinguishesDifferentResources is the negative
// guard: canonicalization must NOT collapse genuinely different paths.
func TestIdempotencyKeyDistinguishesDifferentResources(t *testing.T) {
	t.Parallel()

	pairs := [][2]string{
		{"/v1/jobs", "/v1/runs"},
		{"/v1/jobs/abc", "/v1/jobs/def"},
		{"/v1/jobs", "/v2/jobs"},
	}

	for _, pair := range pairs {
		t.Run(pair[0]+"_vs_"+pair[1], func(t *testing.T) {
			t.Parallel()

			var (
				mu          sync.Mutex
				acquireKeys []string
			)
			ms := &APIStoreMock{
				TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, http.Header, []byte, error) {
					mu.Lock()
					defer mu.Unlock()
					acquireKeys = append(acquireKeys, key)
					return "acquired", 0, nil, nil, nil
				},
				CompleteIdempotencyKeyFunc: func(context.Context, string, string, int, http.Header, []byte) error {
					return nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			wrapped := srv.idempotencyMiddleware(handler)

			for _, p := range pair {
				r := httptest.NewRequest(http.MethodPost, p, nil)
				r.Header.Set("Idempotency-Key", "shared")
				ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-1")
				ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:alice")
				r = r.WithContext(ctx)
				wrapped.ServeHTTP(httptest.NewRecorder(), r)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(acquireKeys) != 2 {
				t.Fatalf("acquire calls = %d, want 2", len(acquireKeys))
			}
			if acquireKeys[0] == acquireKeys[1] {
				t.Fatalf("distinct paths %q and %q produced identical composite key %q",
					pair[0], pair[1], acquireKeys[0])
			}
		})
	}
}
