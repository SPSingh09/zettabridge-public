package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// SymbolTokenEntry is a cached exchange+symbol → broker instrument mapping.
type SymbolTokenEntry struct {
	Token         string `json:"token"`
	TradingSymbol string `json:"tradingsymbol"`
}

// GetSymbolToken returns a cached symbol token or nil on miss.
func (r *RedisStore) GetSymbolToken(ctx context.Context, brokerType, exchange, symbol string) (*SymbolTokenEntry, error) {
	val, err := r.client.Get(ctx, symbolTokenCacheKey(brokerType, exchange, symbol)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entry SymbolTokenEntry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		return nil, err
	}
	if entry.Token == "" {
		return nil, nil
	}
	return &entry, nil
}

// SetSymbolToken caches a resolved symbol token.
func (r *RedisStore) SetSymbolToken(ctx context.Context, brokerType, exchange, symbol string, entry SymbolTokenEntry, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, symbolTokenCacheKey(brokerType, exchange, symbol), b, ttl).Err()
}

func symbolTokenCacheKey(brokerType, exchange, symbol string) string {
	bt := strings.ToLower(strings.TrimSpace(brokerType))
	if bt == "" {
		bt = "default"
	}
	ex := strings.ToUpper(strings.TrimSpace(exchange))
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	return fmt.Sprintf("symtoken:%s:%s:%s", bt, ex, sym)
}
