package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAPIKeyTestServer(t *testing.T, maxLifetimeDays int) *Server {
	t.Helper()

	ms := &APIStoreMock{
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{
				ProjectID:          "proj-1",
				MaxKeyLifetimeDays: maxLifetimeDays,
			}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-new"
			key.CreatedAt = time.Now()
			return nil
		},
	}

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   ms,
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func createKeyRequest(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestCreateAPIKey_MaxLifetime_AutoCaps(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 90)
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"]}`)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp CreateAPIKeyResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotNil(t, resp.ExpiresAt)

	maxExpected := time.Now().Add(91 * 24 * time.Hour)
	assert.False(
		t, resp.ExpiresAt.
			After(maxExpected))

	// generous check

}

func TestCreateAPIKey_MaxLifetime_AcceptsWithinLimit(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 90)
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"],"expires_in_days":30}`)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

}

func TestCreateAPIKey_MaxLifetime_RejectsExceeding(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 90)
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"],"expires_in_days":120}`)
	require.Equal(t, http.StatusBadRequest,

		w.Code)

	body := w.Body.String()
	assert.True(t,
		strings.Contains(body, "exceeds project maximum"))

}

func TestCreateAPIKey_NoMaxLifetime_RequiresExplicitExpiry(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 0) // no limit
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"]}`)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.True(
		t, strings.Contains(w.Body.
			String(), "expires_in_days is required",
		))

}

func TestCreateAPIKey_NoMaxLifetime_LongExpiry_Accepted(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 0) // no limit
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"],"expires_in_days":365}`)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

}

func TestCreateAPIKey_QuotaLookupFailureFailsClosed(t *testing.T) {
	t.Parallel()

	createCalled := false
	ms := &APIStoreMock{
		GetProjectQuotaFunc: func(context.Context, string) (*store.ProjectQuota, error) {
			return nil, errors.New("quota store unavailable")
		},
		CreateAPIKeyFunc: func(context.Context, *domain.APIKey) error {
			createCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"],"expires_in_days":365}`)
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
	require.False(t, createCalled)

}

func TestRotateAPIKey_MaxLifetime_AutoCapsLegacyNoExpiry(t *testing.T) {
	t.Parallel()

	var created *domain.APIKey
	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "legacy-no-expiry",
				Scopes:    []string{domain.ScopeJobsRead},
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxKeyLifetimeDays: 30}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			created = key
			key.ID = "key-rotated"
			return nil
		},
		MarkAPIKeyRotatedFunc: func(context.Context, string, string, time.Time) error { return nil },
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleRotateAPIKey(ctx, &RotateAPIKeyInput{KeyID: "key-old"})
	require.NoError(t, err)
	require.False(t, created == nil ||
		created.
			ExpiresAt ==
			nil)
	require.False(t, created.ExpiresAt.
		After(time.Now().
			Add(31*24*time.Hour)))

}

func TestRotateAPIKey_NoMaxLifetime_RejectsLegacyNoExpiry(t *testing.T) {
	t.Parallel()

	createCalled := false
	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "legacy-no-expiry",
				Scopes:    []string{domain.ScopeJobsRead},
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxKeyLifetimeDays: 0}, nil
		},
		CreateAPIKeyFunc: func(context.Context, *domain.APIKey) error {
			createCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleRotateAPIKey(ctx, &RotateAPIKeyInput{KeyID: "key-old"})
	require.True(
		t, isHumaStatusError(err, http.
			StatusBadRequest,
		))
	require.False(t, createCalled)

}

func TestRotateAPIKey_MaxLifetime_RejectsOverlongLegacyExpiry(t *testing.T) {
	t.Parallel()

	overlong := time.Now().Add(365 * 24 * time.Hour)
	createCalled := false
	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "overlong-expiry",
				Scopes:    []string{domain.ScopeJobsRead},
				ExpiresAt: &overlong,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxKeyLifetimeDays: 30}, nil
		},
		CreateAPIKeyFunc: func(context.Context, *domain.APIKey) error {
			createCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleRotateAPIKey(ctx, &RotateAPIKeyInput{KeyID: "key-old"})
	require.True(
		t, isHumaStatusError(err, http.
			StatusBadRequest,
		))
	require.False(t, createCalled)

}

func TestCreateAPIKey_Adversarial_ExpiresZero_Rejected(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 90)
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"],"expires_in_days":0}`)
	require.False(t, w.Code != http.
		StatusUnprocessableEntity &&
		w.Code != http.
			StatusBadRequest,
	)
	require.False(t, !strings.Contains(w.Body.
		String(),
		"ExpiresIn") && !strings.Contains(w.Body.String(),
		"expires_in_days",
	))

}

func TestCreateAPIKey_Adversarial_ExpiresNegative_Rejected(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 90)
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"],"expires_in_days":-7}`)
	require.False(t, w.Code != http.
		StatusUnprocessableEntity &&
		w.Code != http.
			StatusBadRequest,
	)
	require.False(t, !strings.Contains(w.Body.
		String(),
		"ExpiresIn") && !strings.Contains(w.Body.String(),
		"expires_in_days",
	))

}
