package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/SPSingh09/zettabridge/internal/execution"
)

// RedisIdempotencyCache reads/writes adapter idempotency keys in Redis.
type RedisIdempotencyCache interface {
	GetAdapterIdempotency(ctx context.Context, key string) ([]byte, bool, error)
	PutAdapterIdempotency(ctx context.Context, key string, payload []byte, ttl time.Duration) error
}

// RedisIdempotency implements IdempotencyStore using Redis byte payloads.
type RedisIdempotency struct {
	Redis RedisIdempotencyCache
}

func (r *RedisIdempotency) GetAdapterOrderOutcome(ctx context.Context, key string) (*execution.ExecutionOutcome, bool, error) {
	if r == nil || r.Redis == nil {
		return nil, false, nil
	}
	raw, ok, err := r.Redis.GetAdapterIdempotency(ctx, key)
	if err != nil || !ok || len(raw) == 0 {
		return nil, ok, err
	}
	var out execution.ExecutionOutcome
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false, err
	}
	return &out, true, nil
}

func (r *RedisIdempotency) PutAdapterOrderOutcome(ctx context.Context, key string, out execution.ExecutionOutcome, ttl time.Duration) error {
	if r == nil || r.Redis == nil {
		return nil
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return r.Redis.PutAdapterIdempotency(ctx, key, raw, ttl)
}
