package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const (
	MinPasswordLength = 8
	refreshTokenBytes = 32
)

var (
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
)

// ValidatePassword checks minimum password requirements.
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}

// IssueAccessToken signs a short-lived JWT for API access.
func IssueAccessToken(secret, userID string, ttl time.Duration) (string, int64, error) {
	expiresAt := time.Now().Add(ttl)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": expiresAt.Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", 0, err
	}
	return signed, int64(ttl.Seconds()), nil
}

// GenerateRefreshToken returns a high-entropy opaque refresh token.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return "rt_" + hex.EncodeToString(b), nil
}

// HashRefreshToken returns a stable SHA-256 hex digest for storage.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
