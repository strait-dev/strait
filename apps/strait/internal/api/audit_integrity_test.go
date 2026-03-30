package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
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

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var result domain.AuditChainVerification
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !result.Valid {
		t.Error("expected valid chain")
	}
	if result.EventsChecked != 5 {
		t.Errorf("events_checked = %d, want 5", result.EventsChecked)
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var result domain.AuditChainVerification
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result.Valid {
		t.Error("expected invalid chain")
	}
	if result.BrokenAtID != "ev-3" {
		t.Errorf("broken_at_id = %q, want %q", result.BrokenAtID, "ev-3")
	}
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

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestComputeAuditSignature_ConsistentWithStore(t *testing.T) {
	t.Parallel()

	key, err := store.DeriveAuditSigningKey("consistency-test")
	if err != nil {
		t.Fatal(err)
	}

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
	if len(sig) != 64 {
		t.Errorf("signature length = %d, want 64", len(sig))
	}
}
