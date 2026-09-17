// Package publisher handles Zerodha Kite Publisher order flow.
// State tokens are used to safely correlate Kite's callback with internal publisher orders.
package publisher

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	stateMaxAge    = 30 * time.Minute
	stateSep       = "|"
	statePartCount = 3 // publisherOrderID | expiresUnix | hmac
)

// GenerateState returns a URL-safe, HMAC-signed state token embedding the
// publisher order ID and a 30-minute expiry. Verified by ParseState on callback.
func GenerateState(publisherOrderID, secret string) string {
	expires := strconv.FormatInt(time.Now().Add(stateMaxAge).Unix(), 10)
	payload := strings.Join([]string{publisherOrderID, expires}, stateSep)
	sig := signHMAC(payload, secret)
	raw := payload + stateSep + sig
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ParseState decodes and verifies a publisher callback state token.
// Returns the embedded publisherOrderID, or an error if the token is invalid or expired.
func ParseState(state, secret string) (publisherOrderID string, err error) {
	b, decErr := base64.RawURLEncoding.DecodeString(state)
	if decErr != nil {
		return "", fmt.Errorf("publisher state: invalid encoding")
	}
	parts := strings.Split(string(b), stateSep)
	if len(parts) != statePartCount {
		return "", fmt.Errorf("publisher state: invalid format")
	}
	orderID, expires, sig := parts[0], parts[1], parts[2]
	payload := strings.Join([]string{orderID, expires}, stateSep)
	if sig != signHMAC(payload, secret) {
		return "", fmt.Errorf("publisher state: invalid signature")
	}
	expiresInt, parseErr := strconv.ParseInt(expires, 10, 64)
	if parseErr != nil {
		return "", fmt.Errorf("publisher state: invalid expiry")
	}
	if time.Now().Unix() > expiresInt {
		return "", fmt.Errorf("publisher state: token expired")
	}
	if strings.TrimSpace(orderID) == "" {
		return "", fmt.Errorf("publisher state: empty order ID")
	}
	return orderID, nil
}

func signHMAC(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
