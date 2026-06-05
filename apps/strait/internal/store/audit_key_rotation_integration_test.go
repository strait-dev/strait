//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
	"strait/internal/store"
)

const testEncKey = "test-encryption-key-32bytes!!!!"

// TestRotateAuditSigningKey_EmitsAnchor asserts that rotation writes a
// single anchor row with action=audit.key_rotated, is_anchor=true, and
// monotonically-increasing rotation_epoch, with the correct details shape.
func TestRotateAuditSigningKey_EmitsAnchor(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	key, _ := store.DeriveAuditSigningKey("rotate-anchor-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-rotate-anchor"
	insertTestChain(ctx, t, q, projectID, 3)

	newEpoch, err := q.RotateAuditSigningKey(ctx, projectID, "actor-rotator")
	require.NoError(t, err)
	assert.EqualValues(t, 1, newEpoch)

	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 1000, nil, nil, nil, true)
	require.NoError(t, err)

	var anchors []domain.AuditEvent
	for _, ev := range events {
		if ev.Action == domain.AuditActionKeyRotated {
			anchors = append(anchors, ev)
		}
	}
	require.Len(t, anchors,

		1)

	a := anchors[0]
	assert.True(t, a.IsAnchor)
	assert.EqualValues(t, 1, a.RotationEpoch)

	var details map[string]any
	require.NoError(t, json.
		Unmarshal(a.Details,
			&details))

	if got, want := details["previous_epoch"], float64(0); got != want {
		assert.Failf(t, "test failure",

			"previous_epoch = %v, want %v", got, want)
	}
	if got, want := details["new_epoch"], float64(1); got != want {
		assert.Failf(t, "test failure",

			"new_epoch = %v, want %v", got, want)
	}
	if got, want := details["rotated_by"], "actor-rotator"; got != want {
		assert.Failf(t, "test failure",

			"rotated_by = %v, want %v", got, want)
	}
}

// TestVerifyAuditChain_AcceptsAnchorBoundary seeds 5 events in epoch 0,
// rotates the signing key, then seeds 5 events in epoch 1. VerifyAuditChain
// must succeed and span both epochs.
func TestVerifyAuditChain_AcceptsAnchorBoundary(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	key, _ := store.DeriveAuditSigningKey("rotate-boundary-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-rotate-boundary"

	// Epoch 0: 5 events.
	insertTestChain(ctx, t, q, projectID, 5)

	// Rotate → anchor row under epoch 1.
	newEpoch, err := q.RotateAuditSigningKey(ctx, projectID, "actor-boundary")
	require.NoError(t, err)
	require.EqualValues(t, 1, newEpoch)

	// Epoch 1: 5 more events. CreateAuditEvent must assign the current
	// rotation epoch automatically; callers should not need to know the
	// current per-project signing epoch.
	for range 5 {
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      "actor",
			ActorType:    "user",
			Action:       domain.AuditActionJobCreated,
			ResourceType: "job",
			ResourceID:   "post-rotation",
			Details:      json.RawMessage(`{"post":true}`),
		}
		require.NoError(t, q.CreateAuditEvent(ctx, ev))
		require.EqualValues(t, 1, ev.
			RotationEpoch,
		)

	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, result.
		Valid,
	)
	assert.EqualValues(t, 11, result.
		EventsChecked,
	)

	// 5 pre + 1 anchor + 5 post = 11.

}

