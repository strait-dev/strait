package store

import (
	"crypto/sha256"
	"testing"
)

func TestSecretKey_HKDF_Returns32Bytes(t *testing.T) {
	t.Parallel()

	q := &Queries{secretEncryptionKey: "my-test-encryption-key"}
	key, err := q.secretKey()
	if err != nil {
		t.Fatalf("secretKey error: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("key length = %d, want 32", len(key))
	}
}

func TestSecretKey_HKDF_Deterministic(t *testing.T) {
	t.Parallel()

	q := &Queries{secretEncryptionKey: "deterministic-test-key"}
	key1, err := q.secretKey()
	if err != nil {
		t.Fatalf("secretKey error: %v", err)
	}
	key2, err := q.secretKey()
	if err != nil {
		t.Fatalf("secretKey error: %v", err)
	}
	if string(key1) != string(key2) {
		t.Error("HKDF key derivation is not deterministic")
	}
}

func TestSecretKey_HKDF_DifferentFromSHA256(t *testing.T) {
	t.Parallel()

	passphrase := "compare-derivation-methods"
	q := &Queries{secretEncryptionKey: passphrase}

	hkdfKey, err := q.secretKey()
	if err != nil {
		t.Fatalf("secretKey error: %v", err)
	}

	legacySum := sha256.Sum256([]byte(passphrase))
	legacyKey := legacySum[:]

	if string(hkdfKey) == string(legacyKey) {
		t.Error("HKDF key should differ from raw SHA-256 key")
	}
}

func TestSecretKey_EmptyKey_ReturnsError(t *testing.T) {
	t.Parallel()

	q := &Queries{secretEncryptionKey: ""}
	_, err := q.secretKey()
	if err == nil {
		t.Fatal("expected error for empty encryption key")
	}
}

func TestEncryptDecrypt_Roundtrip_HKDF(t *testing.T) {
	t.Parallel()

	q := &Queries{secretEncryptionKey: "roundtrip-test-key-hkdf"}
	key, err := q.secretKey()
	if err != nil {
		t.Fatalf("secretKey error: %v", err)
	}

	plaintext := "my-secret-database-password"
	encrypted, err := encryptSecret(plaintext, key)
	if err != nil {
		t.Fatalf("encryptSecret error: %v", err)
	}

	decrypted, err := decryptSecret(encrypted, key)
	if err != nil {
		t.Fatalf("decryptSecret error: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptSecretWithFallback_NewKey(t *testing.T) {
	t.Parallel()

	q := &Queries{secretEncryptionKey: "fallback-test-key"}
	key, err := q.secretKey()
	if err != nil {
		t.Fatalf("secretKey error: %v", err)
	}

	encrypted, err := encryptSecret("new-format-secret", key)
	if err != nil {
		t.Fatalf("encryptSecret error: %v", err)
	}

	decrypted, err := q.decryptSecretWithFallback(encrypted)
	if err != nil {
		t.Fatalf("decryptSecretWithFallback error: %v", err)
	}

	if decrypted != "new-format-secret" {
		t.Errorf("decrypted = %q, want %q", decrypted, "new-format-secret")
	}
}

func TestDecryptSecretWithFallback_LegacyKey(t *testing.T) {
	t.Parallel()

	passphrase := "legacy-migration-test"
	q := &Queries{secretEncryptionKey: passphrase}

	// Encrypt with the old SHA-256 key.
	legacySum := sha256.Sum256([]byte(passphrase))
	legacyKey := legacySum[:]

	encrypted, err := encryptSecret("legacy-secret-value", legacyKey)
	if err != nil {
		t.Fatalf("encryptSecret error: %v", err)
	}

	// Decrypt should fall back to legacy key.
	decrypted, err := q.decryptSecretWithFallback(encrypted)
	if err != nil {
		t.Fatalf("decryptSecretWithFallback error: %v", err)
	}

	if decrypted != "legacy-secret-value" {
		t.Errorf("decrypted = %q, want %q", decrypted, "legacy-secret-value")
	}
}

func TestDecryptSecretWithFallback_WrongKey_Fails(t *testing.T) {
	t.Parallel()

	q1 := &Queries{secretEncryptionKey: "key-one"}
	key1, _ := q1.secretKey()

	encrypted, err := encryptSecret("secret-for-key-one", key1)
	if err != nil {
		t.Fatalf("encryptSecret error: %v", err)
	}

	q2 := &Queries{secretEncryptionKey: "key-two-completely-different"}
	_, err = q2.decryptSecretWithFallback(encrypted)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
}

func TestDecryptSecretWithFallback_OldConfiguredKey(t *testing.T) {
	t.Parallel()

	oldQ := &Queries{secretEncryptionKey: "old-secret-key"}
	oldKey, err := oldQ.secretKey()
	if err != nil {
		t.Fatalf("old secretKey error: %v", err)
	}
	encrypted, err := encryptSecret("rotated-secret", oldKey)
	if err != nil {
		t.Fatalf("encryptSecret error: %v", err)
	}

	newQ := &Queries{
		secretEncryptionKey:     "new-secret-key",
		oldSecretEncryptionKeys: []string{"old-secret-key"},
	}
	decrypted, err := newQ.decryptSecretWithFallback(encrypted)
	if err != nil {
		t.Fatalf("decryptSecretWithFallback error: %v", err)
	}
	if decrypted != "rotated-secret" {
		t.Fatalf("decryptSecretWithFallback = %q, want rotated-secret", decrypted)
	}
}

func FuzzSecretEncryptDecrypt(f *testing.F) {
	f.Add("hello world")
	f.Add("")
	f.Add("a")
	f.Add("special chars: !@#$%^&*()_+-=[]{}|;':\",./<>?")
	f.Add("\x00\x01\x02\x03")

	f.Fuzz(func(t *testing.T, plaintext string) {
		q := &Queries{secretEncryptionKey: "fuzz-test-key-for-secrets"}
		key, err := q.secretKey()
		if err != nil {
			t.Fatalf("secretKey error: %v", err)
		}

		encrypted, err := encryptSecret(plaintext, key)
		if err != nil {
			t.Fatalf("encryptSecret error: %v", err)
		}

		decrypted, err := decryptSecret(encrypted, key)
		if err != nil {
			t.Fatalf("decryptSecret error: %v", err)
		}

		if decrypted != plaintext {
			t.Errorf("roundtrip failed: got %q, want %q", decrypted, plaintext)
		}
	})
}
