//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// TestCreateAuditEvent_ShardChainsAreIndependent asserts that two shards in
// the same project form independent sub-chains: each shard starts from
// ZeroHash, each row's previous_hash chains only against the prior tail of
// the same shard, and a row in shard A cannot serve as the previous-hash
// source for a row in shard B.
func TestCreateAuditEvent_ShardChainsAreIndependent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("shard-independence-secret")
	q.SetAuditSigningKey(key)

	const projectID = "proj-shards"

	// Write two events into shard "job".
	jobA := &domain.AuditEvent{
		ProjectID:    projectID,
		ShardID:      "job",
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "j-1",
		Details:      json.RawMessage(`{"i":0}`),
	}
	if err := q.CreateAuditEvent(ctx, jobA); err != nil {
		t.Fatalf("CreateAuditEvent jobA: %v", err)
	}
	jobB := &domain.AuditEvent{
		ProjectID:    projectID,
		ShardID:      "job",
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "j-2",
		Details:      json.RawMessage(`{"i":1}`),
	}
	if err := q.CreateAuditEvent(ctx, jobB); err != nil {
		t.Fatalf("CreateAuditEvent jobB: %v", err)
	}

	// Write two events into shard "workflow".
	wfA := &domain.AuditEvent{
		ProjectID:    projectID,
		ShardID:      "workflow",
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "workflow",
		ResourceID:   "w-1",
		Details:      json.RawMessage(`{"i":0}`),
	}
	if err := q.CreateAuditEvent(ctx, wfA); err != nil {
		t.Fatalf("CreateAuditEvent wfA: %v", err)
	}
	wfB := &domain.AuditEvent{
		ProjectID:    projectID,
		ShardID:      "workflow",
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "workflow",
		ResourceID:   "w-2",
		Details:      json.RawMessage(`{"i":1}`),
	}
	if err := q.CreateAuditEvent(ctx, wfB); err != nil {
		t.Fatalf("CreateAuditEvent wfB: %v", err)
	}

	// Each shard's first row must start from ZeroHash. If the tail read
	// were not shard-scoped, the second shard's first event would chain
	// from the first shard's tail instead.
	if jobA.PreviousHash != store.ZeroHash {
		t.Errorf("jobA.PreviousHash = %s, want ZeroHash", jobA.PreviousHash)
	}
	if wfA.PreviousHash != store.ZeroHash {
		t.Errorf("wfA.PreviousHash = %s, want ZeroHash (shards must be independent)", wfA.PreviousHash)
	}

	// Within a shard, the second event must chain from the first.
	if jobB.PreviousHash != jobA.Signature {
		t.Errorf("jobB.PreviousHash = %s, want jobA.Signature = %s", jobB.PreviousHash, jobA.Signature)
	}
	if wfB.PreviousHash != wfA.Signature {
		t.Errorf("wfB.PreviousHash = %s, want wfA.Signature = %s", wfB.PreviousHash, wfA.Signature)
	}

	// All shard rows must be v4 because the canonical form binds shard_id.
	for name, ev := range map[string]*domain.AuditEvent{"jobA": jobA, "jobB": jobB, "wfA": wfA, "wfB": wfB} {
		if ev.SchemaVersion < 4 {
			t.Errorf("%s.SchemaVersion = %d, want >= 4 (shard-aware writes force v4)", name, ev.SchemaVersion)
		}
	}

	// VerifyAuditChain must walk each shard as its own sub-chain and
	// report a clean Valid=true.
	result, verr := q.VerifyAuditChain(ctx, projectID)
	if verr != nil {
		t.Fatalf("VerifyAuditChain: %v", verr)
	}
	if !result.Valid {
		t.Fatalf("VerifyAuditChain invalid: %s (broken at %s)", result.Error, result.BrokenAtID)
	}
	if result.EventsChecked != 4 {
		t.Errorf("EventsChecked = %d, want 4", result.EventsChecked)
	}
}

