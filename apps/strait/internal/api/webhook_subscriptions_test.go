package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

func TestHandleCreateWebhookSubscription_Success(t *testing.T) {
	t.Parallel()

	called := false
	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, sub *domain.WebhookSubscription) error {
			called = true
			sub.ID = "sub-1"
			sub.CreatedAt = time.Now().UTC()
			return nil
		},
	}

	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

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

	ms := &APIStoreMock{
		ListWebhookSubscriptionsFunc: func(_ context.Context, projectID string) ([]domain.WebhookSubscription, error) {
			if projectID != "proj-1" {
				t.Fatalf("projectID = %q, want %q", projectID, "proj-1")
			}
			return []domain.WebhookSubscription{{ID: "sub-1", ProjectID: projectID, WebhookURL: "https://example.com/hook", EventTypes: []string{"run.failed"}, Active: true}}, nil
		},
	}

	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/webhooks/subscriptions", "", "proj-1"))

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

func TestWebhookSubscriptions_EnvironmentScopedKeyRejected(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-staging")

	srv := newTestServer(t, &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(context.Context, *domain.WebhookSubscription) error {
			t.Fatal("CreateWebhookSubscription must not be called for environment-scoped caller")
			return nil
		},
		ListWebhookSubscriptionsFunc: func(context.Context, string) ([]domain.WebhookSubscription, error) {
			t.Fatal("ListWebhookSubscriptions must not be called for environment-scoped caller")
			return nil, nil
		},
		GetWebhookSubscriptionFunc: func(context.Context, string) (*domain.WebhookSubscription, error) {
			t.Fatal("GetWebhookSubscription must not be called for environment-scoped caller")
			return nil, nil
		},
		RotateWebhookSecretFunc: func(context.Context, string, string, time.Time) error {
			t.Fatal("RotateWebhookSecret must not be called for environment-scoped caller")
			return nil
		},
	}, &mockQueue{}, nil)

	cases := []struct {
		name string
		call func() error
	}{
		{
			name: "create",
			call: func() error {
				_, err := srv.handleCreateWebhookSubscription(ctx, &CreateWebhookSubscriptionInput{Body: CreateWebhookSubscriptionRequest{
					ProjectID:  "proj-1",
					WebhookURL: "https://example.com/hook",
					EventTypes: []string{"run.completed"},
				}})
				return err
			},
		},
		{
			name: "list",
			call: func() error {
				_, err := srv.handleListWebhookSubscriptions(ctx, &ListWebhookSubscriptionsInput{})
				return err
			},
		},
		{
			name: "delete",
			call: func() error {
				_, err := srv.handleDeleteWebhookSubscription(ctx, &DeleteWebhookSubscriptionInput{ID: "sub-1"})
				return err
			},
		},
		{
			name: "rotate",
			call: func() error {
				_, err := srv.handleRotateWebhookSecret(ctx, &RotateWebhookSecretInput{ID: "sub-1"})
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			var statusErr huma.StatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("expected huma.StatusError, got %T: %v", err, err)
			}
			if statusErr.GetStatus() != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", statusErr.GetStatus())
			}
		})
	}
}

func TestHandleDeleteWebhookSubscription_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWebhookSubscriptionFunc: func(_ context.Context, _ string) (*domain.WebhookSubscription, error) {
			return nil, store.ErrWebhookSubscriptionNotFound
		},
		DeleteWebhookSubscriptionFunc: func(_ context.Context, _ string) error {
			return store.ErrWebhookSubscriptionNotFound
		},
	}

	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/webhooks/subscriptions/sub-missing", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleCreateWebhookSubscription_InvalidEventType(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["invalid.event"],"secret":"secret"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid event type, got %d: %s", w.Code, w.Body.String())
	}
}
