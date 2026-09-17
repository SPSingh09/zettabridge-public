package marketdata

import (
	"context"
	"log"
	"strings"
)

// SettingsReader reads runtime platform_settings (same surface SnapshotJob uses).
type SettingsReader interface {
	GetPlatformSetting(ctx context.Context, key string) (value string, ok bool, err error)
}

// ActiveProviderName returns the configured provider name, falling back to the
// registry default when unset or unrecognized.
func (r *Registry) ActiveProviderName(ctx context.Context, settings SettingsReader) string {
	if r == nil {
		return ""
	}
	name, ok, err := settings.GetPlatformSetting(ctx, SettingKeyProvider)
	if err != nil {
		log.Printf("marketdata: read %s setting failed, using default: %v", SettingKeyProvider, err)
	}
	if ok && name != "" {
		if _, exists := r.providers[name]; exists {
			return name
		}
	}
	return r.fallback
}

// ActiveProvider resolves the currently configured provider from platform_settings.
func (r *Registry) ActiveProvider(ctx context.Context, settings SettingsReader) Provider {
	if r == nil {
		return nil
	}
	return r.Resolve(r.ActiveProviderName(ctx, settings))
}

// ResolveMarketFillPrice fetches LTP for a paper MARKET fill when the admin
// has switched off SignalPriceProvider. Returns ErrNoQuote when the active
// provider is "signal" or no quote is available right now.
func (r *Registry) ResolveMarketFillPrice(ctx context.Context, settings SettingsReader, exchange, symbol string) (float64, error) {
	if r == nil || r.ActiveProviderName(ctx, settings) == ProviderSignal {
		return 0, ErrNoQuote
	}
	exchange = strings.ToUpper(strings.TrimSpace(exchange))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	quote, err := r.ActiveProvider(ctx, settings).GetLTP(ctx, Symbol{Exchange: exchange, Symbol: symbol})
	if err != nil {
		return 0, err
	}
	if quote == nil || quote.LTP <= 0 {
		return 0, ErrNoQuote
	}
	return quote.LTP, nil
}
