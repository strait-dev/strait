package crypto

import (
	"bytes"
	"errors"
	"testing"
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
	if err != nil {
		t.Fatalf("newEncryptorFromBytes(keyA) error = %v", err)
	}

	plaintext := []byte("rotate-me-please")
	ciphertext, err := encA.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Create rotator with keyB as primary and keyA as old key.
	rotator, err := NewKeyRotator(keyB, keyA)
	if err != nil {
		t.Fatalf("NewKeyRotator() error = %v", err)
	}

	decrypted, err := rotator.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("rotator.Decrypt() error = %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("rotator.Decrypt() = %q, want %q", decrypted, plaintext)
	}
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
	if err != nil {
		t.Fatalf("newEncryptorFromBytes() error = %v", err)
	}

	plaintext := []byte("identical-plaintext")
	seen := make(map[string]struct{}, 100)

	for i := range 100 {
		ct, encErr := enc.Encrypt(plaintext)
		if encErr != nil {
			t.Fatalf("Encrypt() iteration %d error = %v", i, encErr)
		}
		key := string(ct)
		if _, dup := seen[key]; dup {
			t.Fatalf("duplicate ciphertext at iteration %d out of 100", i)
		}
		seen[key] = struct{}{}
	}

	if len(seen) != 100 {
		t.Fatalf("expected 100 unique ciphertexts, got %d", len(seen))
	}
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
	if err != nil {
		t.Fatalf("newEncryptorFromBytes() error = %v", err)
	}

	plaintext := []byte("super-secret-database-password-that-must-not-leak")
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	if bytes.Contains(ciphertext, plaintext) {
		t.Fatal("ciphertext contains plaintext bytes verbatim")
	}

	// Also verify via the string variant.
	ctStr, err := enc.EncryptString(string(plaintext))
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	if bytes.Contains([]byte(ctStr), plaintext) {
		t.Fatal("base64 ciphertext string contains plaintext bytes verbatim")
	}
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
	if err != nil {
		t.Fatalf("newEncryptorFromBytes() error = %v", err)
	}

	// Byte-level round-trip.
	ciphertext, err := enc.Encrypt([]byte{})
	if err != nil {
		t.Fatalf("Encrypt(empty) error = %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt(empty ciphertext) error = %v", err)
	}

	if len(decrypted) != 0 {
		t.Fatalf("Decrypt(empty) = %q, want empty", decrypted)
	}

	// String-level round-trip.
	ctStr, err := enc.EncryptString("")
	if err != nil {
		t.Fatalf("EncryptString(empty) error = %v", err)
	}

	ptStr, err := enc.DecryptString(ctStr)
	if err != nil {
		t.Fatalf("DecryptString(empty ciphertext) error = %v", err)
	}

	if ptStr != "" {
		t.Fatalf("DecryptString(empty) = %q, want empty", ptStr)
	}
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
		if err == nil {
			t.Errorf("newEncryptorFromBytes(len=%d) expected error, got nil", length)
			continue
		}

		if !errors.Is(err, errInvalidKeyLength) {
			t.Errorf("newEncryptorFromBytes(len=%d) error = %v, want %v", length, err, errInvalidKeyLength)
		}
	}
}
