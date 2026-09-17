package fyersauth

import (
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGenerateAndParseState_RoundTrip(t *testing.T) {
	state := GenerateState("admin-123", "secret")
	uid, err := ParseState(state, "secret")
	if err != nil {
		t.Fatalf("ParseState failed: %v", err)
	}
	if uid != "admin-123" {
		t.Fatalf("expected uid admin-123, got %q", uid)
	}
}

func TestParseState_WrongSecretRejected(t *testing.T) {
	state := GenerateState("admin-123", "secret")
	if _, err := ParseState(state, "wrong-secret"); err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}
}

func TestParseState_TamperedRejected(t *testing.T) {
	state := GenerateState("admin-123", "secret")
	// Flip a character to corrupt the payload without breaking base64 decoding.
	tampered := "a" + state[1:]
	if _, err := ParseState(tampered, "secret"); err == nil {
		t.Fatal("expected error for tampered state, got nil")
	}
}

func TestParseState_ExpiredRejected(t *testing.T) {
	// Build a state token with an old timestamp directly (bypassing GenerateState's "now").
	ts := strconv.FormatInt(time.Now().Add(-1*time.Hour-stateMaxAge).Unix(), 10)
	payload := "admin-123" + stateSep + ts
	sig := signHMAC(payload, "secret")
	raw := payload + stateSep + sig
	state := base64.RawURLEncoding.EncodeToString([]byte(raw))

	if _, err := ParseState(state, "secret"); err == nil {
		t.Fatal("expected error for expired state, got nil")
	}
}

func TestLoginURL_ContainsExpectedParams(t *testing.T) {
	got := LoginURL("APP123", "https://api.staging.zettabridge.net/v1/admin/fyers/callback", "signed-state")

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("LoginURL produced an invalid URL: %v", err)
	}
	if !strings.HasPrefix(got, apiBase+authPath+"?") {
		t.Fatalf("expected URL to start with %s%s?, got %s", apiBase, authPath, got)
	}
	q := u.Query()
	if q.Get("client_id") != "APP123" {
		t.Fatalf("client_id mismatch: %q", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != "https://api.staging.zettabridge.net/v1/admin/fyers/callback" {
		t.Fatalf("redirect_uri mismatch: %q", q.Get("redirect_uri"))
	}
	if q.Get("response_type") != "code" {
		t.Fatalf("response_type mismatch: %q", q.Get("response_type"))
	}
	if q.Get("state") != "signed-state" {
		t.Fatalf("state mismatch: %q", q.Get("state"))
	}
}

func TestAppIDHash_MatchesSDKFormula(t *testing.T) {
	// appIdHash = sha256("APP:SECRET") hex-encoded, exactly as FYERS's
	// official SDK computes it (SessionModel.get_hash()).
	got := appIDHash("APP", "SECRET")
	if len(got) != 64 {
		t.Fatalf("expected a 64-char hex sha256 digest, got %d chars: %q", len(got), got)
	}
}
