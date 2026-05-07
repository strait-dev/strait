package crypto

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/sourcegraph/conc"
)

func TestNewEncryptor_AcceptsRaw32ByteKey(t *testing.T) {
	t.Parallel()

	key := "0123456789abcdef0123456789abcdef"
	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}
	if enc == nil {
		t.Fatal("NewEncryptor() returned nil encryptor")
	}
}

func TestNewEncryptor_AcceptsHexKey(t *testing.T) {
	t.Parallel()

	raw := "0123456789abcdef0123456789abcdef"
	hexKey := hex.EncodeToString([]byte(raw))

	enc, err := NewEncryptor(hexKey)
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}
	if enc == nil {
		t.Fatal("NewEncryptor() returned nil encryptor")
	}
}

func TestNewEncryptor_InvalidKeyLength(t *testing.T) {
	t.Parallel()

	_, err := NewEncryptor("short")
	if err == nil {
		t.Fatal("NewEncryptor() expected error, got nil")
	}
	if err.Error() != "invalid key length" {
		t.Fatalf("NewEncryptor() error = %q, want %q", err.Error(), "invalid key length")
	}
}

func TestNewEncryptor_InvalidHexKey(t *testing.T) {
	t.Parallel()

	_, err := NewEncryptor(strings.Repeat("z", 64))
	if err == nil {
		t.Fatal("NewEncryptor() expected error, got nil")
	}
	if err.Error() != "invalid key length" {
		t.Fatalf("NewEncryptor() error = %q, want %q", err.Error(), "invalid key length")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	t.Parallel()

	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")
	plaintext := []byte("webhook-secret-value")

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if len(ciphertext) <= 12 {
		t.Fatalf("Encrypt() ciphertext length = %d, want > 12", len(ciphertext))
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("Decrypt(Encrypt(x)) = %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	t.Parallel()

	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")

	ciphertext, err := enc.Encrypt(nil)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if len(decrypted) != 0 {
		t.Fatalf("Decrypt() length = %d, want 0", len(decrypted))
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	t.Parallel()

	encryptorA := mustEncryptor(t, "0123456789abcdef0123456789abcdef")
	encryptorB := mustEncryptor(t, "fedcba9876543210fedcba9876543210")

	ciphertext, err := encryptorA.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	_, err = encryptorB.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("Decrypt() expected error, got nil")
	}
	if err.Error() != "decrypt failed" {
		t.Fatalf("Decrypt() error = %q, want %q", err.Error(), "decrypt failed")
	}
}

func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	t.Parallel()

	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")

	_, err := enc.Decrypt([]byte("short"))
	if err == nil {
		t.Fatal("Decrypt() expected error, got nil")
	}
	if err.Error() != "ciphertext too short" {
		t.Fatalf("Decrypt() error = %q, want %q", err.Error(), "ciphertext too short")
	}
}

func TestEncryptStringDecryptString_RoundTrip(t *testing.T) {
	t.Parallel()

	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")
	plaintext := "x-api-key: abc123"

	ciphertext, err := enc.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}
	if strings.TrimSpace(ciphertext) == "" {
		t.Fatal("EncryptString() returned empty ciphertext")
	}

	decrypted, err := enc.DecryptString(ciphertext)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("DecryptString(EncryptString(x)) = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptor_ConcurrentUse(t *testing.T) {
	t.Parallel()

	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")

	const workers = 32
	const iterations = 50

	var wg conc.WaitGroup
	for i := range workers {
		wg.Go(func() {
			for j := range iterations {
				plaintext := []byte("payload-" + strings.Repeat("x", i+j))
				ciphertext, err := enc.Encrypt(plaintext)
				if err != nil {
					t.Errorf("Encrypt() error = %v", err)
					return
				}

				decrypted, err := enc.Decrypt(ciphertext)
				if err != nil {
					t.Errorf("Decrypt() error = %v", err)
					return
				}

				if !bytes.Equal(decrypted, plaintext) {
					t.Errorf("roundtrip mismatch: got %q want %q", decrypted, plaintext)
					return
				}
			}
		})
	}

	wg.Wait()
}

func TestKeyRotator_EncryptUsesPrimary(t *testing.T) {
	t.Parallel()

	primaryKey := []byte("0123456789abcdef0123456789abcdef")
	oldKey := []byte("fedcba9876543210fedcba9876543210")

	rotator, err := NewKeyRotator(primaryKey, oldKey)
	if err != nil {
		t.Fatalf("NewKeyRotator() error = %v", err)
	}

	plaintext := []byte("key-rotation-payload")
	ciphertext, err := rotator.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	decrypted, err := rotator.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("Decrypt(Encrypt(x)) = %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestKeyRotator_DecryptWithOldKey(t *testing.T) {
	t.Parallel()

	primaryKey := []byte("0123456789abcdef0123456789abcdef")
	oldKey := []byte("fedcba9876543210fedcba9876543210")

	oldEncryptor, err := newEncryptorFromBytes(oldKey)
	if err != nil {
		t.Fatalf("newEncryptorFromBytes() error = %v", err)
	}

	ciphertext, err := oldEncryptor.Encrypt([]byte("legacy-secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	rotator, err := NewKeyRotator(primaryKey, oldKey)
	if err != nil {
		t.Fatalf("NewKeyRotator() error = %v", err)
	}

	decrypted, err := rotator.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if string(decrypted) != "legacy-secret" {
		t.Fatalf("Decrypt() = %q, want %q", string(decrypted), "legacy-secret")
	}
}

func TestNewKeyRotatorFromStrings_DecryptsOldStringKey(t *testing.T) {
	t.Parallel()

	oldEncryptor := mustEncryptor(t, "fedcba9876543210fedcba9876543210")
	ciphertext, err := oldEncryptor.Encrypt([]byte("legacy-secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	rotator, err := NewKeyRotatorFromStrings("0123456789abcdef0123456789abcdef", "fedcba9876543210fedcba9876543210")
	if err != nil {
		t.Fatalf("NewKeyRotatorFromStrings() error = %v", err)
	}
	decrypted, err := rotator.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if string(decrypted) != "legacy-secret" {
		t.Fatalf("Decrypt() = %q, want %q", string(decrypted), "legacy-secret")
	}

	encryptedString, err := rotator.EncryptString("new-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}
	decryptedString, err := rotator.DecryptString(encryptedString)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}
	if decryptedString != "new-secret" {
		t.Fatalf("DecryptString() = %q, want %q", decryptedString, "new-secret")
	}
}

func TestKeyRotator_RotateKeyFlow(t *testing.T) {
	t.Parallel()

	oldPrimary := []byte("0123456789abcdef0123456789abcdef")
	newPrimary := []byte("1234567890abcdef1234567890abcdef")

	rotator, err := NewKeyRotator(oldPrimary)
	if err != nil {
		t.Fatalf("NewKeyRotator() error = %v", err)
	}

	beforeRotation, err := rotator.Encrypt([]byte("before-rotation"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	if err := rotator.RotateKey(newPrimary); err != nil {
		t.Fatalf("RotateKey() error = %v", err)
	}

	afterRotation, err := rotator.Encrypt([]byte("after-rotation"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	plainBefore, err := rotator.Decrypt(beforeRotation)
	if err != nil {
		t.Fatalf("Decrypt(before) error = %v", err)
	}
	if string(plainBefore) != "before-rotation" {
		t.Fatalf("Decrypt(before) = %q, want %q", string(plainBefore), "before-rotation")
	}

	plainAfter, err := rotator.Decrypt(afterRotation)
	if err != nil {
		t.Fatalf("Decrypt(after) error = %v", err)
	}
	if string(plainAfter) != "after-rotation" {
		t.Fatalf("Decrypt(after) = %q, want %q", string(plainAfter), "after-rotation")
	}
}

func mustEncryptor(t *testing.T, key string) *Encryptor {
	t.Helper()

	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}

	return enc
}
