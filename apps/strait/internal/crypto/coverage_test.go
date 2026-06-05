package crypto

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecryptString_InvalidBase64(t *testing.T) {
	t.Parallel()
	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")

	_, err := enc.DecryptString("not-valid-base64!!!")
	require.Error(
		t, err)
}

func TestDecryptString_ValidBase64ButBadCiphertext(t *testing.T) {
	t.Parallel()
	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")

	// Valid base64 but too short to contain a nonce.
	short := base64.StdEncoding.EncodeToString([]byte("short"))
	_, err := enc.DecryptString(short)
	require.Error(
		t, err)
}

func TestDecryptString_ValidBase64WrongKey(t *testing.T) {
	t.Parallel()
	encA := mustEncryptor(t, "0123456789abcdef0123456789abcdef")
	encB := mustEncryptor(t, "fedcba9876543210fedcba9876543210")

	ct, err := encA.EncryptString("secret-value")
	require.NoError(t, err)

	_, err = encB.DecryptString(ct)
	require.Error(
		t, err)
}

func TestNewEncryptorFromBytes_TooShort(t *testing.T) {
	t.Parallel()
	_, err := newEncryptorFromBytes([]byte("short"))
	require.Error(
		t, err)
	require.ErrorIs(t,
		err, errInvalidKeyLength)
}

func TestNewEncryptorFromBytes_TooLong(t *testing.T) {
	t.Parallel()
	_, err := newEncryptorFromBytes(make([]byte, 64))
	require.Error(
		t, err)
	require.ErrorIs(t,
		err, errInvalidKeyLength)
}

func TestNewKeyRotator_InvalidPrimary(t *testing.T) {
	t.Parallel()
	_, err := NewKeyRotator([]byte("bad"))
	require.Error(
		t, err)
}

func TestNewKeyRotator_InvalidOldKey(t *testing.T) {
	t.Parallel()
	good := []byte("0123456789abcdef0123456789abcdef")
	_, err := NewKeyRotator(good, []byte("bad"))
	require.Error(
		t, err)
}

func TestKeyRotator_DecryptTooShort(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789abcdef0123456789abcdef")
	rotator, err := NewKeyRotator(key)
	require.NoError(t, err)

	_, err = rotator.Decrypt([]byte("x"))
	require.Error(
		t, err)
	require.ErrorIs(t,
		err, errCiphertextTooShort)
}

func TestKeyRotator_DecryptNoMatchingKey(t *testing.T) {
	t.Parallel()
	keyA := []byte("0123456789abcdef0123456789abcdef")
	keyB := []byte("fedcba9876543210fedcba9876543210")
	keyC := []byte("1234567890abcdef1234567890abcdef")

	encC, err := newEncryptorFromBytes(keyC)
	require.NoError(t, err)

	ct, err := encC.Encrypt([]byte("secret"))
	require.NoError(t, err)

	rotator, err := NewKeyRotator(keyA, keyB)
	require.NoError(t, err)

	_, err = rotator.Decrypt(ct)
	require.Error(
		t, err)
	require.ErrorIs(t,
		err, errDecryptFailed)
}

func TestRotateKey_InvalidKey(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789abcdef0123456789abcdef")
	rotator, err := NewKeyRotator(key)
	require.NoError(t, err)

	err = rotator.RotateKey([]byte("bad"))
	require.Error(
		t, err)
}

func TestParseKey_Base64Standard(t *testing.T) {
	t.Parallel()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)

	enc, err := NewEncryptor(encoded)
	require.NoError(t, err)
	require.NotNil(t, enc)
}

func TestParseKey_Base64RawStandard(t *testing.T) {
	t.Parallel()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 10)
	}
	encoded := base64.RawStdEncoding.EncodeToString(raw)

	enc, err := NewEncryptor(encoded)
	require.NoError(t, err)
	require.NotNil(t, enc)
}

func TestParseKey_Base64URLSafe(t *testing.T) {
	t.Parallel()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = 0xff - byte(i)
	}

	for _, encoded := range []string{
		base64.URLEncoding.EncodeToString(raw),
		base64.RawURLEncoding.EncodeToString(raw),
	} {
		enc, err := NewEncryptor(encoded)
		require.NoError(t, err)
		require.NotNil(t, enc)
	}
}

func TestParseKey_64CharsNonHex(t *testing.T) {
	t.Parallel()
	// 64 chars that are not valid hex.
	key := strings.Repeat("zz", 32)
	_, err := NewEncryptor(key)
	require.Error(
		t, err)
}

func TestParseKey_WrongLength(t *testing.T) {
	t.Parallel()
	_, err := NewEncryptor("12345")
	require.Error(
		t, err)
}

func TestDecrypt_ExactlyNonceSizeBytes(t *testing.T) {
	t.Parallel()

	enc := testEncryptor(t)
	nonceSize := enc.aead.NonceSize()

	ciphertext := make([]byte, nonceSize)
	_, err := enc.Decrypt(ciphertext)
	require.Error(
		t, err)
	require.NotErrorIs(
		t, err, errCiphertextTooShort,
	)
	require.ErrorIs(t,
		err, errDecryptFailed)
}

func TestKeyRotator_DecryptNoOldKeys(t *testing.T) {
	t.Parallel()

	key := []byte("0123456789abcdef0123456789abcdef")
	rotator, err := NewKeyRotator(key)
	require.NoError(t, err)

	plaintext := []byte("no-old-keys-test")
	ciphertext, err := rotator.Encrypt(plaintext)
	require.NoError(t, err)

	decrypted, err := rotator.Decrypt(ciphertext)
	require.NoError(t, err)
	require.True(t,
		bytes.Equal(plaintext,
			decrypted,
		))
}

func TestKeyRotator_DecryptExactlyNonceSizeBytes(t *testing.T) {
	t.Parallel()

	key := []byte("0123456789abcdef0123456789abcdef")
	rotator, err := NewKeyRotator(key)
	require.NoError(t, err)

	nonceSize := rotator.primary.aead.NonceSize()
	ciphertext := make([]byte, nonceSize)
	_, err = rotator.Decrypt(ciphertext)
	require.Error(
		t, err)
	require.NotErrorIs(
		t, err, errCiphertextTooShort,
	)
	require.ErrorIs(t,
		err, errDecryptFailed)
}

func TestEncrypt_ProducesUniqueCiphertexts(t *testing.T) {
	t.Parallel()
	enc := mustEncryptor(t, "0123456789abcdef0123456789abcdef")
	plaintext := []byte("same-input")

	ct1, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	ct2, err := enc.Encrypt(plaintext)
	require.NoError(t, err)
	require.NotEqual(t, string(ct2),
		string(ct1))
}
