// Package marketdata defines the LTP (last-traded-price) provider interface
// used by the paper trading mark-to-market background job (Phase 6 of
// DEMO_EXECUTION_PLAN.md). SignalPriceProvider is the default — paper
// positions only reprice on incoming signals, same as Phase 5 — and real
// external feeds (e.g. fyers.Provider) plug in behind the same interface
// without any change to the background job itself.
package marketdata

import (
	"context"
	"errors"
)

// ErrNoQuote is returned when a provider has no new price to report for a
// symbol right now. This is an expected, non-fatal outcome — not a runtime
// failure — for both SignalPriceProvider (permanent, by design: it never
// fetches external prices) and an unconfigured real provider (temporary:
// e.g. fyers.Provider before FYERS_API_KEY is set). Callers should keep the
// position's existing last_price unchanged when they see this error.
var ErrNoQuote = errors.New("marketdata: no quote available")

// Symbol identifies a tradable instrument for an LTP lookup.
type Symbol struct {
	Exchange string
	Symbol   string
}

// Quote is a single last-traded-price observation.
type Quote struct {
	Symbol Symbol
	LTP    float64
}

// Provider fetches last-traded-price quotes for the mark-to-market
// background job. Implementations: SignalPriceProvider (default) and
// fyers.Provider (real feed, currently a credential-gated stub).
type Provider interface {
	GetLTP(ctx context.Context, symbol Symbol) (*Quote, error)
	GetBatchLTP(ctx context.Context, symbols []Symbol) ([]Quote, error)
}
