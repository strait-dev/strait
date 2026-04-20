//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/google/uuid"
)

// TestCreateAuditEvent_CrashBetweenInsertAndSignature_DoesNotBreakChain
// asserts that a failure between the INSERT and the (now implicit) commit
// leaves NO row behind — the transaction rolls back entirely. Under the
// prior two-statement (INSERT empty-sig + UPDATE sig) design, a crash
// between the two statements left a permanently-unsigned row that
// VerifyAuditChain flagged as broken. The new atomic design emits either
// a fully-signed row or no row at all; the chain remains verifiable.
func TestCreateAuditEvent_CrashBetweenInsertAndSignature_DoesNotBreakChain(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("atomic-insert-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-atomic-insert"
	// Seed a handful of healthy rows so the chain has a tail for the
	// aborted insert to fail against.
	insertTestChain(ctx, t, q, projectID, 3)

	// Force a failure in the window that used to live between INSERT and
	// the signature UPDATE. With the atomic design, this failure is
	// observed AFTER the signed INSERT statement; the tx rolls back.
	forced := errors.New("forced post-insert failure")
	store.SetAuditEventPostInsertHookForTest(q, func(context.Context) error {
		return forced
	})
	t.Cleanup(func() { store.SetAuditEventPostInsertHookForTest(q, nil) })

	// Attempt to write a 4th event — this must return the forced error.
	attemptID := uuid.Must(uuid.NewV7()).String()
	ev := &domain.AuditEvent{
		ID:           attemptID,
		ProjectID:    projectID,
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "aborted",
		Details:      json.RawMessage(`{"aborted":true}`),
	}
	err := q.CreateAuditEvent(ctx, ev)
	if err == nil {
		t.Fatalf("CreateAuditEvent: expected forced error, got nil")
	}
	if !errors.Is(err, forced) {
		t.Errorf("err = %v, want wrap of forced sentinel", err)
	}

	// The row must not exist — the tx rolled back.
	var count int
	if qerr := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_events WHERE id = $1
	`, attemptID).Scan(&count); qerr != nil {
		t.Fatalf("count: %v", qerr)
	}
	if count != 0 {
		t.Errorf("row count after aborted insert = %d, want 0 (tx must roll back)", count)
	}

	// Clear the hook and verify the chain. With the atomic design, no
	// row was ever written with an empty signature, so the chain over
	// the 3 healthy seed events still verifies.
	store.SetAuditEventPostInsertHookForTest(q, nil)
	result, verr := q.VerifyAuditChain(ctx, projectID)
	if verr != nil {
		t.Fatalf("VerifyAuditChain: %v", verr)
	}
	if !result.Valid {
		t.Fatalf("chain invalid after aborted insert: %s", result.Error)
	}
	if result.EventsChecked != 3 {
		t.Errorf("EventsChecked = %d, want 3 (aborted row must not appear)", result.EventsChecked)
	}
}
