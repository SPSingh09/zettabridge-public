package credenc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	versionPrefix = "v1:"
	nonceSize     = 12
	keySize       = 32
)

var (
	ErrInvalidKey       = errors.New("AES_KEY must be 32 bytes as 64 hex characters")
	ErrWeakKey          = errors.New("AES_KEY must not be all zeros in production")
	ErrInvalidBlob      = errors.New("invalid encrypted credential blob")
	ErrDecrypt          = errors.New("credential decryption failed")
	ErrEmptyPlaintext   = errors.New("credential payload is empty")
)

// ParseKey decodes a 64-character hex string into a 32-byte AES-256 key.
func ParseKey(hexKey string) ([]byte, error) {
	key, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil || len(key) != keySize {
		return nil, ErrInvalidKey
	}
	return key, nil
}

// IsWeakKey reports whether the key is the all-zero development default.
func IsWeakKey(key []byte) bool {
	if len(key) != keySize {
		return true
	}
	for _, b := range key {
		if b != 0 {
			return false
		}
	}
	return true
}

// IsEncrypted reports whether a stored blob uses the versioned ciphertext format.
func IsEncrypted(blob string) bool {
	return strings.HasPrefix(blob, versionPrefix)
}

// Encrypt seals plaintext with AES-256-GCM. credID is used as additional authenticated data.
func Encrypt(plaintext string, key []byte, credID string) (string, error) {
	if strings.TrimSpace(plaintext) == "" {
		return "", ErrEmptyPlaintext
	}
	if len(key) != keySize {
		return "", ErrInvalidKey
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), []byte(credID))
	payload := append(nonce, ciphertext...)
	return versionPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

// Decrypt opens a v1 blob sealed with Encrypt.
func Decrypt(blob string, key []byte, credID string) (string, error) {
	if !IsEncrypted(blob) {
		return "", ErrInvalidBlob
	}
	if len(key) != keySize {
		return "", ErrInvalidKey
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(blob, versionPrefix))
	if err != nil || len(raw) < nonceSize+1 {
		return "", ErrInvalidBlob
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ErrDecrypt
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrDecrypt
	}

	nonce := raw[:nonceSize]
	ciphertext := raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(credID))
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plaintext), nil
}

// PlaintextOrDecrypt returns legacy plaintext blobs unchanged, or decrypts v1 blobs.
func PlaintextOrDecrypt(blob string, key []byte, credID string) (string, error) {
	if !IsEncrypted(blob) {
		if strings.TrimSpace(blob) == "" {
			return "", ErrEmptyPlaintext
		}
		return blob, nil
	}
	return Decrypt(blob, key, credID)
}
