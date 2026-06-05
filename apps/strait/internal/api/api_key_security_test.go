package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// TestAPIKey_HashNeverReversible verifies that the SHA-256 hash of an API key
// cannot be reversed to recover the original key.
func TestAPIKey_HashNeverReversible(t *testing.T) {
	t.Parallel()

	rawKey, err := generateAPIKey()
	require.NoError(t, err)

	hash := hashAPIKey(rawKey)
	require.Len(t,
		hash, 64)
	require.False(t, strings.Contains(hash, rawKey))
	require.NotEqual(t, rawKey,
		hash)

	// Deterministic.
	hash2 := hashAPIKey(rawKey)
	require.Equal(t, hash2, hash)

	// Different keys produce different hashes.
	rawKey2, err := generateAPIKey()
	require.NoError(t, err)

	hash3 := hashAPIKey(rawKey2)
	require.NotEqual(t, hash3, hash)

}

// TestAPIKey_RotationGracePeriod verifies that an old key still works during
// the grace period after rotation.
func TestAPIKey_RotationGracePeriod(t *testing.T) {
	t.Parallel()

	graceExpiry := time.Now().Add(1 * time.Hour)

	oldKey := &domain.APIKey{
		ID:             "key-old",
		ProjectID:      "proj-1",
		Name:           "old-key",
		KeyHash:        hashAPIKey("strait_old_key_value_for_testing_rotation"),
		KeyPrefix:      "strait_old_",
		Scopes:         []string{},
		RevokedAt:      nil,
		GraceExpiresAt: &graceExpiry,
	}

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, keyHash string) (*domain.APIKey, error) {
			if keyHash == oldKey.KeyHash {
				return oldKey, nil
			}
			return nil, errors.New("not found")
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error {
			return nil
		},
		ListJobSecretsFunc: func(_ context.Context, _, _, _ string, _ int, _ *time.Time) ([]domain.JobSecret, error) {
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/secrets?environment=production", nil)
	req.Header.Set("Authorization", "Bearer strait_old_key_value_for_testing_rotation")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project-Id", "proj-1")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.NotEqual(t, http.StatusUnauthorized,

		w.Code)

}

// TestAPIKey_ConcurrentRotation verifies that two simultaneous rotation requests
// do not cause data races or panics.
func TestAPIKey_ConcurrentRotation(t *testing.T) {
	t.Parallel()

	var createCount atomic.Int32
	var markCount atomic.Int32

	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID: id, ProjectID: "proj-1", Name: "rotation-key",
				KeyHash: "hash", KeyPrefix: "strait_rot_", Scopes: []string{},
			}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			createCount.Add(1)
			key.ID = fmt.Sprintf("key-new-%d", createCount.Load())
			key.CreatedAt = time.Now()
			return nil
		},
		MarkAPIKeyRotatedFunc: func(_ context.Context, _, _ string, _ time.Time) error {
			markCount.Add(1)
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	var wg conc.WaitGroup
	for range 2 {
		wg.Go(func() {
			body := `{"grace_period_minutes":60}`
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/key-1/rotate", body))
			assert.NotEqual(t, http.StatusInternalServerError,

				w.Code)

		})
	}
	wg.Wait()
}

// TestAPIKey_RevocationImmediate verifies that a revoked key is immediately rejected.
func TestAPIKey_RevocationImmediate(t *testing.T) {
	t.Parallel()

	revokedAt := time.Now()
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID: "key-revoked", ProjectID: "proj-1", Name: "revoked",
				KeyHash: "hash", KeyPrefix: "strait_rev_", Scopes: []string{},
				RevokedAt: &revokedAt,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer strait_revoked_key_for_testing_purposes")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project-Id", "proj-1")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized,
		w.Code,
	)
	require.True(
		t, strings.Contains(w.Body.String(), "revoked"))

}

// TestAPIKey_ExpiredKeyRejected verifies that an expired key returns 401.
func TestAPIKey_ExpiredKeyRejected(t *testing.T) {
	t.Parallel()

	expiredAt := time.Now().Add(-1 * time.Hour)
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID: "key-expired", ProjectID: "proj-1", Name: "expired",
				KeyHash: "hash", KeyPrefix: "strait_exp_", Scopes: []string{},
				ExpiresAt: &expiredAt,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer strait_expired_key_for_testing_purpose")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project-Id", "proj-1")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized,
		w.Code,
	)
	require.True(
		t, strings.Contains(w.Body.String(), "expired"))

}

