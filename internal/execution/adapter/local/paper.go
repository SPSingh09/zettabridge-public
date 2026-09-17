package local

import (
	"context"
	"fmt"

	"github.com/SPSingh09/zettabridge/internal/domain"
)

// PaperAdapter wraps paperengine for the execution layer.
type PaperAdapter struct {
	engine domain.ExecutionDestination
}

func NewPaper(engine domain.ExecutionDestination) *PaperAdapter {
	return &PaperAdapter{engine: engine}
}

func (p *PaperAdapter) Execute(ctx context.Context, req domain.ExecutionRequest) (*domain.ExecutionResult, error) {
	if p == nil || p.engine == nil {
		return nil, fmt.Errorf("paper trading engine not configured")
	}
	return p.engine.Execute(ctx, req)
}