func TestVerifyAuditChain_MissingNonzeroEpochKeyFailsClosed(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	key, _ := store.DeriveAuditSigningKey("missing-epoch-key-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-missing-epoch-key"
	insertTestChain(ctx, t, q, projectID, 2)
	if _, err := q.RotateAuditSigningKey(ctx, projectID, "actor-rotator"); err != nil {
		require.Failf(t, "test failure",

			"RotateAuditSigningKey: %v", err)
	}

	if _, err := testDB.Pool.Exec(ctx, `
		DELETE FROM audit_signing_keys
		WHERE project_id = $1 AND rotation_epoch = 1
	`, projectID); err != nil {
		require.Failf(t, "test failure",

			"delete epoch key: %v", err)
	}

	_, err := q.VerifyAuditChain(ctx, projectID)
	require.Error(t, err)

	if got := err.Error(); !strings.Contains(got, "no stored key for epoch 1") {
		require.Failf(t, "test failure",

			"VerifyAuditChain error = %v, want missing epoch key", err)
	}
}

// TestVerifyAuditChain_ForgedAnchor_Fails asserts that tampering with an
// anchor row's HMAC signature is detected. Flipping the is_anchor flag
// alone is not enough to forge — the row's canonical signature is bound
// to action/previous_hash etc., so a forger must either produce a valid
// HMAC (impossible without the secret) or corrupt an existing anchor's
// signature (detected here).
func TestVerifyAuditChain_ForgedAnchor_Fails(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	key, _ := store.DeriveAuditSigningKey("rotate-forge-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-rotate-forge"
	insertTestChain(ctx, t, q, projectID, 3)

	if _, err := q.RotateAuditSigningKey(ctx, projectID, "actor-forge"); err != nil {
		require.Failf(t, "test failure",

			"RotateAuditSigningKey: %v", err)
	}

	// Tamper: rewrite the anchor's signature. This simulates an attacker
	// who fabricates an anchor without access to the signing key.
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE audit_events
		SET signature = '00000000000000000000000000000000000000000000000000000000deadbeef'
		WHERE project_id = $1 AND is_anchor = TRUE
	`, projectID); err != nil {
		require.Failf(t, "test failure",

			"tamper anchor signature: %v", err)
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

// TestRotateAuditSigningKey_SerializedUnderContention asserts that two
// concurrent rotations complete without corrupting the chain and produce
// strictly monotonically-increasing epochs.
func TestRotateAuditSigningKey_SerializedUnderContention(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	key, _ := store.DeriveAuditSigningKey("rotate-contention-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-rotate-contention"
	insertTestChain(ctx, t, q, projectID, 2)

	var (
		wg     conc.WaitGroup
		mu     sync.Mutex
		epochs []int
		errs   []error
	)
	for range 2 {
		wg.Go(func() {
			e, err := q.RotateAuditSigningKey(ctx, projectID, "actor-contention")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			epochs = append(epochs, e)
		})
	}
	wg.Wait()
	require.LessOrEqual(t,
		len(errs),
		0)
	require.Len(t, epochs,
		2,
	)
	assert.NotEqual(t, epochs[1], epochs[0])

	// Chain must still verify.
	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, result.
		Valid,
	)

}

// TestRotateAuditSigningKey_UniqueAnchorPerEpoch_Integration drives 50
// concurrent rotations against a single project. The advisory lock plus
// the unique partial index on (project_id, rotation_epoch) WHERE is_anchor
// must guarantee that every rotation receives a distinct, monotonically
// increasing epoch and that no second-loser unique-violation leaks to
// callers — retries happen transparently inside RotateAuditSigningKey.
func TestRotateAuditSigningKey_UniqueAnchorPerEpoch_Integration(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("rotate-unique-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-rotate-unique"
	insertTestChain(ctx, t, q, projectID, 2)

	const n = 50
	var (
		wg     conc.WaitGroup
		mu     sync.Mutex
		epochs = make([]int, 0, n)
		errs   = make([]error, 0)
	)
	for range n {
		wg.Go(func() {
			e, err := q.RotateAuditSigningKey(ctx, projectID, "actor-heavy")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			epochs = append(epochs, e)
		})
	}
	wg.Wait()
	require.LessOrEqual(t,
		len(errs),
		0)
	require.Len(t, epochs,
		n,
	)

	seen := make(map[int]struct{}, n)
	minEpoch, maxEpoch := epochs[0], epochs[0]
	for _, e := range epochs {
		if _, dup := seen[e]; dup {
			assert.Failf(t, "test failure",

				"duplicate epoch %d", e)
		}
		seen[e] = struct{}{}
		if e < minEpoch {
			minEpoch = e
		}
		if e > maxEpoch {
			maxEpoch = e
		}
	}
	assert.False(t, minEpoch !=
		1 ||
		maxEpoch !=
			n)

	// Verify the chain is still intact — every rotation's anchor verifies
	// under its own epoch's stored key.
	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, result.
		Valid,
	)

}

// TestVerifyAuditChain_RealPerEpochKeys asserts that post-rotation events
// verify under the NEW epoch's stored key — not the global fallback.
// Swapping the global key after rotation still permits verification of
// the post-rotation segment, which is only possible if VerifyAuditChain
// is loading epoch-specific keys from audit_signing_keys.
func TestVerifyAuditChain_RealPerEpochKeys(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	globalKey, _ := store.DeriveAuditSigningKey("per-epoch-secret")
	q.SetAuditSigningKey(globalKey)

	projectID := "proj-per-epoch"
	insertTestChain(ctx, t, q, projectID, 3)

	if _, err := q.RotateAuditSigningKey(ctx, projectID, "actor"); err != nil {
		require.Failf(t, "test failure",

			"rotate: %v", err)
	}

	// Emit post-rotation events under epoch 1. CreateAuditEvent signs
	// with q.auditSigningKey (still the global key here) — but the
	// verify path looks up epoch 1's stored key. For the post-rotation
	// events to verify, we need to emit them signed under the epoch-1
	// key. Do that by fetching the stored key and switching it in.
	epochKey, err := q.GetAuditSigningKey(ctx, projectID, 1)
	require.NoError(t, err)
	require.NotNil(t, epochKey)

	q.SetAuditSigningKey(epochKey)

	for range 3 {
		ev := &domain.AuditEvent{
			ProjectID:     projectID,
			ActorID:       "actor",
			ActorType:     "user",
			Action:        domain.AuditActionJobCreated,
			ResourceType:  "job",
			ResourceID:    "post",
			Details:       json.RawMessage(`{"i":0}`),
			RotationEpoch: 1,
		}
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, result.
		Valid,
	)
	assert.EqualValues(t, 7, result.
		EventsChecked,
	)

}

// TestVerifyAuditChain_WrongEpochKey_Fails corrupts the stored key
// material for epoch 1 and asserts that verification fails at the first
// epoch-1 row (the anchor) because the recomputed HMAC no longer matches.
func TestVerifyAuditChain_WrongEpochKey_Fails(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	globalKey, _ := store.DeriveAuditSigningKey("wrong-epoch-secret")
	q.SetAuditSigningKey(globalKey)

	projectID := "proj-wrong-epoch"
	insertTestChain(ctx, t, q, projectID, 2)

	if _, err := q.RotateAuditSigningKey(ctx, projectID, "actor"); err != nil {
		require.Failf(t, "test failure",

			"rotate: %v", err)
	}

	// Overwrite the stored epoch-1 key material with garbage bytes.
	// AES-GCM decryption will fail the auth tag check and return an
	// error, which VerifyAuditChain surfaces — a distinct negative
	// outcome from "signature mismatch". Both outcomes prove the
	// verifier consulted the stored per-epoch key.
	garbage := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE audit_signing_keys
		SET key_material = $1
		WHERE project_id = $2 AND rotation_epoch = 1
	`, garbage, projectID); err != nil {
		require.Failf(t, "test failure",

			"overwrite key_material: %v", err)
	}

	result, verr := q.VerifyAuditChain(ctx, projectID)
	require.False(t, verr ==

		nil && result.
		Valid,
	)

	// Either the decrypt fails (returned as error) or the HMAC check
	// fails (returned with result.Valid=false). Both are acceptable
	// negative outcomes — they prove the verifier actually consults the
	// per-epoch stored key.

}

// TestRotateAuditSigningKey_StoresDistinctKeyPerEpoch asserts that two
// rotations produce two distinct rows in audit_signing_keys with
// different key_material.
func TestRotateAuditSigningKey_StoresDistinctKeyPerEpoch(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	globalKey, _ := store.DeriveAuditSigningKey("distinct-secret")
	q.SetAuditSigningKey(globalKey)

	projectID := "proj-distinct"
	insertTestChain(ctx, t, q, projectID, 1)

	for i := range 2 {
		if _, err := q.RotateAuditSigningKey(ctx, projectID, "actor"); err != nil {
			require.Failf(t, "test failure",

				"rotate %d: %v", i, err)
		}
	}

	k1, err := q.GetAuditSigningKey(ctx, projectID, 1)
	require.False(t, err !=

		nil || k1 ==
		nil)

	k2, err := q.GetAuditSigningKey(ctx, projectID, 2)
	require.False(t, err !=

		nil || k2 ==
		nil)
	assert.False(t, len(k1) !=
		32 ||
		len(k2) !=
			32)

	same := true
	for i := range k1 {
		if k1[i] != k2[i] {
			same = false
			break
		}
	}
	assert.False(t, same)

	// Expected rows: 3.
	//   - epoch 0: bootstrapped by the first insertTestChain CreateAuditEvent
	//     call via resolveSigningKeyForEpoch (caveat-1 fix — signer and
	//     verifier must agree on a stable per-epoch key even before any
	//     rotation has occurred).
	//   - epoch 1: written explicitly by the first RotateAuditSigningKey.
	//   - epoch 2: written explicitly by the second RotateAuditSigningKey.
	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*) FROM audit_signing_keys WHERE project_id = $1
	`,

		projectID).Scan(
		&count))
	assert.EqualValues(t, 3, count)

}

// TestBootstrapKey_PerProjectUniqueness asserts that the per-epoch key
// persisted on first write for a project is derived per-(project, epoch)
// projects must bootstrap to two distinct key_material blobs even when
// they share the same global signing key, proving cross-tenant isolation
// of HMAC signing material.
func TestBootstrapKey_PerProjectUniqueness(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	globalKey, _ := store.DeriveAuditSigningKey("bootstrap-uniqueness-secret")
	q.SetAuditSigningKey(globalKey)

	projectA := "proj-bootstrap-a"
	projectB := "proj-bootstrap-b"
	insertTestChain(ctx, t, q, projectA, 1)
	insertTestChain(ctx, t, q, projectB, 1)

	keyA, err := q.GetAuditSigningKey(ctx, projectA, 0)
	require.False(t, err !=

		nil || keyA ==
		nil)

	keyB, err := q.GetAuditSigningKey(ctx, projectB, 0)
	require.False(t, err !=

		nil || keyB ==
		nil)
	require.False(t, len(keyA) != 32 ||
		len(keyB) != 32)

	identical := true
	for i := range keyA {
		if keyA[i] != keyB[i] {
			identical = false
			break
		}
	}
	require.False(t, identical)

	// Both must differ from the global in-memory signing key too.
	sameAsGlobalA := true
	for i := range keyA {
		if keyA[i] != globalKey[i] {
			sameAsGlobalA = false
			break
		}
	}
	require.False(t, sameAsGlobalA)

}

func TestAuditEpochKeys_DeriveFromAuditSigningRootNotEnvelopeKey(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projectID := "proj-audit-root-separation"
	envelopeKey := testEncKey

	qA := mustStore(t)
	qA.SetSecretEncryptionKey(envelopeKey)
	auditRootA, _ := store.DeriveAuditSigningKey("audit-root-a")
	qA.SetAuditSigningKey(auditRootA)
	insertTestChain(ctx, t, qA, projectID, 1)
	keyA, err := qA.GetAuditSigningKey(ctx, projectID, 0)
	require.False(t, err !=

		nil || keyA ==
		nil)

	mustClean(t, ctx)
	qB := mustStore(t)
	qB.SetSecretEncryptionKey(envelopeKey)
	auditRootB, _ := store.DeriveAuditSigningKey("audit-root-b")
	qB.SetAuditSigningKey(auditRootB)
	insertTestChain(ctx, t, qB, projectID, 1)
	keyB, err := qB.GetAuditSigningKey(ctx, projectID, 0)
	require.False(t, err !=

		nil || keyB ==
		nil)
	require.NotEqual(t, string(keyB),
		string(keyA))

	expectedA, err := store.DeriveAuditSigningKeyForEpochFromRoot(auditRootA, projectID, 0)
	require.NoError(t, err)
	require.Equal(t, string(
		expectedA,
	), string(keyA))

}

func TestAuditSigningKeyDecryptsWithOldEnvelopeKey(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projectID := "proj-audit-old-envelope"
	oldQ := mustStore(t)
	oldQ.SetSecretEncryptionKey("old-audit-envelope-key")
	auditRoot, _ := store.DeriveAuditSigningKey("audit-root-old-envelope")
	oldQ.SetAuditSigningKey(auditRoot)
	insertTestChain(ctx, t, oldQ, projectID, 1)

	newQ := mustStore(t)
	newQ.SetSecretEncryptionKey("new-audit-envelope-key")
	newQ.SetOldSecretEncryptionKeys([]string{"old-audit-envelope-key"})
	newQ.SetAuditSigningKey(auditRoot)

	key, err := newQ.GetAuditSigningKey(ctx, projectID, 0)
	require.NoError(t, err)
	require.NotEmpty(t, key)

	withoutOld := mustStore(t)
	withoutOld.SetSecretEncryptionKey("new-audit-envelope-key")
	withoutOld.SetAuditSigningKey(auditRoot)
	if _, err := withoutOld.GetAuditSigningKey(ctx, projectID, 0); err == nil {
		require.Fail(t,

			"GetAuditSigningKey(without old envelope) error = nil, want decrypt failure")
	}
}

func TestVerifyAuditChain_EpochZeroBootstrapPreservesLegacyRows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	globalKey, _ := store.DeriveAuditSigningKey("legacy-epoch-zero-secret")
	q.SetAuditSigningKey(globalKey)

	projectID := "proj-legacy-epoch-zero"
	_ = insertLegacyEpochZeroAuditEvent(ctx, t, projectID, globalKey)

	ev := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "actor-new",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "job-new",
		Details:      json.RawMessage(`{"new":true}`),
	}
	require.NoError(t, q.CreateAuditEvent(ctx, ev))
	require.EqualValues(t, 0, ev.
		RotationEpoch,
	)
	require.Equal(t, "job",

		ev.ShardID,
	)
	require.Equal(t, store.
		ZeroHash,

		ev.PreviousHash,
	)

	// Auto-shard-derivation: the new event with ResourceType "job" lands in
	// shard "job" while the legacy row carries the migration-default ''
	// shard. Each is the head of its own sub-chain; the legacy row no
	// longer serves as a chain ancestor for new writes.

	epochKey, err := q.GetAuditSigningKey(ctx, projectID, 0)
	require.NoError(t, err)
	require.NotNil(t, epochKey)
	require.NotEqual(t, string(globalKey), string(epochKey))

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, result.
		Valid,
	)
	require.EqualValues(t, 2, result.
		EventsChecked,
	)

}

func insertLegacyEpochZeroAuditEvent(ctx context.Context, t *testing.T, projectID string, key []byte) domain.AuditEvent {
	t.Helper()

	ev := domain.AuditEvent{
		ID:            "legacy-epoch-zero-" + newID(),
		ProjectID:     projectID,
		ActorID:       "actor-legacy",
		ActorType:     "user",
		Action:        domain.AuditActionJobCreated,
		ResourceType:  "job",
		ResourceID:    "job-legacy",
		Details:       json.RawMessage(`{"legacy":true}`),
		PreviousHash:  store.ZeroHash,
		CreatedAt:     time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond),
		SchemaVersion: domain.AuditEventSchemaVersionCurrent,
		RotationEpoch: 0,
	}
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		INSERT INTO audit_events (
			id, project_id, actor_id, actor_type, action, resource_type, resource_id,
			details, signature, previous_hash, created_at,
			remote_ip, user_agent, request_id, trace_id, schema_version,
			is_anchor, rotation_epoch
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8::jsonb, '', $9, $10,
			$11, $12, $13, $14, $15,
			$16, $17
		)
		RETURNING details
	`,

		ev.ID, ev.ProjectID, ev.
			ActorID, ev.ActorType, ev.Action, ev.ResourceType, ev.ResourceID,
		ev.Details, ev.PreviousHash,
		ev.CreatedAt,

		ev.RemoteIP, ev.UserAgent,
		ev.RequestID, ev.TraceID, ev.SchemaVersion, ev.IsAnchor,
		ev.RotationEpoch).Scan(&ev.Details))

	ev.Signature = store.ComputeAuditSignature(&ev, key)
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE audit_events
		SET signature = $1
		WHERE id = $2
	`, ev.Signature, ev.ID); err != nil {
		require.Failf(t, "test failure",

			"sign legacy audit event: %v", err)
	}
	return ev
}

// TestRotateAuditSigningKey_WritesCreatedBy asserts migration 000194's
// created_by column is populated from the actorID passed to rotation.
// The forensic trail now lives on both the chain event (audit.key_rotated
// details.rotated_by) AND the key row itself — mirrored redundantly so a
// lost chain event still leaves audit_signing_keys.created_by as evidence.
func TestRotateAuditSigningKey_WritesCreatedBy(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("created-by-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-created-by"
	insertTestChain(ctx, t, q, projectID, 1)

	if _, err := q.RotateAuditSigningKey(ctx, projectID, "actor-rotator-42"); err != nil {
		require.Failf(t, "test failure",

			"RotateAuditSigningKey: %v", err)
	}

	var createdBy *string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT created_by FROM audit_signing_keys
		WHERE project_id = $1 AND rotation_epoch = 1
	`,

		projectID).Scan(
		&createdBy))
	require.NotNil(t, createdBy)
	assert.Equal(t, "actor-rotator-42",

		*createdBy,
	)

}

// TestStoreAuditSigningKey_RejectsTooShortMaterial exercises migration
// 000194's octet_length(key_material) >= 28 CHECK. A key shorter than
// the AES-GCM minimum (12-byte nonce + 16-byte tag) could only come from
// a write path that skipped encryption — the CHECK prevents that
// misconfiguration from persisting.
func TestStoreAuditSigningKey_RejectsTooShortMaterial(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	// Insert a direct short row via SQL to bypass the encrypt helper —
	// the CHECK must still fire.
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO audit_signing_keys (project_id, rotation_epoch, key_material)
		VALUES ($1, $2, $3)
	`, "proj-short-key", 1, []byte{0x00, 0x01, 0x02, 0x03})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.
		Error(),
		"audit_signing_keys_key_material_length",
	))

}
