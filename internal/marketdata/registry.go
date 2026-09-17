package marketdata

// Known provider names — the single source of truth used by app.go (to
// build the registry), the admin API (to validate a requested provider and
// advertise the available list), and SnapshotJob (as the safe fallback).
const (
	ProviderSignal = "signal"
	ProviderFyers  = "fyers"
)

// KnownProviders lists every registered provider name, in a stable order.
var KnownProviders = []string{ProviderSignal, ProviderFyers}

// SettingKeyProvider is the platform_settings key the admin market-data-
// provider toggle reads/writes.
const SettingKeyProvider = "market_data_provider"

// SettingKeyIntervalSec is the platform_settings key the admin market-data
// fetch-interval toggle reads/writes — SnapshotJob re-reads it before every
// sweep cycle, same no-restart pattern as SettingKeyProvider.
const SettingKeyIntervalSec = "market_data_snapshot_interval_sec"

// MinSnapshotIntervalSec/MaxSnapshotIntervalSec bound the admin-settable
// sweep cadence: floor guards against hammering the upstream quote provider
// (and FYERS's rate limits) from a mistyped setting; ceiling keeps paper
// account marks from going stale for more than an hour.
const (
	MinSnapshotIntervalSec = 5
	MaxSnapshotIntervalSec = 3600
)

// Registry resolves a provider by name, with a fallback for an unset or
// unrecognized name. Built once at startup (internal/app/app.go) with every
// compiled-in provider; which one is *active* can change at runtime via the
// admin-configurable platform_settings row SnapshotJob re-reads on every sweep.
type Registry struct {
	providers map[string]Provider
	fallback  string
}

// NewRegistry returns a Registry. fallback must be a key present in providers.
func NewRegistry(fallback string, providers map[string]Provider) *Registry {
	return &Registry{providers: providers, fallback: fallback}
}

// Resolve returns the provider for name, or the fallback provider if name is
// empty or unrecognized.
func (r *Registry) Resolve(name string) Provider {
	if p, ok := r.providers[name]; ok {
		return p
	}
	return r.providers[r.fallback]
}
