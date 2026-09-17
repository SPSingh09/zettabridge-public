package emailverify

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const prefix = "ev_"

// Generate returns a one-time verification token (store only the hash).
func Generate() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = prefix + hex.EncodeToString(b)
	return raw, Hash(raw), nil
}

// Hash returns the SHA-256 hex digest of a raw verification token.
func Hash(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}

// ValidateFormat checks the raw token shape before hashing.
func ValidateFormat(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.HasPrefix(raw, prefix) && len(raw) > len(prefix)+16
}

// Mask returns a redacted token for logs.
func Mask(raw string) string {
	if len(raw) <= 12 {
		return "***"
	}
	return fmt.Sprintf("%s...%s", raw[:8], raw[len(raw)-4:])
}
