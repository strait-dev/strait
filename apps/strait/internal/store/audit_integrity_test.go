package store

import (
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAuditEvent(id, projectID, action string) *domain.AuditEvent {
	return &domain.AuditEvent{
		ID:           id,
		ProjectID:    projectID,
		ActorID:      "actor-1",
		ActorType:    "api_key",
		Action:       action,
		ResourceType: "job",
		ResourceID:   "job-1",
		Details:      json.RawMessage(`{"key":"value"}`),
		PreviousHash: ZeroHash,
		CreatedAt:    time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
	}
}

func TestDeriveAuditSigningKey_Returns32Bytes(t *testing.T) {
	t.Parallel()

	key, err := DeriveAuditSigningKey("test-internal-secret")
	require.NoError(t, err)
	assert.Len(t, key, 32)
}

func TestDeriveAuditSigningKey_Deterministic(t *testing.T) {
	t.Parallel()

	key1, _ := DeriveAuditSigningKey("same-secret")
	key2, _ := DeriveAuditSigningKey("same-secret")
	assert.Equal(t, string(key2), string(key1))
}

func TestDeriveAuditSigningKey_EmptySecret_Error(t *testing.T) {
	t.Parallel()

	_, err := DeriveAuditSigningKey("")
	require.Error(t, err)
}

func TestComputeAuditSignature_Deterministic(t *testing.T) {
	t.Parallel()

	key, _ := DeriveAuditSigningKey("test-secret-value")
	ev := testAuditEvent("ev-1", "proj-1", "create")

	sig1 := ComputeAuditSignature(ev, key)
	sig2 := ComputeAuditSignature(ev, key)
	assert.Equal(t, sig2,
		sig1)
	assert.Len(t, sig1, 64)
}

func BenchmarkComputeAuditSignature(b *testing.B) {
	key, err := DeriveAuditSigningKey("test-secret-value")
	require.NoError(b, err)
	ev := testAuditEvent("ev-1", "proj-1", "trigger")
	ev.SchemaVersion = domain.AuditEventSchemaVersionCurrent
	ev.RemoteIP = "198.51.100.42"
	ev.UserAgent = "strait-benchmark/1.0"
	ev.RequestID = "req-0123456789abcdef"
	ev.TraceID = "trace-0123456789abcdef"
	ev.IsAnchor = false
	ev.RotationEpoch = 7
	ev.ShardID = "job"

	b.ReportAllocs()
	for b.Loop() {
		sig := ComputeAuditSignature(ev, key)
		if len(sig) != 64 {
			b.Fatalf("ComputeAuditSignature() length = %d, want 64", len(sig))
		}
	}
}

func TestComputeAuditSignature_ChangesWithFields(t *testing.T) {
	t.Parallel()

	key, _ := DeriveAuditSigningKey("test-secret-value")
	base := testAuditEvent("ev-1", "proj-1", "create")
	baseSig := ComputeAuditSignature(base, key)

	tests := []struct {
		name   string
		modify func(ev *domain.AuditEvent)
	}{
		{"different ID", func(ev *domain.AuditEvent) { ev.ID = "ev-2" }},
		{"different project", func(ev *domain.AuditEvent) { ev.ProjectID = "proj-2" }},
		{"different actor", func(ev *domain.AuditEvent) { ev.ActorID = "actor-2" }},
		{"different action", func(ev *domain.AuditEvent) { ev.Action = "delete" }},
		{"different resource", func(ev *domain.AuditEvent) { ev.ResourceID = "job-2" }},
		{"different details", func(ev *domain.AuditEvent) { ev.Details = json.RawMessage(`{"other":"val"}`) }},
		{"different timestamp", func(ev *domain.AuditEvent) { ev.CreatedAt = ev.CreatedAt.Add(time.Second) }},
		{"different previous_hash", func(ev *domain.AuditEvent) { ev.PreviousHash = "abcd" }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			modified := testAuditEvent("ev-1", "proj-1", "create")
			tc.modify(modified)
			modSig := ComputeAuditSignature(modified, key)
			assert.NotEqual(t, baseSig,
				modSig,
			)
		})
	}
}

