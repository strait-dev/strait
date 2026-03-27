package crypto

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestDecryptString_InvalidBase64(t *testing.T) {
	t.Parallel()
	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")

	_, err := enc.DecryptString("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestDecryptString_ValidBase64ButBadCiphertext(t *testing.T) {
	t.Parallel()
	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")

	// Valid base64 but too short to contain a nonce.
	short := base64.StdEncoding.EncodeToString([]byte("short"))
	_, err := enc.DecryptString(short)
	if err == nil {
		t.Fatal("expected error for truncated ciphertext")
	}
}

func TestDecryptString_ValidBase64WrongKey(t *testing.T) {
	t.Parallel()
	encA := mustEncryptor(t, "0123456789abcdef0123456789abcdef")
	encB := mustEncryptor(t, "fedcba9876543210fedcba9876543210")

	ct, err := encA.EncryptString("secret-value")
	if err != nil {
		t.Fatalf("EncryptString error: %v", err)
	}

	_, err = encB.DecryptString(ct)
	if err == nil {
		t.Fatal("expected decrypt failed error")
	}
}

func TestNewEncryptorFromBytes_TooShort(t *testing.T) {
	t.Parallel()
	_, err := newEncryptorFromBytes([]byte("short"))
	if err == nil {
		t.Fatal("expected error for short key")
	}
	if !errors.Is(err, errInvalidKeyLength) {
		t.Fatalf("error = %v, want errInvalidKeyLength", err)
	}
}

func TestNewEncryptorFromBytes_TooLong(t *testing.T) {
	t.Parallel()
	_, err := newEncryptorFromBytes(make([]byte, 64))
	if err == nil {
		t.Fatal("expected error for 64-byte key")
	}
	if !errors.Is(err, errInvalidKeyLength) {
		t.Fatalf("error = %v, want errInvalidKeyLength", err)
	}
}

func TestNewKeyRotator_InvalidPrimary(t *testing.T) {
	t.Parallel()
	_, err := NewKeyRotator([]byte("bad"))
	if err == nil {
		t.Fatal("expected error for invalid primary key")
	}
}

func TestNewKeyRotator_InvalidOldKey(t *testing.T) {
	t.Parallel()
	good := []byte("0123456789abcdef0123456789abcdef")
	_, err := NewKeyRotator(good, []byte("bad"))
	if err == nil {
		t.Fatal("expected error for invalid old key")
	}
}

func TestKeyRotator_DecryptTooShort(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789abcdef0123456789abcdef")
	rotator, err := NewKeyRotator(key)
	if err != nil {
		t.Fatalf("NewKeyRotator error: %v", err)
	}

	_, err = rotator.Decrypt([]byte("x"))
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
	if !errors.Is(err, errCiphertextTooShort) {
		t.Fatalf("error = %v, want errCiphertextTooShort", err)
	}
}

func TestKeyRotator_DecryptNoMatchingKey(t *testing.T) {
	t.Parallel()
	keyA := []byte("0123456789abcdef0123456789abcdef")
	keyB := []byte("fedcba9876543210fedcba9876543210")
	keyC := []byte("1234567890abcdef1234567890abcdef")

	encC, err := newEncryptorFromBytes(keyC)
	if err != nil {
		t.Fatalf("newEncryptorFromBytes error: %v", err)
	}
	ct, err := encC.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}

	rotator, err := NewKeyRotator(keyA, keyB)
	if err != nil {
		t.Fatalf("NewKeyRotator error: %v", err)
	}

	_, err = rotator.Decrypt(ct)
	if err == nil {
		t.Fatal("expected decrypt failed error")
	}
	if !errors.Is(err, errDecryptFailed) {
		t.Fatalf("error = %v, want errDecryptFailed", err)
	}
}

func TestRotateKey_InvalidKey(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789abcdef0123456789abcdef")
	rotator, err := NewKeyRotator(key)
	if err != nil {
		t.Fatalf("NewKeyRotator error: %v", err)
	}

	err = rotator.RotateKey([]byte("bad"))
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestParseKey_Base64Standard(t *testing.T) {
	t.Parallel()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)

	enc, err := NewEncryptor(encoded)
	if err != nil {
		t.Fatalf("NewEncryptor error: %v", err)
	}
	if enc == nil {
		t.Fatal("expected non-nil encryptor")
	}
}

func TestParseKey_Base64RawStandard(t *testing.T) {
	t.Parallel()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 10)
	}
	encoded := base64.RawStdEncoding.EncodeToString(raw)

	enc, err := NewEncryptor(encoded)
	if err != nil {
		t.Fatalf("NewEncryptor error: %v", err)
	}
	if enc == nil {
		t.Fatal("expected non-nil encryptor")
	}
}

func TestParseKey_64CharsNonHex(t *testing.T) {
	t.Parallel()
	// 64 chars that are not valid hex.
	key := strings.Repeat("zz", 32)
	_, err := NewEncryptor(key)
	if err == nil {
		t.Fatal("expected error for non-hex 64-char key")
	}
}

func TestParseKey_WrongLength(t *testing.T) {
	t.Parallel()
	_, err := NewEncryptor("12345")
	if err == nil {
		t.Fatal("expected error for wrong length key")
	}
}

func TestEncrypt_ProducesUniqueCiphertexts(t *testing.T) {
	t.Parallel()
	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")
	plaintext := []byte("same-input")

	ct1, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	ct2, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}

	if string(ct1) == string(ct2) {
		t.Fatal("two encryptions of the same plaintext should produce different ciphertext")
	}
}