// TestCreateAuditEvent_ShardDoesNotPolluteLegacyChain asserts that a row
// written under a non-empty shard_id does not appear as the tail for the
// next legacy ('') write — and vice versa. The tail-read filter must be
// shard-scoped on both paths.
func TestCreateAuditEvent_ShardDoesNotPolluteLegacyChain(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("shard-pollution-secret")
	q.SetAuditSigningKey(key)

	const projectID = "proj-shard-legacy"

	// Write into shard "job" first.
	shardEv := &domain.AuditEvent{
		ProjectID:    projectID,
		ShardID:      "job",
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "j-1",
		Details:      json.RawMessage(`{}`),
	}
	if err := q.CreateAuditEvent(ctx, shardEv); err != nil {
		t.Fatalf("CreateAuditEvent shardEv: %v", err)
	}

	// Now write a legacy row (no ShardID). It must start from ZeroHash
	// because the only existing tail in the project lives in a different
	// shard and the tail-read filter excludes it.
	legacyEv := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "legacy",
		Details:      json.RawMessage(`{}`),
	}
	if err := q.CreateAuditEvent(ctx, legacyEv); err != nil {
		t.Fatalf("CreateAuditEvent legacyEv: %v", err)
	}
	if legacyEv.PreviousHash != store.ZeroHash {
		t.Errorf("legacyEv.PreviousHash = %s, want ZeroHash (legacy chain must not chain from a shard row)", legacyEv.PreviousHash)
	}
	if legacyEv.ShardID != "" {
		t.Errorf("legacyEv.ShardID = %q, want '' (legacy writers must not be retagged)", legacyEv.ShardID)
	}

	// Another shard write must chain from the prior shard tail, NOT from
	// the legacy row that landed in between.
	shardEv2 := &domain.AuditEvent{
		ProjectID:    projectID,
		ShardID:      "job",
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "j-2",
		Details:      json.RawMessage(`{}`),
	}
	if err := q.CreateAuditEvent(ctx, shardEv2); err != nil {
		t.Fatalf("CreateAuditEvent shardEv2: %v", err)
	}
	if shardEv2.PreviousHash != shardEv.Signature {
		t.Errorf("shardEv2.PreviousHash = %s, want shardEv.Signature = %s (shard chain must not chain from legacy row)", shardEv2.PreviousHash, shardEv.Signature)
	}

	// Verifier accepts the mixed-shard chain.
	result, verr := q.VerifyAuditChain(ctx, projectID)
	if verr != nil {
		t.Fatalf("VerifyAuditChain: %v", verr)
	}
	if !result.Valid {
		t.Fatalf("VerifyAuditChain invalid: %s", result.Error)
	}
	if result.EventsChecked != 3 {
		t.Errorf("EventsChecked = %d, want 3", result.EventsChecked)
	}
}

// TestVerifyAuditChain_DetectsCrossShardForgery asserts that flipping a
// row's shard_id at the storage layer breaks signature verification. The
// v4 canonical form binds shard_id into the HMAC, so a row that claims to
// belong to a different shard than it was signed under fails the signature
// check, not just the linkage check.
func TestVerifyAuditChain_DetectsCrossShardForgery(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("shard-forgery-secret")
	q.SetAuditSigningKey(key)

	const projectID = "proj-shard-forgery"

	ev := &domain.AuditEvent{
		ProjectID:    projectID,
		ShardID:      "job",
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "j-1",
		Details:      json.RawMessage(`{}`),
	}
	if err := q.CreateAuditEvent(ctx, ev); err != nil {
		t.Fatalf("CreateAuditEvent: %v", err)
	}

	// Flip the row's shard_id out-of-band, bypassing CreateAuditEvent so
	// the signature is not recomputed. The verifier must detect the
	// mismatch via the v4 HMAC binding.
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE audit_events SET shard_id = 'workflow' WHERE id = $1
	`, ev.ID); err != nil {
		t.Fatalf("forge shard_id: %v", err)
	}

	result, verr := q.VerifyAuditChain(ctx, projectID)
	if verr != nil {
		t.Fatalf("VerifyAuditChain: %v", verr)
	}
	if result.Valid {
		t.Fatal("VerifyAuditChain returned Valid=true for a row whose shard_id was forged out-of-band")
	}
	if result.BrokenAtID != ev.ID {
		t.Errorf("BrokenAtID = %s, want %s", result.BrokenAtID, ev.ID)
	}
}
