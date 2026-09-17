package symboltoken

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/SPSingh09/zettabridge/internal/store"
)

var ErrNotFound = errors.New("symbol token not found")

// TokenCache reads and writes resolved instrument tokens.
type TokenCache interface {
	GetSymbolToken(ctx context.Context, brokerType, exchange, symbol string) (*store.SymbolTokenEntry, error)
	SetSymbolToken(ctx context.Context, brokerType, exchange, symbol string, entry store.SymbolTokenEntry, ttl time.Duration) error
}

// LookupFunc resolves exchange+symbol via broker API on cache miss.
type LookupFunc func(ctx context.Context, exchange, symbol string) (token, tradingSymbol string, err error)

// Resolver caches symbol tokens in Redis (optional) with broker lookup on miss.
type Resolver struct {
	cache TokenCache
}

// NewResolver returns a resolver. cache may be nil (always calls lookup).
func NewResolver(cache TokenCache) *Resolver {
	return &Resolver{cache: cache}
}

// Resolve returns symboltoken and canonical tradingsymbol for broker+exchange+symbol.
func (r *Resolver) Resolve(ctx context.Context, brokerType, exchange, symbol string, lookup LookupFunc) (token, tradingSymbol string, err error) {
	if r.cache != nil {
		entry, err := r.cache.GetSymbolToken(ctx, brokerType, exchange, symbol)
		if err != nil {
			return "", "", err
		}
		if entry != nil {
			return entry.Token, entry.TradingSymbol, nil
		}
	}
	if lookup == nil {
		return "", "", fmt.Errorf("symbol token lookup not configured")
	}

	token, tradingSymbol, err = lookup(ctx, exchange, symbol)
	if err != nil {
		return "", "", err
	}
	if token == "" {
		return "", "", ErrNotFound
	}

	if r.cache != nil {
		_ = r.cache.SetSymbolToken(ctx, brokerType, exchange, symbol, store.SymbolTokenEntry{
			Token:         token,
			TradingSymbol: tradingSymbol,
		}, DefaultTTL)
	}
	return token, tradingSymbol, nil
}
