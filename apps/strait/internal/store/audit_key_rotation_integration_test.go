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

// TestRotateAuditSigningKey_EmitsAnchor asserts that rotation writes a
// single anchor row with action=audit.key_rotated, is_anchor=true, and
// monotonically-increasing rotation_epoch, with the correct details shape.
func TestRotateAuditSigningKey_EmitsAnchor(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
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
