package handler

import (
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestNormalizeAccountEmail(t *testing.T) {
	got, err := normalizeAccountEmail("  Trader@Example.COM ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "trader@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestLoginEmailMatchesRegisterNormalization(t *testing.T) {
	registered, err := normalizeAccountEmail("Trader@Example.COM")
	if err != nil {
		t.Fatal(err)
	}
	loginLookup, err := normalizeAccountEmail("  trader@EXAMPLE.com  ")
	if err != nil {
		t.Fatal(err)
	}
	if loginLookup != registered {
		t.Fatalf("login lookup %q != registered %q", loginLookup, registered)
	}
}

func TestLoginBlockedByEmailVerification(t *testing.T) {
	cfg := &config.Config{EmailVerificationRequired: true}
	user := &store.User{}
	if !loginBlockedByEmailVerification(cfg, user) {
		t.Fatal("expected unverified user blocked")
	}
	now := time.Now()
	user.EmailVerifiedAt = &now
	if loginBlockedByEmailVerification(cfg, user) {
		t.Fatal("expected verified user allowed")
	}
	cfg.EmailVerificationRequired = false
	user.EmailVerifiedAt = nil
	if loginBlockedByEmailVerification(cfg, user) {
		t.Fatal("expected gate disabled")
	}
}

func TestUserEmailVerifiedFields(t *testing.T) {
	verified, at := userEmailVerifiedFields(&store.User{})
	if verified || at != nil {
		t.Fatal("expected unverified")
	}
	now := time.Now()
	verified, at = userEmailVerifiedFields(&store.User{EmailVerifiedAt: &now})
	if !verified || at == nil {
		t.Fatal("expected verified fields")
	}
}

func TestVerificationURL(t *testing.T) {
	h := &Handler{cfg: &config.Config{AppPublicURL: "https://api.example.com"}}
	got := h.verificationURL("ev_abc")
	want := "https://api.example.com/v1/auth/verify-email?token=ev_abc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
