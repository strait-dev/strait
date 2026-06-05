package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecrets_SurviveKeyRotation verifies that data encrypted with an old key can
// be decrypted by a KeyRotator that has the old key in its fallback list.
func TestSecrets_SurviveKeyRotation(t *testing.T) {
	t.Parallel()

	keyA := make([]byte, 32)
	keyB := make([]byte, 32)
	for i := range keyA {
		keyA[i] = byte(i)
	}
	for i := range keyB {
		keyB[i] = byte(i + 100)
	}

	encA, err := newEncryptorFromBytes(keyA)
	require.NoError(t, err)

	plaintext := []byte("rotate-me-please")
	ciphertext, err := encA.Encrypt(plaintext)
	require.NoError(t, err)

	// Create rotator with keyB as primary and keyA as old key.
	rotator, err := NewKeyRotator(keyB, keyA)
	require.NoError(t, err)

	decrypted, err := rotator.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

// TestSecrets_DifferentNoncePerEncryption verifies that encrypting the same plaintext
// 100 times produces 100 distinct ciphertexts, confirming unique nonces.
func TestSecrets_DifferentNoncePerEncryption(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 50)
	}

	enc, err := newEncryptorFromBytes(key)
	require.NoError(t, err)

	plaintext := []byte("identical-plaintext")
	seen := make(map[string]struct{}, 100)

	for i := range 100 {
		ct, encErr := enc.Encrypt(plaintext)
		require.NoError(t, encErr)

		key := string(ct)
		require.NotContains(t, seen, key, "iteration %d", i)
		seen[key] = struct{}{}
	}
	require.Len(t, seen, 100)
}

// TestSecrets_EncryptedFieldsNeverPlaintext verifies that the ciphertext bytes do not
// contain the plaintext as a substring, confirming actual encryption occurred.
func TestSecrets_EncryptedFieldsNeverPlaintext(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}

	enc, err := newEncryptorFromBytes(key)
	require.NoError(t, err)

	plaintext := []byte("super-secret-database-password-that-must-not-leak")
	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)
	require.NotContains(t, ciphertext, plaintext)

	// Also verify via the string variant.
	ctStr, err := enc.EncryptString(string(plaintext))
	require.NoError(t, err)
	require.NotContains(t, []byte(ctStr), plaintext)
}

// TestSecrets_EmptyPlaintextRoundTrip verifies that encrypting and decrypting an
// empty string produces an empty string without errors.
func TestSecrets_EmptyPlaintextRoundTrip(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 10)
	}

	enc, err := newEncryptorFromBytes(key)
	require.NoError(t, err)

	// Byte-level round-trip.
	ciphertext, err := enc.Encrypt([]byte{})
	require.NoError(t, err)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Empty(t, decrypted)

	// String-level round-trip.
	ctStr, err := enc.EncryptString("")
	require.NoError(t, err)

	ptStr, err := enc.DecryptString(ctStr)
	require.NoError(t, err)
	require.Empty(t, ptStr)
}

// TestSecrets_KeyLengthValidation verifies that creating an encryptor with invalid
// key lengths (16, 24, 48 bytes) returns an error.
func TestSecrets_KeyLengthValidation(t *testing.T) {
	t.Parallel()

	badLengths := []int{16, 24, 48}

	for _, length := range badLengths {
		key := make([]byte, length)
		for i := range key {
			key[i] = byte(i)
		}

		_, err := newEncryptorFromBytes(key)
		require.ErrorIs(t, err, errInvalidKeyLength)
	}
}
