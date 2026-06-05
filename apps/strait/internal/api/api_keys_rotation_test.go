package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestHandleRotateAPIKey(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(24 * time.Hour)
	ms := &APIStoreMock{}
	ms.GetAPIKeyByIDFunc = func(_ context.Context, id string) (*domain.APIKey, error) {
		return &domain.APIKey{ID: id, ProjectID: "proj-1", OrgID: "org-1", Name: "prod key", Scopes: []string{"jobs:read"}, ExpiresAt: &expiresAt}, nil
	}
	ms.CreateAPIKeyFunc = func(_ context.Context, key *domain.APIKey) error {
		require.Equal(t, "proj-1",
			key.ProjectID,
		)
		require.Equal(t, "org-1",
			key.OrgID,
		)

		key.ID = "key-2"
		return nil
	}
	ms.MarkAPIKeyRotatedFunc = func(_ context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error {
		require.False(t, oldKeyID !=
			"key-1" ||
			newKeyID == "")

		return nil
	}

	srv := newTestServer(t, ms, nil, nil)
	req := authedRequest(http.MethodPost, "/v1/api-keys/key-1/rotate", `{"grace_period_minutes":30}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.Contains(
		t, w.
			Body.String(), "grace_expires_at")
}

func TestHandleRotateAPIKey_PublishesWorkerExpiryDeadline(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(24 * time.Hour)
	ms := &APIStoreMock{}
	ms.GetAPIKeyByIDFunc = func(_ context.Context, id string) (*domain.APIKey, error) {
		return &domain.APIKey{ID: id, ProjectID: "proj-1", OrgID: "org-1", Name: "worker key", Scopes: []string{domain.ScopeWorkersConnect}, ExpiresAt: &expiresAt}, nil
	}
	ms.CreateAPIKeyFunc = func(_ context.Context, key *domain.APIKey) error {
		key.ID = "key-new"
		return nil
	}
	ms.MarkAPIKeyRotatedFunc = func(_ context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error {
		require.False(t, oldKeyID !=
			"key-old" ||
			newKeyID != "key-new",
		)
		require.Positive(t, time.Until(graceExpiresAt))

		return nil
	}

	var publishedChannel string
	var publishedDeadline time.Time
	pub := &mockPublisher{publishFn: func(_ context.Context, channel string, data []byte) error {
		publishedChannel = channel
		var err error
		publishedDeadline, err = time.Parse(time.RFC3339Nano, string(data))
		require.NoError(t, err)

		return nil
	}}
	srv := newTestServer(t, ms, nil, pub)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleRotateAPIKey(ctx, &RotateAPIKeyInput{
		KeyID: "key-old",
		Body:  RotateAPIKeyRequest{GracePeriodMinutes: 30},
	})
	require.NoError(t, err)
	require.Equal(t, "apikey:expires:key-old",

		publishedChannel,
	)
	require.Positive(t, time.Until(publishedDeadline))
}

func TestHandleRotateAPIKey_GRPCEnabledRequiresPubSubBeforeRotating(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(24 * time.Hour)
	created := false
	rotated := false
	ms := &APIStoreMock{}
	ms.GetAPIKeyByIDFunc = func(_ context.Context, id string) (*domain.APIKey, error) {
		return &domain.APIKey{ID: id, ProjectID: "proj-1", OrgID: "org-1", Name: "worker key", Scopes: []string{domain.ScopeWorkersConnect}, ExpiresAt: &expiresAt}, nil
	}
	ms.CreateAPIKeyFunc = func(_ context.Context, _ *domain.APIKey) error {
		created = true
		return nil
	}
	ms.MarkAPIKeyRotatedFunc = func(context.Context, string, string, time.Time) error {
		rotated = true
		return nil
	}

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
			GRPCEnabled:         true,
		},
		Store:   ms,
		Queue:   &mockQueue{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/key-old/rotate", `{"grace_period_minutes":30}`))
	require.Equal(t, http.StatusServiceUnavailable,

		w.Code)
	require.False(t, created)
	require.False(t, rotated)
}

func TestHandleRotateAPIKey_RevokeReplacementWhenMarkFails(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(24 * time.Hour)
	var revokedReplacement atomic.Bool
	ms := &APIStoreMock{}
	ms.GetAPIKeyByIDFunc = func(_ context.Context, id string) (*domain.APIKey, error) {
		return &domain.APIKey{ID: id, ProjectID: "proj-1", OrgID: "org-1", Name: "prod key", Scopes: []string{"jobs:read"}, ExpiresAt: &expiresAt}, nil
	}
	ms.CreateAPIKeyFunc = func(_ context.Context, key *domain.APIKey) error {
		key.ID = "key-replacement"
		return nil
	}
	ms.MarkAPIKeyRotatedFunc = func(_ context.Context, oldKeyID, newKeyID string, _ time.Time) error {
		require.False(t, oldKeyID !=
			"key-1" ||
			newKeyID != "key-replacement",
		)

		return errors.New("lost rotation race")
	}
	ms.RevokeAPIKeyFunc = func(_ context.Context, id string) error {
		require.Equal(t, "key-replacement",

			id)

		revokedReplacement.Store(true)
		return nil
	}

	srv := newTestServer(t, ms, nil, nil)
	req := authedRequest(http.MethodPost, "/v1/api-keys/key-1/rotate", `{"grace_period_minutes":30}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
	require.True(
		t, revokedReplacement.
			Load())
}

func TestAPIKeyAuth_RejectsExpiredRotationGrace(t *testing.T) {
	t.Parallel()

	past := time.Now().Add(-time.Minute)
	ms := &APIStoreMock{}
	ms.GetAPIKeyByHashFunc = func(_ context.Context, _ string) (*domain.APIKey, error) {
		return &domain.APIKey{ID: "k1", ProjectID: "proj-1", Scopes: []string{"stats:read"}, GraceExpiresAt: &past}, nil
	}
	srv := newTestServer(t, ms, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer strait_test")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)
}