// TestAPIKey_ScopeEscalation verifies that a key with limited scopes cannot
// access endpoints requiring broader scopes.
func TestAPIKey_ScopeEscalation(t *testing.T) {
	t.Parallel()

	limitedScopes := []string{"jobs:read"}
	require.True(
		t, domain.HasScope(limitedScopes,
			"jobs:read"))
	require.False(t, domain.HasScope(limitedScopes,
		"jobs:write"),
	)
	require.False(t, domain.HasScope(limitedScopes,
		"secrets:read",
	))
	require.False(t, domain.HasScope(limitedScopes,
		"rbac:manage",
	))

	wildcardScopes := []string{"*"}
	require.True(
		t, domain.HasScope(wildcardScopes,
			"jobs:write"),
	)

}

// TestAPIKey_PrefixFormat verifies that generated API keys always start with
// the "strait_" prefix.
func TestAPIKey_PrefixFormat(t *testing.T) {
	t.Parallel()

	for range 100 {
		key, err := generateAPIKey()
		require.NoError(t, err)
		require.True(
			t, strings.HasPrefix(key, "strait_"))
		require.Len(t,
			key, 71)

		// Key should be "strait_" + 64 hex characters = 71 characters total.

	}
}

// TestAPIKey_BruteForceResistance verifies that 10000 random keys are all
// rejected as invalid (none match any stored hash).
func TestAPIKey_BruteForceResistance(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return nil, errors.New("not found")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	for range 10000 {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			require.Failf(t, "test failure",

				"failed to generate random bytes: %v", err)
		}
		fakeKey := "strait_" + hex.EncodeToString(b)

		req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		req.Header.Set("Authorization", "Bearer "+fakeKey)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized,
			w.Code,
		)

	}
}

// TestAPIKey_NullByteInKey verifies that a null byte in the auth header is
// handled without panic.
func TestAPIKey_NullByteInKey(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return nil, errors.New("not found")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer strait_"+strings.Repeat("a", 32)+"\x00"+strings.Repeat("b", 32))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized,
		w.Code,
	)

}

// TestAPIKey_EmptyBearerToken verifies that an empty bearer token is rejected.
func TestAPIKey_EmptyBearerToken(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	cases := []struct {
		name   string
		header string
	}{
		{name: "empty_bearer", header: "Bearer "},
		{name: "bearer_only", header: "Bearer"},
		{name: "no_prefix", header: "strait_abc123"},
		{name: "empty_string", header: ""},
		{name: "basic_auth", header: "Basic dXNlcjpwYXNz"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized,
				w.Code,
			)

		})
	}
}

// TestAPIKey_InvalidRotationInterval verifies that negative or zero rotation
// intervals are handled gracefully.
func TestAPIKey_InvalidRotationInterval(t *testing.T) {
	t.Parallel()

	intervals := []int{-1, 0, -100}
	for _, interval := range intervals {
		t.Run(fmt.Sprintf("interval_%d", interval), func(t *testing.T) {
			t.Parallel()
			var captured *domain.APIKey
			ms := &APIStoreMock{
				CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
					captured = key
					key.ID = "key-rot-invalid"
					key.CreatedAt = time.Now()
					return nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)

			body := fmt.Sprintf(`{"project_id":"proj-1","name":"interval-test","scopes":["jobs:read"],"expires_in_days":30,"rotation_interval_days":%d}`, interval)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", body))
			require.NotEqual(t, http.StatusInternalServerError,

				w.Code)

			if w.Code == http.StatusCreated && captured != nil {
				require.False(t, captured.NextRotationAt !=
					nil &&
					interval <=
						0)

			}
		})
	}
}

func TestHandleCreateAPIKey_RejectsEmptyScopes(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, _ *domain.APIKey) error {
			require.Fail(t,

				"CreateAPIKey must not be called for empty scopes")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeAPIKeysManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateAPIKey(ctx, &CreateAPIKeyInput{Body: CreateAPIKeyRequest{
		ProjectID: "proj-1",
		Name:      "empty-scope",
	}})
	require.True(
		t, isHumaStatusError(err, http.StatusBadRequest),
	)

}

func TestHandleCreateAPIKey_RejectsCrossOrgID(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, _ *domain.APIKey) error {
			require.Fail(t,

				"CreateAPIKey must not be called for cross-org create")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeAll})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxOrgIDKey, "org-1")

	_, err := srv.handleCreateAPIKey(ctx, &CreateAPIKeyInput{Body: CreateAPIKeyRequest{
		ProjectID: "proj-1",
		OrgID:     "org-2",
		Name:      "cross-org",
		Scopes:    []string{domain.ScopeJobsRead},
	}})
	require.True(
		t, isHumaStatusError(err, http.StatusForbidden))

}

