package webhooktoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// Generate returns a cryptographically random webhook token and its SHA-256 hash.
// raw is shown to the user once at creation; only hash is persisted.
func Generate() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	raw = "wh_" + base64.RawURLEncoding.EncodeToString(b)
	hash = Hash(raw)
	return
}

// Hash returns the SHA-256 hex digest of a raw webhook token.
func Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
