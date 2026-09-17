package security

import (
	"testing"
	"time"
)

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("short"); err == nil {
		t.Fatal("expected error for short password")
	}
	if err := ValidatePassword("longenough"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHashRefreshTokenStable(t *testing.T) {
	a := HashRefreshToken("rt_abc")
	b := HashRefreshToken("rt_abc")
	if a != b || a == "" {
		t.Fatalf("expected stable non-empty hash")
	}
}

func TestIssueAccessToken(t *testing.T) {
	token, exp, err := IssueAccessToken("test-secret-min-32-chars!!!!!!!!", "user-1", time.Hour)
	if err != nil || token == "" || exp != 3600 {
		t.Fatalf("token=%q exp=%d err=%v", token, exp, err)
	}
}
