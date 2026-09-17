package marketdata

import "context"

// SignalPriceProvider is the default Provider: it never fetches an external
// price. Paper positions only reprice from incoming ORDER_SIGNAL fills or
// PRICE_UPDATE signals (Phase 5's rule), so every lookup here is a
// deliberate, permanent no-op — not a missing-configuration error.
type SignalPriceProvider struct{}

func (SignalPriceProvider) GetLTP(_ context.Context, _ Symbol) (*Quote, error) {
	return nil, ErrNoQuote
}

func (SignalPriceProvider) GetBatchLTP(_ context.Context, _ []Symbol) ([]Quote, error) {
	return nil, ErrNoQuote
}