func TestComputeAuditSignatureV3_LengthDelimitsPipeFields(t *testing.T) {
	t.Parallel()

	key, _ := DeriveAuditSigningKey("test-secret-value")
	left := testAuditEvent("ev-1", "proj-1", "create")
	left.SchemaVersion = domain.AuditEventSchemaVersionCurrent
	left.ActorID = "actor|api"
	left.ActorType = "key"

	right := testAuditEvent("ev-1", "proj-1", "create")
	right.SchemaVersion = domain.AuditEventSchemaVersionCurrent
	right.ActorID = "actor"
	right.ActorType = "api|key"

	if sigLeft, sigRight := ComputeAuditSignature(left, key), ComputeAuditSignature(right, key); sigLeft == sigRight {
		require.Fail(t,

			"v3 audit signatures collided for pipe-ambiguous adjacent fields")
	}
}

func TestComputeAuditSignatureLegacyVersions_LengthDelimitPipeFields(t *testing.T) {
	t.Parallel()

	key, _ := DeriveAuditSigningKey("test-secret-value")
	for _, version := range []uint16{0, 1, 2} {
		left := testAuditEvent("ev-1", "proj-1", "create")
		left.SchemaVersion = version
		left.ActorID = "actor|api"
		left.ActorType = "key"

		right := testAuditEvent("ev-1", "proj-1", "create")
		right.SchemaVersion = version
		right.ActorID = "actor"
		right.ActorType = "api|key"

		if sigLeft, sigRight := ComputeAuditSignature(left, key), ComputeAuditSignature(right, key); sigLeft == sigRight {
			require.Failf(t, "test failure",

				"schema version %d audit signatures collided for pipe-ambiguous adjacent fields", version)
		}
	}
}

func TestKeyForEpoch_RejectsMissingNonzeroEpochKey(t *testing.T) {
	t.Parallel()

	key, _ := DeriveAuditSigningKey("test-secret-value")
	q := &Queries{auditSigningKey: key}

	if _, err := q.keyForEpoch(map[int][]byte{1: nil}, 1); err == nil {
		require.Fail(t,

			"expected missing nonzero epoch key to be rejected")
	}
	if got, err := q.keyForEpoch(map[int][]byte{0: nil}, 0); err != nil || string(got) != string(key) {
		require.Failf(t, "test failure",

			"epoch zero fallback = (%x, %v), want legacy key", got, err)
	}
}

func TestComputeAuditSignatureV3_BindsAnchorAndRotationEpoch(t *testing.T) {
	t.Parallel()

	key, _ := DeriveAuditSigningKey("test-secret-value")
	base := testAuditEvent("ev-1", "proj-1", "create")
	base.SchemaVersion = domain.AuditEventSchemaVersionCurrent
	base.RotationEpoch = 7
	baseSig := ComputeAuditSignature(base, key)

	anchorChanged := *base
	anchorChanged.IsAnchor = true
	require.NotEqual(t, baseSig,
		ComputeAuditSignature(&anchorChanged,
			key,
		))

	epochChanged := *base
	epochChanged.RotationEpoch = 8
	require.NotEqual(t, baseSig,
		ComputeAuditSignature(&epochChanged, key),
	)
}

func TestComputeAuditSignatureV4_BindsShardID(t *testing.T) {
	t.Parallel()

	key, _ := DeriveAuditSigningKey("test-secret-value")
	base := testAuditEvent("ev-1", "proj-1", "create")
	base.SchemaVersion = 4
	base.RotationEpoch = 0
	base.ShardID = "job"
	baseSig := ComputeAuditSignature(base, key)

	shardChanged := *base
	shardChanged.ShardID = "workflow"
	require.NotEqual(t, baseSig,
		ComputeAuditSignature(&shardChanged, key),
	)

	// Empty shard_id under v4 must not collide with a v3 row that has the
	// same content but no shard binding. The version prefix in the
	// canonical form differentiates them ('audit:v3' vs 'audit:v4').
	v3 := *base
	v3.SchemaVersion = 3
	v3.ShardID = ""
	require.NotEqual(t, ComputeAuditSignature(base, key), ComputeAuditSignature(&v3, key))
}

func TestComputeAuditSignatureV4_LengthDelimitsShardID(t *testing.T) {
	t.Parallel()

	key, _ := DeriveAuditSigningKey("test-secret-value")
	left := testAuditEvent("ev-1", "proj-1", "create")
	left.SchemaVersion = 4
	left.RotationEpoch = 0
	left.ShardID = "job|workflow"

	right := testAuditEvent("ev-1", "proj-1", "create")
	right.SchemaVersion = 4
	right.RotationEpoch = 0
	right.ShardID = "job"
	right.ActorID += "|workflow"

	if sigLeft, sigRight := ComputeAuditSignature(left, key), ComputeAuditSignature(right, key); sigLeft == sigRight {
		require.Fail(t,

			"v4 audit signatures collided across pipe-ambiguous shard_id boundary")
	}
}

