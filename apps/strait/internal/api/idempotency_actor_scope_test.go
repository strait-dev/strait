package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestDifferentActorsHaveSeparateCacheEntries pins the new
// composite-key shape: two callers in the same project who happen to
// pick the same Idempotency-Key string must NOT share a cache entry.
// Otherwise a low-privilege actor could read another actor's cached
// response by guessing or reusing a key.
func TestDifferentActorsHaveSeparateCacheEntries(t *testing.T) {
	t.Parallel()

	var (
		mu              sync.Mutex
		acquireKeys     []string
		completeKeys    []string
		acquireProjects []string
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, projectID, key string, _ time.Duration) (string, int, []byte, error) {
			mu.Lock()
			defer mu.Unlock()
			acquireKeys = append(acquireKeys, key)
			acquireProjects = append(acquireProjects, projectID)
			return "acquired", 0, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, key string, _ int, _ []byte) error {
			mu.Lock()
			defer mu.Unlock()
			completeKeys = append(completeKeys, key)
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	wrapped := srv.idempotencyMiddleware(handler)

	for _, actorID := range []string{"apikey:alice-key", "apikey:bob-key"} {
		r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
		r.Header.Set("Idempotency-Key", "shared-key")
		ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-1")
		ctx = context.WithValue(ctx, ctxActorIDKey, actorID)
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, r)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(acquireKeys) != 2 {
		t.Fatalf("TryAcquireIdempotencyKey calls = %d, want 2", len(acquireKeys))
	}
	if acquireKeys[0] == acquireKeys[1] {
		t.Fatalf("expected per-actor composite keys to differ, both = %q", acquireKeys[0])
	}
	for i, k := range acquireKeys {
		if !isHexDigest(k) {
			t.Fatalf("acquireKeys[%d] = %q, want hashed digest", i, k)
		}
	}
	if len(completeKeys) != 2 {
		t.Fatalf("CompleteIdempotencyKey calls = %d, want 2", len(completeKeys))
	}
	if completeKeys[0] != acquireKeys[0] || completeKeys[1] != acquireKeys[1] {
		t.Fatal("Complete must use the same actor-scoped composite key as Acquire")
	}
}

// TestSameActorReplaysCache regresses the happy path: the same
// actor calling twice with the same key reaches the same composite
// key, so a "complete" status from the store replays the cached body.
func TestSameActorReplaysCache(t *testing.T) {
	t.Parallel()

	cachedBody := []byte(`{"replay":true}`)
	var (
		mu          sync.Mutex
		acquireKeys []string
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, []byte, error) {
			mu.Lock()
			defer mu.Unlock()
			acquireKeys = append(acquireKeys, key)
			if len(acquireKeys) == 1 {
				return "acquired", 0, nil, nil
			}
			return "complete", http.StatusCreated, cachedBody, nil
		},
		CompleteIdempotencyKeyFunc: func(context.Context, string, string, int, []byte) error {
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(cachedBody)
	})
	wrapped := srv.idempotencyMiddleware(handler)

	for i := range 2 {
		r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
		r.Header.Set("Idempotency-Key", "same-actor-key")
		ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-1")
		ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:alice-key")
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("iteration %d: status = %d, want 201", i, w.Code)
		}
		if w.Body.String() != string(cachedBody) {
			t.Fatalf("iteration %d: body = %q, want %q", i, w.Body.String(), cachedBody)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(acquireKeys) != 2 {
		t.Fatalf("TryAcquireIdempotencyKey calls = %d, want 2", len(acquireKeys))
	}
	if acquireKeys[0] != acquireKeys[1] {
		t.Fatalf("same actor + key must produce identical composite keys; got %q vs %q", acquireKeys[0], acquireKeys[1])
	}
}

// TestOIDCAndAPIKeyActorsAreDistinct is the adversarial guard:
// an OIDC user and an API key sharing the same key string in the same
// project must hash to different composite keys, even when their actor
// id strings happen to overlap after stripping the prefix.
func TestOIDCAndAPIKeyActorsAreDistinct(t *testing.T) {
	t.Parallel()

	var (
		mu          sync.Mutex
		acquireKeys []string
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, []byte, error) {
			mu.Lock()
			defer mu.Unlock()
			acquireKeys = append(acquireKeys, key)
			return "acquired", 0, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(context.Context, string, string, int, []byte) error {
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	wrapped := srv.idempotencyMiddleware(handler)

	for _, actorID := range []string{"apikey:overlap", "user:overlap"} {
		r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
		r.Header.Set("Idempotency-Key", "shared-key")
		ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-1")
		ctx = context.WithValue(ctx, ctxActorIDKey, actorID)
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, r)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(acquireKeys) != 2 {
		t.Fatalf("TryAcquireIdempotencyKey calls = %d, want 2", len(acquireKeys))
	}
	if acquireKeys[0] == acquireKeys[1] {
		t.Fatalf("OIDC user and API key must produce distinct composite keys; both = %q", acquireKeys[0])
	}
}

// TestAnonymousActorStillScoped ensures the middleware does not
// crash and produces a deterministic composite key when actorFromContext
// returns the empty string (e.g. an internal-secret request that still
// happens to carry an Idempotency-Key). Two anonymous calls with the
// same key MUST collide, so the middleware still serializes them.
func TestAnonymousActorStillScoped(t *testing.T) {
	t.Parallel()

	var (
		mu          sync.Mutex
		acquireKeys []string
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, []byte, error) {
			mu.Lock()
			defer mu.Unlock()
			acquireKeys = append(acquireKeys, key)
			return "acquired", 0, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(context.Context, string, string, int, []byte) error {
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	wrapped := srv.idempotencyMiddleware(handler)

	for range 2 {
		r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
		r.Header.Set("Idempotency-Key", "anon-key")
		r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, r)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(acquireKeys) != 2 {
		t.Fatalf("TryAcquireIdempotencyKey calls = %d, want 2", len(acquireKeys))
	}
	if acquireKeys[0] != acquireKeys[1] {
		t.Fatalf("anonymous calls with the same key must collide; got %q vs %q", acquireKeys[0], acquireKeys[1])
	}
}
