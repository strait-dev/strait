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

// TestCreateAuditEvent_AutoDerivesShardFromResourceType asserts that
// CreateAuditEvent auto-derives shard_id from resource_type when the caller
// leaves it blank. This is the production emit path post-Phase-4c: every
// resource type lands in its own sub-chain without callers having to opt in.
// Anchor rows are exempt (rotation / retention carry explicit shard_id).
func TestCreateAuditEvent_AutoDerivesShardFromResourceType(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("shard-autoderive-secret")
	q.SetAuditSigningKey(key)

	const projectID = "proj-shard-autoderive"

	jobEv := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "j-1",
		Details:      json.RawMessage(`{}`),
	}
	if err := q.CreateAuditEvent(ctx, jobEv); err != nil {
		t.Fatalf("CreateAuditEvent jobEv: %v", err)
	}
	if jobEv.ShardID != "job" {
		t.Errorf("jobEv.ShardID = %q, want %q (auto-derived from resource_type)", jobEv.ShardID, "job")
	}
	if jobEv.SchemaVersion < 4 {
		t.Errorf("jobEv.SchemaVersion = %d, want >= 4 (shard-aware writes force v4)", jobEv.SchemaVersion)
	}

	// A second event of a different resource_type lands in its own shard
	// and chains from ZeroHash, not from the prior jobEv.
	wfEv := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "workflow",
		ResourceID:   "w-1",
		Details:      json.RawMessage(`{}`),
	}
	if err := q.CreateAuditEvent(ctx, wfEv); err != nil {
		t.Fatalf("CreateAuditEvent wfEv: %v", err)
	}
	if wfEv.ShardID != "workflow" {
		t.Errorf("wfEv.ShardID = %q, want %q (auto-derived from resource_type)", wfEv.ShardID, "workflow")
	}
	if wfEv.PreviousHash != store.ZeroHash {
		t.Errorf("wfEv.PreviousHash = %s, want ZeroHash (different shard must not chain from job tail)", wfEv.PreviousHash)
	}

	// A subsequent job event chains from the job shard tail, not the
	// workflow row that landed in between.
	jobEv2 := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "j-2",
		Details:      json.RawMessage(`{}`),
	}
	if err := q.CreateAuditEvent(ctx, jobEv2); err != nil {
		t.Fatalf("CreateAuditEvent jobEv2: %v", err)
	}
	if jobEv2.ShardID != "job" {
		t.Errorf("jobEv2.ShardID = %q, want %q", jobEv2.ShardID, "job")
	}
	if jobEv2.PreviousHash != jobEv.Signature {
		t.Errorf("jobEv2.PreviousHash = %s, want jobEv.Signature = %s (shard tail must skip workflow row)", jobEv2.PreviousHash, jobEv.Signature)
	}

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

// TestCreateAuditEvent_LegacyEmptyShardRowNotPolluted asserts that a
// pre-Phase-4 legacy row (seeded with shard_id = '' directly, simulating
// rows that existed before the shard column landed) does not poison the
// tail read for new auto-sharded writes. The empty shard remains its own
// sub-chain and the new auto-derived shard chains from ZeroHash.
func TestCreateAuditEvent_LegacyEmptyShardRowNotPolluted(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("shard-legacy-isolation-secret")
	q.SetAuditSigningKey(key)

	const projectID = "proj-shard-legacy-isolation"

	// Seed a legacy row directly with shard_id = '' to simulate a row
	// that existed before the shard column was introduced. We bypass
	// CreateAuditEvent's auto-derivation by inserting raw — the row carries
	// an empty signature, which the shard-scoped tail read filters out
	// via signature != '' anyway, so it cannot serve as a chain anchor.
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO audit_events (
			id, project_id, actor_id, actor_type, action,
			resource_type, resource_id, details, signature, previous_hash,
			created_at, schema_version, is_anchor, rotation_epoch, shard_id
		) VALUES (
			'legacy-1', $1, 'actor', 'user', 'test.legacy',
			'job', 'r-1', '{}', '', '',
			NOW() - INTERVAL '1 minute', 3, FALSE, 0, ''
		)
	`, projectID); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	// Auto-derived shard write: ResourceType "job" → shard "job". Must
	// start from ZeroHash because the only project row lives in shard '',
	// not in shard "job".
	jobEv := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "actor",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "j-1",
		Details:      json.RawMessage(`{}`),
	}
	if err := q.CreateAuditEvent(ctx, jobEv); err != nil {
		t.Fatalf("CreateAuditEvent jobEv: %v", err)
	}
	if jobEv.ShardID != "job" {
		t.Errorf("jobEv.ShardID = %q, want %q", jobEv.ShardID, "job")
	}
	if jobEv.PreviousHash != store.ZeroHash {
		t.Errorf("jobEv.PreviousHash = %s, want ZeroHash (shard 'job' must not chain from legacy '' row)", jobEv.PreviousHash)
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
