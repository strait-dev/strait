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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Error(t, err)
	assert.True(t, errors.Is(err, forced))

	// The row must not exist — the tx rolled back.
	var count int
	require.Nil(t, testDB.
		Pool.
		QueryRow(ctx,
			`
		SELECT COUNT(*) FROM audit_events WHERE id = $1
	`,

			attemptID,
		).Scan(&count))
	assert.EqualValues(t, 0, count)

	// Clear the hook and verify the chain. With the atomic design, no
	// row was ever written with an empty signature, so the chain over
	// the 3 healthy seed events still verifies.
	store.SetAuditEventPostInsertHookForTest(q, nil)
	result, verr := q.VerifyAuditChain(ctx, projectID)
	require.Nil(t, verr)
	require.True(t, result.
		Valid,
	)
	assert.EqualValues(t, 3, result.
		EventsChecked,
	)

}

func TestCreateAuditEvent_FailedAttemptDoesNotPinRotationEpoch(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	rootKey, _ := store.DeriveAuditSigningKey("stale-epoch-retry-secret")
	q.SetAuditSigningKey(rootKey)

	projectID := "proj-stale-epoch-retry"
	require.NoError(t, q.CreateAuditEvent(ctx, &domain.AuditEvent{ProjectID: projectID,

		ActorID: "actor", ActorType: "user", Action: domain.
				AuditActionJobCreated, ResourceType: "job", ResourceID: "seed", Details: json.RawMessage(`{"seed":true}`)}))

	if _, err := q.RotateAuditSigningKey(ctx, projectID, "rotator-1"); err != nil {
		require.Failf(t, "test failure",

			"first rotation: %v", err)
	}

	forced := errors.New("forced failed audit attempt")
	store.SetAuditEventPostInsertHookForTest(q, func(context.Context) error {
		return forced
	})
	t.Cleanup(func() { store.SetAuditEventPostInsertHookForTest(q, nil) })

	ev := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "retry-after-rotation",
		Details:      json.RawMessage(`{"retry":true}`),
	}
	err := q.CreateAuditEvent(ctx, ev)
	require.Error(t, err)
	require.True(t, errors.Is(err, forced))
	require.False(t, ev.ID !=
		"" || ev.
		RotationEpoch !=
		0 || !ev.
		CreatedAt.
		IsZero() ||

		ev.PreviousHash != "" || ev.
		Signature != "" || ev.ShardID !=
		"")

	store.SetAuditEventPostInsertHookForTest(q, nil)
	if _, err := q.RotateAuditSigningKey(ctx, projectID, "rotator-2"); err != nil {
		require.Failf(t, "test failure",

			"second rotation: %v", err)
	}
	require.NoError(t, q.CreateAuditEvent(ctx, ev))
	require.EqualValues(t, 2, ev.
		RotationEpoch,
	)

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, result.
		Valid,
	)

}
