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
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, projectID, key string, _ time.Duration) (string, int, http.Header, []byte, error) {
			mu.Lock()
			defer mu.Unlock()
			acquireKeys = append(acquireKeys, key)
			acquireProjects = append(acquireProjects, projectID)
			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, key string, _ int, _ http.Header, _ []byte) error {
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
	require.Len(t,
		acquireKeys,
		2)
	require.NotEqual(t, acquireKeys[1], acquireKeys[0])

	for _, k := range acquireKeys {
		require.True(
			t, isHexDigest(k))

	}
	require.Len(t,
		completeKeys,
		2)
	require.False(t, completeKeys[0] != acquireKeys[0] ||
		completeKeys[1] !=
			acquireKeys[1])

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
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, http.Header, []byte, error) {
			mu.Lock()
			defer mu.Unlock()
			acquireKeys = append(acquireKeys, key)
			if len(acquireKeys) == 1 {
				return "acquired", 0, nil, nil, nil
			}
			return "complete", http.StatusCreated, nil, cachedBody, nil
		},
		CompleteIdempotencyKeyFunc: func(context.Context, string, string, int, http.Header, []byte) error {
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(cachedBody)
	})
	wrapped := srv.idempotencyMiddleware(handler)

	for range 2 {
		r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
		r.Header.Set("Idempotency-Key", "same-actor-key")
		ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-1")
		ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:alice-key")
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, r)
		require.Equal(t, http.StatusCreated,
			w.Code)
		require.Equal(t, string(cachedBody), w.Body.String())

	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t,
		acquireKeys,
		2)
	require.Equal(t, acquireKeys[1], acquireKeys[0])

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
	require.Len(t,
		acquireKeys,
		2)
	require.NotEqual(t, acquireKeys[1], acquireKeys[0])

}

// TestAnonymousActorBypassesIdempotency pins the bypass behavior: when
// actorFromContext returns the empty string (e.g. an internal-secret
// request, or a misconfigured route that admits an unauthenticated
// caller), the middleware skips dedupe entirely and runs the handler
// directly. Without an actor, two callers in the same project who pick
// the same Idempotency-Key would collapse into one cache entry and one
// of them would silently replay the other's response — a cross-tenant
// disclosure risk. The conservative choice is to let every anonymous
// request execute its handler; trusted internal paths carry their own
// dedupe tokens or are naturally idempotent.
func TestAnonymousActorBypassesIdempotency(t *testing.T) {
	t.Parallel()

	var (
		mu          sync.Mutex
		acquireKeys []string
		handled     int
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
		mu.Lock()
		handled++
		mu.Unlock()
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
	require.Len(t,
		acquireKeys,
		0)
	require.EqualValues(t, 2, handled)

}
