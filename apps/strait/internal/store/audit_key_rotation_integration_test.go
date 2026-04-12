//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

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
	if err != nil {
		t.Fatalf("RotateAuditSigningKey: %v", err)
	}
	if newEpoch != 1 {
		t.Errorf("newEpoch = %d, want 1", newEpoch)
	}

	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 1000, nil, nil, nil, true)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}

	var anchors []domain.AuditEvent
	for _, ev := range events {
		if ev.Action == domain.AuditActionKeyRotated {
			anchors = append(anchors, ev)
		}
	}
	if len(anchors) != 1 {
		t.Fatalf("expected 1 anchor, got %d", len(anchors))
	}
	a := anchors[0]
	if !a.IsAnchor {
		t.Error("anchor is_anchor = false")
	}
	if a.RotationEpoch != 1 {
		t.Errorf("anchor rotation_epoch = %d, want 1", a.RotationEpoch)
	}

	var details map[string]any
	if err := json.Unmarshal(a.Details, &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	if got, want := details["previous_epoch"], float64(0); got != want {
		t.Errorf("previous_epoch = %v, want %v", got, want)
	}
	if got, want := details["new_epoch"], float64(1); got != want {
		t.Errorf("new_epoch = %v, want %v", got, want)
	}
	if got, want := details["rotated_by"], "actor-rotator"; got != want {
		t.Errorf("rotated_by = %v, want %v", got, want)
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
	if err != nil {
		t.Fatalf("RotateAuditSigningKey: %v", err)
	}
	if newEpoch != 1 {
		t.Fatalf("newEpoch = %d, want 1", newEpoch)
	}

	// Epoch 1: 5 more events. They inherit rotation_epoch=0 by default
	// (column default), so to simulate real post-rotation emit we manually
	// set the epoch on each event.
	for i := 0; i < 5; i++ {
		ev := &domain.AuditEvent{
			ProjectID:     projectID,
			ActorID:       "actor",
			ActorType:     "user",
			Action:        domain.AuditActionJobCreated,
			ResourceType:  "job",
			ResourceID:    "post-rotation",
			Details:       json.RawMessage(`{"post":true}`),
			RotationEpoch: 1,
		}
		if err := q.CreateAuditEvent(ctx, ev); err != nil {
			t.Fatalf("CreateAuditEvent post-rotation %d: %v", i, err)
		}
	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !result.Valid {
		t.Fatalf("chain invalid: %s (broken at %q)", result.Error, result.BrokenAtID)
	}
	// 5 pre + 1 anchor + 5 post = 11.
	if result.EventsChecked != 11 {
		t.Errorf("EventsChecked = %d, want 11", result.EventsChecked)
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
		t.Fatalf("RotateAuditSigningKey: %v", err)
	}

	// Tamper: rewrite the anchor's signature. This simulates an attacker
	// who fabricates an anchor without access to the signing key.
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE audit_events
		SET signature = '00000000000000000000000000000000000000000000000000000000deadbeef'
		WHERE project_id = $1 AND is_anchor = TRUE
	`, projectID); err != nil {
		t.Fatalf("tamper anchor signature: %v", err)
	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if result.Valid {
		t.Fatal("expected chain invalid after anchor signature tamper")
	}
	if result.BrokenAtID == "" {
		t.Error("BrokenAtID is empty — expected anchor id")
	}
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
		wg     sync.WaitGroup
		mu     sync.Mutex
		epochs []int
		errs   []error
	)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e, err := q.RotateAuditSigningKey(ctx, projectID, "actor-contention")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			epochs = append(epochs, e)
		}(i)
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("rotation errors: %v", errs)
	}
	if len(epochs) != 2 {
		t.Fatalf("got %d epochs, want 2", len(epochs))
	}
	if epochs[0] == epochs[1] {
		t.Errorf("duplicate epochs %v — serialization failed", epochs)
	}

	// Chain must still verify.
	result, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !result.Valid {
		t.Fatalf("chain invalid after concurrent rotations: %s", result.Error)
	}
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
		wg     sync.WaitGroup
		mu     sync.Mutex
		epochs = make([]int, 0, n)
		errs   = make([]error, 0)
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e, err := q.RotateAuditSigningKey(ctx, projectID, "actor-heavy")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			epochs = append(epochs, e)
		}()
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("rotation errors leaked: %v", errs)
	}
	if len(epochs) != n {
		t.Fatalf("got %d epochs, want %d", len(epochs), n)
	}
	seen := make(map[int]struct{}, n)
	minEpoch, maxEpoch := epochs[0], epochs[0]
	for _, e := range epochs {
		if _, dup := seen[e]; dup {
			t.Errorf("duplicate epoch %d", e)
		}
		seen[e] = struct{}{}
		if e < minEpoch {
			minEpoch = e
		}
		if e > maxEpoch {
			maxEpoch = e
		}
	}
	if minEpoch != 1 || maxEpoch != n {
		t.Errorf("epoch range = [%d,%d], want [1,%d]", minEpoch, maxEpoch, n)
	}

	// Verify the chain is still intact — every rotation's anchor verifies
	// under its own epoch's stored key.
	result, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !result.Valid {
		t.Fatalf("chain invalid after %d rotations: %s (broken at %s)", n, result.Error, result.BrokenAtID)
	}
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
		t.Fatalf("rotate: %v", err)
	}

	// Emit post-rotation events under epoch 1. CreateAuditEvent signs
	// with q.auditSigningKey (still the global key here) — but the
	// verify path looks up epoch 1's stored key. For the post-rotation
	// events to verify, we need to emit them signed under the epoch-1
	// key. Do that by fetching the stored key and switching it in.
	epochKey, err := q.GetAuditSigningKey(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("GetAuditSigningKey: %v", err)
	}
	if epochKey == nil {
		t.Fatal("expected stored epoch-1 key, got nil")
	}
	q.SetAuditSigningKey(epochKey)

	for i := 0; i < 3; i++ {
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
		if err := q.CreateAuditEvent(ctx, ev); err != nil {
			t.Fatalf("CreateAuditEvent post %d: %v", i, err)
		}
	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !result.Valid {
		t.Fatalf("chain invalid: %s", result.Error)
	}
	if result.EventsChecked != 7 {
		t.Errorf("EventsChecked = %d, want 7", result.EventsChecked)
	}
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
		t.Fatalf("rotate: %v", err)
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
		t.Fatalf("overwrite key_material: %v", err)
	}

	result, verr := q.VerifyAuditChain(ctx, projectID)
	// Either the decrypt fails (returned as error) or the HMAC check
	// fails (returned with result.Valid=false). Both are acceptable
	// negative outcomes — they prove the verifier actually consults the
	// per-epoch stored key.
	if verr == nil && result.Valid {
		t.Fatal("expected verification failure after corrupting epoch-1 key")
	}
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

	for i := 0; i < 2; i++ {
		if _, err := q.RotateAuditSigningKey(ctx, projectID, "actor"); err != nil {
			t.Fatalf("rotate %d: %v", i, err)
		}
	}

	k1, err := q.GetAuditSigningKey(ctx, projectID, 1)
	if err != nil || k1 == nil {
		t.Fatalf("epoch 1 key: %v / nil=%v", err, k1 == nil)
	}
	k2, err := q.GetAuditSigningKey(ctx, projectID, 2)
	if err != nil || k2 == nil {
		t.Fatalf("epoch 2 key: %v / nil=%v", err, k2 == nil)
	}
	if len(k1) != 32 || len(k2) != 32 {
		t.Errorf("key lengths = %d,%d, want 32,32", len(k1), len(k2))
	}
	same := true
	for i := range k1 {
		if k1[i] != k2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("epoch 1 and epoch 2 keys are identical")
	}

	// Expected rows: 3.
	//   - epoch 0: bootstrapped by the first insertTestChain CreateAuditEvent
	//     call via resolveSigningKeyForEpoch (caveat-1 fix — signer and
	//     verifier must agree on a stable per-epoch key even before any
	//     rotation has occurred).
	//   - epoch 1: written explicitly by the first RotateAuditSigningKey.
	//   - epoch 2: written explicitly by the second RotateAuditSigningKey.
	var count int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_signing_keys WHERE project_id = $1
	`, projectID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("audit_signing_keys rows = %d, want 3 (epoch 0 bootstrap + 2 rotations)", count)
	}
}
