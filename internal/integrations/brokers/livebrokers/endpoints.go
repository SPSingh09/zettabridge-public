package livebrokers

import (
	"strings"
)

const (
	accountDemo = "demo"
	accountLive = "live"

	defaultMT5BaseURL     = "https://mt-client-api-v1.london.agiliumtrade.ai"
	defaultZerodhaBaseURL = "https://api.kite.trade"
	defaultAngelBaseURL   = "https://apiconnect.angelone.in"
	defaultDhanBaseURL    = "https://api.dhan.co"
)

// URLs holds configurable broker API base URLs (live and optional demo overrides).
type URLs struct {
	MT5Live     string
	MT5Demo     string
	ZerodhaLive string
	ZerodhaDemo string
	AngelLive   string
	AngelDemo   string
	DhanLive    string
	DhanDemo    string
}

// MT5DemoConfigured reports whether MT5 paper trading via MetaApi demo server is available.
func (u URLs) MT5DemoConfigured() bool {
	return trimBase(u.MT5Demo) != ""
}

// HasDemoURL reports whether a paper-trading base URL is configured for brokerType.
// Indian brokers (zerodha, angel, dhan) have no official paper-trading API, so this
// returns false unless an operator has explicitly set a demo URL override.
func (u URLs) HasDemoURL(brokerType string) bool {
	switch brokerType {
	case "mt5_cloud":
		return trimBase(u.MT5Demo) != ""
	case "zerodha":
		return trimBase(u.ZerodhaDemo) != ""
	case "angel":
		return trimBase(u.AngelDemo) != ""
	case "dhan":
		return trimBase(u.DhanDemo) != ""
	default:
		return false
	}
}

// BaseURL returns the API root for brokerType and account_mode (demo | live).
func (u URLs) BaseURL(brokerType, accountMode string) string {
	mode := normalizeAccountMode(accountMode)
	switch brokerType {
	case "mt5_cloud":
		if mode == accountDemo && u.MT5Demo != "" {
			return trimBase(u.MT5Demo)
		}
		if u.MT5Live != "" {
			return trimBase(u.MT5Live)
		}
		return defaultMT5BaseURL
	case "zerodha":
		if mode == accountDemo && u.ZerodhaDemo != "" {
			return trimBase(u.ZerodhaDemo)
		}
		if u.ZerodhaLive != "" {
			return trimBase(u.ZerodhaLive)
		}
		return defaultZerodhaBaseURL
	case "angel":
		if mode == accountDemo && u.AngelDemo != "" {
			return trimBase(u.AngelDemo)
		}
		if u.AngelLive != "" {
			return trimBase(u.AngelLive)
		}
		return defaultAngelBaseURL
	case "dhan":
		if mode == accountDemo && u.DhanDemo != "" {
			return trimBase(u.DhanDemo)
		}
		if u.DhanLive != "" {
			return trimBase(u.DhanLive)
		}
		return defaultDhanBaseURL
	default:
		return ""
	}
}

func trimBase(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}

func normalizeAccountMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), accountLive) {
		return accountLive
	}
	return accountDemo
}
