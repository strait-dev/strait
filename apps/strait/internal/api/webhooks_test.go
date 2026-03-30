package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fmt"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

func TestHandleTestWebhook_TargetUnreachable(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	// Use a valid public URL that will fail to connect (port 1 is typically unreachable)
	body, _ := json.Marshal(map[string]string{"url": "https://192.0.2.1:443/webhook"})
	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/test", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleTestWebhook)(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even for failed connection, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if resp["success"] != false {
		t.Fatalf("expected success=false for unreachable target, got %v", resp["success"])
	}
	if resp["error"] == nil || resp["error"] == "" {
		t.Fatal("expected error field for unreachable target")
	}
}

func TestHandleTestWebhook_InvalidURL(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	body, _ := json.Marshal(map[string]string{"url": "not-a-url"})
	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/test", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleTestWebhook)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTestWebhook_MissingURL(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/test", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleTestWebhook)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing url, got %d", w.Code)
	}
}

func TestHandleReplayWebhookDelivery_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(_ context.Context, id string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{
				ID:       id,
				JobID:    "job-1",
				RunID:    "run-1",
				Status:   domain.WebhookStatusDelivered,
				Attempts: 1,
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		ReplayWebhookDeliveryFunc: func(_ context.Context, _ string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{
				ID:     "new-replay-id",
				Status: domain.WebhookStatusPending,
			}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/deliveries/del-1/replay", nil)
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "del-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusCreated, srv.handleReplayWebhookDelivery)(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReplayWebhookDelivery_WrongProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(_ context.Context, _ string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: "del-1", JobID: "job-1"}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "other-project"}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/deliveries/del-1/replay", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "del-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "my-project"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusCreated, srv.handleReplayWebhookDelivery)(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong project, got %d", w.Code)
	}
}

func TestHandleReplayWebhookDelivery_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(context.Context, string) (*domain.WebhookDelivery, error) {
			return nil, fmt.Errorf("webhook delivery not found")
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/deliveries/del-1/replay", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "del-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusCreated, srv.handleReplayWebhookDelivery)(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
