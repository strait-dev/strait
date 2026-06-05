package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestHandleCreateAPIKey_Success(t *testing.T) {
	t.Parallel()
	var created atomic.Bool
	var captured *domain.APIKey

	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			created.Store(true)
			captured = key
			key.ID = "key-123"
			key.CreatedAt = time.Now().UTC()
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{"project_id":"proj-1","name":"CLI key","scopes":["jobs:read"],"expires_in_days":30}`
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.True(
		t, created.Load(),
	)
	require.NotNil(t, captured)
	require.Equal(t, "proj-1", captured.
		ProjectID,
	)
	require.Equal(t, "CLI key", captured.
		Name,
	)
	require.False(t, len(captured.
		Scopes) !=
		1 || captured.
		Scopes[0] != "jobs:read",
	)
	require.NotEmpty(t, captured.
		KeyHash,
	)
	require.NotNil(t, captured.ExpiresAt)
	require.True(
		t, strings.HasPrefix(captured.
			KeyPrefix,
			"strait_"))

	var resp CreateAPIKeyResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.NotEmpty(t, resp.ID)
	require.True(
		t, strings.HasPrefix(resp.Key,
			"strait_",
		))
	require.NotEmpty(t, resp.KeyPrefix)
	require.Equal(t, "proj-1", resp.
		ProjectID,
	)
	require.Equal(t, "CLI key", resp.
		Name)
	require.False(t, len(resp.Scopes) != 1 ||
		resp.Scopes[0] != "jobs:read")
	require.False(t, resp.CreatedAt.
		IsZero(),
	)
}

func TestHandleCreateAPIKey_WithExpiry(t *testing.T) {
	t.Parallel()
	var captured *domain.APIKey
	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			captured = key
			key.ID = "key-123"
			key.CreatedAt = time.Now().UTC()
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"expiring","scopes":["jobs:read"],"expires_in_days":30}`
	now := time.Now()

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.False(t, captured == nil ||
		captured.
			ExpiresAt ==
			nil)

	earliest := now.Add(29 * 24 * time.Hour)
	latest := now.Add(31 * 24 * time.Hour)
	require.False(t, captured.ExpiresAt.
		Before(earliest) ||
		captured.ExpiresAt.
			After(latest))
}

func TestHandleCreateAPIKey_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", ""))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code,
	)
}

func TestHandleCreateAPIKey_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", `{"name":"missing project"}`))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code,
	)
}

