package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// PaperFillOutcome is what a PaperFillFunc computes from the locked account
// and position snapshot, for ExecutePaperFill to persist atomically.
// Order is always set (FILLED or REJECTED); Position, Trade, and
// NewCashBalance are nil/unset when the order was rejected.
type PaperFillOutcome struct {
	Order          *PaperOrder
	Position       *PaperPosition // upserted by (paper_account_id, exchange, symbol, product)
	Trade          *PaperTrade
	NewCashBalance *float64
}

// PaperFillFunc computes the outcome of one paper order given the current
// account and position (position is nil if none exists yet). It must be pure
// (no I/O) — ExecutePaperFill calls it while holding row locks.
type PaperFillFunc func(account *PaperAccount, position *PaperPosition) (*PaperFillOutcome, error)

// ExecutePaperFill locks the paper account and (if present) the position row
// for symbol+exchange+product, runs fn to compute the fill outcome, and
// persists the order/position/trade/account rows in one transaction. This is
// the only write path for paper order execution so concurrent signals against
// the same account+symbol can never race on position/cash-balance updates.
func (p *PGStore) ExecutePaperFill(ctx context.Context, accountID, exchange, symbol, product string, fn PaperFillFunc) (*PaperFillOutcome, error) {
	exchange = NormalizePaperExchange(exchange)
	symbol = NormalizePaperSymbol(symbol)
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: begin paper fill tx: %w", err)
	}
	defer tx.Rollback()

	account := &PaperAccount{}
	err = tx.QueryRowContext(ctx, `
		SELECT id, user_id, label, market_profile, base_currency,
		       starting_balance, cash_balance, exchange, default_product, status, created_at, updated_at
		FROM paper_accounts WHERE id = $1 FOR UPDATE`, accountID,
	).Scan(
		&account.ID, &account.UserID, &account.Label, &account.MarketProfile, &account.BaseCurrency,
		&account.StartingBalance, &account.CashBalance, &account.Exchange, &account.DefaultProduct, &account.Status, &account.CreatedAt, &account.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("store: paper account not found")
	}
	if err != nil {
		return nil, fmt.Errorf("store: lock paper account: %w", err)
	}

	position := &PaperPosition{}
	err = tx.QueryRowContext(ctx, `
		SELECT id, user_id, paper_account_id, symbol, exchange, product,
		       quantity, avg_entry_price, last_price, unrealized_pnl,
		       created_at, updated_at
		FROM paper_positions
		WHERE paper_account_id = $1
		  AND UPPER(exchange) = UPPER($2)
		  AND UPPER(symbol) = UPPER($3)
		  AND product = $4
		FOR UPDATE`,
		accountID, exchange, symbol, product,
	).Scan(
		&position.ID, &position.UserID, &position.PaperAccountID, &position.Symbol, &position.Exchange, &position.Product,
		&position.Quantity, &position.AvgEntryPrice, &position.LastPrice, &position.UnrealizedPnL,
		&position.CreatedAt, &position.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		position = nil
	} else if err != nil {
		return nil, fmt.Errorf("store: lock paper position: %w", err)
	}

	outcome, err := fn(account, position)
	if err != nil {
		return nil, err
	}

	// pq mishandles nil []byte as empty bytea rather than SQL NULL for JSONB columns.
	var rawSignal interface{}
	if len(outcome.Order.RawSignal) > 0 {
		rawSignal = outcome.Order.RawSignal
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO paper_orders (
			id, user_id, paper_account_id, webhook_id, signal_id,
			symbol, exchange, side, order_type, product, quantity,
			requested_price, fill_price, status, reason, raw_signal,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		outcome.Order.ID, outcome.Order.UserID, outcome.Order.PaperAccountID, outcome.Order.WebhookID, outcome.Order.SignalID,
		outcome.Order.Symbol, outcome.Order.Exchange, outcome.Order.Side, outcome.Order.OrderType, outcome.Order.Product, outcome.Order.Quantity,
		outcome.Order.RequestedPrice, outcome.Order.FillPrice, outcome.Order.Status, outcome.Order.Reason, rawSignal,
		outcome.Order.CreatedAt, outcome.Order.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("store: insert paper order: %w", err)
	}

	if outcome.Position != nil {
		outcome.Position.Symbol = NormalizePaperSymbol(outcome.Position.Symbol)
		outcome.Position.Exchange = NormalizePaperExchange(outcome.Position.Exchange)
		if position == nil {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO paper_positions (
					id, user_id, paper_account_id, symbol, exchange, product,
					quantity, avg_entry_price, last_price, unrealized_pnl,
					created_at, updated_at
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
				outcome.Position.ID, outcome.Position.UserID, outcome.Position.PaperAccountID,
				outcome.Position.Symbol, outcome.Position.Exchange, outcome.Position.Product,
				outcome.Position.Quantity, outcome.Position.AvgEntryPrice, outcome.Position.LastPrice, outcome.Position.UnrealizedPnL,
				outcome.Position.CreatedAt, outcome.Position.UpdatedAt,
			)
		} else {
			_, err = tx.ExecContext(ctx, `
				UPDATE paper_positions
				SET quantity = $1, avg_entry_price = $2, last_price = $3, unrealized_pnl = $4, updated_at = $5
				WHERE id = $6`,
				outcome.Position.Quantity, outcome.Position.AvgEntryPrice, outcome.Position.LastPrice, outcome.Position.UnrealizedPnL,
				outcome.Position.UpdatedAt, position.ID,
			)
		}
		if err != nil {
			return nil, fmt.Errorf("store: upsert paper position: %w", err)
		}
	}

	if outcome.Trade != nil {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO paper_trades (
				id, user_id, paper_account_id, order_id, symbol, exchange,
				side, quantity, price, realized_pnl, fees, created_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			outcome.Trade.ID, outcome.Trade.UserID, outcome.Trade.PaperAccountID, outcome.Trade.OrderID, outcome.Trade.Symbol, outcome.Trade.Exchange,
			outcome.Trade.Side, outcome.Trade.Quantity, outcome.Trade.Price, outcome.Trade.RealizedPnL, outcome.Trade.Fees, outcome.Trade.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("store: insert paper trade: %w", err)
		}
	}

	if outcome.NewCashBalance != nil {
		_, err = tx.ExecContext(ctx, `
			UPDATE paper_accounts SET cash_balance = $1, updated_at = NOW() WHERE id = $2`,
			*outcome.NewCashBalance, accountID,
		)
		if err != nil {
			return nil, fmt.Errorf("store: update paper account cash balance: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit paper fill: %w", err)
	}
	if err := p.RecordPaperAccountEquitySnapshot(ctx, accountID, account.UserID); err != nil {
		log.Printf("store: equity snapshot after fill account=%s: %v", accountID, err)
	}
	return outcome, nil
}
