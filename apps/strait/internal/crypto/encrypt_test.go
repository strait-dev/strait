package crypto

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEncryptor_AcceptsRaw32ByteKey(t *testing.T) {
	t.Parallel()

	key := "0123456789abcdef0123456789abcdef"
	enc, err := NewEncryptor(key)
	require.NoError(t, err)
	require.NotNil(t, enc)
}

func TestNewEncryptor_AcceptsHexKey(t *testing.T) {
	t.Parallel()

	raw := "0123456789abcdef0123456789abcdef"
	hexKey := hex.EncodeToString([]byte(raw))

	enc, err := NewEncryptor(hexKey)
	require.NoError(t, err)
	require.NotNil(t, enc)
}

func TestNewEncryptor_InvalidKeyLength(t *testing.T) {
	t.Parallel()

	_, err := NewEncryptor("short")
	require.Error(
		t, err)
	require.Equal(
		t, "invalid key length",
		err.
			Error())
}

func TestNewEncryptor_InvalidHexKey(t *testing.T) {
	t.Parallel()

	_, err := NewEncryptor(strings.Repeat("z", 64))
	require.Error(
		t, err)
	require.Equal(
		t, "invalid key length",
		err.
			Error())
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	t.Parallel()

	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")
	plaintext := []byte("webhook-secret-value")

	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)
	require.Greater(
		t, len(ciphertext), 12)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.True(t,
		bytes.Equal(decrypted,
			plaintext,
		))
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	t.Parallel()

	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")

	ciphertext, err := enc.Encrypt(nil)
	require.NoError(t, err)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Empty(t,
		decrypted)
}

func TestDecrypt_WrongKey(t *testing.T) {
	t.Parallel()

	encryptorA := mustEncryptor(t, "0123456789abcdef0123456789abcdef")
	encryptorB := mustEncryptor(t, "fedcba9876543210fedcba9876543210")

	ciphertext, err := encryptorA.Encrypt([]byte("secret"))
	require.NoError(t, err)

	_, err = encryptorB.Decrypt(ciphertext)
	require.Error(
		t, err)
	require.Equal(
		t, "decrypt failed",
		err.Error())
}

func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	t.Parallel()

	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")

	_, err := enc.Decrypt([]byte("short"))
	require.Error(
		t, err)
	require.Equal(
		t, "ciphertext too short",

		err.Error(),
	)
}

func TestEncryptStringDecryptString_RoundTrip(t *testing.T) {
	t.Parallel()

	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")
	plaintext := "x-api-key: abc123"

	ciphertext, err := enc.EncryptString(plaintext)
	require.NoError(t, err)
	require.NotEmpty(t, strings.TrimSpace(ciphertext))

	decrypted, err := enc.DecryptString(ciphertext)
	require.NoError(t, err)
	require.Equal(
		t, plaintext, decrypted,
	)
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
					require.NoError(t, err)
					return
				}

				decrypted, err := enc.Decrypt(ciphertext)
				if err != nil {
					require.NoError(t, err)
					return
				}

				if !assert.Equal(t, plaintext, decrypted) {
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
	require.NoError(t, err)

	plaintext := []byte("key-rotation-payload")
	ciphertext, err := rotator.Encrypt(plaintext)
	require.NoError(t, err)

	decrypted, err := rotator.Decrypt(ciphertext)
	require.NoError(t, err)
	require.True(t,
		bytes.Equal(decrypted,
			plaintext,
		))
}

func TestKeyRotator_DecryptWithOldKey(t *testing.T) {
	t.Parallel()

	primaryKey := []byte("0123456789abcdef0123456789abcdef")
	oldKey := []byte("fedcba9876543210fedcba9876543210")

	oldEncryptor, err := newEncryptorFromBytes(oldKey)
	require.NoError(t, err)

	ciphertext, err := oldEncryptor.Encrypt([]byte("legacy-secret"))
	require.NoError(t, err)

	rotator, err := NewKeyRotator(primaryKey, oldKey)
	require.NoError(t, err)

	decrypted, err := rotator.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(
		t, "legacy-secret",
		string(
			decrypted))
}

func TestNewKeyRotatorFromStrings_DecryptsOldStringKey(t *testing.T) {
	t.Parallel()

	oldEncryptor := mustEncryptor(t, "fedcba9876543210fedcba9876543210")
	ciphertext, err := oldEncryptor.Encrypt([]byte("legacy-secret"))
	require.NoError(t, err)

	rotator, err := NewKeyRotatorFromStrings("0123456789abcdef0123456789abcdef", "fedcba9876543210fedcba9876543210")
	require.NoError(t, err)

	decrypted, err := rotator.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(
		t, "legacy-secret",
		string(
			decrypted))

	encryptedString, err := rotator.EncryptString("new-secret")
	require.NoError(t, err)

	decryptedString, err := rotator.DecryptString(encryptedString)
	require.NoError(t, err)
	require.Equal(
		t, "new-secret",
		decryptedString,
	)
}

func TestKeyRotator_RotateKeyFlow(t *testing.T) {
	t.Parallel()

	oldPrimary := []byte("0123456789abcdef0123456789abcdef")
	newPrimary := []byte("1234567890abcdef1234567890abcdef")

	rotator, err := NewKeyRotator(oldPrimary)
	require.NoError(t, err)

	beforeRotation, err := rotator.Encrypt([]byte("before-rotation"))
	require.NoError(t, err)
	require.NoError(t, rotator.RotateKey(newPrimary))

	afterRotation, err := rotator.Encrypt([]byte("after-rotation"))
	require.NoError(t, err)

	plainBefore, err := rotator.Decrypt(beforeRotation)
	require.NoError(t, err)
	require.Equal(
		t, "before-rotation",
		string(plainBefore))

	plainAfter, err := rotator.Decrypt(afterRotation)
	require.NoError(t, err)
	require.Equal(
		t, "after-rotation",
		string(plainAfter),
	)
}

func mustEncryptor(t *testing.T, key string) *Encryptor {
	t.Helper()

	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	return enc
}
