package server

import (
	"context"
	"time"

	"github.com/SPSingh09/zettabridge/internal/execution"
)

const adapterIdempotencyTTL = 24 * time.Hour

// IdempotencyStore persists adapter order outcomes for deduplication.
type IdempotencyStore interface {
	GetAdapterOrderOutcome(ctx context.Context, key string) (*execution.ExecutionOutcome, bool, error)
	PutAdapterOrderOutcome(ctx context.Context, key string, out execution.ExecutionOutcome, ttl time.Duration) error
}

// IdempotentExecutor wraps an Executor with Redis-backed idempotency.
type IdempotentExecutor struct {
	Inner Executor
	Store IdempotencyStore
}

func (w *IdempotentExecutor) PlaceOrder(ctx context.Context, cmd execution.ExecutionCommand) (execution.ExecutionOutcome, error) {
	key := cmd.IdempotencyKey
	if key == "" {
		key = cmd.RequestID
	}
	if key != "" && w.Store != nil {
		if cached, ok, err := w.Store.GetAdapterOrderOutcome(ctx, key); err == nil && ok && cached != nil {
			return *cached, nil
		}
	}
	out, err := w.Inner.PlaceOrder(ctx, cmd)
	if err != nil {
		return out, err
	}
	if key != "" && w.Store != nil {
		_ = w.Store.PutAdapterOrderOutcome(ctx, key, out, adapterIdempotencyTTL)
	}
	return out, nil
}

func (w *IdempotentExecutor) CancelOrder(ctx context.Context, cmd execution.CancelCommand) (execution.ExecutionOutcome, error) {
	return w.Inner.CancelOrder(ctx, cmd)
}
