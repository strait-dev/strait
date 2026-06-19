package crypto

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fieldCipherStub struct {
	encryptErr error
	decryptErr error
}

func (s fieldCipherStub) Encrypt(plaintext []byte) ([]byte, error) {
	if s.encryptErr != nil {
		return nil, s.encryptErr
	}
	out := append([]byte("cipher:"), plaintext...)
	return out, nil
}

func (s fieldCipherStub) Decrypt(ciphertext []byte) ([]byte, error) {
	if s.decryptErr != nil {
		return nil, s.decryptErr
	}
	const prefix = "cipher:"
	if len(ciphertext) < len(prefix) || string(ciphertext[:len(prefix)]) != prefix {
		return nil, errors.New("bad ciphertext")
	}
	return append([]byte(nil), ciphertext[len(prefix):]...), nil
}

func TestEncryptField(t *testing.T) {
	t.Parallel()

	t.Run("empty plaintext stays empty without encryptor", func(t *testing.T) {
		t.Parallel()

		got, err := EncryptField(nil, "")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("requires encryptor for non-empty plaintext", func(t *testing.T) {
		t.Parallel()

		got, err := EncryptField(nil, "secret")
		require.Error(t, err)
		assert.Empty(t, got)
	})

	t.Run("encrypts and marks field", func(t *testing.T) {
		t.Parallel()

		got, err := EncryptField(fieldCipherStub{}, "secret")
		require.NoError(t, err)
		assert.True(t, IsEncryptedField(got))
		assert.Contains(t, got, "Y2lwaGVyOnNlY3JldA==")
	})

	t.Run("returns encryptor error", func(t *testing.T) {
		t.Parallel()

		errBoom := errors.New("boom")
		got, err := EncryptField(fieldCipherStub{encryptErr: errBoom}, "secret")
		require.ErrorIs(t, err, errBoom)
		assert.Empty(t, got)
	})
}

func TestPreserveOrEncryptField(t *testing.T) {
	t.Parallel()

	encrypted := encryptedFieldPrefix + "already"

	got, err := PreserveOrEncryptField(nil, "")
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = PreserveOrEncryptField(nil, encrypted)
	require.NoError(t, err)
	assert.Equal(t, encrypted, got)

	got, err = PreserveOrEncryptField(fieldCipherStub{}, "secret")
	require.NoError(t, err)
	assert.True(t, IsEncryptedField(got))
}

func TestDecryptField(t *testing.T) {
	t.Parallel()

	encrypted, err := EncryptField(fieldCipherStub{}, "secret")
	require.NoError(t, err)

	t.Run("empty and plaintext values pass through without decryptor", func(t *testing.T) {
		t.Parallel()

		got, err := DecryptField(nil, "")
		require.NoError(t, err)
		assert.Empty(t, got)

		got, err = DecryptField(nil, "plaintext")
		require.NoError(t, err)
		assert.Equal(t, "plaintext", got)
	})

	t.Run("requires decryptor for encrypted values", func(t *testing.T) {
		t.Parallel()

		got, err := DecryptField(nil, encrypted)
		require.Error(t, err)
		assert.Empty(t, got)
	})

	t.Run("rejects invalid base64 payload", func(t *testing.T) {
		t.Parallel()

		got, err := DecryptField(fieldCipherStub{}, encryptedFieldPrefix+"not base64!")
		require.Error(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns decryptor error", func(t *testing.T) {
		t.Parallel()

		errBoom := errors.New("boom")
		got, err := DecryptField(fieldCipherStub{decryptErr: errBoom}, encrypted)
		require.ErrorIs(t, err, errBoom)
		assert.Empty(t, got)
	})

	t.Run("decrypts encrypted field", func(t *testing.T) {
		t.Parallel()

		got, err := DecryptField(fieldCipherStub{}, encrypted)
		require.NoError(t, err)
		assert.Equal(t, "secret", got)
	})
}
