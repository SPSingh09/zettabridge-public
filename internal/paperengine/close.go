package paperengine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/marketdata"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// PositionCloserStore is the store surface FlattenPosition needs beyond the
// paper fill transaction (instrument/profile lookup and platform settings for
// live quote resolution).
type PositionCloserStore interface {
	GetInstrument(ctx context.Context, marketProfileCode, exchange, symbol string) (*store.Instrument, error)
	GetMarketProfileByCode(ctx context.Context, code string) (*store.MarketProfile, error)
	GetPlatformSetting(ctx context.Context, key string) (value string, ok bool, err error)
}

// FlattenPosition closes an open paper position at market using the position's
// own exchange/product keys. When enforceMarketHours is false (MIS auto
// square-off), trading-hours guards are skipped so positions can be flattened
// during the exchange square-off window.
func FlattenPosition(
	ctx context.Context,
	engine domain.ExecutionDestination,
	md *marketdata.Registry,
	pg PositionCloserStore,
	account *store.PaperAccount,
	position *store.PaperPosition,
	signalID string,
	enforceMarketHours bool,
) (*domain.ExecutionResult, error) {
	if position == nil || position.Quantity == 0 {
		return nil, fmt.Errorf("no open position to close")
	}
	if strings.TrimSpace(signalID) == "" {
		return nil, fmt.Errorf("signal id is required")
	}

	exchange := store.NormalizePaperExchange(position.Exchange)
	if exchange == "" {
		exchange = store.NormalizePaperExchange(account.Exchange)
	}
	symbol := store.NormalizePaperSymbol(position.Symbol)
	product := strings.TrimSpace(position.Product)
	if product == "" {
		product = account.DefaultProduct
	}

	fillPrice, err := resolveCloseFillPrice(ctx, md, pg, exchange, symbol, position.LastPrice)
	if err != nil {
		return nil, err
	}

	instrument, err := pg.GetInstrument(ctx, account.MarketProfile, exchange, symbol)
	if err != nil {
		return nil, fmt.Errorf("instrument lookup failed: %w", err)
	}
	if instrument == nil || !instrument.Active {
		return nil, fmt.Errorf("unknown or inactive symbol %q for market profile %q", symbol, account.MarketProfile)
	}
	if strings.TrimSpace(instrument.Symbol) != "" {
		symbol = store.NormalizePaperSymbol(instrument.Symbol)
	}

	profile, err := pg.GetMarketProfileByCode(ctx, account.MarketProfile)
	if err != nil {
		return nil, fmt.Errorf("market profile lookup failed: %w", err)
	}
	if profile == nil {
		return nil, fmt.Errorf("unknown market profile %q", account.MarketProfile)
	}

	if enforceMarketHours {
		if reason := guard.CheckMarketHours(ctx, pg, account.MarketProfile, time.Now().UTC()); reason != "" {
			return nil, fmt.Errorf("%s", reason)
		}
	}

	req := domain.ExecutionRequest{
		AccountID:     account.ID,
		UserID:        account.UserID,
		SignalID:      signalID,
		MarketProfile: account.MarketProfile,
		Exchange:      exchange,
		Symbol:        symbol,
		Side:          string(domain.SideClose),
		OrderType:     string(domain.OrderTypeMarket),
		Product:       product,
		Price:         fillPrice,
		TickSize:      instrument.TickSize,
		LotSize:       instrument.LotSize,
		SlippageBps:   profile.SlippageBps,
		FeeBps:        profile.FeeBps,
	}
	return engine.Execute(ctx, req)
}

func resolveCloseFillPrice(
	ctx context.Context,
	md *marketdata.Registry,
	pg PositionCloserStore,
	exchange, symbol string,
	lastPrice float64,
) (float64, error) {
	if lastPrice > 0 {
		return lastPrice, nil
	}
	if md != nil {
		price, err := md.ResolveMarketFillPrice(ctx, pg, exchange, symbol)
		if err != nil && !errors.Is(err, marketdata.ErrNoQuote) {
			return 0, fmt.Errorf("live quote lookup failed: %w", err)
		}
		if price > 0 {
			return price, nil
		}
	}
	return 0, fmt.Errorf("no known price for %s:%s", exchange, symbol)
}
