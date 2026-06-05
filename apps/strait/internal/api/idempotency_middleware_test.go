package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// idempotencyTestCtx populates the project + actor context the
// idempotencyMiddleware now requires post-Fix #10. Tests that wrap the
// middleware directly (no router auth) need both keys set so the
// composite-key path runs.
func idempotencyTestCtx(parent context.Context, projectID string) context.Context {
	ctx := context.WithValue(parent, ctxProjectIDKey, projectID)
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-actor")
	return ctx
}

// isHexDigest returns true if s is a 64-character lowercase SHA-256
// hex digest, the format produced by idempotencyCompositeKey.
func isHexDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}

func TestIdempotencyMiddleware_NoHeader_PassThrough(t *testing.T) {
	t.Parallel()
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new-resource"}`))
	})

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)
	require.True(t, called)
	require.Equal(t, http.
		StatusCreated, w.
		Code)

}

func TestIdempotencyMiddleware_NewKey_ExecutesHandler(t *testing.T) {
	t.Parallel()
	handlerCalls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"job-123"}`))
	})

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, projectID, key string, _ time.Duration) (string, int, http.Header, []byte, error) {
			require.Equal(t, "proj-1",
				projectID)
			require.True(t, isHexDigest(key))

			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, status int, _ http.Header, body []byte) error {
			require.Equal(t, http.
				StatusCreated, status,
			)
			require.Equal(t, `{"id":"job-123"}`,
				string(body),
			)

			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "my-key")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)
	require.EqualValues(t, 1, handlerCalls)
	require.Equal(t, http.
		StatusCreated, w.
		Code)
	require.Len(t, ms.CompleteIdempotencyKeyCalls(),
		1)

}

func TestIdempotencyMiddleware_DuplicateKey_ReplaysResponse(t *testing.T) {
	t.Parallel()
	handlerCalls := 0
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		handlerCalls++
	})

	cachedBody := []byte(`{"id":"job-123","status":"created"}`)
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "completed", http.StatusCreated, nil, cachedBody, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "my-key")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)
	require.EqualValues(t, 0, handlerCalls)
	require.Equal(t, http.
		StatusCreated, w.
		Code)
	require.Equal(t, "true",
		w.Header().Get("Idempotency-Replayed"))

	var body map[string]any
	require.NoError(t, json.
		Unmarshal(w.Body.
			Bytes(),
			&body))
	require.Equal(t, "job-123",
		body["id"])

}

func TestIdempotencyMiddleware_AuthorizationRunsBeforeReplay(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(context.Context, string, string, time.Duration) (string, int, http.Header, []byte, error) {
			require.Fail(t,

				"idempotency store must not be consulted before permission checks")
			return "", 0, nil, nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	inner := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		require.Fail(t,

			"handler must not run when caller lacks the route permission")
	})
	handler := srv.requirePermission(domain.ScopeJobsWrite)(srv.idempotencyMiddleware(inner))

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(`{}`))
	req.Header.Set("Idempotency-Key", "cached-create-job")
	ctx := idempotencyTestCtx(req.Context(), "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:read-only")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusForbidden,
		w.Code)
	require.Len(t, ms.TryAcquireIdempotencyKeyCalls(),
		0)

}

func TestCreateAPIKeyRoute_DoesNotCacheRawSecretResponses(t *testing.T) {
	t.Parallel()

	createCalls := 0
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(context.Context, string, string, time.Duration) (string, int, http.Header, []byte, error) {
			require.Fail(t,

				"api key creation responses contain raw secrets and must not be idempotency-cached")
			return "", 0, nil, nil, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			createCalls++
			key.ID = "key-created"
			key.CreatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedProjectRequest(http.MethodPost, "/v1/api-keys", `{"project_id":"proj-1","name":"deploy","scopes":["jobs:read"],"expires_in_days":30}`, "proj-1")
	req.Header.Set("Idempotency-Key", "create-api-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusCreated, w.
		Code)
	require.EqualValues(t, 1, createCalls)
	require.Len(t, ms.TryAcquireIdempotencyKeyCalls(),
		0)

}

func TestCreateWebhookSubscriptionRoute_DoesNotCacheSigningSecretResponses(t *testing.T) {
	t.Parallel()

	createCalls := 0
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(context.Context, string, string, time.Duration) (string, int, http.Header, []byte, error) {
			require.Fail(t,

				"webhook subscription creation responses contain raw signing secrets and must not be idempotency-cached")
			return "", 0, nil, nil, nil
		},
		CreateWebhookSubscriptionFunc: func(_ context.Context, sub *domain.WebhookSubscription) error {
			createCalls++
			sub.ID = "sub-created"
			sub.CreatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	req := authedProjectRequest(http.MethodPost, "/v1/webhooks/subscriptions", `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"]}`, "proj-1")
	req.Header.Set("Idempotency-Key", "create-webhook-subscription")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusCreated, w.
		Code)
	require.EqualValues(t, 1, createCalls)
	require.Len(t, ms.TryAcquireIdempotencyKeyCalls(),
		0)

}

func TestIdempotencyMiddleware_PendingKey_Returns409(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "pending", 0, nil, nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		require.Fail(t,

			"handler should not be called when key is pending")
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("X-Idempotency-Key", "in-flight-key")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusConflict, w.
		Code)

}

func TestIdempotencyMiddleware_KeyTooLong_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		require.Fail(t,

			"handler should not be called for oversized key")
	})
	wrapped := srv.idempotencyMiddleware(handler)

	longKey := strings.Repeat("x", maxIdempotencyKeyLength+1)
	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", longKey)
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusBadRequest,
		w.Code)

}

func TestIdempotencyMiddleware_XHeader_Works(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, http.Header, []byte, error) {
			require.True(t, isHexDigest(key))

			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ int, _ http.Header, _ []byte) error {
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("X-Idempotency-Key", "x-header-key")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusCreated, w.
		Code)

}

