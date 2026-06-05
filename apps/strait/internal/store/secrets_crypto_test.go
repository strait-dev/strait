package store

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretKey_HKDF_Returns32Bytes(t *testing.T) {
	t.Parallel()

	q := &Queries{secretEncryptionKey: "my-test-encryption-key"}
	key, err := q.secretKey()
	require.NoError(t, err)
	assert.Len(t, key, 32)

}

func TestSecretKey_HKDF_Deterministic(t *testing.T) {
	t.Parallel()

	q := &Queries{secretEncryptionKey: "deterministic-test-key"}
	key1, err := q.secretKey()
	require.NoError(t, err)

	key2, err := q.secretKey()
	require.NoError(t, err)
	assert.Equal(t, string(key2), string(key1))

}

func TestSecretKey_HKDF_DifferentFromSHA256(t *testing.T) {
	t.Parallel()

	passphrase := "compare-derivation-methods"
	q := &Queries{secretEncryptionKey: passphrase}

	hkdfKey, err := q.secretKey()
	require.NoError(t, err)

	legacySum := sha256.Sum256([]byte(passphrase))
	legacyKey := legacySum[:]
	assert.NotEqual(t, string(legacyKey), string(hkdfKey))

}

func TestSecretKey_EmptyKey_ReturnsError(t *testing.T) {
	t.Parallel()

	q := &Queries{secretEncryptionKey: ""}
	_, err := q.secretKey()
	require.Error(t, err)

}

func TestEncryptDecrypt_Roundtrip_HKDF(t *testing.T) {
	t.Parallel()

	q := &Queries{secretEncryptionKey: "roundtrip-test-key-hkdf"}
	key, err := q.secretKey()
	require.NoError(t, err)

	plaintext := "my-secret-database-password"
	encrypted, err := encryptSecret(plaintext, key)
	require.NoError(t, err)

	decrypted, err := decryptSecret(encrypted, key)
	require.NoError(t, err)
	assert.Equal(t, plaintext,
		decrypted,
	)

}

func TestDecryptSecretWithFallback_NewKey(t *testing.T) {
	t.Parallel()

	q := &Queries{secretEncryptionKey: "fallback-test-key"}
	key, err := q.secretKey()
	require.NoError(t, err)

	encrypted, err := encryptSecret("new-format-secret", key)
	require.NoError(t, err)

	decrypted, err := q.decryptSecretWithFallback(encrypted)
	require.NoError(t, err)
	assert.Equal(t, "new-format-secret",

		decrypted)

}

func TestDecryptSecretWithFallback_LegacyKey(t *testing.T) {
	t.Parallel()

	passphrase := "legacy-migration-test"
	q := &Queries{secretEncryptionKey: passphrase}

	// Encrypt with the old SHA-256 key.
	legacySum := sha256.Sum256([]byte(passphrase))
	legacyKey := legacySum[:]

	encrypted, err := encryptSecret("legacy-secret-value", legacyKey)
	require.NoError(t, err)

	// Decrypt should fall back to legacy key.
	decrypted, err := q.decryptSecretWithFallback(encrypted)
	require.NoError(t, err)
	assert.Equal(t, "legacy-secret-value",

		decrypted)

}

func TestDecryptSecretWithFallback_WrongKey_Fails(t *testing.T) {
	t.Parallel()

	q1 := &Queries{secretEncryptionKey: "key-one"}
	key1, _ := q1.secretKey()

	encrypted, err := encryptSecret("secret-for-key-one", key1)
	require.NoError(t, err)

	q2 := &Queries{secretEncryptionKey: "key-two-completely-different"}
	_, err = q2.decryptSecretWithFallback(encrypted)
	require.Error(t, err)

}

func TestDecryptSecretWithFallback_OldConfiguredKey(t *testing.T) {
	t.Parallel()

	oldQ := &Queries{secretEncryptionKey: "old-secret-key"}
	oldKey, err := oldQ.secretKey()
	require.NoError(t, err)

	encrypted, err := encryptSecret("rotated-secret", oldKey)
	require.NoError(t, err)

	newQ := &Queries{
		secretEncryptionKey:     "new-secret-key",
		oldSecretEncryptionKeys: []string{"old-secret-key"},
	}
	decrypted, err := newQ.decryptSecretWithFallback(encrypted)
	require.NoError(t, err)
	require.Equal(t, "rotated-secret",

		decrypted)

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
		require.NoError(t, err)

		encrypted, err := encryptSecret(plaintext, key)
		require.NoError(t, err)

		decrypted, err := decryptSecret(encrypted, key)
		require.NoError(t, err)
		assert.Equal(t, plaintext,
			decrypted,
		)

	})
}
