package api

import (
	"bytes"
	"testing"
)

// TestDeriveAuditSigningKey_DeterministicOutput verifies that the same master
// key always produces the same derived signing key (HKDF is deterministic).
func TestDeriveAuditSigningKey_DeterministicOutput(t *testing.T) {
	t.Parallel()

	masterKey := []byte("test-master-key-32-bytes-abcdefgh")

	key1, err := deriveAuditSigningKey(masterKey)
	if err != nil {
		t.Fatalf("first derive failed: %v", err)
	}
	key2, err := deriveAuditSigningKey(masterKey)
	if err != nil {
		t.Fatalf("second derive failed: %v", err)
	}

	if !bytes.Equal(key1, key2) {
		t.Errorf("derived keys differ for same input: %x != %x", key1, key2)
	}
	if len(key1) != 32 {
		t.Errorf("expected 32-byte derived key, got %d bytes", len(key1))
	}
}

// TestDeriveAuditSigningKey_DifferentInputs_DifferentKeys verifies that
// different master keys produce different derived signing keys.
func TestDeriveAuditSigningKey_DifferentInputs_DifferentKeys(t *testing.T) {
	t.Parallel()

	key1, err := deriveAuditSigningKey([]byte("master-key-one"))
	if err != nil {
		t.Fatalf("derive key1 failed: %v", err)
	}
	key2, err := deriveAuditSigningKey([]byte("master-key-two"))
	if err != nil {
		t.Fatalf("derive key2 failed: %v", err)
	}

	if bytes.Equal(key1, key2) {
		t.Error("expected different derived keys for different inputs, got equal keys")
	}
}