func TestIdempotencyMiddleware_ErrorResponse_DeletesPendingKey(t *testing.T) {
	t.Parallel()
	var deleteCalled bool
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "acquired", 0, nil, nil, nil
		},
		DeleteIdempotencyKeyFunc: func(_ context.Context, _, _ string) (int64, error) {
			deleteCalled = true
			return 1, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"something broke"}`))
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "fail-key")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusInternalServerError,

		w.Code)
	require.True(t, deleteCalled)
	require.Len(t, ms.CompleteIdempotencyKeyCalls(),
		0)

}

func TestIdempotencyMiddleware_KeyScopedToPath(t *testing.T) {
	t.Parallel()
	var captured []string
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, http.Header, []byte, error) {
			captured = append(captured, key)
			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ int, _ http.Header, _ []byte) error {
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	wrapped := srv.idempotencyMiddleware(handler)

	for _, path := range []string{"/v1/jobs", "/v1/runs"} {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		r.Header.Set("Idempotency-Key", "same-key")
		r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, r)
	}
	require.Len(t, captured,
		2)

	for _, k := range captured {
		require.True(t, isHexDigest(k))

	}
	require.NotEqual(t, captured[1], captured[0])

}

func TestIdempotencyMiddleware_KeyScopedToEnvironment(t *testing.T) {
	t.Parallel()

	var capturedKeys []string
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, http.Header, []byte, error) {
			capturedKeys = append(capturedKeys, key)
			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ int, _ http.Header, _ []byte) error {
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	wrapped := srv.idempotencyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	for _, environmentID := range []string{"env-production", "env-staging"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-1/clone", nil)
		req.Header.Set("Idempotency-Key", "clone-key")
		ctx := idempotencyTestCtx(req.Context(), "proj-1")
		ctx = context.WithValue(ctx, ctxEnvironmentIDKey, environmentID)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)
		require.Equal(t, http.
			StatusCreated, w.
			Code)

	}
	require.Len(t, capturedKeys,
		2)

	for _, k := range capturedKeys {
		require.True(t, isHexDigest(k))

	}
	require.NotEqual(t, capturedKeys[1], capturedKeys[0])

}

func TestIdempotencyMiddleware_LogsHashInsteadOfRawKey(t *testing.T) {
	rawKey := "raw-client-supplied-key"
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(context.Context, string, string, time.Duration) (string, int, http.Header, []byte, error) {
			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(context.Context, string, string, int, http.Header, []byte) error {
			return errors.New("forced complete failure")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	wrapped := srv.idempotencyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	req.Header.Set("Idempotency-Key", rawKey)
	req = req.WithContext(idempotencyTestCtx(req.Context(), "proj-1"))
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	logText := logs.String()
	require.False(t, strings.Contains(logText,
		rawKey,
	))
	require.True(t, strings.Contains(logText,
		"idempotency_key_hash",
	))

}

func TestTriggerRoute_IdempotencyReplaySkipsDebounceMutation(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(context.Context, string, string, time.Duration) (string, int, http.Header, []byte, error) {
			return "completed", http.StatusCreated, nil, []byte(`{"debounced":true,"idempotency_hit":true}`), nil
		},
		UpsertDebouncePendingFunc: func(context.Context, *domain.DebouncePending) error {
			require.Fail(t,

				"debounce mutation must not run for idempotency replay")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"a":1}}`, "proj-1")
	req.Header.Set("Idempotency-Key", "debounce-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusCreated, w.
		Code)
	require.Equal(t, "true",
		w.Header().Get("Idempotency-Replayed"))
	require.Len(t, ms.TryAcquireIdempotencyKeyCalls(),
		1)

}

func TestTriggerRoute_IdempotencyReplaySkipsBatchMutation(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(context.Context, string, string, time.Duration) (string, int, http.Header, []byte, error) {
			return "completed", http.StatusCreated, nil, []byte(`{"buffered":true,"idempotency_hit":true}`), nil
		},
		InsertBatchBufferItemFunc: func(context.Context, *domain.BatchBufferItem) error {
			require.Fail(t,

				"batch buffer mutation must not run for idempotency replay")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"a":1}}`, "proj-1")
	req.Header.Set("Idempotency-Key", "batch-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusCreated, w.
		Code)
	require.Equal(t, "true",
		w.Header().Get("Idempotency-Replayed"))
	require.Len(t, ms.TryAcquireIdempotencyKeyCalls(),
		1)

}

// TestIdempotencyMiddleware_LogsUnknownStoreStatus pins the visibility
// guarantee for the default branch: if the store ever returns a status
// outside the documented set ("acquired" / "pending" / "completed"),
// the middleware must log an error before falling through. Otherwise
// a future store change that silently introduces a new status would
// degrade idempotency to "no-op" with no operator-visible signal.
func TestIdempotencyMiddleware_LogsUnknownStoreStatus(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	const bogusStatus = "totally-unknown-status"
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(context.Context, string, string, time.Duration) (string, int, http.Header, []byte, error) {
			return bogusStatus, 0, nil, nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	handlerCalls := 0
	wrapped := srv.idempotencyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalls++
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	req.Header.Set("Idempotency-Key", "any-key")
	req = req.WithContext(idempotencyTestCtx(req.Context(), "proj-1"))
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	require.EqualValues(t, 1, handlerCalls)
	require.Equal(t, http.
		StatusOK, rec.Code,
	)

	logText := logs.String()
	require.True(t, strings.Contains(logText,
		"unrecognized status",
	))
	require.True(t, strings.Contains(logText,
		bogusStatus,
	))
	require.True(t, strings.Contains(logText,
		"level=ERROR",
	))

}
