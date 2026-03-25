package crypto

import (
	"bytes"
	cryptorand "crypto/rand"
	mathrand "math/rand/v2"
	"testing"
)

// testEncryptor creates an Encryptor with a random 32-byte key for testing.
func testEncryptor(t *testing.T) *Encryptor {
	t.Helper()
	key := make([]byte, 32)
	if _, err := cryptorand.Read(key); err != nil {
		t.Fatalf("generating random key: %v", err)
	}
	enc, err := newEncryptorFromBytes(key)
	if err != nil {
		t.Fatalf("creating encryptor: %v", err)
	}
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
		if err != nil {
			t.Fatalf("Encrypt failed for len=%d: %v", length, err)
		}

		decrypted, err := enc.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("Decrypt failed for len=%d: %v", length, err)
		}

		if !bytes.Equal(plaintext, decrypted) {
			t.Fatalf("round-trip failed for len=%d: plaintext != decrypted", length)
		}
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
		if err != nil {
			t.Fatalf("Encrypt (1) failed: %v", err)
		}

		ct2, err := enc.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt (2) failed: %v", err)
		}

		if bytes.Equal(ct1, ct2) {
			t.Fatalf("two encryptions of same plaintext (len=%d) produced identical ciphertext", length)
		}
	}
}