func TestHandleCreateAPIKey_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, _ *domain.APIKey) error {
			return errors.New("boom")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"failing key","scopes":["jobs:read"],"expires_in_days":30}`

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", body))
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
}

func TestHandleListAPIKeys_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListAPIKeysByProjectFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.APIKey, error) {
			require.Equal(t, "proj-1", projectID)

			now := time.Now().UTC()
			return []domain.APIKey{
				{ID: "key-1", ProjectID: projectID, Name: "first", KeyPrefix: "strait_aaaaaaaa", CreatedAt: now},
				{ID: "key-2", ProjectID: projectID, Name: "second", KeyPrefix: "strait_bbbbbbbb", CreatedAt: now},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/api-keys/", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []domain.APIKey
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 2)
}

func TestHandleListAPIKeys_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/api-keys/", ""))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleListAPIKeys_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListAPIKeysByProjectFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.APIKey, error) {
			return nil, errors.New("boom")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/api-keys/", "", "proj-1"))
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
}

func TestHandleRevokeAPIKey_Success(t *testing.T) {
	t.Parallel()
	var revokedID string
	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: id, ProjectID: "proj-1", Name: "test"}, nil
		},
		RevokeAPIKeyFunc: func(_ context.Context, id string) error {
			revokedID = id
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/api-keys/key-123", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, "key-123", revokedID)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.Equal(t, "revoked", resp["status"])
}

func TestHandleRevokeAPIKey_GRPCEnabledRequiresPubSubBeforeRevoking(t *testing.T) {
	t.Parallel()
	revoked := false
	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: id, ProjectID: "proj-1", Name: "test"}, nil
		},
		RevokeAPIKeyFunc: func(_ context.Context, _ string) error {
			revoked = true
			return nil
		},
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
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/api-keys/key-123", ""))
	require.Equal(t, http.StatusServiceUnavailable,

		w.Code,
	)
	require.False(t, revoked)
}

func TestHandleRevokeAPIKey_GRPCEnabledPublishFailureReturnsUnavailable(t *testing.T) {
	t.Parallel()
	revoked := false
	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: id, ProjectID: "proj-1", Name: "test"}, nil
		},
		RevokeAPIKeyFunc: func(_ context.Context, _ string) error {
			revoked = true
			return nil
		},
	}

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
			GRPCEnabled:         true,
		},
		Store: ms,
		Queue: &mockQueue{},
		PubSub: &mockPublisher{publishFn: func(context.Context, string, []byte) error {
			return errors.New("redis down")
		}},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/api-keys/key-123", ""))
	require.Equal(t, http.StatusServiceUnavailable,

		w.Code,
	)
	require.True(
		t, revoked)
}

func TestHandleRevokeAPIKey_AlreadyRevokedRetriesBroadcast(t *testing.T) {
	t.Parallel()

	revokedAt := time.Now().Add(-time.Minute)
	revokeCalled := false
	published := false
	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: id, ProjectID: "proj-1", Name: "test", RevokedAt: &revokedAt}, nil
		},
		RevokeAPIKeyFunc: func(_ context.Context, _ string) error {
			revokeCalled = true
			return nil
		},
	}

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
			GRPCEnabled:         true,
		},
		Store: ms,
		Queue: &mockQueue{},
		PubSub: &mockPublisher{publishFn: func(_ context.Context, channel string, data []byte) error {
			published = true
			require.Equal(t, "apikey:revoked:key-123",

				channel)
			require.Equal(t, "key-123", string(data))

			return nil
		}},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/api-keys/key-123", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, revokeCalled)
	require.True(
		t, published)
}

func TestHandleRevokeAPIKey_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		RevokeAPIKeyFunc: func(_ context.Context, _ string) error {
			return errors.New("not found")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/api-keys/key-123", ""))
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
}

func TestGenerateAPIKey_Format(t *testing.T) {
	t.Parallel()
	key, err := generateAPIKey()
	require.NoError(t, err)
	require.True(
		t, strings.HasPrefix(key, "strait_"))
	require.Len(t,
		key, 71)
}

func TestGenerateAPIKey_Uniqueness(t *testing.T) {
	t.Parallel()
	keyA, err := generateAPIKey()
	require.NoError(t, err)

	keyB, err := generateAPIKey()
	require.NoError(t, err)
	require.NotEqual(t, keyB, keyA)
}

func TestHashAPIKey_Deterministic(t *testing.T) {
	t.Parallel()
	key := "strait_" + strings.Repeat("ab", 32)
	h1 := hashAPIKey(key)
	h2 := hashAPIKey(key)
	require.Equal(t, h2, h1)
}

func TestHashAPIKey_DifferentKeys(t *testing.T) {
	t.Parallel()
	h1 := hashAPIKey("strait_" + strings.Repeat("ab", 32))
	h2 := hashAPIKey("strait_" + strings.Repeat("cd", 32))
	require.NotEqual(t, h2, h1)
}

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("ab", 32)
	wantHash := hashAPIKey(rawKey)
	touched := make(chan string, 1)

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, keyHash string) (*domain.APIKey, error) {
			require.Equal(t, wantHash, keyHash)

			return &domain.APIKey{ID: "key-123", ProjectID: "proj-1"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, id string) error {
			touched <- id
			return nil
		},
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 1, Executing: 2, Delayed: 3}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)

	select {
	case id := <-touched:
		if id != "key-123" {
			require.Failf(t, "test failure",

				"expected touched id key-123, got %q", id)
		}
	case <-time.After(250 * time.Millisecond):
		require.Fail(t, "expected TouchAPIKeyLastUsed to be called")
	}
}

func TestAPIKeyAuth_TouchUsesBoundedDetachedContext(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("ef", 32)
	wantHash := hashAPIKey(rawKey)

	type touchCall struct {
		hasDeadline bool
		deadline    time.Time
		err         error
	}

	touchCh := make(chan touchCall, 1)

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, keyHash string) (*domain.APIKey, error) {
			require.Equal(t, wantHash, keyHash)

			return &domain.APIKey{ID: "key-touch", ProjectID: "proj-ctx"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(ctx context.Context, id string) error {
			require.Equal(t, "key-touch",
				id)

			dl, ok := ctx.Deadline()
			touchCh <- touchCall{hasDeadline: ok, deadline: dl, err: ctx.Err()}
			return nil
		},
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 1, Executing: 0, Delayed: 0}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)

	select {
	case call := <-touchCh:
		if call.err != nil {
			require.Failf(t, "test failure",

				"expected detached touch context to be usable, got err: %v", call.err)
		}
		if !call.hasDeadline {
			require.Fail(t,

				"expected touch context deadline to be set")
		}
		remaining := time.Until(call.deadline)
		if remaining <= 0 || remaining > 3*time.Second {
			require.Failf(t, "test failure",

				"expected touch context deadline near 2s, remaining=%s", remaining)
		}
	case <-time.After(300 * time.Millisecond):
		require.Fail(t, "expected TouchAPIKeyLastUsed to be called")
	}
}

func TestAPIKeyAuth_ExpiredKey(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("ab", 32)
	expired := time.Now().Add(-time.Hour)

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-123", ProjectID: "proj-1", ExpiresAt: &expired}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.False(t, resp.Error ==
		nil || resp.
		Error.Message !=
		"api key has expired",
	)
}

func TestAPIKeyAuth_RevokedKey(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("ab", 32)
	revoked := time.Now().Add(-time.Minute)

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-123", ProjectID: "proj-1", RevokedAt: &revoked}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp,
	))
	require.False(t, resp.Error ==
		nil || resp.
		Error.Message !=
		"api key has been revoked",
	)
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return nil, errors.New("missing")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer strait_badkey")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)
}

func TestAPIKeyAuth_MissingHeader(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)
}

func TestDualAuth_FallbackToInternalSecret(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 7, Executing: 0, Delayed: 0}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/stats", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestDualAuth_APIKeyTakesPrecedence(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("ab", 32)
	wantHash := hashAPIKey(rawKey)
	var lookedUp atomic.Bool

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, keyHash string) (*domain.APIKey, error) {
			lookedUp.Store(true)
			require.Equal(t, wantHash, keyHash)

			return &domain.APIKey{ID: "key-1", ProjectID: "proj-1"}, nil
		},
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 1, Executing: 1, Delayed: 1}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/stats", "")
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, lookedUp.Load())
}

func TestProjectIDFromContext(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	require.Equal(t, "proj-1", projectIDFromContext(ctx))
	require.Empty(t, projectIDFromContext(context.Background()))
}

func TestCreateAPIKey_OrgScoped_Success(t *testing.T) {
	t.Parallel()
	var captured *domain.APIKey
	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			captured = key
			key.ID = "key-org-1"
			key.CreatedAt = time.Now().UTC()
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{"project_id":"proj-1","org_id":"org-1","name":"Org key","scopes":["jobs:read"],"expires_in_days":30}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, captured)
	require.Equal(t, "org-1", captured.
		OrgID,
	)
	require.Equal(t, "proj-1", captured.
		ProjectID,
	)
}

func TestCreateAPIKey_OrgScoped_RequiresInternalSecret(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	// Using a non-internal-secret auth (no X-Internal-Secret, no valid API key)
	req := httptest.NewRequest(http.MethodPost, "/v1/api-keys/", strings.NewReader(`{"project_id":"proj-1","org_id":"org-1","name":"Org key"}`))
	req.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)
}

func TestOrgScopedKey_CanAccessAllProjectsInOrg(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("cc", 32)
	wantHash := hashAPIKey(rawKey)

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, keyHash string) (*domain.APIKey, error) {
			require.Equal(t, wantHash, keyHash)

			return &domain.APIKey{ID: "key-org-1", ProjectID: "proj-1", OrgID: "org-1"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
		ListRunsByOrgFunc: func(_ context.Context, orgID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			require.Equal(t, "org-1", orgID)

			return []domain.JobRun{
				{ID: "run-1", ProjectID: "proj-1", CreatedAt: time.Now().UTC()},
				{ID: "run-2", ProjectID: "proj-2", CreatedAt: time.Now().UTC()},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/org-1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	require.Len(t,
		runs, 2)
}

func TestOrgScopedKey_CannotAccessOtherOrg(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("dd", 32)

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-org-1", ProjectID: "proj-1", OrgID: "org-1"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/org-other/runs", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden,
		w.
			Code)
}

func TestOrgScopedKey_ListReturnsOrgKeys(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	rawKey := "strait_" + strings.Repeat("ee", 32)
	wantHash := hashAPIKey(rawKey)
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, keyHash string) (*domain.APIKey, error) {
			require.Equal(t, wantHash, keyHash)

			return &domain.APIKey{
				ID:        "key-org-1",
				ProjectID: "proj-anchor",
				OrgID:     "org-1",
				Scopes:    []string{domain.ScopeAPIKeysManage},
			}, nil
		},
		TouchAPIKeyLastUsedFunc: func(context.Context, string) error { return nil },
		ListAPIKeysByOrgFunc: func(_ context.Context, orgID string, _ int, _ *time.Time) ([]domain.APIKey, error) {
			require.Equal(t, "org-1", orgID)

			return []domain.APIKey{
				{ID: "key-1", ProjectID: "proj-1", OrgID: "org-1", Name: "first", KeyPrefix: "strait_aaa", CreatedAt: now},
				{ID: "key-2", ProjectID: "proj-2", OrgID: "org-1", Name: "second", KeyPrefix: "strait_bbb", CreatedAt: now},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/api-keys/?org_id=org-1", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []domain.APIKey
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	require.Len(t,
		resp, 2)
	require.Equal(t, "org-1", resp[0].OrgID)
}

func TestOrgScopedKey_ListRejectsProjectScopedCaller(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("fa", 32)
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(context.Context, string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        "key-project-1",
				ProjectID: "proj-1",
				Scopes:    []string{domain.ScopeAPIKeysManage},
			}, nil
		},
		TouchAPIKeyLastUsedFunc: func(context.Context, string) error { return nil },
		ListAPIKeysByOrgFunc: func(context.Context, string, int, *time.Time) ([]domain.APIKey, error) {
			require.Fail(t,

				"project-scoped caller must not reach org-wide key list")
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/api-keys/?org_id=org-1", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden,
		w.
			Code)
}

type apiKeyListContextStore struct {
	APIStore
	setProjects []string
	clearCalls  int
}

func (s *apiKeyListContextStore) SetProjectContext(_ context.Context, projectID string) error {
	s.setProjects = append(s.setProjects, projectID)
	return nil
}

func (s *apiKeyListContextStore) ClearProjectContext(context.Context) error {
	s.clearCalls++
	return nil
}

func TestOrgScopedKey_ListClearsAndRestoresProjectRLSContext(t *testing.T) {
	rawKey := "strait_" + strings.Repeat("ef", 32)
	base := &APIStoreMock{
		GetAPIKeyByHashFunc: func(context.Context, string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        "key-org-1",
				ProjectID: "proj-anchor",
				OrgID:     "org-1",
				Scopes:    []string{domain.ScopeAPIKeysManage},
			}, nil
		},
		TouchAPIKeyLastUsedFunc: func(context.Context, string) error { return nil },
		ListAPIKeysByOrgFunc: func(context.Context, string, int, *time.Time) ([]domain.APIKey, error) {
			return []domain.APIKey{{ID: "key-1", ProjectID: "proj-other", OrgID: "org-1", CreatedAt: time.Now().UTC()}}, nil
		},
	}
	wrappedStore := &apiKeyListContextStore{APIStore: base}
	srv := newTestServer(t, wrappedStore, &mockQueue{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/api-keys/?org_id=org-1", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, 2, wrappedStore.
		clearCalls,
	)
	require.False(t, len(wrappedStore.
		setProjects,
	) != 2 ||
		wrappedStore.setProjects[0] !=
			"proj-anchor" ||
		wrappedStore.
			setProjects[1] != "proj-anchor",
	)
}

func TestOrgScopedKey_RevokeWorks(t *testing.T) {
	t.Parallel()
	var revokedID string
	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: id, ProjectID: "proj-1", Name: "test"}, nil
		},
		RevokeAPIKeyFunc: func(_ context.Context, id string) error {
			revokedID = id
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/api-keys/key-org-1", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, "key-org-1",
		revokedID)
}
