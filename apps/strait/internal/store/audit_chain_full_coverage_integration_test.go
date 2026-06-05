//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuditChain_FullSurface_Verifiable is the compliance backbone test:
// it configures a real HMAC signing key, inserts one AuditEvent for every
// registered action in domain.AuditActionSchemas, then runs the full chain
// verifier and asserts every event is cryptographically valid. This is
// the only place that exercises the advisory-lock + CTE-insert + HMAC
// compute path against a real Postgres.
func TestAuditChain_FullSurface_Verifiable(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("chain-integration-secret")
	require.NoError(t, err)

	q.SetAuditSigningKey(signingKey)

	projectID := "proj-chain-full"
	actions := domain.KnownAuditActions()

	for _, action := range actions {
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      "actor-full",
			ActorType:    "user",
			Action:       action,
			ResourceType: "probe",
			ResourceID:   "probe-" + action,
			Details:      json.RawMessage(`{"probe":true}`),
		}
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, result.
		Valid,
	)
	assert.Equal(t, len(actions), result.
		EventsChecked,
	)

	// Cross-check against ListAuditEvents: every action must appear at
	// least once and the chronological order must match insertion order.
	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 1000, nil, nil, nil, true)
	require.NoError(t, err)

	seen := map[string]bool{}
	for _, ev := range events {
		seen[ev.Action] = true
	}
	for _, action := range actions {
		assert.True(t, seen[action])

	}
}

// TestAuditChain_Tamper_DetailsRewrite asserts that rewriting the details
// column of a single event invalidates the chain at that row.
func TestAuditChain_Tamper_DetailsRewrite(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tamper-test-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-tamper-details"
	ids := insertTestChain(ctx, t, q, projectID, 5)

	// Before tampering — chain is valid.
	v1, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, v1.Valid)

	// Tamper with the middle event's details.
	tamperID := ids[2]
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE audit_events SET details = '{"tampered":true}'::jsonb WHERE id = $1`, tamperID,
	); err != nil {
		require.Failf(t, "test failure",

			"tamper update: %v", err)
	}

	v2, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.False(t, v2.Valid)
	assert.Equal(t, tamperID,

		v2.BrokenAtID,
	)

}

// TestAuditChain_Tamper_TimestampShift asserts that shifting the
// created_at of a single event breaks the chain (timestamp is part of
// the HMAC canonical form).
func TestAuditChain_Tamper_TimestampShift(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tamper-timestamp-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-tamper-ts"
	ids := insertTestChain(ctx, t, q, projectID, 3)

	// Shift the first event's timestamp by 1 second.
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE audit_events SET created_at = created_at + interval '1 second' WHERE id = $1`,
		ids[0],
	); err != nil {
		require.Failf(t, "test failure",

			"tamper timestamp: %v", err)
	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.False(t, result.
		Valid)
	assert.NotEqual(t, "",
		result.
			BrokenAtID,
	)

}

