package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"sync"
)

var (
	errInvalidKeyLength   = errors.New("invalid key length")
	errCiphertextTooShort = errors.New("ciphertext too short")
	errDecryptFailed      = errors.New("decrypt failed")
)

type Encryptor struct {
	aead cipher.AEAD
}

type KeyRotator struct {
	mu      sync.RWMutex
	primary *Encryptor
	old     []*Encryptor
}

func NewEncryptor(key string) (*Encryptor, error) {
	keyBytes, err := parseKey(key)
	if err != nil {
		return nil, err
	}

	return newEncryptorFromBytes(keyBytes)
}

func NewKeyRotator(primaryKey []byte, oldKeys ...[]byte) (*KeyRotator, error) {
	primary, err := newEncryptorFromBytes(primaryKey)
	if err != nil {
		return nil, err
	}

	old := make([]*Encryptor, 0, len(oldKeys))
	for _, key := range oldKeys {
		e, encryptorErr := newEncryptorFromBytes(key)
		if encryptorErr != nil {
			return nil, encryptorErr
		}
		old = append(old, e)
	}

	return &KeyRotator{primary: primary, old: old}, nil
}

func NewKeyRotatorFromStrings(primaryKey string, oldKeys ...string) (*KeyRotator, error) {
	primary, err := parseKey(primaryKey)
	if err != nil {
		return nil, err
	}
	old := make([][]byte, 0, len(oldKeys))
	for _, key := range oldKeys {
		if key == "" {
			continue
		}
		parsed, parseErr := parseKey(key)
		if parseErr != nil {
			return nil, parseErr
		}
		old = append(old, parsed)
	}
	return NewKeyRotator(primary, old...)
}

func (k *KeyRotator) Encrypt(plaintext []byte) ([]byte, error) {
	k.mu.RLock()
	primary := k.primary
	k.mu.RUnlock()

	return primary.Encrypt(plaintext)
}

func (k *KeyRotator) Decrypt(ciphertext []byte) ([]byte, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	if len(ciphertext) < k.primary.aead.NonceSize() {
		return nil, errCiphertextTooShort
	}

	plaintext, err := k.primary.Decrypt(ciphertext)
	if err == nil {
		return plaintext, nil
	}
	if !errors.Is(err, errDecryptFailed) {
		return nil, err
	}

	for _, e := range k.old {
		plaintext, err = e.Decrypt(ciphertext)
		if err == nil {
			return plaintext, nil
		}
		if !errors.Is(err, errDecryptFailed) {
			return nil, err
		}
	}
	return nil, errDecryptFailed
}

func (k *KeyRotator) EncryptString(plaintext string) (string, error) {
	ciphertext, err := k.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (k *KeyRotator) DecryptString(ciphertext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	plaintext, err := k.Decrypt(raw)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (k *KeyRotator) RotateKey(newPrimary []byte) error {
	encryptor, err := newEncryptorFromBytes(newPrimary)
	if err != nil {
		return err
	}

	k.mu.Lock()
	k.old = append([]*Encryptor{k.primary}, k.old...)
	k.primary = encryptor
	k.mu.Unlock()

	return nil
}

func newEncryptorFromBytes(key []byte) (*Encryptor, error) {
	if len(key) != 32 {
		return nil, errInvalidKeyLength
	}

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	block, err := aes.NewCipher(keyCopy)
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
	nonceSize := e.aead.NonceSize()
	out := make([]byte, nonceSize, nonceSize+len(plaintext)+e.aead.Overhead())
	nonce := out[:nonceSize]
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return e.aead.Seal(out, nonce, plaintext, nil), nil
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

	// Try base64 decoding (standard or URL-safe, with or without padding).
	if decoded, err := base64.StdEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		return decoded, nil
	}

	return nil, errInvalidKeyLength
}
