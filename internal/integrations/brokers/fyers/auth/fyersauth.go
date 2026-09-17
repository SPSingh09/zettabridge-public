// Package fyersauth handles the FYERS API v3 OAuth login/token-exchange flow
// for ZettaBridge's single, platform-wide, admin-managed market-data
// connection (never used for order placement).
//
// Endpoint paths and request/response shapes below (generate-authcode,
// validate-authcode, appIdHash computation, the /data/quotes endpoint) are
// taken directly from FYERS's own official `fyers-apiv3` Python SDK source
// (fyersModel.py, Config class) — not guessed from third-party docs.
//
// The refresh-token flow (validate-refresh-token) is NOT implemented in
// FYERS's official SDK at all. Its endpoint/field names here are inferred
// from community reports, not verified against an official source — treat
// RefreshAccessToken as best-effort; a failure here is expected to be
// handled by falling back to the admin re-login flow (see
// internal/integrations/brokers/fyers/refresh).
package fyersauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// apiBase is a var (not const) so tests can point it at an httptest server.
var apiBase = "https://api-t1.fyers.in/api/v3"

// SetAPIBaseForTest points apiBase at a test server and returns a function
// that restores the real FYERS base URL. Test-only — never call in production.
func SetAPIBaseForTest(url string) (restore func()) {
	prev := apiBase
	apiBase = url
	return func() { apiBase = prev }
}

const (
	authPath      = "/generate-authcode"
	tokenPath     = "/validate-authcode"
	refreshPath   = "/validate-refresh-token" // best-effort, see package docs
	stateMaxAge   = 15 * time.Minute
	stateSep      = "|"
	statePartCount = 3 // userID | timestamp | hmac

	// AccessTokenValidity and RefreshTokenValidity are FYERS's documented
	// (community-reported, not SDK-enforced) token lifetimes — used only to
	// compute expires_at for our own bookkeeping; FYERS's actual server-side
	// expiry is authoritative regardless of what we compute here.
	AccessTokenValidity  = 24 * time.Hour
	RefreshTokenValidity = 15 * 24 * time.Hour
)

// GenerateState returns a signed, URL-safe state token embedding the admin
// userID who initiated the connection. Verified by ParseState.
func GenerateState(adminUserID, secret string) string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := strings.Join([]string{adminUserID, ts}, stateSep)
	sig := signHMAC(payload, secret)
	raw := payload + stateSep + sig
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ParseState verifies the state token and returns the admin userID that
// initiated the connection.
func ParseState(state, secret string) (adminUserID string, err error) {
	b, decErr := base64.RawURLEncoding.DecodeString(state)
	if decErr != nil {
		return "", fmt.Errorf("invalid state encoding")
	}
	parts := strings.Split(string(b), stateSep)
	if len(parts) != statePartCount {
		return "", fmt.Errorf("invalid state format")
	}
	uid, ts, sig := parts[0], parts[1], parts[2]
	payload := strings.Join([]string{uid, ts}, stateSep)
	if sig != signHMAC(payload, secret) {
		return "", fmt.Errorf("invalid state signature")
	}
	tsInt, parseErr := strconv.ParseInt(ts, 10, 64)
	if parseErr != nil {
		return "", fmt.Errorf("invalid state timestamp")
	}
	if time.Since(time.Unix(tsInt, 0)) > stateMaxAge {
		return "", fmt.Errorf("state token expired")
	}
	return uid, nil
}

// LoginURL builds the FYERS OAuth login URL. Verified against the official
// SDK's SessionModel.generate_authcode().
func LoginURL(appID, redirectURI, state string) string {
	params := url.Values{
		"client_id":     {appID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"state":         {state},
	}
	return apiBase + authPath + "?" + params.Encode()
}

// appIDHash computes appIdHash = sha256(appID + ":" + secretID), exactly as
// FYERS's SDK does (SessionModel.get_hash()).
func appIDHash(appID, secretID string) string {
	h := sha256.Sum256([]byte(appID + ":" + secretID))
	return hex.EncodeToString(h[:])
}

// TokenResult carries what FYERS returns from a successful token exchange
// or refresh.
type TokenResult struct {
	AccessToken  string
	RefreshToken string
}

// ExchangeAuthCode exchanges an OAuth auth_code for an access_token (and
// refresh_token, if FYERS returns one — the official SDK's response shape
// isn't fully documented for refresh_token, so it's read defensively).
func ExchangeAuthCode(appID, secretID, authCode string) (*TokenResult, error) {
	body := map[string]string{
		"grant_type": "authorization_code",
		"appIdHash":  appIDHash(appID, secretID),
		"code":       authCode,
	}
	return doTokenRequest(apiBase+tokenPath, body)
}

// RefreshAccessToken attempts FYERS's undocumented (community-reported)
// silent refresh flow. Best-effort — see package docs. Callers should treat
// any error here as "refresh unavailable, fall back to admin re-login."
func RefreshAccessToken(appID, secretID, refreshToken, pin string) (*TokenResult, error) {
	body := map[string]string{
		"grant_type":    "refresh_token",
		"appIdHash":     appIDHash(appID, secretID),
		"refresh_token": refreshToken,
		"pin":           pin,
	}
	return doTokenRequest(apiBase+refreshPath, body)
}

func doTokenRequest(url string, body map[string]string) (*TokenResult, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fyers unreachable: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fyers %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		S            string `json:"s"`
		Message      string `json:"message"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("fyers parse error: %w", err)
	}
	if result.S != "ok" || result.AccessToken == "" {
		msg := result.Message
		if msg == "" {
			msg = string(respBody)
		}
		return nil, fmt.Errorf("fyers: %s", msg)
	}
	return &TokenResult{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken}, nil
}

func signHMAC(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
