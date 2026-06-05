package api

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeriveAuditSigningKey_DeterministicOutput verifies that the same master
// key always produces the same derived signing key (HKDF is deterministic).
func TestDeriveAuditSigningKey_DeterministicOutput(t *testing.T) {
	t.Parallel()

	masterKey := []byte("test-master-key-32-bytes-abcdefgh")

	key1, err := deriveAuditSigningKey(masterKey)
	require.NoError(t, err)

	key2, err := deriveAuditSigningKey(masterKey)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(key1, key2))
	assert.Len(
		t, key1, 32,
	)
}

// TestDeriveAuditSigningKey_DifferentInputs_DifferentKeys verifies that
// different master keys produce different derived signing keys.
func TestDeriveAuditSigningKey_DifferentInputs_DifferentKeys(t *testing.T) {
	t.Parallel()

	key1, err := deriveAuditSigningKey([]byte("master-key-one"))
	require.NoError(t, err)

	key2, err := deriveAuditSigningKey([]byte("master-key-two"))
	require.NoError(t, err)
	assert.False(t, bytes.
		Equal(key1, key2))
}
