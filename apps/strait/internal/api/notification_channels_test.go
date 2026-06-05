package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestHandleCreateNotificationChannel_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateNotificationChannelFunc: func(_ context.Context, ch *domain.NotificationChannel) error {
			ch.ID = "ch-1"
			require.Equal(t, "slack", ch.ChannelType)
			require.Equal(t, "proj-1", ch.
				ProjectID)

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"slack","name":"alerts","config":{"webhook_url":"https://hooks.slack.com/services/T00/B00/abc"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
}

func TestHandleCreateNotificationChannel_RejectsPrivateIP(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"slack","name":"alerts","config":{"webhook_url":"http://127.0.0.1/hook"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleCreateNotificationChannel_RejectsLocalhostURL(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"webhook","name":"alerts","config":{"url":"http://localhost:9090/hook"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleCreateNotificationChannel_RejectsMetadataEndpoint(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"discord","name":"alerts","config":{"webhook_url":"http://169.254.169.254/metadata"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleCreateNotificationChannel_RejectsMissingWebhookURL(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"slack","name":"alerts","config":{"other":"val"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleCreateNotificationChannel_RejectsMissingURL(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"webhook","name":"alerts","config":{"secret":"s3cret"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleCreateNotificationChannel_RejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"slack","name":"alerts","config":"not-json-object"}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleCreateNotificationChannel_RejectsMissingProjectID(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"slack","name":"alerts","config":{"webhook_url":"https://hooks.slack.com/services/T00/B00/abc"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, ""))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

func TestHandleCreateNotificationChannel_RejectsUnsupportedChannelType(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"channel_type":"email","name":"alerts","config":{"address":"test@example.com"}}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", body, "proj-1"))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
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
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)
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
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	if _, ok := resp["config"]; !ok {
		require.Fail(t,

			"response missing config field")
	}
	require.False(t, string(resp["config"]) ==
		"null" || string(resp["config"]) == `""`)
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	if _, ok := resp["config"]; !ok {
		require.Fail(t,

			"response missing config field")
	}
	require.Contains(
		t, string(resp["config"]), "webhook_url")
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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Contains(
		t, w.Body.String(), `"config"`)
}

func TestGlobalAllowPrivateEndpoints_ResetBetweenServers(t *testing.T) {
	cfg1 := &config.Config{
		InternalSecret:        "test-secret-value",
		MaxBulkTriggerItems:   500,
		JWTSigningKey:         testJWTSigningKey,
		AllowPrivateEndpoints: true,
	}
	srv1 := NewServer(ServerDeps{Config: cfg1, Store: &APIStoreMock{}, Queue: &mockQueue{}})
	t.Cleanup(srv1.Close)
	require.True(
		t, globalAllowPrivateEndpoints.
			Load())

	cfg2 := &config.Config{
		InternalSecret:        "test-secret-value",
		MaxBulkTriggerItems:   500,
		JWTSigningKey:         testJWTSigningKey,
		AllowPrivateEndpoints: false,
	}
	srv2 := NewServer(ServerDeps{Config: cfg2, Store: &APIStoreMock{}, Queue: &mockQueue{}})
	t.Cleanup(srv2.Close)
	require.False(t, globalAllowPrivateEndpoints.
		Load())
}

// Regression: notification channel config carries webhook URLs that act as
// bearer secrets (Slack/Discord) and may include explicit secret fields.
// The API must never echo them back to the caller verbatim.

func TestHandleNotificationChannel_ConfigRedactedOnGet(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetNotificationChannelFunc: func(_ context.Context, id, projectID string) (*domain.NotificationChannel, error) {
			return &domain.NotificationChannel{
				ID: id, ProjectID: projectID, Name: "alerts",
				ChannelType: domain.ChannelTypeSlack,
				Config:      json.RawMessage(`{"webhook_url":"https://hooks.slack.com/services/T00/B00/SUPER-SECRET-TOKEN"}`),
				Enabled:     true,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/notification-channels/ch-1", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotContains(t, w.Body.String(), "SUPER-SECRET-TOKEN")
	require.NotContains(t, w.Body.String(), "hooks.slack.com")
}

func TestHandleNotificationChannel_ConfigRedactedOnList(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListNotificationChannelsFunc: func(_ context.Context, projectID string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{
				ID: "ch-1", ProjectID: projectID, Name: "alerts",
				ChannelType: domain.ChannelTypeWebhook,
				Config:      json.RawMessage(`{"url":"https://example.com/hook","secret":"LIST-LEAK-SECRET"}`),
				Enabled:     true,
			}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/notification-channels", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotContains(t, w.Body.String(), "LIST-LEAK-SECRET")
}

func TestGlobalAllowPrivateEndpoints_DefaultFalse(t *testing.T) {
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{Config: cfg, Store: &APIStoreMock{}, Queue: &mockQueue{}})
	t.Cleanup(srv.Close)
	require.False(t, globalAllowPrivateEndpoints.
		Load())
}
