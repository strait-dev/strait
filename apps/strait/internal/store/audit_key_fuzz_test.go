package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzDeriveAuditSigningKey asserts that DeriveAuditSigningKey is total
// for any input secret: empty returns an error, anything else returns a
// 32-byte key deterministically. Never panics.
func FuzzDeriveAuditSigningKey(f *testing.F) {
	seeds := []string{
		"",
		"a",
		"secret",
		"01HXABCXYZ1234567890ABCDEF",
		"very-long-secret-with-unicode-\u00e9\u00e8\u00e7-\u4e2d\u6587",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, secret string) {
		key, err := store.DeriveAuditSigningKey(secret)
		if secret == "" {
			assert.Error(t, err)

			return
		}
		require.NoError(t, err)
		assert.Len(t, key, 32)

		// Determinism: same input produces same key.
		key2, _ := store.DeriveAuditSigningKey(secret)
		assert.Equal(t, string(key2), string(
			key))
	})
}

// FuzzComputeAuditSignature asserts that ComputeAuditSignature never
// panics and always returns a 64-char hex signature. Determinism holds
// across identical inputs.
func FuzzComputeAuditSignature(f *testing.F) {
	f.Add("id", "proj", "actor", "user", "job.created", "job", "rid", "{}")
	f.Add("", "", "", "", "", "", "", "")
	f.Add("id\x00", "proj\n", "actor|", "user", "a.b", "x", "y", `{"k":"v"}`)

	key, _ := store.DeriveAuditSigningKey("fuzz-key")

	f.Fuzz(func(t *testing.T, id, projectID, actorID, actorType, action, rtype, rid, details string) {
		ev := &domain.AuditEvent{
			ID:           id,
			ProjectID:    projectID,
			ActorID:      actorID,
			ActorType:    actorType,
			Action:       action,
			ResourceType: rtype,
			ResourceID:   rid,
			Details:      json.RawMessage(details),
			CreatedAt:    time.Unix(1700000000, 0).UTC(),
			PreviousHash: store.ZeroHash,
		}

		sig := store.ComputeAuditSignature(ev, key)
		assert.Len(t, sig, 64)

		sig2 := store.ComputeAuditSignature(ev, key)
		assert.Equal(t, sig2, sig)
	})
}
