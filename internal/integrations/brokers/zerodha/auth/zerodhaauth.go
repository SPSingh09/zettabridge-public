// Package zerodhaauth handles Kite Connect OAuth state tokens and token exchange.
// Each user registers their own Kite Connect app and supplies their own API key and secret.
package zerodhaauth

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

const (
	kiteLoginBase  = "https://kite.zerodha.com/connect/login"
	kiteSessionURL = "https://api.kite.trade/session/token"
	stateMaxAge    = 15 * time.Minute
	stateSep       = "|"
	statePartCount = 5 // userID | credentialID | configJSON(b64) | timestamp | hmac
)

// ConnectConfig carries the credential settings the user configured on the form
// before the OAuth redirect so they survive the Kite round-trip.
type ConnectConfig struct {
	AccountLabel     string  `json:"label,omitempty"`
	AccountMode      string  `json:"mode,omitempty"`
	Exchange         string  `json:"exchange,omitempty"`
	Product          string  `json:"product,omitempty"`
	OrderType        string  `json:"order_type,omitempty"`
	MarketProtection float64 `json:"market_protection,omitempty"`
	AlgoID           string  `json:"algo_id,omitempty"`
	OrgID            string  `json:"org_id,omitempty"` // set when user is an org member (household plan)
}

// GenerateState returns a signed, URL-safe state token embedding userID,
// an optional credentialID (for reconnect flows), and an optional ConnectConfig.
// Verified by ParseState.
func GenerateState(userID, credentialID string, cfg *ConnectConfig, secret string) string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	extra := ""
	if cfg != nil {
		if b, err := json.Marshal(cfg); err == nil {
			extra = base64.RawURLEncoding.EncodeToString(b)
		}
	}
	payload := strings.Join([]string{userID, credentialID, extra, ts}, stateSep)
	sig := signHMAC(payload, secret)
	raw := payload + stateSep + sig
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ParseState verifies the state token and returns userID, credentialID, and
// any ConnectConfig encoded in the state. Returns an error if the signature is
// wrong or the token has expired.
func ParseState(state, secret string) (userID, credentialID string, cfg *ConnectConfig, err error) {
	b, decErr := base64.RawURLEncoding.DecodeString(state)
	if decErr != nil {
		return "", "", nil, fmt.Errorf("invalid state encoding")
	}
	parts := strings.Split(string(b), stateSep)
	if len(parts) != statePartCount {
		return "", "", nil, fmt.Errorf("invalid state format")
	}
	uid, cid, extra, ts, sig := parts[0], parts[1], parts[2], parts[3], parts[4]
	payload := strings.Join([]string{uid, cid, extra, ts}, stateSep)
	if sig != signHMAC(payload, secret) {
		return "", "", nil, fmt.Errorf("invalid state signature")
	}
	tsInt, parseErr := strconv.ParseInt(ts, 10, 64)
	if parseErr != nil {
		return "", "", nil, fmt.Errorf("invalid state timestamp")
	}
	if time.Since(time.Unix(tsInt, 0)) > stateMaxAge {
		return "", "", nil, fmt.Errorf("state token expired")
	}
	if extra != "" {
		if raw, decErr := base64.RawURLEncoding.DecodeString(extra); decErr == nil {
			var c ConnectConfig
			if json.Unmarshal(raw, &c) == nil {
				cfg = &c
			}
		}
	}
	return uid, cid, cfg, nil
}

// LoginURL builds the Kite OAuth login URL for the ZettaBridge app.
// Kite does not support the standard OAuth `state` param directly — it forwards
// arbitrary parameters via `redirect_params` (URL-encoded key=value string).
// The callback will receive those params unpacked as individual query params.
func LoginURL(apiKey, state string) string {
	redirectParams := url.Values{"state": {state}}.Encode()
	params := url.Values{
		"api_key":         {apiKey},
		"v":               {"3"},
		"redirect_params": {redirectParams},
	}
	return kiteLoginBase + "?" + params.Encode()
}

// ExchangeToken calls Kite Connect to exchange a request_token for an access_token.
// checksum = SHA-256(api_key + request_token + api_secret) as required by Kite.
func ExchangeToken(apiKey, apiSecret, requestToken string) (string, error) {
	checksum := sha256Hex(apiKey + requestToken + apiSecret)
	form := url.Values{
		"api_key":       {apiKey},
		"request_token": {requestToken},
		"checksum":      {checksum},
	}
	req, err := http.NewRequest(http.MethodPost, kiteSessionURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Kite-Version", "3")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("kite connect unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kite connect %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Status string `json:"status"`
		Data   struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("kite connect parse error: %w", err)
	}
	if result.Status != "success" || result.Data.AccessToken == "" {
		return "", fmt.Errorf("kite connect: no access_token in response")
	}
	return result.Data.AccessToken, nil
}

func signHMAC(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
