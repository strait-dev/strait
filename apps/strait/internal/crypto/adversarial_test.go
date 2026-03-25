package crypto

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
)

// TestDecrypt_TruncatedCiphertextAdversarial verifies that decryption rejects ciphertext shorter than the nonce size.
func TestDecrypt_TruncatedCiphertextAdversarial(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating key: %v", err)
	}

	enc, err := newEncryptorFromBytes(key)
	if err != nil {
		t.Fatalf("creating encryptor: %v", err)
	}

	// Ciphertext shorter than nonce size (12 bytes for AES-GCM).
	short := make([]byte, enc.aead.NonceSize()-1)
	_, err = enc.Decrypt(short)
	if err == nil {
		t.Fatal("expected error for truncated ciphertext, got nil")
	}
	if !errors.Is(err, errCiphertextTooShort) {
		t.Fatalf("expected errCiphertextTooShort, got: %v", err)
	}
}

// TestDecrypt_CorruptedNonce verifies that flipping bits in the nonce causes decryption failure.
func TestDecrypt_CorruptedNonce(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating key: %v", err)
	}

	enc, err := newEncryptorFromBytes(key)
	if err != nil {
		t.Fatalf("creating encryptor: %v", err)
	}

	plaintext := []byte("sensitive data for nonce corruption test")
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypting: %v", err)
	}

	// Flip every bit in the nonce section.
	corrupted := make([]byte, len(ciphertext))
	copy(corrupted, ciphertext)
	for i := range enc.aead.NonceSize() {
		corrupted[i] ^= 0xFF
	}

	_, err = enc.Decrypt(corrupted)
	if err == nil {
		t.Fatal("expected error for corrupted nonce, got nil")
	}
	if !errors.Is(err, errDecryptFailed) {
		t.Fatalf("expected errDecryptFailed, got: %v", err)
	}
}

// TestDecrypt_WrongKeyAdversarial verifies that decrypting with a different key fails.
func TestDecrypt_WrongKeyAdversarial(t *testing.T) {
	t.Parallel()

	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	if _, err := rand.Read(key1); err != nil {
		t.Fatalf("generating key1: %v", err)
	}
	if _, err := rand.Read(key2); err != nil {
		t.Fatalf("generating key2: %v", err)
	}

	enc1, err := newEncryptorFromBytes(key1)
	if err != nil {
		t.Fatalf("creating encryptor1: %v", err)
	}
	enc2, err := newEncryptorFromBytes(key2)
	if err != nil {
		t.Fatalf("creating encryptor2: %v", err)
	}

	plaintext := []byte("wrong key test payload")
	ciphertext, err := enc1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypting: %v", err)
	}

	_, err = enc2.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key, got nil")
	}
	if !errors.Is(err, errDecryptFailed) {
		t.Fatalf("expected errDecryptFailed, got: %v", err)
	}
}

// TestDecrypt_EmptyCiphertext verifies that empty input is rejected.
func TestDecrypt_EmptyCiphertext(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating key: %v", err)
	}

	enc, err := newEncryptorFromBytes(key)
	if err != nil {
		t.Fatalf("creating encryptor: %v", err)
	}

	_, err = enc.Decrypt([]byte{})
	if err == nil {
		t.Fatal("expected error for empty ciphertext, got nil")
	}
	if !errors.Is(err, errCiphertextTooShort) {
		t.Fatalf("expected errCiphertextTooShort, got: %v", err)
	}

	_, err = enc.Decrypt(nil)
	if err == nil {
		t.Fatal("expected error for nil ciphertext, got nil")
	}
}

// TestEncrypt_LargePayload verifies that a 10MB plaintext round-trips correctly.
func TestEncrypt_LargePayload(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating key: %v", err)
	}

	enc, err := newEncryptorFromBytes(key)
	if err != nil {
		t.Fatalf("creating encryptor: %v", err)
	}

	// 10MB payload.
	plaintext := make([]byte, 10*1024*1024)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatalf("generating plaintext: %v", err)
	}

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypting 10MB payload: %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypting 10MB payload: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatal("decrypted 10MB payload does not match original")
	}
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
		if err != nil {
			t.Fatalf("encrypting: %v", err)
		}

		decrypted, err := rotator.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("decrypting: %v", err)
		}

		if !bytes.Equal(plaintext, decrypted) {
			t.Fatal("round-trip mismatch through key rotator")
		}
	})
}

// TestEncrypt_AllZeroKey verifies that a key consisting of all zero bytes works correctly.
func TestEncrypt_AllZeroKey(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32) // all zeros
	enc, err := newEncryptorFromBytes(key)
	if err != nil {
		t.Fatalf("creating encryptor with all-zero key: %v", err)
	}

	plaintext := []byte("test with zero key")
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypting with all-zero key: %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypting with all-zero key: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatal("round-trip failed with all-zero key")
	}
}
