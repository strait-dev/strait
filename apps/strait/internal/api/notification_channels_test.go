package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleCreateNotificationChannel_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateNotificationChannelFunc: func(_ context.Context, ch *domain.NotificationChannel) error {
			ch.ID = "ch-1"
			if ch.ChannelType != "slack" {
				t.Fatalf("expected slack, got %s", ch.ChannelType)
			}
			if ch.ProjectID != "proj-1" {
				t.Fatalf("expected proj-1, got %s", ch.ProjectID)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"slack","name":"alerts","config":{"webhook_url":"https://hooks.slack.com/services/T00/B00/abc"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateNotificationChannel_SuccessDiscord(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateNotificationChannelFunc: func(_ context.Context, ch *domain.NotificationChannel) error {
			ch.ID = "ch-2"
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"discord","name":"alerts","config":{"webhook_url":"https://discord.com/api/webhooks/123/abc"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateNotificationChannel_SuccessWebhook(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateNotificationChannelFunc: func(_ context.Context, ch *domain.NotificationChannel) error {
			ch.ID = "ch-3"
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"webhook","name":"alerts","config":{"url":"https://example.com/hook","secret":"s3cret"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateNotificationChannel_RejectsPrivateIP(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"slack","name":"alerts","config":{"webhook_url":"http://127.0.0.1/hook"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateNotificationChannel_RejectsLocalhostURL(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"webhook","name":"alerts","config":{"url":"http://localhost:9090/hook"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateNotificationChannel_RejectsMetadataEndpoint(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"discord","name":"alerts","config":{"webhook_url":"http://169.254.169.254/metadata"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateNotificationChannel_RejectsMissingWebhookURL(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"slack","name":"alerts","config":{"other":"val"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateNotificationChannel_RejectsMissingURL(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"webhook","name":"alerts","config":{"secret":"s3cret"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateNotificationChannel_RejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"slack","name":"alerts","config":"not-json-object"}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateNotificationChannel_RejectsMissingProjectID(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"slack","name":"alerts","config":{"webhook_url":"https://hooks.slack.com/services/T00/B00/abc"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateNotificationChannel_RejectsUnsupportedChannelType(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"email","name":"alerts","config":{"address":"test@example.com"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateNotificationChannel_ValidatesNewConfig(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetNotificationChannelFunc: func(_ context.Context, id, projectID string) (*domain.NotificationChannel, error) {
			return &domain.NotificationChannel{
				ID:          id,
				ProjectID:   projectID,
				ChannelType: "slack",
				Name:        "alerts",
				Config:      json.RawMessage(`{"webhook_url":"https://hooks.slack.com/old"}`),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"config":{"webhook_url":"http://127.0.0.1/hook"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/notification-channels/ch-1", body, "proj-1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateNotificationChannel_AcceptsValidConfig(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetNotificationChannelFunc: func(_ context.Context, id, projectID string) (*domain.NotificationChannel, error) {
			return &domain.NotificationChannel{
				ID:          id,
				ProjectID:   projectID,
				ChannelType: "slack",
				Name:        "alerts",
				Config:      json.RawMessage(`{"webhook_url":"https://hooks.slack.com/old"}`),
			}, nil
		},
		UpdateNotificationChannelFunc: func(_ context.Context, _ *domain.NotificationChannel) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"config":{"webhook_url":"https://hooks.slack.com/services/T00/B00/new"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/notification-channels/ch-1", body, "proj-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateNotificationChannel_SkipsValidationWhenConfigUnchanged(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetNotificationChannelFunc: func(_ context.Context, id, projectID string) (*domain.NotificationChannel, error) {
			return &domain.NotificationChannel{
				ID:          id,
				ProjectID:   projectID,
				ChannelType: "slack",
				Name:        "alerts",
				Config:      json.RawMessage(`{"webhook_url":"https://hooks.slack.com/old"}`),
			}, nil
		},
		UpdateNotificationChannelFunc: func(_ context.Context, _ *domain.NotificationChannel) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"name":"new-name"}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/notification-channels/ch-1", body, "proj-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateNotificationChannel_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetNotificationChannelFunc: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			return nil, store.ErrNotificationChannelNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"name":"new-name"}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/notification-channels/ch-missing", body, "proj-1"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateNotificationChannel_ReturnsConfig(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateNotificationChannelFunc: func(_ context.Context, ch *domain.NotificationChannel) error {
			ch.ID = "ch-1"
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"slack","name":"alerts","config":{"webhook_url":"https://hooks.slack.com/services/T00/B00/abc"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := resp["config"]; !ok {
		t.Fatal("response missing config field")
	}
	if string(resp["config"]) == "null" || string(resp["config"]) == `""` {
		t.Fatalf("config should not be empty, got %s", string(resp["config"]))
	}
}

func TestHandleGetNotificationChannel_ReturnsConfig(t *testing.T) {
	t.Parallel()
	cfgJSON := json.RawMessage(`{"webhook_url":"https://hooks.slack.com/services/T00/B00/abc"}`)
	ms := &APIStoreMock{
		GetNotificationChannelFunc: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			return &domain.NotificationChannel{
				ID:          "ch-1",
				ProjectID:   "proj-1",
				ChannelType: "slack",
				Name:        "alerts",
				Config:      cfgJSON,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/notification-channels/ch-1", "", "proj-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := resp["config"]; !ok {
		t.Fatal("response missing config field")
	}
	if !strings.Contains(string(resp["config"]), "webhook_url") {
		t.Fatalf("config should contain webhook_url, got %s", string(resp["config"]))
	}
}

func TestHandleListNotificationChannels_ReturnsConfig(t *testing.T) {
	t.Parallel()
	cfgJSON := json.RawMessage(`{"webhook_url":"https://hooks.slack.com/services/T00/B00/abc"}`)
	ms := &APIStoreMock{
		ListNotificationChannelsFunc: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{
				{ID: "ch-1", ProjectID: "proj-1", ChannelType: "slack", Name: "alerts", Config: cfgJSON},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/notification-channels", "", "proj-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"config"`) {
		t.Fatalf("response should contain config field, got %s", w.Body.String())
	}
}

func TestGlobalAllowPrivateEndpoints_ResetBetweenServers(t *testing.T) {
	cfg1 := &config.Config{
		InternalSecret:        "test-secret-value",
		MaxBulkTriggerItems:   500,
		JWTSigningKey:         "01234567890123456789012345678901",
		AllowPrivateEndpoints: true,
	}
	srv1 := NewServer(ServerDeps{Config: cfg1, Store: &APIStoreMock{}, Queue: &mockQueue{}})
	t.Cleanup(srv1.Close)
	if !globalAllowPrivateEndpoints.Load() {
		t.Fatal("expected globalAllowPrivateEndpoints to be true after first server")
	}

	cfg2 := &config.Config{
		InternalSecret:        "test-secret-value",
		MaxBulkTriggerItems:   500,
		JWTSigningKey:         "01234567890123456789012345678901",
		AllowPrivateEndpoints: false,
	}
	srv2 := NewServer(ServerDeps{Config: cfg2, Store: &APIStoreMock{}, Queue: &mockQueue{}})
	t.Cleanup(srv2.Close)
	if globalAllowPrivateEndpoints.Load() {
		t.Fatal("expected globalAllowPrivateEndpoints to be false after second server")
	}
}

func TestGlobalAllowPrivateEndpoints_DefaultFalse(t *testing.T) {
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       "01234567890123456789012345678901",
	}
	srv := NewServer(ServerDeps{Config: cfg, Store: &APIStoreMock{}, Queue: &mockQueue{}})
	t.Cleanup(srv.Close)
	if globalAllowPrivateEndpoints.Load() {
		t.Fatal("expected globalAllowPrivateEndpoints to be false by default")
	}
}
