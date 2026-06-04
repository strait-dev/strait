package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
)

func TestHandleRotateAPIKey(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(24 * time.Hour)
	ms := &APIStoreMock{}
	ms.GetAPIKeyByIDFunc = func(_ context.Context, id string) (*domain.APIKey, error) {
		return &domain.APIKey{ID: id, ProjectID: "proj-1", OrgID: "org-1", Name: "prod key", Scopes: []string{"jobs:read"}, ExpiresAt: &expiresAt}, nil
	}
	ms.CreateAPIKeyFunc = func(_ context.Context, key *domain.APIKey) error {
		if key.ProjectID != "proj-1" {
			t.Fatalf("project_id mismatch: %s", key.ProjectID)
		}
		if key.OrgID != "org-1" {
			t.Fatalf("org_id mismatch: %s", key.OrgID)
		}
		key.ID = "key-2"
		return nil
	}
	ms.MarkAPIKeyRotatedFunc = func(_ context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error {
		if oldKeyID != "key-1" || newKeyID == "" {
			t.Fatalf("unexpected rotate args: %s %s", oldKeyID, newKeyID)
		}
		return nil
	}

	srv := newTestServer(t, ms, nil, nil)
	req := authedRequest(http.MethodPost, "/v1/api-keys/key-1/rotate", `{"grace_period_minutes":30}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "grace_expires_at") {
		t.Fatalf("expected grace_expires_at in response, got: %s", w.Body.String())
	}
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
		if oldKeyID != "key-old" || newKeyID != "key-new" {
			t.Fatalf("unexpected rotate args: %s %s", oldKeyID, newKeyID)
		}
		if time.Until(graceExpiresAt) <= 0 {
			t.Fatalf("grace deadline is not in the future: %s", graceExpiresAt)
		}
		return nil
	}

	var publishedChannel string
	var publishedDeadline time.Time
	pub := &mockPublisher{publishFn: func(_ context.Context, channel string, data []byte) error {
		publishedChannel = channel
		var err error
		publishedDeadline, err = time.Parse(time.RFC3339Nano, string(data))
		if err != nil {
			t.Fatalf("expiry payload is not RFC3339Nano: %q", string(data))
		}
		return nil
	}}
	srv := newTestServer(t, ms, nil, pub)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleRotateAPIKey(ctx, &RotateAPIKeyInput{
		KeyID: "key-old",
		Body:  RotateAPIKeyRequest{GracePeriodMinutes: 30},
	})
	if err != nil {
		t.Fatalf("handleRotateAPIKey: %v", err)
	}
	if publishedChannel != "apikey:expires:key-old" {
		t.Fatalf("published channel = %q, want apikey:expires:key-old", publishedChannel)
	}
	if time.Until(publishedDeadline) <= 0 {
		t.Fatalf("published deadline is not in the future: %s", publishedDeadline)
	}
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

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
	if created {
		t.Fatal("replacement key was created even though worker-stream expiry broadcast was unavailable")
	}
	if rotated {
		t.Fatal("old key was marked rotated even though worker-stream expiry broadcast was unavailable")
	}
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
		if oldKeyID != "key-1" || newKeyID != "key-replacement" {
			t.Fatalf("unexpected rotate args: %s %s", oldKeyID, newKeyID)
		}
		return errors.New("lost rotation race")
	}
	ms.RevokeAPIKeyFunc = func(_ context.Context, id string) error {
		if id != "key-replacement" {
			t.Fatalf("revoked key = %q, want replacement", id)
		}
		revokedReplacement.Store(true)
		return nil
	}

	srv := newTestServer(t, ms, nil, nil)
	req := authedRequest(http.MethodPost, "/v1/api-keys/key-1/rotate", `{"grace_period_minutes":30}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body: %s", w.Code, w.Body.String())
	}
	if !revokedReplacement.Load() {
		t.Fatal("replacement key was not revoked after MarkAPIKeyRotated failed")
	}
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

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
