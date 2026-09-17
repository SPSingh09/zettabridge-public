package adapter

import (
	"context"

	"github.com/SPSingh09/zettabridge/internal/domain"
)

// Client executes normalized orders against a paper or live broker destination.
// Phase 1 uses in-process implementations; HTTP transport comes in phase 3.
type Client interface {
	Execute(ctx context.Context, req domain.ExecutionRequest) (*domain.ExecutionResult, error)
}
