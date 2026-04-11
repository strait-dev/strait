package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
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
			if err == nil {
				t.Errorf("empty secret should error")
			}
			return
		}
		if err != nil {
			t.Errorf("derive failed for non-empty secret: %v", err)
			return
		}
		if len(key) != 32 {
			t.Errorf("key length = %d, want 32", len(key))
		}

		// Determinism: same input produces same key.
		key2, _ := store.DeriveAuditSigningKey(secret)
		if string(key) != string(key2) {
			t.Error("derive is not deterministic")
		}
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
		if len(sig) != 64 {
			t.Errorf("sig length = %d, want 64", len(sig))
		}

		sig2 := store.ComputeAuditSignature(ev, key)
		if sig != sig2 {
			t.Error("sig is not deterministic for identical input")
		}
	})
}
