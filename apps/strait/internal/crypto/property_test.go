package crypto

import (
	"bytes"
	cryptorand "crypto/rand"
	mathrand "math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEncryptor creates an Encryptor with a random 32-byte key for testing.
func testEncryptor(t *testing.T) *Encryptor {
	t.Helper()
	key := make([]byte, 32)
	_, err := cryptorand.Read(key)
	require.NoError(t, err)
	enc, err := newEncryptorFromBytes(key)
	require.NoError(t, err)

	return enc
}

// TestProperty_EncryptionRoundTrip verifies that for any random plaintext,
// decrypting the encrypted output yields the original plaintext.
func TestProperty_EncryptionRoundTrip(t *testing.T) {
	t.Parallel()
	enc := testEncryptor(t)

	for range 2000 {
		length := mathrand.IntN(4096)
		plaintext := make([]byte, length)
		for j := range plaintext {
			plaintext[j] = byte(mathrand.IntN(256))
		}

		ciphertext, err := enc.Encrypt(plaintext)
		require.NoError(t, err)

		decrypted, err := enc.Decrypt(ciphertext)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(plaintext, decrypted))
	}
}

// TestProperty_EncryptionDifferentNonce verifies that encrypting the same
// plaintext twice produces different ciphertexts (due to random nonces).
func TestProperty_EncryptionDifferentNonce(t *testing.T) {
	t.Parallel()
	enc := testEncryptor(t)

	for range 1000 {
		length := mathrand.IntN(1024) + 1
		plaintext := make([]byte, length)
		for j := range plaintext {
			plaintext[j] = byte(mathrand.IntN(256))
		}

		ct1, err := enc.Encrypt(plaintext)
		require.NoError(t, err)

		ct2, err := enc.Encrypt(plaintext)
		require.NoError(t, err)
		require.NotEqual(t, ct1, ct2)
	}
}
