package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleCreateWebhookSubscription_Success(t *testing.T) {
	t.Parallel()

	called := false
	ms := &mockAPIStore{
		createWebhookSubscriptionFn: func(_ context.Context, sub *domain.WebhookSubscription) error {
			called = true
			sub.ID = "sub-1"
			sub.CreatedAt = time.Now().UTC()
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFWebhookSubscriptions = true

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"],"secret":"secret"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("CreateWebhookSubscription was not called")
	}
}

func TestHandleListWebhookSubscriptions_Success(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		listWebhookSubscriptionsFn: func(_ context.Context, projectID string) ([]domain.WebhookSubscription, error) {
			if projectID != "proj-1" {
				t.Fatalf("projectID = %q, want %q", projectID, "proj-1")
			}
			return []domain.WebhookSubscription{{ID: "sub-1", ProjectID: projectID, WebhookURL: "https://example.com/hook", EventTypes: []string{"run.failed"}, Active: true}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFWebhookSubscriptions = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/webhooks/subscriptions?project_id=proj-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var subs []domain.WebhookSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &subs); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("len(subs) = %d, want 1", len(subs))
	}
}

func TestHandleDeleteWebhookSubscription_NotFound(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		deleteWebhookSubscriptionFn: func(_ context.Context, _ string) error {
			return store.ErrWebhookSubscriptionNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFWebhookSubscriptions = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/webhooks/subscriptions/sub-missing", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
