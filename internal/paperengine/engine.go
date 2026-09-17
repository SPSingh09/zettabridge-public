// Package paperengine implements ZettaBridge's broker-free Paper Trading
// Engine (Phase 2 MVP). It never calls any broker package — a paper account
// only ever touches the paper_accounts/orders/positions/trades tables.
package paperengine

import (
	"context"
	"time"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// Store is the persistence dependency Engine needs. *store.PGStore satisfies
// this; the narrow interface (mirroring internal/platform/queue.TradeStore's
// approach) lets fill-rule tests run against a fake with no database.
type Store interface {
	ExecutePaperFill(ctx context.Context, accountID, exchange, symbol, product string, fn store.PaperFillFunc) (*store.PaperFillOutcome, error)
}

// Engine implements domain.ExecutionDestination for DestinationKindPaperTrading.
type Engine struct {
	Store Store
}

func New(s Store) *Engine { return &Engine{Store: s} }

var _ domain.ExecutionDestination = (*Engine)(nil)

// Execute validates and fills req against the paper account named by
// req.AccountID, updating its position and cash balance. It always returns a
// non-nil result (Status FILLED or REJECTED) rather than an error for
// business-rule rejections; err is only set for infrastructure failures
// (e.g. the account does not exist, or the write failed).
func (e *Engine) Execute(ctx context.Context, req domain.ExecutionRequest) (*domain.ExecutionResult, error) {
	now := time.Now()

	outcome, err := e.Store.ExecutePaperFill(ctx, req.AccountID, req.Exchange, req.Symbol, req.Product,
		func(account *store.PaperAccount, position *store.PaperPosition) (*store.PaperFillOutcome, error) {
			return computeFill(now, account, position, req)
		},
	)
	if err != nil {
		return nil, err
	}

	result := &domain.ExecutionResult{
		OrderID: outcome.Order.ID,
		Status:  outcome.Order.Status,
		Reason:  outcome.Order.Reason,
	}
	if outcome.Order.FillPrice != nil {
		result.FillPrice = *outcome.Order.FillPrice
	}
	if outcome.Trade != nil {
		result.RealizedPnL = outcome.Trade.RealizedPnL
	}
	return result, nil
}