func TestHandleCreateAPIKey_EnvironmentScopedCallerCannotCreateProjectWideKey(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, _ *domain.APIKey) error {
			require.Fail(t,

				"CreateAPIKey must not be called for env scope mismatch")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeAPIKeysManage, domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleCreateAPIKey(ctx, &CreateAPIKeyInput{Body: CreateAPIKeyRequest{
		ProjectID: "proj-1",
		Name:      "project-wide",
		Scopes:    []string{domain.ScopeJobsRead},
	}})
	require.True(
		t, isHumaStatusError(err, http.StatusNotFound))

}

func TestHandleListAPIKeys_FiltersEnvironmentScopedCaller(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ms := &APIStoreMock{
		ListAPIKeysByProjectFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.APIKey, error) {
			require.Equal(t, "proj-1", projectID)

			return []domain.APIKey{
				{ID: "key-prod", ProjectID: "proj-1", EnvironmentID: "env-prod", CreatedAt: now},
				{ID: "key-staging", ProjectID: "proj-1", EnvironmentID: "env-staging", CreatedAt: now},
				{ID: "key-wide", ProjectID: "proj-1", CreatedAt: now},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeAPIKeysManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	out, err := srv.handleListAPIKeys(ctx, &ListAPIKeysInput{})
	require.NoError(t, err)

	keys, ok := out.Body.Data.([]domain.APIKey)
	require.True(
		t, ok)
	require.False(t, len(keys) !=
		1 || keys[0].ID !=
		"key-prod")

}

func TestHandleRevokeAPIKey_EnvironmentScopedCallerCannotRevokeOtherEnvironment(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: id, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
		RevokeAPIKeyFunc: func(_ context.Context, _ string) error {
			require.Fail(t,

				"RevokeAPIKey must not be called for env scope mismatch")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeAPIKeysManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleRevokeAPIKey(ctx, &RevokeAPIKeyInput{KeyID: "key-staging"})
	require.True(
		t, isHumaStatusError(err, http.StatusNotFound))

}

func TestHandleRotateAPIKey_EnvironmentScopedCallerCannotRotateOtherEnvironment(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:            id,
				ProjectID:     "proj-1",
				Name:          "staging",
				EnvironmentID: "env-staging",
				Scopes:        []string{domain.ScopeJobsRead},
			}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, _ *domain.APIKey) error {
			require.Fail(t,

				"CreateAPIKey must not be called for env scope mismatch")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeAPIKeysManage, domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleRotateAPIKey(ctx, &RotateAPIKeyInput{
		KeyID: "key-staging",
		Body:  RotateAPIKeyRequest{GracePeriodMinutes: 30},
	})
	require.True(
		t, isHumaStatusError(err, http.StatusNotFound))

}

func TestHandleRotateAPIKey_LimitedCallerCannotRotateLegacyBroadKey(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAPIKeyByIDFunc: func(_ context.Context, id string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "legacy-wide",
				Scopes:    []string{},
			}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, _ *domain.APIKey) error {
			require.Fail(t,

				"CreateAPIKey must not be called when caller cannot receive wildcard-equivalent key")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeAPIKeysManage, domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")

	_, err := srv.handleRotateAPIKey(ctx, &RotateAPIKeyInput{
		KeyID: "key-legacy",
		Body:  RotateAPIKeyRequest{GracePeriodMinutes: 30},
	})
	require.True(
		t, isHumaStatusError(err, http.StatusForbidden))

}

// FuzzAPIKeyAuth fuzzes Authorization headers to ensure the auth middleware
// does not panic on arbitrary input.
func FuzzAPIKeyAuth(f *testing.F) {
	f.Add("Bearer strait_abc123")
	f.Add("Bearer ")
	f.Add("")
	f.Add("Basic dXNlcjpwYXNz")
	f.Add("Bearer strait_" + strings.Repeat("x", 64))
	f.Add("Bearer strait_\x00\x01\x02")
	f.Add(strings.Repeat("A", 10000))

	f.Fuzz(func(t *testing.T, header string) {
		ms := &APIStoreMock{
			GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
				return nil, errors.New("not found")
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)

		req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		_ = w.Code
	})
}

// Ensure sha256 import is used (the hashAPIKey function uses it from the main package).
var _ = sha256.New
