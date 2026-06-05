package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue 1: CLI device-code keys must have explicit scopes, not wildcard.

func TestHandleApproveDeviceCode_ExplicitScopes(t *testing.T) {
	t.Parallel()

	var createdKey *domain.APIKey

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-1",
				DeviceCode: "test-device-code",
				UserCode:   "ABCD1234",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-generated"
			key.CreatedAt = time.Now()
			createdKey = key
			return nil
		},
		ApproveDeviceCodeByUserCodeFunc: func(_ context.Context, _, _, _, _ string, _ []string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"user_code":"ABCD1234","project_id":"proj-1"}`
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/cli/device-codes/approve", body)
	r.Header.Set("X-Project-Id", "proj-1")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotNil(t, createdKey)
	require.NotEmpty(t, createdKey.
		Scopes)
	require.NotNil(t, createdKey.ExpiresAt)
	require.False(t, createdKey.ExpiresAt.
		After(time.Now().Add(time.
			Duration(defaultCLIKeyLifetimeDays+
				1)*24*time.Hour)),
	)

	// Verify all scopes are valid.
	for _, s := range createdKey.Scopes {
		assert.True(t,
			domain.ValidScopes[s])
	}
}

func TestHandleApproveDeviceCode_ExcludesAdminScopes(t *testing.T) {
	t.Parallel()

	var createdKey *domain.APIKey

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-1",
				DeviceCode: "test-device-code",
				UserCode:   "ABCD1234",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-generated"
			key.CreatedAt = time.Now()
			createdKey = key
			return nil
		},
		ApproveDeviceCodeByUserCodeFunc: func(_ context.Context, _, _, _, _ string, _ []string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"user_code":"ABCD1234","project_id":"proj-1"}`
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/cli/device-codes/approve", body)
	r.Header.Set("X-Project-Id", "proj-1")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotNil(t, createdKey)

	adminScopes := map[string]bool{
		domain.ScopeAPIKeysManage:  true,
		domain.ScopeRBACManage:     true,
		domain.ScopeProjectsManage: true,
		domain.ScopeAll:            true,
	}
	for _, s := range createdKey.Scopes {
		assert.False(
			t, adminScopes[s],
		)
	}
}

func TestHandleApproveDeviceCode_ScopesMatchCLIDefaults(t *testing.T) {
	t.Parallel()

	var createdKey *domain.APIKey

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-1",
				DeviceCode: "test-device-code",
				UserCode:   "ABCD1234",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-generated"
			key.CreatedAt = time.Now()
			createdKey = key
			return nil
		},
		ApproveDeviceCodeByUserCodeFunc: func(_ context.Context, _, _, _, _ string, _ []string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"user_code":"ABCD1234","project_id":"proj-1"}`
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/cli/device-codes/approve", body)
	r.Header.Set("X-Project-Id", "proj-1")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.NotNil(t, createdKey)
	require.Len(t,
		createdKey.Scopes,
		len(domain.
			CLIDefaultScopes))

	scopeSet := make(map[string]bool, len(createdKey.Scopes))
	for _, s := range createdKey.Scopes {
		scopeSet[s] = true
	}
	for _, s := range domain.CLIDefaultScopes {
		assert.True(t,
			scopeSet[s])
	}
}

// Issue 2: Cross-project event trigger access must return 404, not 403.

func TestHandleSendEvent_CrossProject_Returns404(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, key string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-1",
				EventKey:  key,
				ProjectID: "proj-owner",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
	}
	srv := newEventTriggersTestServer(t, ms, nil)

	body := `{"payload":{"data":"test"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/some-key/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	// Set a different project context to simulate cross-project access.
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-attacker")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeAll})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:attacker-key")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound,
		rr.Code,
	)

	// Verify the error message does not leak resource existence.
	var errResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err == nil {
		detail, _ := errResp["detail"].(string)
		assert.NotContains(
			t, strings.ToLower(detail), "does not belong")
	}
}

func TestHandleSendEvent_SameProject_Succeeds(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, key string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:                "evt-1",
				EventKey:          key,
				ProjectID:         "proj-1",
				SourceType:        "external",
				Status:            domain.EventTriggerStatusWaiting,
				WorkflowStepRunID: "",
				JobRunID:          "",
			}, nil
		},
		UpdateEventTriggerStatusFunc: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		SetEventTriggerSentByFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	srv := newEventTriggersTestServer(t, ms, nil)

	body := `{"payload":{"data":"test"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/some-key/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeAll})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-key")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK,
		rr.Code)
}

func TestHandleCancelEventTrigger_CrossProject_Returns404(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, key string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "evt-1",
				EventKey:  key,
				ProjectID: "proj-owner",
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
	}
	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodDelete, "/v1/events/some-key", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-attacker")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeAll})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:attacker-key")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound,
		rr.Code,
	)

	var errResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err == nil {
		detail, _ := errResp["detail"].(string)
		assert.NotContains(
			t, strings.ToLower(detail), "does not belong")
	}
}

func TestHandleCancelEventTrigger_SameProject_Succeeds(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, key string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:         "evt-1",
				EventKey:   key,
				ProjectID:  "proj-1",
				SourceType: "external",
				Status:     domain.EventTriggerStatusWaiting,
			}, nil
		},
		UpdateEventTriggerStatusFunc: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
	}
	srv := newEventTriggersTestServer(t, ms, nil)

	req := httptest.NewRequest(http.MethodDelete, "/v1/events/some-key", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeAll})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-key")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK,
		rr.Code)
}