func TestComputeAuditSignature_DifferentKeys(t *testing.T) {
	t.Parallel()

	key1, _ := DeriveAuditSigningKey("secret-one")
	key2, _ := DeriveAuditSigningKey("secret-two")
	ev := testAuditEvent("ev-1", "proj-1", "create")

	sig1 := ComputeAuditSignature(ev, key1)
	sig2 := ComputeAuditSignature(ev, key2)
	assert.NotEqual(t, sig2,
		sig1)
}

func TestAuditChain_ManualVerification(t *testing.T) {
	t.Parallel()

	key, _ := DeriveAuditSigningKey("chain-test-secret")

	// Build a 3-event chain manually.
	ev1 := testAuditEvent("ev-1", "proj-1", "create")
	ev1.PreviousHash = ZeroHash
	ev1.Signature = ComputeAuditSignature(ev1, key)

	ev2 := testAuditEvent("ev-2", "proj-1", "update")
	ev2.PreviousHash = ev1.Signature
	ev2.CreatedAt = ev1.CreatedAt.Add(time.Second)
	ev2.Signature = ComputeAuditSignature(ev2, key)

	ev3 := testAuditEvent("ev-3", "proj-1", "delete")
	ev3.PreviousHash = ev2.Signature
	ev3.CreatedAt = ev2.CreatedAt.Add(time.Second)
	ev3.Signature = ComputeAuditSignature(ev3, key)

	// Verify chain integrity.
	chain := []*domain.AuditEvent{ev1, ev2, ev3}
	expectedPrev := ZeroHash
	for _, ev := range chain {
		assert.Equal(t, expectedPrev,
			ev.
				PreviousHash,
		)

		recomputed := ComputeAuditSignature(ev, key)
		assert.Equal(t, recomputed,
			ev.Signature,
		)

		expectedPrev = ev.Signature
	}
}

func TestAuditChain_Adversarial_TamperedEvent(t *testing.T) {
	t.Parallel()

	key, _ := DeriveAuditSigningKey("tamper-test-secret")

	ev := testAuditEvent("ev-1", "proj-1", "create")
	ev.PreviousHash = ZeroHash
	ev.Signature = ComputeAuditSignature(ev, key)

	// Tamper with the event after signing.
	ev.Action = "delete"

	recomputed := ComputeAuditSignature(ev, key)
	assert.NotEqual(t, recomputed,
		ev.
			Signature,
	)
}

func TestAuditChain_Adversarial_BrokenChain(t *testing.T) {
	t.Parallel()

	key, _ := DeriveAuditSigningKey("broken-chain-secret")

	ev1 := testAuditEvent("ev-1", "proj-1", "create")
	ev1.PreviousHash = ZeroHash
	ev1.Signature = ComputeAuditSignature(ev1, key)

	ev2 := testAuditEvent("ev-2", "proj-1", "update")
	ev2.PreviousHash = "wrong-hash-simulating-deleted-event"
	ev2.CreatedAt = ev1.CreatedAt.Add(time.Second)
	ev2.Signature = ComputeAuditSignature(ev2, key)
	assert.NotEqual(t, ev1.
		Signature,
		ev2.PreviousHash,
	)

	// Chain verification: ev2's previous_hash should be ev1's signature.
}

func FuzzComputeAuditSignature(f *testing.F) {
	f.Add("ev-1", "proj-1", "actor-1", "create", "job", "job-1", `{"k":"v"}`, "prev-hash")
	f.Add("", "", "", "", "", "", "{}", "")
	f.Add("a", "b", "c", "d", "e", "f", `null`, "0000")

	f.Fuzz(func(t *testing.T, id, projectID, actorID, action, resourceType, resourceID, details, prevHash string) {
		key, err := DeriveAuditSigningKey("fuzz-secret-key-value")
		require.NoError(t, err)

		ev := &domain.AuditEvent{
			ID: id, ProjectID: projectID, ActorID: actorID, ActorType: "api_key",
			Action: action, ResourceType: resourceType, ResourceID: resourceID,
			Details: json.RawMessage(details), PreviousHash: prevHash,
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		}

		sig1 := ComputeAuditSignature(ev, key)
		sig2 := ComputeAuditSignature(ev, key)
		assert.Equal(t, sig2,
			sig1)
		assert.Len(t, sig1, 64)
	})
}
