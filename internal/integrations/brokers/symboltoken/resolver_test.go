package symboltoken

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/store"
)

type memCache struct {
	mu   sync.Mutex
	data map[string]store.SymbolTokenEntry
}

func (m *memCache) GetSymbolToken(_ context.Context, brokerType, exchange, symbol string) (*store.SymbolTokenEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.data[CacheKey(brokerType, exchange, symbol)]; ok {
		cp := e
		return &cp, nil
	}
	return nil, nil
}

func (m *memCache) SetSymbolToken(_ context.Context, brokerType, exchange, symbol string, entry store.SymbolTokenEntry, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string]store.SymbolTokenEntry)
	}
	m.data[CacheKey(brokerType, exchange, symbol)] = entry
	return nil
}

func TestResolverCacheHit(t *testing.T) {
	cache := &memCache{
		data: map[string]store.SymbolTokenEntry{
			CacheKey("angel", "NSE", "RELIANCE"): {Token: "2885", TradingSymbol: "RELIANCE-EQ"},
		},
	}
	r := NewResolver(cache)
	calls := 0
	lookup := LookupFunc(func(context.Context, string, string) (string, string, error) {
		calls++
		return "9999", "SHOULD-NOT-CALL", nil
	})

	token, ts, err := r.Resolve(context.Background(), "angel", "NSE", "RELIANCE", lookup)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if token != "2885" || ts != "RELIANCE-EQ" {
		t.Fatalf("got token=%q ts=%q", token, ts)
	}
	if calls != 0 {
		t.Fatalf("lookup called %d times on cache hit", calls)
	}
}

func TestResolverBrokerScopedKeys(t *testing.T) {
	cache := &memCache{
		data: map[string]store.SymbolTokenEntry{
			CacheKey("angel", "NSE", "TCS"): {Token: "111", TradingSymbol: "TCS-EQ"},
			CacheKey("dhan", "NSE", "TCS"):  {Token: "11536", TradingSymbol: "TCS"},
		},
	}
	r := NewResolver(cache)

	angelToken, _, err := r.Resolve(context.Background(), "angel", "NSE", "TCS", nil)
	if err != nil {
		t.Fatalf("angel cache: %v", err)
	}
	if angelToken != "111" {
		t.Fatalf("angel token=%q", angelToken)
	}

	dhanToken, _, err := r.Resolve(context.Background(), "dhan", "NSE", "TCS", nil)
	if err != nil {
		t.Fatalf("dhan cache: %v", err)
	}
	if dhanToken != "11536" {
		t.Fatalf("dhan token=%q", dhanToken)
	}
}

func TestResolverCacheMiss(t *testing.T) {
	r := NewResolver(&memCache{})
	calls := 0
	lookup := LookupFunc(func(_ context.Context, exchange, symbol string) (string, string, error) {
		calls++
		if exchange != "NSE" || symbol != "SBIN" {
			t.Fatalf("lookup args exchange=%q symbol=%q", exchange, symbol)
		}
		return "3045", "SBIN-EQ", nil
	})

	token, ts, err := r.Resolve(context.Background(), "dhan", "NSE", "SBIN", lookup)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if token != "3045" || ts != "SBIN-EQ" {
		t.Fatalf("got token=%q ts=%q", token, ts)
	}
	if calls != 1 {
		t.Fatalf("lookup calls=%d", calls)
	}

	_, _, err = r.Resolve(context.Background(), "dhan", "NSE", "SBIN", lookup)
	if err != nil {
		t.Fatalf("Resolve cached: %v", err)
	}
	if calls != 1 {
		t.Fatalf("lookup calls after cache=%d", calls)
	}
}

func TestResolverLookupError(t *testing.T) {
	r := NewResolver(&memCache{})
	lookup := LookupFunc(func(context.Context, string, string) (string, string, error) {
		return "", "", errors.New("broker down")
	})
	_, _, err := r.Resolve(context.Background(), "dhan", "NSE", "X", lookup)
	if err == nil {
		t.Fatal("expected error")
	}
}
