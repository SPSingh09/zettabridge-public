package marketdata

import (
	"context"
	"testing"
)

type stubSettings struct {
	values map[string]string
}

func (s stubSettings) GetPlatformSetting(_ context.Context, key string) (string, bool, error) {
	v, ok := s.values[key]
	return v, ok, nil
}

type fixedQuoteProvider struct {
	ltp float64
}

func (p fixedQuoteProvider) GetLTP(_ context.Context, _ Symbol) (*Quote, error) {
	return &Quote{LTP: p.ltp}, nil
}

func (p fixedQuoteProvider) GetBatchLTP(ctx context.Context, symbols []Symbol) ([]Quote, error) {
	q, err := p.GetLTP(ctx, symbols[0])
	if err != nil {
		return nil, err
	}
	return []Quote{*q}, nil
}

func TestResolveMarketFillPrice_SignalProviderRequiresSignalPrice(t *testing.T) {
	reg := NewRegistry(ProviderSignal, map[string]Provider{
		ProviderSignal: SignalPriceProvider{},
		ProviderFyers:  fixedQuoteProvider{ltp: 850},
	})
	_, err := reg.ResolveMarketFillPrice(context.Background(), stubSettings{
		values: map[string]string{SettingKeyProvider: ProviderSignal},
	}, "NSE", "SBIN")
	if err != ErrNoQuote {
		t.Fatalf("expected ErrNoQuote, got %v", err)
	}
}

func TestResolveMarketFillPrice_FyersProviderReturnsLTP(t *testing.T) {
	reg := NewRegistry(ProviderSignal, map[string]Provider{
		ProviderSignal: SignalPriceProvider{},
		ProviderFyers:  fixedQuoteProvider{ltp: 850.5},
	})
	ltp, err := reg.ResolveMarketFillPrice(context.Background(), stubSettings{
		values: map[string]string{SettingKeyProvider: ProviderFyers},
	}, "nse", "sbin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ltp != 850.5 {
		t.Fatalf("expected 850.5, got %v", ltp)
	}
}
