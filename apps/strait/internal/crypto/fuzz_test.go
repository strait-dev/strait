package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func FuzzEncryptDecryptRoundTrip(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("\x00\x00\x00"))
	f.Add(make([]byte, 1024))

	f.Fuzz(func(t *testing.T, plaintext []byte) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Fatal(err)
		}

		enc, err := newEncryptorFromBytes(key)
		if err != nil {
			t.Fatal(err)
		}

		ciphertext, err := enc.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt failed: %v", err)
		}

		decrypted, err := enc.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("Decrypt failed: %v", err)
		}

		if !bytes.Equal(plaintext, decrypted) {
			t.Fatal("roundtrip mismatch")
		}
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
		if _, err := rand.Read(key); err != nil {
			t.Fatal(err)
		}

		enc, err := newEncryptorFromBytes(key)
		if err != nil {
			t.Fatal(err)
		}

		// Decrypting random bytes should return an error, never panic.
		_, _ = enc.Decrypt(data)
	})
}
