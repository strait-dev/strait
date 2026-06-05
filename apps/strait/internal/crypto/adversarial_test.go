package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDecrypt_TruncatedCiphertextAdversarial verifies that decryption rejects ciphertext shorter than the nonce size.
func TestDecrypt_TruncatedCiphertextAdversarial(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	enc, err := newEncryptorFromBytes(key)
	require.NoError(t, err)

	// Ciphertext shorter than nonce size (12 bytes for AES-GCM).
	short := make([]byte, enc.aead.NonceSize()-1)
	_, err = enc.Decrypt(short)
	require.ErrorIs(t, err, errCiphertextTooShort)
}

// TestDecrypt_CorruptedNonce verifies that flipping bits in the nonce causes decryption failure.
func TestDecrypt_CorruptedNonce(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	enc, err := newEncryptorFromBytes(key)
	require.NoError(t, err)

	plaintext := []byte("sensitive data for nonce corruption test")
	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	// Flip every bit in the nonce section.
	corrupted := make([]byte, len(ciphertext))
	copy(corrupted, ciphertext)
	for i := range enc.aead.NonceSize() {
		corrupted[i] ^= 0xFF
	}

	_, err = enc.Decrypt(corrupted)
	require.ErrorIs(t, err, errDecryptFailed)
}

// TestDecrypt_WrongKeyAdversarial verifies that decrypting with a different key fails.
func TestDecrypt_WrongKeyAdversarial(t *testing.T) {
	t.Parallel()

	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	_, err := rand.Read(key1)
	require.NoError(t, err)
	_, err = rand.Read(key2)
	require.NoError(t, err)

	enc1, err := newEncryptorFromBytes(key1)
	require.NoError(t, err)

	enc2, err := newEncryptorFromBytes(key2)
	require.NoError(t, err)

	plaintext := []byte("wrong key test payload")
	ciphertext, err := enc1.Encrypt(plaintext)
	require.NoError(t, err)

	_, err = enc2.Decrypt(ciphertext)
	require.ErrorIs(t, err, errDecryptFailed)
}

// TestDecrypt_EmptyCiphertext verifies that empty input is rejected.
func TestDecrypt_EmptyCiphertext(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	enc, err := newEncryptorFromBytes(key)
	require.NoError(t, err)

	_, err = enc.Decrypt([]byte{})
	require.ErrorIs(t, err, errCiphertextTooShort)

	_, err = enc.Decrypt(nil)
	require.Error(t, err)
}

// TestEncrypt_LargePayload verifies that a 10MB plaintext round-trips correctly.
func TestEncrypt_LargePayload(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	enc, err := newEncryptorFromBytes(key)
	require.NoError(t, err)

	// 10MB payload.
	plaintext := make([]byte, 10*1024*1024)
	_, err = rand.Read(plaintext)
	require.NoError(t, err)

	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

// FuzzEncryptDecryptKeyRotation fuzzes encrypt/decrypt through the key rotator with multiple keys.
func FuzzEncryptDecryptKeyRotation(f *testing.F) {
	f.Add([]byte("hello world"), []byte{0x01})
	f.Add([]byte(""), []byte{0xFF})
	f.Add([]byte("a]b[c{d}e"), []byte{0x00, 0x01, 0x02})

	primaryKey := make([]byte, 32)
	oldKey := make([]byte, 32)
	if _, err := rand.Read(primaryKey); err != nil {
		f.Fatalf("generating primary key: %v", err)
	}
	if _, err := rand.Read(oldKey); err != nil {
		f.Fatalf("generating old key: %v", err)
	}

	rotator, err := NewKeyRotator(primaryKey, oldKey)
	if err != nil {
		f.Fatalf("creating key rotator: %v", err)
	}

	f.Fuzz(func(t *testing.T, plaintext []byte, _ []byte) {
		ciphertext, err := rotator.Encrypt(plaintext)
		require.NoError(t, err)

		decrypted, err := rotator.Decrypt(ciphertext)
		require.NoError(t, err)
		require.True(t, bytes.Equal(plaintext, decrypted))
	})
}

// TestEncrypt_AllZeroKey verifies that a key consisting of all zero bytes works correctly.
func TestEncrypt_AllZeroKey(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32) // all zeros
	enc, err := newEncryptorFromBytes(key)
	require.NoError(t, err)

	plaintext := []byte("test with zero key")
	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}
