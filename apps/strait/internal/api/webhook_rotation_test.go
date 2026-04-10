package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

func TestRotateWebhookSecret_Success(t *testing.T) {
	t.Parallel()
	var rotatedID, rotatedSecret string
	var rotatedGrace time.Time

	ms := &APIStoreMock{
		GetWebhookSubscriptionFunc: func(_ context.Context, id string) (*domain.WebhookSubscription, error) {
			return &domain.WebhookSubscription{ID: id, ProjectID: "proj-1", Secret: "old-secret"}, nil
		},
		RotateWebhookSecretFunc: func(_ context.Context, id, newSecret string, grace time.Time) error {
			rotatedID = id
			rotatedSecret = newSecret
			rotatedGrace = grace
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"grace_period_minutes": 120}`
	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/subscriptions/sub-1/rotate-secret", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Internal-Secret", testInternalSecret)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "sub-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleRotateWebhookSecret)(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if rotatedID != "sub-1" {
		t.Fatalf("expected rotated ID sub-1, got %s", rotatedID)
	}
	if rotatedSecret == "" || rotatedSecret == "old-secret" {
		t.Fatal("expected new secret to be generated")
	}
	if rotatedSecret[:6] != "whsec_" {
		t.Fatalf("expected whsec_ prefix, got %s", rotatedSecret[:6])
	}
	if time.Until(rotatedGrace) < 119*time.Minute {
		t.Fatalf("grace period too short: %v", rotatedGrace)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["new_secret"] == nil {
		t.Fatal("response should contain new_secret")
	}
}

func TestRotateWebhookSecret_DefaultGracePeriod(t *testing.T) {
	t.Parallel()
	var graceMins float64

	ms := &APIStoreMock{
		GetWebhookSubscriptionFunc: func(_ context.Context, id string) (*domain.WebhookSubscription, error) {
			return &domain.WebhookSubscription{ID: id, ProjectID: "proj-1"}, nil
		},
		RotateWebhookSecretFunc: func(_ context.Context, _, _ string, grace time.Time) error {
			graceMins = time.Until(grace).Minutes()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/subscriptions/sub-1/rotate-secret", bytes.NewBufferString(`{}`))
	r.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "sub-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleRotateWebhookSecret)(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// Default is 60 minutes.
	if graceMins < 59 {
		t.Fatalf("default grace should be ~60 min, got %.1f", graceMins)
	}
}

func TestRotateWebhookSecret_MaxGracePeriodExceeded(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWebhookSubscriptionFunc: func(_ context.Context, id string) (*domain.WebhookSubscription, error) {
			return &domain.WebhookSubscription{ID: id, ProjectID: "proj-1"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"grace_period_minutes": 20000}`
	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/subscriptions/sub-1/rotate-secret", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "sub-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleRotateWebhookSecret)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for exceeded grace period, got %d", w.Code)
	}
}

func TestRotateWebhookSecret_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWebhookSubscriptionFunc: func(_ context.Context, _ string) (*domain.WebhookSubscription, error) {
			return nil, store.ErrWebhookSubscriptionNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/subscriptions/nonexistent/rotate-secret", bytes.NewBufferString(`{}`))
	r.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "nonexistent")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleRotateWebhookSecret)(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRotateWebhookSecret_WrongProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWebhookSubscriptionFunc: func(_ context.Context, id string) (*domain.WebhookSubscription, error) {
			return &domain.WebhookSubscription{ID: id, ProjectID: "other-project"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/subscriptions/sub-1/rotate-secret", bytes.NewBufferString(`{}`))
	r.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "sub-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleRotateWebhookSecret)(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong project, got %d", w.Code)
	}
}

func TestRotateWebhookSecret_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWebhookSubscriptionFunc: func(_ context.Context, id string) (*domain.WebhookSubscription, error) {
			return &domain.WebhookSubscription{ID: id, ProjectID: "proj-1"}, nil
		},
		RotateWebhookSecretFunc: func(_ context.Context, _, _ string, _ time.Time) error {
			return errors.New("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/subscriptions/sub-1/rotate-secret", bytes.NewBufferString(`{}`))
	r.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "sub-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleRotateWebhookSecret)(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
