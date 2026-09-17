package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/execution/adapter/local"
)

var ErrOrchestratorNotConfigured = errors.New("execution orchestrator not configured")

// Orchestrator routes execution requests to in-process or HTTP adapters.
type Orchestrator struct {
	live   LiveRunner
	paper  PaperRunner
	router *Router
}

type OrchestratorDeps struct {
	Live   LiveRunner
	Paper  PaperRunner
	Router *Router
}

func NewOrchestrator(deps OrchestratorDeps) *Orchestrator {
	return &Orchestrator{
		live:   deps.Live,
		paper:  deps.Paper,
		router: deps.Router,
	}
}

func (o *Orchestrator) PaperRunner() PaperRunner {
	if o == nil {
		return nil
	}
	return o.paper
}

func (o *Orchestrator) SetPaperEngine(e domain.ExecutionDestination) {
	if o == nil {
		return
	}
	o.paper = local.NewPaper(e)
}

func (o *Orchestrator) SetBrokerFactory(f local.BrokerFactory) {
	if la, ok := o.live.(*local.LiveAdapter); ok && la != nil {
		la.SetBrokerFactory(f)
	}
}

func (o *Orchestrator) ExecuteLive(ctx context.Context, req domain.ExecutionRequest) (*domain.ExecutionResult, error) {
	if o == nil || o.live == nil {
		return nil, fmt.Errorf("%w", ErrOrchestratorNotConfigured)
	}
	return o.live.Execute(ctx, req)
}

func (o *Orchestrator) ExecutePaper(ctx context.Context, req domain.ExecutionRequest) (*domain.ExecutionResult, error) {
	if o == nil || o.paper == nil {
		return nil, fmt.Errorf("%w", ErrOrchestratorNotConfigured)
	}
	return o.paper.Execute(ctx, req)
}

func (o *Orchestrator) Router() *Router {
	if o == nil {
		return nil
	}
	if o.router != nil {
		return o.router
	}
	if la, ok := o.live.(*local.LiveAdapter); ok && la != nil {
		if gate := la.Router(); gate != nil {
			if r, ok := gate.(*Router); ok {
				return r
			}
		}
	}
	return nil
}
