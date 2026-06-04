package api

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRotateAuditSigningKey_RequiresAdmin(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		RotateAuditSigningKeyFunc: func(_ context.Context, _, _ string) (int, error) {
			t.Error("store must not be called when caller is not admin")
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleRotateAuditSigningKey(nonAdminCtx("proj-a"), &RotateAuditSigningKeyInput{ID: "proj-a"})
	if err == nil {
		t.Fatal("expected 403 for non-admin caller, got nil")
	}
	if !strings.Contains(err.Error(), "admin") {
		t.Errorf("expected admin-required error, got %v", err)
	}
}

func TestRotateAuditSigningKey_RejectsCrossTenant(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		RotateAuditSigningKeyFunc: func(_ context.Context, _, _ string) (int, error) {
			t.Error("store must not be called on cross-tenant request")
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleRotateAuditSigningKey(adminCtx("proj-a"), &RotateAuditSigningKeyInput{ID: "proj-b"})
	if err == nil {
		t.Fatal("expected 404 for cross-tenant request, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestRotateAuditSigningKey_ReturnsEpochs(t *testing.T) {
	t.Parallel()

	var (
		seenProject atomic.Value
		seenActor   atomic.Value
	)
	ms := &APIStoreMock{
		RotateAuditSigningKeyFunc: func(_ context.Context, projectID, actorID string) (int, error) {
			seenProject.Store(projectID)
			seenActor.Store(actorID)
			// Bootstrap case: previous epoch 0, new epoch 1.
			return 1, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	out, err := srv.handleRotateAuditSigningKey(adminCtx("proj-a"), &RotateAuditSigningKeyInput{ID: "proj-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("nil output")
		return
	}
	if out.Body.NewEpoch != 1 {
		t.Errorf("new_epoch = %d, want 1", out.Body.NewEpoch)
	}
	if out.Body.PreviousEpoch != 0 {
		t.Errorf("previous_epoch = %d, want 0", out.Body.PreviousEpoch)
	}
	if got, _ := seenProject.Load().(string); got != "proj-a" {
		t.Errorf("store called with project %q, want proj-a", got)
	}
	if got, _ := seenActor.Load().(string); got != "internal:admin" {
		t.Errorf("store called with actor %q, want internal:admin", got)
	}
}

func TestRotateAuditSigningKey_StorePropagatesError(t *testing.T) {
	t.Parallel()

	// Simulate a store-layer SQL-ish failure. The handler must surface a
	// generic 500 without leaking the underlying error message into the
	// response body.
	sqlLike := errors.New("pq: duplicate key value violates unique constraint audit_events_pkey")
	ms := &APIStoreMock{
		RotateAuditSigningKeyFunc: func(_ context.Context, _, _ string) (int, error) {
			return 0, sqlLike
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleRotateAuditSigningKey(adminCtx("proj-a"), &RotateAuditSigningKeyInput{ID: "proj-a"})
	if err == nil {
		t.Fatal("expected error from store failure, got nil")
	}
	if strings.Contains(err.Error(), "pq:") || strings.Contains(err.Error(), "audit_events_pkey") {
		t.Errorf("handler leaked SQL-layer error text: %v", err)
	}
}
