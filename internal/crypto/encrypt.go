package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

var (
	errInvalidKeyLength   = errors.New("invalid key length")
	errCiphertextTooShort = errors.New("ciphertext too short")
	errDecryptFailed      = errors.New("decrypt failed")
)

type Encryptor struct {
	aead cipher.AEAD
}

func NewEncryptor(key string) (*Encryptor, error) {
	keyBytes, err := parseKey(key)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, errInvalidKeyLength
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &Encryptor{aead: aead}, nil
}

func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	sealed := e.aead.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)

	return out, nil
}

func (e *Encryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := e.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errCiphertextTooShort
	}

	nonce := ciphertext[:nonceSize]
	data := ciphertext[nonceSize:]
	plaintext, err := e.aead.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, errDecryptFailed
	}

	return plaintext, nil
}

func (e *Encryptor) EncryptString(plaintext string) (string, error) {
	ciphertext, err := e.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (e *Encryptor) DecryptString(ciphertext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	plaintext, err := e.Decrypt(raw)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func parseKey(key string) ([]byte, error) {
	if len(key) == 32 {
		return []byte(key), nil
	}

	if len(key) == 64 {
		decoded, err := hex.DecodeString(key)
		if err != nil || len(decoded) != 32 {
			return nil, errInvalidKeyLength
		}
		return decoded, nil
	}

	return nil, errInvalidKeyLength
}
