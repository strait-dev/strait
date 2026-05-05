package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
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

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp CreateAPIKeyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.ExpiresAt == nil {
		t.Fatal("expected auto-capped expiry, got nil")
	}

	maxExpected := time.Now().Add(91 * 24 * time.Hour) // generous check
	if resp.ExpiresAt.After(maxExpected) {
		t.Errorf("expiry %v exceeds expected max ~90 days from now", resp.ExpiresAt)
	}
}

func TestCreateAPIKey_MaxLifetime_AcceptsWithinLimit(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 90)
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"],"expires_in_days":30}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
}

func TestCreateAPIKey_MaxLifetime_RejectsExceeding(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 90)
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"],"expires_in_days":120}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "exceeds project maximum") {
		t.Errorf("error should mention exceeding max: %s", body)
	}
}

func TestCreateAPIKey_NoMaxLifetime_BackwardCompatible(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 0) // no limit
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"]}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp CreateAPIKeyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	// No auto-capping when max=0.
	if resp.ExpiresAt != nil {
		t.Errorf("expected no expiry when max_lifetime=0, got %v", resp.ExpiresAt)
	}
}

func TestCreateAPIKey_NoMaxLifetime_LongExpiry_Accepted(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 0) // no limit
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"],"expires_in_days":365}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
}

func TestCreateAPIKey_Adversarial_ExpiresZero_WithMaxLifetime(t *testing.T) {
	t.Parallel()

	srv := newAPIKeyTestServer(t, 90)
	// expires_in_days=0 should be treated as "no expiry" and auto-capped.
	w := createKeyRequest(t, srv, `{"project_id":"proj-1","name":"test-key","scopes":["jobs:read"],"expires_in_days":0}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp CreateAPIKeyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.ExpiresAt == nil {
		t.Fatal("expected auto-capped expiry for expires_in_days=0 with max lifetime")
	}
}
