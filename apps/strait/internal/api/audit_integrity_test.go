package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
	"strait/internal/ratelimit"
	"strait/internal/store"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleVerifyAuditChain_ValidChain(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		VerifyAuditChainFunc: func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
			return &domain.AuditChainVerification{
				ProjectID:     projectID,
				Valid:         true,
				EventsChecked: 5,
				FirstEventID:  "ev-1",
				LastEventID:   "ev-5",
			}, nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeRBACManage)(
		TypedHandler(srv, http.StatusOK, srv.handleVerifyAuditChain),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/verify", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-key")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)

	var result domain.AuditChainVerification
	require.NoError(t, json.NewDecoder(w.Body).
		Decode(&result))
	assert.True(t,
		result.Valid)
	assert.EqualValues(t, 5, result.EventsChecked)

}

func TestHandleVerifyAuditChain_BrokenChain(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		VerifyAuditChainFunc: func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
			return &domain.AuditChainVerification{
				ProjectID:     projectID,
				Valid:         false,
				EventsChecked: 3,
				FirstEventID:  "ev-1",
				LastEventID:   "ev-3",
				BrokenAtID:    "ev-3",
				Error:         "chain broken at event ev-3: previous_hash mismatch",
			}, nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	handler := srv.requirePermission(domain.ScopeRBACManage)(
		TypedHandler(srv, http.StatusOK, srv.handleVerifyAuditChain),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/verify", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-key")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)

	var result domain.AuditChainVerification
	require.NoError(t, json.NewDecoder(w.Body).
		Decode(&result))
	assert.False(
		t, result.Valid)
	assert.Equal(
		t, "ev-3", result.
			BrokenAtID)

}

func TestHandleVerifyAuditChain_Adversarial_WrongScope(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)
	handler := srv.requirePermission(domain.ScopeRBACManage)(
		TypedHandler(srv, http.StatusOK, srv.handleVerifyAuditChain),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/verify", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-key")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(
		t, http.StatusForbidden,
		w.Code,
	)

}

// TestVerifyAuditChain_RateLimited asserts the per-project rate limit on
// /v1/audit-events/verify rejects the second request from the same project
// inside the 60s window with 429 + Retry-After. Backed by miniredis so the
// rate limiter actually counts (the fail-open path is exercised by the
// existing TestProjectRateLimit_NoRedis_FailsOpen test).
func TestVerifyAuditChain_RateLimited(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ms := &APIStoreMock{
		VerifyAuditChainFunc: func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
			return &domain.AuditChainVerification{ProjectID: projectID, Valid: true}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	srv.rateLimiter = ratelimit.NewRedisRateLimiter(rdb, true)

	handler := srv.requirePermission(domain.ScopeRBACManage)(
		srv.auditVerifyRateLimit(
			TypedHandler(srv, http.StatusOK, srv.handleVerifyAuditChain),
		),
	)

	makeRequest := func(projectID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/verify", nil)
		ctx := context.WithValue(req.Context(), ctxProjectIDKey, projectID)
		ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRBACManage})
		ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
		ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-key")
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	// First call: 200, with rate-limit headers indicating zero remaining.
	w1 := makeRequest("proj-rl-1")
	require.Equal(t, http.StatusOK,
		w1.Code)
	assert.Equal(
		t, "1", w1.Header().Get("X-RateLimit-Limit"))

	// Second call within window: 429 with Retry-After.
	w2 := makeRequest("proj-rl-1")
	require.Equal(t, http.StatusTooManyRequests,

		w2.Code)
	assert.Equal(
		t, "60", w2.Header().Get("Retry-After"))

	// Different project is unaffected — the limit is per-project.
	w3 := makeRequest("proj-rl-2")
	assert.Equal(
		t, http.StatusOK,
		w3.Code)

}

// TestVerifyAuditChain_RateLimit_NoLimiter_FailsOpen verifies the
// middleware is a no-op when no limiter is configured (test paths,
// installations without Redis).
func TestVerifyAuditChain_RateLimit_NoLimiter_FailsOpen(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		VerifyAuditChainFunc: func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
			return &domain.AuditChainVerification{ProjectID: projectID, Valid: true}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	// newTestServer's rateLimiter is constructed with a nil Redis client
	// and enabled=false (the disabled limiter), but the auditVerifyRateLimit
	// path explicitly checks for a nil rateLimiter — set it to nil so we
	// exercise the early-return branch.
	srv.rateLimiter = nil

	handler := srv.requirePermission(domain.ScopeRBACManage)(
		srv.auditVerifyRateLimit(
			TypedHandler(srv, http.StatusOK, srv.handleVerifyAuditChain),
		),
	)

	for range 10 {
		req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/verify", nil)
		ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-noredis")
		ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRBACManage})
		ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
		ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test-key")
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK,
			w.Code)

	}
}

// TestVerifyAuditChain_RateLimit_InternalSecretBypass verifies internal
// callers (e.g. SOC 2 evidence collection scripts) bypass the per-project
// rate limit so on-call operators are not rate-limited during incidents.
func TestVerifyAuditChain_RateLimit_InternalSecretBypass(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ms := &APIStoreMock{
		VerifyAuditChainFunc: func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
			return &domain.AuditChainVerification{ProjectID: projectID, Valid: true}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	srv.rateLimiter = ratelimit.NewRedisRateLimiter(rdb, true)

	handler := srv.auditVerifyRateLimit(
		TypedHandler(srv, http.StatusOK, srv.handleVerifyAuditChain),
	)

	for range 5 {
		req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/verify", nil)
		// Validated internal callers are identified by ctxInternalCallerKey,
		// set after ConstantTimeCompare passes in internalSecretAuth.
		ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-internal")
		ctx = context.WithValue(ctx, ctxActorTypeKey, "internal")
		ctx = context.WithValue(ctx, ctxInternalCallerKey, true)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK,
			w.Code)

	}
}

func TestComputeAuditSignature_ConsistentWithStore(t *testing.T) {
	t.Parallel()

	key, err := store.DeriveAuditSigningKey("consistency-test")
	require.NoError(t, err)

	ev := &domain.AuditEvent{
		ID:           "ev-1",
		ProjectID:    "proj-1",
		ActorID:      "actor-1",
		ActorType:    "api_key",
		Action:       "create",
		ResourceType: "role",
		ResourceID:   "role-1",
		Details:      json.RawMessage(`{}`),
		PreviousHash: store.ZeroHash,
	}

	sig := store.ComputeAuditSignature(ev, key)
	assert.Len(t,
		sig, 64)

}