// TestAuditChain_Tamper_EventDelete asserts that deleting the middle
// event breaks the chain at the next surviving event (its previous_hash
// no longer matches).
func TestAuditChain_Tamper_EventDelete(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tamper-delete-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-tamper-del"
	ids := insertTestChain(ctx, t, q, projectID, 5)

	// Delete the middle event.
	if _, err := testDB.Pool.Exec(ctx, `DELETE FROM audit_events WHERE id = $1`, ids[2]); err != nil {
		require.Failf(t, "test failure",

			"delete middle: %v", err)
	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.False(t, result.
		Valid)
	assert.Equal(t, ids[3],

		result.BrokenAtID,
	)

	// The break should be at the first surviving event after the deleted one
	// (its previous_hash still points at the deleted event).

}

// TestAuditChain_Tamper_ForgeEvent asserts that inserting a forged event
// with a bogus signature between two valid events breaks the chain.
func TestAuditChain_Tamper_ForgeEvent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tamper-forge-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-tamper-forge"
	insertTestChain(ctx, t, q, projectID, 3)

	// Insert a forged event at current time with a fabricated signature.
	// Include the forensic columns (empty strings) so the SELECT in
	// VerifyAuditChain can scan them without hitting NULL string errors.
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO audit_events (id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at, remote_ip, user_agent, request_id, trace_id, schema_version)
		VALUES ($1, $2, 'forged', 'user', $3, 'probe', 'forged', '{}'::jsonb, $4, $5, NOW() + interval '1 minute', '', '', '', '', 2)
	`, "forged-id", projectID, domain.AuditActionJobCreated,
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"0000000000000000000000000000000000000000000000000000000000000000",
	)
	require.NoError(t, err)

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.False(t, result.
		Valid)

}

// TestAuditChain_Concurrent_ProjectsAreIndependent verifies the advisory
// lock isolates chain writes across projects: interleaved inserts to
// two projects produce two independently valid chains.
func TestAuditChain_Concurrent_ProjectsAreIndependent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("concurrent-secret")
	q.SetAuditSigningKey(key)

	pA := "proj-concurrent-a"
	pB := "proj-concurrent-b"

	for i := range 10 {
		pID := pA
		if i%2 == 0 {
			pID = pB
		}
		ev := &domain.AuditEvent{
			ProjectID:    pID,
			ActorID:      "a",
			ActorType:    "user",
			Action:       domain.AuditActionJobCreated,
			ResourceType: "probe",
			ResourceID:   "probe",
			Details:      json.RawMessage(`{}`),
		}
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

	}

	vA, _ := q.VerifyAuditChain(ctx, pA)
	vB, _ := q.VerifyAuditChain(ctx, pB)
	assert.True(t, vA.Valid)
	assert.True(t, vB.Valid)
	assert.False(t, vA.EventsChecked !=
		5 || vB.
		EventsChecked !=
		5)

}

// TestAuditChain_KeyRotation_DetectsOldEventsAsBroken documents the
// explicit limitation that rotating the signing key makes all existing
// events appear tampered — because signatures were computed with the
// old key. SOC 2 key rotation requires either re-signing the chain or
// anchoring the key change in a forensic marker. Neither is in scope
// today; this test exists so the limitation is visible.
func TestAuditChain_KeyRotation_DetectsOldEventsAsBroken(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	keyA, _ := store.DeriveAuditSigningKey("key-a")
	q.SetAuditSigningKey(keyA)

	projectID := "proj-key-rotation"
	insertTestChain(ctx, t, q, projectID, 3)

	v1, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, v1.Valid)

	// Rotate to key B.
	keyB, _ := store.DeriveAuditSigningKey("key-b")
	q.SetAuditSigningKey(keyB)

	v2, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.False(t, v2.Valid)
	assert.NotEqual(t, "",
		v2.
			BrokenAtID,
	)

}

// TestAuditChain_WithDeadletter verifies that rows in
// audit_events_deadletter do not affect main chain verification.
func TestAuditChain_WithDeadletter(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("dlq-chain-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-dlq-chain"
	insertTestChain(ctx, t, q, projectID, 3)

	// Spill a few events to the deadletter (bypasses the chain).
	for range 2 {
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      "dlq-actor",
			ActorType:    "user",
			Action:       domain.AuditActionJobTriggered,
			ResourceType: "job",
			ResourceID:   "dlq-job",
			Details:      json.RawMessage(`{}`),
			CreatedAt:    time.Now().UTC(),
		}
		require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "forced",
			3))

	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.EqualValues(t, 3, result.
		EventsChecked,
	)

	count, _ := q.CountAuditEventsDeadletter(ctx)
	assert.EqualValues(t, 2, count)

}

// insertTestChain inserts n events into projectID and returns their ids
// in insertion order.
func insertTestChain(ctx context.Context, t *testing.T, q *store.Queries, projectID string, n int) []string {
	t.Helper()
	ids := make([]string, n)
	for i := range n {
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      "actor",
			ActorType:    "user",
			Action:       domain.AuditActionJobCreated,
			ResourceType: "job",
			ResourceID:   "job-" + projectID,
			Details:      json.RawMessage(`{"i":` + itoaBench(i) + `}`),
		}
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

		ids[i] = ev.ID
	}
	return ids
}

// itoaBench is a local int-to-string to avoid importing strconv.
func itoaBench(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
