package security_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"github.com/SPSingh09/zettabridge/internal/platform/security"
)

func TestParseAccessToken(t *testing.T) {
	secret := "test-secret"
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	raw, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	uid, err := security.ParseAccessToken(secret, raw)
	if err != nil || uid != "user-1" {
		t.Fatalf("ParseAccessToken() = %q, %v", uid, err)
	}
}

func TestParseAccessTokenInvalid(t *testing.T) {
	if _, err := security.ParseAccessToken("secret", "not-a-jwt"); err == nil {
		t.Fatal("expected error for invalid token")
	}
}
