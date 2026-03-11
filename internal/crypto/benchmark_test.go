package crypto

import (
	"bytes"
	"testing"
)

const benchmarkKey = "0123456789abcdef0123456789abcdef"

func BenchmarkEncrypt(b *testing.B) {
	encryptor, err := NewEncryptor(benchmarkKey)
	if err != nil {
		b.Fatalf("NewEncryptor() error = %v", err)
	}
	payload := bytes.Repeat([]byte("a"), 1024)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := encryptor.Encrypt(payload); err != nil {
			b.Fatalf("Encrypt() error = %v", err)
		}
	}
}

func BenchmarkDecrypt(b *testing.B) {
	encryptor, err := NewEncryptor(benchmarkKey)
	if err != nil {
		b.Fatalf("NewEncryptor() error = %v", err)
	}
	payload := bytes.Repeat([]byte("a"), 1024)
	ciphertext, err := encryptor.Encrypt(payload)
	if err != nil {
		b.Fatalf("Encrypt() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := encryptor.Decrypt(ciphertext); err != nil {
			b.Fatalf("Decrypt() error = %v", err)
		}
	}
}

func BenchmarkEncryptString(b *testing.B) {
	encryptor, err := NewEncryptor(benchmarkKey)
	if err != nil {
		b.Fatalf("NewEncryptor() error = %v", err)
	}
	plaintext := string(bytes.Repeat([]byte("a"), 1024))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ciphertext, err := encryptor.EncryptString(plaintext)
		if err != nil {
			b.Fatalf("EncryptString() error = %v", err)
		}
		if _, err := encryptor.DecryptString(ciphertext); err != nil {
			b.Fatalf("DecryptString() error = %v", err)
		}
	}
}
