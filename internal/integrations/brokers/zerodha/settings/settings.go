// Package settings provides admin-runtime enable/disable checks for the two
// Zerodha execution paths (OAuth login and Kite Publisher), backed by the
// platform_settings key-value table. Both default to enabled (fail-open)
// when the row is absent, matching guard.MarketHoursEnforcementSettingKey.
package settings

import "context"

const (
	// OAuthEnabledSettingKey gates Zerodha Kite Connect OAuth login and
	// order execution for user_api_oauth/direct_api credentials.
	OAuthEnabledSettingKey = "zerodha_oauth_enabled"
	// PublisherEnabledSettingKey gates Zerodha Kite Publisher credential
	// creation and order execution.
	PublisherEnabledSettingKey = "zerodha_publisher_enabled"
)

// Store is the narrow read set these checks need — satisfied structurally
// by *store.PGStore, so callers can pass it in without importing internal/store.
type Store interface {
	GetPlatformSetting(ctx context.Context, key string) (value string, ok bool, err error)
}

// OAuthEnabled reports whether Zerodha OAuth login/order execution is
// currently enabled. Fails open (true) when unset or unreadable.
func OAuthEnabled(ctx context.Context, s Store) bool {
	return enabled(ctx, s, OAuthEnabledSettingKey)
}

// PublisherEnabled reports whether Zerodha Kite Publisher credential
// creation/order execution is currently enabled. Fails open (true) when
// unset or unreadable.
func PublisherEnabled(ctx context.Context, s Store) bool {
	return enabled(ctx, s, PublisherEnabledSettingKey)
}

func enabled(ctx context.Context, s Store, key string) bool {
	v, ok, err := s.GetPlatformSetting(ctx, key)
	if err != nil || !ok {
		return true
	}
	return v == "true"
}
