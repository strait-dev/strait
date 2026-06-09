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
	"github.com/stretchr/testify/require"
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
	enc := &mockEncryptor{}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, enc)

	body := `{"grace_period_minutes": 120}`
	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/subscriptions/sub-1/rotate-secret", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "sub-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleRotateWebhookSecret)(w, r)
	require.Equal(t, http.
		StatusOK, w.Code)
	require.Equal(t, "sub-1",
		rotatedID)
	require.False(t, rotatedSecret ==
		"" ||
		rotatedSecret ==
			"old-secret",
	)
	require.GreaterOrEqual(t, time.Until(rotatedGrace), 119*time.
		Minute)

	var resp map[string]any
	require.NoError(t, json.
		Unmarshal(w.Body.
			Bytes(),
			&resp))
	require.NotNil(t, resp["new_secret"])

	newSecret, ok := resp["new_secret"].(string)
	require.True(t, ok)

	requireBase64EncryptedSecretPlaintext(t, enc, rotatedSecret, newSecret)

	// Regression (N7): the response is a concrete typed schema, not an untyped
	// map, so it round-trips into RotateWebhookSecretResponse with every field set.
	var typed RotateWebhookSecretResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &typed))
	require.Equal(t, "sub-1", typed.SubscriptionID)
	require.Equal(t, newSecret, typed.NewSecret)
	require.Equal(t, 120, typed.GracePeriodMinutes)
	require.False(t, typed.GraceExpiresAt.IsZero())
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
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/subscriptions/sub-1/rotate-secret", bytes.NewBufferString(`{}`))
	r.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "sub-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleRotateWebhookSecret)(w, r)
	require.Equal(t, http.
		StatusOK, w.Code)
	require.GreaterOrEqual(t, graceMins, 59.0)

	// Default is 60 minutes.
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
	require.Equal(t, http.
		StatusBadRequest,
		w.Code)
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
	require.Equal(t, http.
		StatusNotFound, w.
		Code)
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
	require.Equal(t, http.
		StatusNotFound, w.
		Code)
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
	require.Equal(t, http.
		StatusInternalServerError,

		w.Code)
}
