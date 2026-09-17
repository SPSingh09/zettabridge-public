package brokercreds

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidFormat = errors.New("invalid credential format")
	ErrEmptyField    = errors.New("credential field is empty")
)

// Parsed holds decrypted credential fields per broker_type.
type Parsed struct {
	BrokerType string

	// mt5_cloud
	AuthToken string
	AccountID string

	// zerodha:
	//   publisher mode     — api_key only (1 part)
	//   pre-OAuth mode     — api_key:api_secret (2 parts)
	//   post-OAuth mode    — api_key:api_secret:access_token (3 parts)
	APIKey      string
	APISecret   string
	AccessToken string

	// angel
	ClientCode string
	JWT        string

	// dhan
	ClientID string
	// AccessToken shared with zerodha/dhan
}

// Validate checks raw_creds structure for brokerType without decrypting.
func Validate(brokerType, raw string) error {
	_, err := Parse(brokerType, raw)
	return err
}

// Parse splits colon-delimited raw_creds. Values must not contain ':'.
func Parse(brokerType, raw string) (Parsed, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Parsed{}, ErrEmptyField
	}
	parts := strings.Split(raw, ":")
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return Parsed{}, ErrInvalidFormat
		}
	}

	switch brokerType {
	case "zerodha":
		if len(parts) == 1 {
			// publisher mode: api_key only — no OAuth secret or access token needed
			return Parsed{BrokerType: brokerType, APIKey: parts[0]}, nil
		}
		if len(parts) == 2 {
			// api_key:api_secret (pre-OAuth; access_token not yet exchanged)
			return Parsed{BrokerType: brokerType, APIKey: parts[0], APISecret: parts[1]}, nil
		}
		if len(parts) == 3 {
			// api_key:api_secret:access_token (post-OAuth)
			return Parsed{BrokerType: brokerType, APIKey: parts[0], APISecret: parts[1], AccessToken: parts[2]}, nil
		}
		return Parsed{}, fmt.Errorf("%w: zerodha publisher requires api_key; api oauth requires api_key:api_secret or api_key:api_secret:access_token", ErrInvalidFormat)
	case "mt5_cloud":
		if len(parts) != 2 {
			return Parsed{}, fmt.Errorf("%w: mt5_cloud requires auth_token:account_id", ErrInvalidFormat)
		}
		return Parsed{BrokerType: brokerType, AuthToken: parts[0], AccountID: parts[1]}, nil
	case "dhan":
		if len(parts) != 2 {
			return Parsed{}, fmt.Errorf("%w: dhan requires client_id:access_token", ErrInvalidFormat)
		}
		return Parsed{BrokerType: brokerType, ClientID: parts[0], AccessToken: parts[1]}, nil
	case "angel":
		if len(parts) != 3 {
			return Parsed{}, fmt.Errorf("%w: angel requires api_key:client_code:jwt", ErrInvalidFormat)
		}
		return Parsed{
			BrokerType: brokerType,
			APIKey:     parts[0],
			ClientCode: parts[1],
			JWT:        parts[2],
		}, nil
	default:
		return Parsed{}, fmt.Errorf("unsupported broker_type %q", brokerType)
	}
}

// FormatHint returns a human-readable raw_creds schema for API errors.
// For Zerodha, assumes user_api_oauth mode. Use FormatHintForMode when execution_mode is known.
func FormatHint(brokerType string) string {
	return FormatHintForMode(brokerType, "")
}

// FormatHintForMode returns a mode-aware raw_creds schema for API errors.
func FormatHintForMode(brokerType, executionMode string) string {
	switch brokerType {
	case "zerodha":
		if executionMode == "publisher" {
			return "api_key"
		}
		return "api_key:api_secret"
	case "mt5_cloud":
		return "auth_token:account_id"
	case "dhan":
		return "client_id:access_token"
	case "angel":
		return "api_key:client_code:jwt"
	default:
		return "unknown broker"
	}
}
