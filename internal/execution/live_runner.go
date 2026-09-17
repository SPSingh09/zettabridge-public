package execution

import (
	"context"

	"github.com/SPSingh09/zettabridge/internal/domain"
)

// LiveRunner executes live broker orders (in-process or via HTTP adapter).
type LiveRunner interface {
	Execute(ctx context.Context, req domain.ExecutionRequest) (*domain.ExecutionResult, error)
}

// PaperRunner executes paper trading orders.
type PaperRunner interface {
	Execute(ctx context.Context, req domain.ExecutionRequest) (*domain.ExecutionResult, error)
}
