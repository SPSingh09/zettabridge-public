package publisher

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
)

// ── isTerminal ───────────────────────────────────────────────────────────────

func TestIsTerminal(t *testing.T) {
	terminal := []string{"user_returned", "user_cancelled", "expired"}
	nonTerminal := []string{"created", "awaiting_confirmation", "", "unknown"}

	for _, s := range terminal {
		if !isTerminal(s) {
			t.Errorf("isTerminal(%q) = false, want true", s)
		}
	}
	for _, s := range nonTerminal {
		if isTerminal(s) {
			t.Errorf("isTerminal(%q) = true, want false", s)
		}
	}
}

// ── Callback HTTP gate tests ──────────────────────────────────────────────────

func newCallbackApp(secret string) *fiber.App {
	app := fiber.New()
	h := &Handler{Handler: &shared.Handler{
		Cfg: &config.Config{JWTSecret: secret, FrontendURL: "https://app.example.com"},
	}}
	app.Get("/v1/publisher/callback", h.Callback)
	return app
}

// When state is absent, the handler falls back to a DB lookup.
// In this test h.PG is nil (no store configured), so the guard returns 500.
func TestCallback_MissingState_FallsBackToStore(t *testing.T) {
	app := newCallbackApp("test-secret")

	req, _ := http.NewRequest("GET", "/v1/publisher/callback", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status=%d, want 500 (no store configured)", resp.StatusCode)
	}
}

func TestCallback_InvalidBase64State(t *testing.T) {
	app := newCallbackApp("test-secret")

	req, _ := http.NewRequest("GET", "/v1/publisher/callback?state=!!!not-base64!!!", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status=%d, want 400", resp.StatusCode)
	}
}

func TestCallback_InvalidHMAC(t *testing.T) {
	app := newCallbackApp("server-secret")

	// State was signed with a different secret
	state := buildTestState("order-123", "9999999999", "wrong-secret")
	req, _ := http.NewRequest("GET", "/v1/publisher/callback?state="+state, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status=%d, want 400", resp.StatusCode)
	}
}

func TestCallback_ExpiredState(t *testing.T) {
	const secret = "test-secret"
	app := newCallbackApp(secret)

	// Build a correctly-signed state with a Unix epoch expiry (always expired).
	state := buildTestState("order-123", "1000", secret)
	req, _ := http.NewRequest("GET", "/v1/publisher/callback?state="+state, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status=%d, want 400", resp.StatusCode)
	}
}

func TestCallback_EmptyOrderID(t *testing.T) {
	const secret = "test-secret"
	app := newCallbackApp(secret)

	// Valid HMAC and non-expired, but empty order ID
	state := buildTestState("", "9999999999", secret)
	req, _ := http.NewRequest("GET", "/v1/publisher/callback?state="+state, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status=%d, want 400", resp.StatusCode)
	}
}

// buildTestState constructs a raw publisher callback state token for testing.
// It mirrors the internal format: base64(orderID|expiresUnix|hmac).
func buildTestState(orderID, expiresUnix, secret string) string {
	payload := fmt.Sprintf("%s|%s", orderID, expiresUnix)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	raw := payload + "|" + sig
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
