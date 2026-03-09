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

	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleCreateAPIKey_Success(t *testing.T) {
	t.Parallel()
	var created atomic.Bool
	var captured *domain.APIKey

	ms := &mockAPIStore{
		createAPIKeyFn: func(_ context.Context, key *domain.APIKey) error {
			created.Store(true)
			captured = key
			key.ID = "key-123"
			key.CreatedAt = time.Now().UTC()
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{"project_id":"proj-1","name":"CLI key","scopes":["jobs:read"]}`
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !created.Load() {
		t.Fatal("expected CreateAPIKey to be called")
	}
	if captured == nil {
		t.Fatal("expected captured key")
	}
	if captured.ProjectID != "proj-1" {
		t.Fatalf("expected project id proj-1, got %q", captured.ProjectID)
	}
	if captured.Name != "CLI key" {
		t.Fatalf("expected name CLI key, got %q", captured.Name)
	}
	if len(captured.Scopes) != 1 || captured.Scopes[0] != "jobs:read" {
		t.Fatalf("expected scopes [jobs:read], got %#v", captured.Scopes)
	}
	if captured.KeyHash == "" {
		t.Fatal("expected non-empty key hash")
	}
	if !strings.HasPrefix(captured.KeyPrefix, "strait_") {
		t.Fatalf("expected key prefix to start with strait_, got %q", captured.KeyPrefix)
	}

	var resp CreateAPIKeyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.ID == "" {
		t.Fatal("expected id in response")
	}
	if !strings.HasPrefix(resp.Key, "strait_") {
		t.Fatalf("expected key to start with strait_, got %q", resp.Key)
	}
	if resp.KeyPrefix == "" {
		t.Fatal("expected key_prefix in response")
	}
	if resp.ProjectID != "proj-1" {
		t.Fatalf("expected response project_id proj-1, got %q", resp.ProjectID)
	}
	if resp.Name != "CLI key" {
		t.Fatalf("expected response name CLI key, got %q", resp.Name)
	}
	if len(resp.Scopes) != 1 || resp.Scopes[0] != "jobs:read" {
		t.Fatalf("expected response scopes [jobs:read], got %#v", resp.Scopes)
	}
	if resp.CreatedAt.IsZero() {
		t.Fatal("expected non-zero created_at")
	}
}

func TestHandleCreateAPIKey_WithExpiry(t *testing.T) {
	t.Parallel()
	var captured *domain.APIKey
	ms := &mockAPIStore{
		createAPIKeyFn: func(_ context.Context, key *domain.APIKey) error {
			captured = key
			key.ID = "key-123"
			key.CreatedAt = time.Now().UTC()
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"expiring","expires_in_days":30}`
	now := time.Now()

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil || captured.ExpiresAt == nil {
		t.Fatal("expected expires_at to be set on stored key")
	}
	min := now.Add(29 * 24 * time.Hour)
	max := now.Add(31 * 24 * time.Hour)
	if captured.ExpiresAt.Before(min) || captured.ExpiresAt.After(max) {
		t.Fatalf("expected expires_at around 30 days in future, got %v", captured.ExpiresAt)
	}
}

func TestHandleCreateAPIKey_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateAPIKey_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", `{"name":"missing project"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateAPIKey_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		createAPIKeyFn: func(_ context.Context, _ *domain.APIKey) error {
			return errors.New("boom")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"failing key"}`

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/api-keys/", body))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleListAPIKeys_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listAPIKeysByProjectFn: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.APIKey, error) {
			if projectID != "proj-1" {
				t.Fatalf("expected project id proj-1, got %q", projectID)
			}
			now := time.Now().UTC()
			return []domain.APIKey{
				{ID: "key-1", ProjectID: projectID, Name: "first", KeyPrefix: "strait_aaaaaaaa", CreatedAt: now},
				{ID: "key-2", ProjectID: projectID, Name: "second", KeyPrefix: "strait_bbbbbbbb", CreatedAt: now},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/api-keys/?project_id=proj-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []domain.APIKey
	decodePaginatedList(t, w.Body.Bytes(), &resp)
	if len(resp) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(resp))
	}
}

func TestHandleListAPIKeys_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/api-keys/", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleListAPIKeys_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listAPIKeysByProjectFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.APIKey, error) {
			return nil, errors.New("boom")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/api-keys/?project_id=proj-1", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleRevokeAPIKey_Success(t *testing.T) {
	t.Parallel()
	var revokedID string
	ms := &mockAPIStore{
		revokeAPIKeyFn: func(_ context.Context, id string) error {
			revokedID = id
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/api-keys/key-123", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if revokedID != "key-123" {
		t.Fatalf("expected revoke id key-123, got %q", revokedID)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "revoked" {
		t.Fatalf("expected status=revoked, got %q", resp["status"])
	}
}

func TestHandleRevokeAPIKey_NotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		revokeAPIKeyFn: func(_ context.Context, _ string) error {
			return errors.New("not found")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/api-keys/key-123", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGenerateAPIKey_Format(t *testing.T) {
	t.Parallel()
	key, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generateAPIKey returned error: %v", err)
	}
	if !strings.HasPrefix(key, "strait_") {
		t.Fatalf("expected key prefix strait_, got %q", key)
	}
	if len(key) != 68 {
		t.Fatalf("expected key length 68, got %d", len(key))
	}
}

func TestGenerateAPIKey_Uniqueness(t *testing.T) {
	t.Parallel()
	keyA, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generateAPIKey returned error: %v", err)
	}
	keyB, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generateAPIKey returned error: %v", err)
	}
	if keyA == keyB {
		t.Fatal("expected generated keys to be unique")
	}
}

func TestHashAPIKey_Deterministic(t *testing.T) {
	t.Parallel()
	key := "strait_" + strings.Repeat("ab", 32)
	h1 := hashAPIKey(key)
	h2 := hashAPIKey(key)

	if h1 != h2 {
		t.Fatalf("expected deterministic hash, got %q and %q", h1, h2)
	}
}

func TestHashAPIKey_DifferentKeys(t *testing.T) {
	t.Parallel()
	h1 := hashAPIKey("strait_" + strings.Repeat("ab", 32))
	h2 := hashAPIKey("strait_" + strings.Repeat("cd", 32))

	if h1 == h2 {
		t.Fatal("expected different hashes for different keys")
	}
}

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("ab", 32)
	wantHash := hashAPIKey(rawKey)
	touched := make(chan string, 1)

	ms := &mockAPIStore{
		getAPIKeyByHashFn: func(_ context.Context, keyHash string) (*domain.APIKey, error) {
			if keyHash != wantHash {
				t.Fatalf("expected hash %q, got %q", wantHash, keyHash)
			}
			return &domain.APIKey{ID: "key-123", ProjectID: "proj-1"}, nil
		},
		touchAPIKeyLastUsedFn: func(_ context.Context, id string) error {
			touched <- id
			return nil
		},
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 1, Executing: 2, Delayed: 3}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case id := <-touched:
		if id != "key-123" {
			t.Fatalf("expected touched id key-123, got %q", id)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected TouchAPIKeyLastUsed to be called")
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

	ms := &mockAPIStore{
		getAPIKeyByHashFn: func(_ context.Context, keyHash string) (*domain.APIKey, error) {
			if keyHash != wantHash {
				t.Fatalf("expected hash %q, got %q", wantHash, keyHash)
			}
			return &domain.APIKey{ID: "key-touch", ProjectID: "proj-ctx"}, nil
		},
		touchAPIKeyLastUsedFn: func(ctx context.Context, id string) error {
			if id != "key-touch" {
				t.Fatalf("expected touched id key-touch, got %q", id)
			}
			dl, ok := ctx.Deadline()
			touchCh <- touchCall{hasDeadline: ok, deadline: dl, err: ctx.Err()}
			return nil
		},
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 1, Executing: 0, Delayed: 0}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case call := <-touchCh:
		if call.err != nil {
			t.Fatalf("expected detached touch context to be usable, got err: %v", call.err)
		}
		if !call.hasDeadline {
			t.Fatal("expected touch context deadline to be set")
		}
		remaining := time.Until(call.deadline)
		if remaining <= 0 || remaining > 3*time.Second {
			t.Fatalf("expected touch context deadline near 2s, remaining=%s", remaining)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected TouchAPIKeyLastUsed to be called")
	}
}

func TestAPIKeyAuth_ExpiredKey(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("ab", 32)
	expired := time.Now().Add(-time.Hour)

	ms := &mockAPIStore{
		getAPIKeyByHashFn: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-123", ProjectID: "proj-1", ExpiresAt: &expired}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["error"] != "api key has expired" {
		t.Fatalf("expected expired message, got %q", resp["error"])
	}
}

func TestAPIKeyAuth_RevokedKey(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("ab", 32)
	revoked := time.Now().Add(-time.Minute)

	ms := &mockAPIStore{
		getAPIKeyByHashFn: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-123", ProjectID: "proj-1", RevokedAt: &revoked}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["error"] != "api key has been revoked" {
		t.Fatalf("expected revoked message, got %q", resp["error"])
	}
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getAPIKeyByHashFn: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return nil, errors.New("missing")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer strait_badkey")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeyAuth_MissingHeader(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestDualAuth_FallbackToInternalSecret(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 7, Executing: 0, Delayed: 0}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/stats", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDualAuth_APIKeyTakesPrecedence(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("ab", 32)
	wantHash := hashAPIKey(rawKey)
	var lookedUp atomic.Bool

	ms := &mockAPIStore{
		getAPIKeyByHashFn: func(_ context.Context, keyHash string) (*domain.APIKey, error) {
			lookedUp.Store(true)
			if keyHash != wantHash {
				t.Fatalf("expected hash %q, got %q", wantHash, keyHash)
			}
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-1"}, nil
		},
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 1, Executing: 1, Delayed: 1}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/stats", "")
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !lookedUp.Load() {
		t.Fatal("expected api key lookup to be called")
	}
}

func TestProjectIDFromContext(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	if got := projectIDFromContext(ctx); got != "proj-1" {
		t.Fatalf("expected proj-1, got %q", got)
	}

	if got := projectIDFromContext(context.Background()); got != "" {
		t.Fatalf("expected empty string for missing context value, got %q", got)
	}
}
