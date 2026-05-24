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
	// Not parallel: sets a package-level atomic (globalAllowPrivateEndpoints)
	// that is also written by regression_security_test.go. Running serially
	// avoids a data race between the Store(true) here and Store(false) there.
	globalAllowPrivateEndpoints.Store(true)
	t.Cleanup(func() { globalAllowPrivateEndpoints.Store(false) })

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	srv.config.AllowPrivateEndpoints = true
	globalAllowPrivateEndpoints.Store(true)

	// 192.0.2.1 is RFC 5737 TEST-NET-1, guaranteed unreachable. The SSRF
	// guard is bypassed above so the HTTP client attempts the connection
	// and fails naturally, producing success:false.
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

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTestWebhook_URLValidationErrorIsGeneric(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	body, _ := json.Marshal(map[string]string{"url": "https://127.0.0.1:8443/hook?token=secret"})
	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/test", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleTestWebhook)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	response := w.Body.String()
	if !strings.Contains(response, "invalid webhook URL") {
		t.Fatalf("response = %q, want generic invalid webhook URL message", response)
	}
	for _, leaked := range []string{"127.0.0.1", "token=secret", "private", "loopback"} {
		if strings.Contains(response, leaked) {
			t.Fatalf("response leaked validation detail %q: %s", leaked, response)
		}
	}
}

func TestHandleTestWebhook_MissingURL(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/test", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleTestWebhook)(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing url, got %d", w.Code)
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
				ID:         "new-replay-id",
				WebhookURL: "https://user:pass@hooks.example.com/private/path?token=secret#frag",
				Status:     domain.WebhookStatusPending,
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
	if strings.Contains(w.Body.String(), "token=secret") ||
		strings.Contains(w.Body.String(), "user:pass") ||
		strings.Contains(w.Body.String(), "/private/path") {
		t.Fatalf("replay response leaked webhook URL secret: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "https://hooks.example.com") {
		t.Fatalf("replay response should keep redacted webhook host, got: %s", w.Body.String())
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

func TestHandleReplayWebhookDelivery_EnvironmentScopedCallerCannotReplayOtherEnvironment(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(_ context.Context, _ string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: "del-1", JobID: "job-1", ProjectID: "proj-1"}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
		ReplayWebhookDeliveryFunc: func(_ context.Context, _ string) (*domain.WebhookDelivery, error) {
			t.Fatal("ReplayWebhookDelivery should not be called for a mismatched environment")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleReplayWebhookDelivery(ctx, &ReplayWebhookDeliveryInput{ID: "del-1"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 for environment mismatch, got %v", err)
	}
}

func TestHandleReplayWebhookDelivery_EnvironmentScopedCallerCannotReplayUnscopedSubscriptionDelivery(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(_ context.Context, _ string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: "del-1", ProjectID: "proj-1", SubscriptionID: "sub-1"}, nil
		},
		ReplayWebhookDeliveryFunc: func(_ context.Context, _ string) (*domain.WebhookDelivery, error) {
			t.Fatal("ReplayWebhookDelivery should not be called for an env-scoped caller without job environment")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleReplayWebhookDelivery(ctx, &ReplayWebhookDeliveryInput{ID: "del-1"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 for env-scoped subscription delivery replay, got %v", err)
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
