package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
			if projectID != "proj-1" || key != "my-key" {
				t.Fatalf("unexpected args: %s %s", projectID, key)
			}
			return "acquired", 0, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, projectID, key string, status int, body []byte) error {
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
			if key != "x-header-key" {
				t.Fatalf("expected x-header-key, got %s", key)
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
