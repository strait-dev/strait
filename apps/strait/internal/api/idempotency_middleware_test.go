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
)

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
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	if !called {
		t.Fatal("expected handler to be called without idempotency header")
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
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
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, projectID, key string, _ time.Duration) (string, int, []byte, error) {
			if projectID != "proj-1" {
				t.Fatalf("unexpected project: %s", projectID)
			}
			// Key should be composite: path:key
			if !strings.Contains(key, "my-key") {
				t.Fatalf("expected key to contain 'my-key', got %s", key)
			}
			return "acquired", 0, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, status int, body []byte) error {
			if status != http.StatusCreated {
				t.Fatalf("expected status 201, got %d", status)
			}
			if string(body) != `{"id":"job-123"}` {
				t.Fatalf("unexpected body: %s", body)
			}
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "my-key")
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	if handlerCalls != 1 {
		t.Fatalf("expected handler to be called once, got %d", handlerCalls)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if len(ms.CompleteIdempotencyKeyCalls()) != 1 {
		t.Fatal("expected CompleteIdempotencyKey to be called")
	}
}

func TestIdempotencyMiddleware_DuplicateKey_ReplaysResponse(t *testing.T) {
	t.Parallel()
	handlerCalls := 0
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		handlerCalls++
	})

	cachedBody := []byte(`{"id":"job-123","status":"created"}`)
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, []byte, error) {
			return "completed", http.StatusCreated, cachedBody, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "my-key")
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	if handlerCalls != 0 {
		t.Fatal("handler should not be called for replayed response")
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if w.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatal("expected Idempotency-Replayed header")
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if body["id"] != "job-123" {
		t.Fatalf("expected id=job-123, got %v", body["id"])
	}
}

func TestIdempotencyMiddleware_AuthorizationRunsBeforeReplay(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(context.Context, string, string, time.Duration) (string, int, []byte, error) {
			t.Fatal("idempotency store must not be consulted before permission checks")
			return "", 0, nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	inner := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not run when caller lacks the route permission")
	})
	handler := srv.requirePermission(domain.ScopeJobsWrite)(srv.idempotencyMiddleware(inner))

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(`{}`))
	req.Header.Set("Idempotency-Key", "cached-create-job")
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:read-only")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 before idempotency replay, got %d: %s", w.Code, w.Body.String())
	}
	if len(ms.TryAcquireIdempotencyKeyCalls()) != 0 {
		t.Fatal("idempotency store was consulted before authorization")
	}
}

func TestCreateAPIKeyRoute_DoesNotCacheRawSecretResponses(t *testing.T) {
	t.Parallel()

	createCalls := 0
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(context.Context, string, string, time.Duration) (string, int, []byte, error) {
			t.Fatal("api key creation responses contain raw secrets and must not be idempotency-cached")
			return "", 0, nil, nil
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if createCalls != 1 {
		t.Fatalf("expected api key creation to run once, got %d", createCalls)
	}
	if len(ms.TryAcquireIdempotencyKeyCalls()) != 0 {
		t.Fatal("api key creation unexpectedly used idempotency cache")
	}
}

func TestIdempotencyMiddleware_PendingKey_Returns409(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, []byte, error) {
			return "pending", 0, nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called when key is pending")
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("X-Idempotency-Key", "in-flight-key")
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestIdempotencyMiddleware_KeyTooLong_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called for oversized key")
	})
	wrapped := srv.idempotencyMiddleware(handler)

	longKey := strings.Repeat("x", maxIdempotencyKeyLength+1)
	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", longKey)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestIdempotencyMiddleware_XHeader_Works(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, []byte, error) {
			if !strings.Contains(key, "x-header-key") {
				t.Fatalf("expected key to contain x-header-key, got %s", key)
			}
			return "acquired", 0, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ int, _ []byte) error {
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
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestIdempotencyMiddleware_ErrorResponse_DeletesPendingKey(t *testing.T) {
	t.Parallel()
	var deleteCalled bool
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, []byte, error) {
			return "acquired", 0, nil, nil
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
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if !deleteCalled {
		t.Fatal("expected DeleteIdempotencyKey to be called for error response")
	}
	if len(ms.CompleteIdempotencyKeyCalls()) != 0 {
		t.Fatal("CompleteIdempotencyKey should NOT be called for error response")
	}
}

func TestIdempotencyMiddleware_KeyScopedToPath(t *testing.T) {
	t.Parallel()
	var capturedKey string
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, []byte, error) {
			capturedKey = key
			return "acquired", 0, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ int, _ []byte) error {
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "same-key")
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	if capturedKey != "/v1/jobs:env::same-key" {
		t.Fatalf("expected composite key '/v1/jobs:env::same-key', got %q", capturedKey)
	}
}

func TestIdempotencyMiddleware_KeyScopedToEnvironment(t *testing.T) {
	t.Parallel()

	var capturedKeys []string
	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, key string, _ time.Duration) (string, int, []byte, error) {
			capturedKeys = append(capturedKeys, key)
			return "acquired", 0, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ int, _ []byte) error {
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
		ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
		ctx = context.WithValue(ctx, ctxEnvironmentIDKey, environmentID)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("environment %s status = %d, want %d", environmentID, w.Code, http.StatusCreated)
		}
	}

	if len(capturedKeys) != 2 {
		t.Fatalf("captured keys = %d, want 2", len(capturedKeys))
	}
	if capturedKeys[0] == capturedKeys[1] {
		t.Fatalf("environment-scoped requests reused idempotency key %q", capturedKeys[0])
	}
	if capturedKeys[0] != "/v1/jobs/job-1/clone:env:env-production:clone-key" {
		t.Fatalf("production key = %q", capturedKeys[0])
	}
	if capturedKeys[1] != "/v1/jobs/job-1/clone:env:env-staging:clone-key" {
		t.Fatalf("staging key = %q", capturedKeys[1])
	}
}

func TestIdempotencyMiddleware_LogsHashInsteadOfRawKey(t *testing.T) {
	rawKey := "raw-client-supplied-key"
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(context.Context, string, string, time.Duration) (string, int, []byte, error) {
			return "acquired", 0, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(context.Context, string, string, int, []byte) error {
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
	req = req.WithContext(context.WithValue(req.Context(), ctxProjectIDKey, "proj-1"))
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	logText := logs.String()
	if strings.Contains(logText, rawKey) {
		t.Fatalf("log output leaked raw idempotency key: %s", logText)
	}
	if !strings.Contains(logText, "idempotency_key_hash") {
		t.Fatalf("log output did not include idempotency hash: %s", logText)
	}
}
