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

	"strait/internal/domain"
)

func TestHandleRotateAPIKey(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
	ms.GetAPIKeyByIDFunc = func(_ context.Context, id string) (*domain.APIKey, error) {
		return &domain.APIKey{ID: id, ProjectID: "proj-1", OrgID: "org-1", Name: "prod key", Scopes: []string{"jobs:read"}}, nil
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

func TestHandleRotateAPIKey_RevokeReplacementWhenMarkFails(t *testing.T) {
	t.Parallel()

	var revokedReplacement atomic.Bool
	ms := &APIStoreMock{}
	ms.GetAPIKeyByIDFunc = func(_ context.Context, id string) (*domain.APIKey, error) {
		return &domain.APIKey{ID: id, ProjectID: "proj-1", OrgID: "org-1", Name: "prod key", Scopes: []string{"jobs:read"}}, nil
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
