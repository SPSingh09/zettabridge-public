package store

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const adapterIdempotencyPrefix = "adapter:idempotency:"

// GetAdapterIdempotency returns a cached adapter order outcome payload.
func (r *RedisStore) GetAdapterIdempotency(ctx context.Context, key string) ([]byte, bool, error) {
	if key == "" {
		return nil, false, nil
	}
	val, err := r.client.Get(ctx, adapterIdempotencyPrefix+key).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return []byte(val), true, nil
}

// PutAdapterIdempotency stores a cached adapter order outcome payload.
func (r *RedisStore) PutAdapterIdempotency(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	if key == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return r.client.Set(ctx, adapterIdempotencyPrefix+key, payload, ttl).Err()
}
