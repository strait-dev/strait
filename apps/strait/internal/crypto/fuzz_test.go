package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzEncryptDecryptRoundTrip(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("\x00\x00\x00"))
	f.Add(make([]byte, 1024))

	f.Fuzz(func(t *testing.T, plaintext []byte) {
		key := make([]byte, 32)
		_, err := rand.Read(key)
		require.NoError(t, err)

		enc, err := newEncryptorFromBytes(key)
		require.NoError(t, err)

		ciphertext, err := enc.Encrypt(plaintext)
		require.NoError(t, err)

		decrypted, err := enc.Decrypt(ciphertext)
		require.NoError(t, err)
		require.True(t, bytes.Equal(plaintext, decrypted))
	})
}

func FuzzDecryptMalformed(f *testing.F) {
	f.Add([]byte("short"))
	f.Add([]byte(""))
	f.Add([]byte("\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"))
	f.Add(make([]byte, 100))
	f.Add([]byte("this is definitely not valid ciphertext at all"))

	f.Fuzz(func(t *testing.T, data []byte) {
		key := make([]byte, 32)
		_, err := rand.Read(key)
		require.NoError(t, err)

		enc, err := newEncryptorFromBytes(key)
		require.NoError(t, err)

		// Decrypting random bytes should return an error, never panic.
		_, _ = enc.Decrypt(data)
	})
}
