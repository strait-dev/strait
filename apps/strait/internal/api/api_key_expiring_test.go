package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var result []ExpiringKeyInfo
	require.NoError(t, json.NewDecoder(w.Body).
		Decode(&result))
	require.Len(t,
		result, 1)
	assert.True(t,
		result[0].NoExpiry,
	)
	assert.Nil(t, result[0].
		DaysLeft)

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
	require.Equal(t, http.StatusOK,
		w.Code)

	var result []ExpiringKeyInfo
	require.NoError(t, json.NewDecoder(w.Body).
		Decode(&result))
	require.Len(t,
		result, 1)
	assert.False(
		t, result[0].NoExpiry,
	)
	assert.False(
		t, result[0].DaysLeft ==
			nil ||
			*result[0].DaysLeft <
				9 || *result[0].
			DaysLeft > 11)

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
	require.Equal(t, http.StatusOK,
		w.Code)
	assert.EqualValues(t, 30, capturedDays)

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
	assert.EqualValues(t, 365, capturedDays)

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
	assert.Equal(
		t, http.StatusForbidden,
		w.Code,
	)

}
