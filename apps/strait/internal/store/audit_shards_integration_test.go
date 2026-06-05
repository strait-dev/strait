//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, q.CreateAuditEvent(ctx, jobA))

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
	require.NoError(t, q.CreateAuditEvent(ctx, jobB))

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
	require.NoError(t, q.CreateAuditEvent(ctx, wfA))

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
	require.NoError(t, q.CreateAuditEvent(ctx, wfB))
	assert.Equal(t, store.ZeroHash,

		jobA.
			PreviousHash,
	)
	assert.Equal(t, store.ZeroHash,

		wfA.
			PreviousHash,
	)
	assert.Equal(t, jobA.Signature,

		jobB.
			PreviousHash,
	)
	assert.Equal(t, wfA.Signature,

		wfB.
			PreviousHash,
	)

	// Each shard's first row must start from ZeroHash. If the tail read
	// were not shard-scoped, the second shard's first event would chain
	// from the first shard's tail instead.

	// Within a shard, the second event must chain from the first.

	// All shard rows must be v4 because the canonical form binds shard_id.
	for _, ev := range map[string]*domain.AuditEvent{"jobA": jobA, "jobB": jobB, "wfA": wfA, "wfB": wfB} {
		assert.GreaterOrEqual(t,

			ev.SchemaVersion,
			uint16(4),
		)

	}

	// VerifyAuditChain must walk each shard as its own sub-chain and
	// report a clean Valid=true.
	result, verr := q.VerifyAuditChain(ctx, projectID)
	require.Nil(t, verr)
	require.True(t, result.
		Valid,
	)
	assert.EqualValues(t, 4, result.
		EventsChecked,
	)

}

// TestCreateAuditEvent_AutoDerivesShardFromResourceType asserts that
// CreateAuditEvent auto-derives shard_id from resource_type when the caller
// leaves it blank. Production emitters rely on this path so every resource
// type lands in its own sub-chain without callers having to opt in. Anchor
// rows are exempt because rotation and retention carry an explicit shard_id.
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
	require.NoError(t, q.CreateAuditEvent(ctx, jobEv))
	assert.Equal(t, "job",
		jobEv.
			ShardID,
	)
	assert.GreaterOrEqual(t,

		jobEv.SchemaVersion,

		uint16(4))

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
	require.NoError(t, q.CreateAuditEvent(ctx, wfEv))
	assert.Equal(t, "workflow",

		wfEv.
			ShardID)
	assert.Equal(t, store.ZeroHash,

		wfEv.
			PreviousHash,
	)

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
	require.NoError(t, q.CreateAuditEvent(ctx, jobEv2))
	assert.Equal(t, "job",
		jobEv2.
			ShardID,
	)
	assert.Equal(t, jobEv.Signature,

		jobEv2.PreviousHash,
	)

	result, verr := q.VerifyAuditChain(ctx, projectID)
	require.Nil(t, verr)
	require.True(t, result.
		Valid,
	)
	assert.EqualValues(t, 3, result.
		EventsChecked,
	)

}

// TestCreateAuditEvent_LegacyEmptyShardRowNotPolluted asserts that an
// old empty-shard row does not poison the tail read for new auto-sharded
// writes. The empty shard remains its own sub-chain and the new auto-derived
// shard chains from ZeroHash.
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
		require.Failf(t, "test failure",

			"seed legacy row: %v", err)
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
	require.NoError(t, q.CreateAuditEvent(ctx, jobEv))
	assert.Equal(t, "job",
		jobEv.
			ShardID,
	)
	assert.Equal(t, store.ZeroHash,

		jobEv.
			PreviousHash,
	)

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
	require.NoError(t, q.CreateAuditEvent(ctx, ev))

	// Flip the row's shard_id out-of-band, bypassing CreateAuditEvent so
	// the signature is not recomputed. The verifier must detect the
	// mismatch via the v4 HMAC binding.
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE audit_events SET shard_id = 'workflow' WHERE id = $1
	`, ev.ID); err != nil {
		require.Failf(t, "test failure",

			"forge shard_id: %v", err)
	}

	result, verr := q.VerifyAuditChain(ctx, projectID)
	require.Nil(t, verr)
	require.False(t, result.
		Valid)
	assert.Equal(t, ev.ID,
		result.
			BrokenAtID,
	)

}
