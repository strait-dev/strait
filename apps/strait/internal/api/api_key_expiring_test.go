package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestListExpiringKeys_ReturnsNoExpiryKeys(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListAPIKeysExpiringSoonFunc: func(_ context.Context, _ string, _ int) ([]domain.APIKey, error) {
			return []domain.APIKey{
				{ID: "key-1", Name: "no-expiry-key", KeyPrefix: "strait_abc", ExpiresAt: nil, CreatedAt: time.Now()},
			}, nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeAPIKeysManage)(
		TypedHandler(srv, http.StatusOK, srv.handleListExpiringKeys),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/api-keys/expiring-soon", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeAPIKeysManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var result []ExpiringKeyInfo
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 key, got %d", len(result))
	}
	if !result[0].NoExpiry {
		t.Error("expected no_expiry=true for key without expiration")
	}
	if result[0].DaysLeft != nil {
		t.Error("expected days_left=nil for no-expiry key")
	}
}

func TestListExpiringKeys_ReturnsDaysLeft(t *testing.T) {
	t.Parallel()

	expiry := time.Now().Add(10 * 24 * time.Hour)
	ms := &APIStoreMock{
		ListAPIKeysExpiringSoonFunc: func(_ context.Context, _ string, _ int) ([]domain.APIKey, error) {
			return []domain.APIKey{
				{ID: "key-2", Name: "expiring-key", KeyPrefix: "strait_xyz", ExpiresAt: &expiry, CreatedAt: time.Now()},
			}, nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeAPIKeysManage)(
		TypedHandler(srv, http.StatusOK, srv.handleListExpiringKeys),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/api-keys/expiring-soon?within_days=30", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeAPIKeysManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var result []ExpiringKeyInfo
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 key, got %d", len(result))
	}
	if result[0].NoExpiry {
		t.Error("expected no_expiry=false for key with expiration")
	}
	if result[0].DaysLeft == nil || *result[0].DaysLeft < 9 || *result[0].DaysLeft > 11 {
		t.Errorf("expected days_left ~10, got %v", result[0].DaysLeft)
	}
}

func TestListExpiringKeys_DefaultWithinDays(t *testing.T) {
	t.Parallel()

	var capturedDays int
	ms := &APIStoreMock{
		ListAPIKeysExpiringSoonFunc: func(_ context.Context, _ string, withinDays int) ([]domain.APIKey, error) {
			capturedDays = withinDays
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeAPIKeysManage)(
		TypedHandler(srv, http.StatusOK, srv.handleListExpiringKeys),
	)

	// No within_days param -- should default to 30.
	req := httptest.NewRequest(http.MethodGet, "/v1/api-keys/expiring-soon", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeAPIKeysManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if capturedDays != 30 {
		t.Errorf("within_days = %d, want default 30", capturedDays)
	}
}

func TestListExpiringKeys_CapsAt365(t *testing.T) {
	t.Parallel()

	var capturedDays int
	ms := &APIStoreMock{
		ListAPIKeysExpiringSoonFunc: func(_ context.Context, _ string, withinDays int) ([]domain.APIKey, error) {
			capturedDays = withinDays
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeAPIKeysManage)(
		TypedHandler(srv, http.StatusOK, srv.handleListExpiringKeys),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/api-keys/expiring-soon?within_days=999", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeAPIKeysManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if capturedDays != 365 {
		t.Errorf("within_days = %d, want capped at 365", capturedDays)
	}
}

func TestListExpiringKeys_RequiresPermission(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeAPIKeysManage)(
		TypedHandler(srv, http.StatusOK, srv.handleListExpiringKeys),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/api-keys/expiring-soon", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}
