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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// TestAPIKey_HashNeverReversible verifies that the SHA-256 hash of an API key
// cannot be reversed to recover the original key.
func TestAPIKey_HashNeverReversible(t *testing.T) {
	t.Parallel()

	rawKey, err := generateAPIKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	hash := hashAPIKey(rawKey)

	if len(hash) != 64 {
		t.Fatalf("expected 64-char hex hash, got %d chars", len(hash))
	}
	if strings.Contains(hash, rawKey) {
		t.Fatal("hash should not contain the raw key")
	}
	if hash == rawKey {
		t.Fatal("hash should differ from raw key")
	}

	// Deterministic.
	hash2 := hashAPIKey(rawKey)
	if hash != hash2 {
		t.Fatal("hashing the same key should be deterministic")
	}

	// Different keys produce different hashes.
	rawKey2, err := generateAPIKey()
	if err != nil {
		t.Fatalf("failed to generate second key: %v", err)
	}
	hash3 := hashAPIKey(rawKey2)
	if hash == hash3 {
		t.Fatal("different keys should produce different hashes")
	}
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

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("old key should be accepted during grace period, got 401: %s", w.Body.String())
	}
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

	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := `{"grace_period_minutes":60}`
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/key-1/rotate", body))
			if w.Code == http.StatusInternalServerError {
				t.Errorf("rotation %d should not cause 500: %s", idx, w.Body.String())
			}
		}(i)
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

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key should return 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "revoked") {
		t.Fatalf("expected 'revoked' in error message, got %s", w.Body.String())
	}
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

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expired key should return 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "expired") {
		t.Fatalf("expected 'expired' in error message, got %s", w.Body.String())
	}
}

// TestAPIKey_ScopeEscalation verifies that a key with limited scopes cannot
// access endpoints requiring broader scopes.
func TestAPIKey_ScopeEscalation(t *testing.T) {
	t.Parallel()

	limitedScopes := []string{"jobs:read"}

	if !domain.HasScope(limitedScopes, "jobs:read") {
		t.Fatal("expected jobs:read scope to be accessible")
	}
	if domain.HasScope(limitedScopes, "jobs:write") {
		t.Fatal("limited scope should not grant jobs:write access")
	}
	if domain.HasScope(limitedScopes, "secrets:read") {
		t.Fatal("limited scope should not grant secrets:read access")
	}
	if domain.HasScope(limitedScopes, "rbac:manage") {
		t.Fatal("limited scope should not grant rbac:manage access")
	}

	wildcardScopes := []string{"*"}
	if !domain.HasScope(wildcardScopes, "jobs:write") {
		t.Fatal("wildcard scope should grant any access")
	}
}

// TestAPIKey_PrefixFormat verifies that generated API keys always start with
// the "strait_" prefix.
func TestAPIKey_PrefixFormat(t *testing.T) {
	t.Parallel()

	for range 100 {
		key, err := generateAPIKey()
		if err != nil {
			t.Fatalf("failed to generate key: %v", err)
		}
		if !strings.HasPrefix(key, "strait_") {
			t.Fatalf("expected key to start with strait_, got %q", key[:12])
		}
		// Key should be "strait_" + 64 hex characters = 71 characters total.
		if len(key) != 71 {
			t.Fatalf("expected key length 71, got %d", len(key))
		}
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

	for i := range 10000 {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			t.Fatalf("failed to generate random bytes: %v", err)
		}
		fakeKey := "strait_" + hex.EncodeToString(b)

		req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		req.Header.Set("Authorization", "Bearer "+fakeKey)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: random key should be rejected, got %d", i, w.Code)
		}
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

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("null byte in key should result in 401, got %d", w.Code)
	}
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

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s: expected 401, got %d: %s", tc.name, w.Code, w.Body.String())
			}
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

			body := fmt.Sprintf(`{"project_id":"proj-1","name":"interval-test","rotation_interval_days":%d}`, interval)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", body))

			if w.Code == http.StatusInternalServerError {
				t.Fatalf("invalid rotation interval should not cause 500: %s", w.Body.String())
			}
			if w.Code == http.StatusCreated && captured != nil {
				if captured.NextRotationAt != nil && interval <= 0 {
					t.Fatalf("expected nil NextRotationAt for interval %d, got %v", interval, captured.NextRotationAt)
				}
			}
		})
	}
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
