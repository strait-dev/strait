package crypto

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const encryptedFieldPrefix = "enc:v1:"

type fieldEncryptor interface {
	Encrypt([]byte) ([]byte, error)
}

type fieldDecryptor interface {
	Decrypt([]byte) ([]byte, error)
}

func IsEncryptedField(value string) bool {
	return strings.HasPrefix(value, encryptedFieldPrefix)
}

func EncryptField(enc fieldEncryptor, plaintext string) (string, error) {
	if plaintext == "" {
		return plaintext, nil
	}
	if enc == nil {
		return "", fmt.Errorf("encrypt field: encryptor is not configured")
	}
	ciphertext, err := enc.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return encryptedFieldPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

func PreserveOrEncryptField(enc fieldEncryptor, value string) (string, error) {
	if value == "" || IsEncryptedField(value) {
		return value, nil
	}
	return EncryptField(enc, value)
}

func DecryptField(dec fieldDecryptor, value string) (string, error) {
	if value == "" || !IsEncryptedField(value) {
		return value, nil
	}
	if dec == nil {
		return "", fmt.Errorf("decrypt field: decryptor is not configured")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encryptedFieldPrefix))
	if err != nil {
		return "", err
	}
	plaintext, err := dec.Decrypt(raw)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
