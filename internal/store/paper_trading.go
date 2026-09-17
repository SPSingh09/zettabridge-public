package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// ── Paper Accounts ────────────────────────────────────────────────────────────

// CreatePaperAccount inserts a new paper_accounts row.
func (p *PGStore) CreatePaperAccount(ctx context.Context, a *PaperAccount) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO paper_accounts (
			id, user_id, label, market_profile, base_currency,
			starting_balance, cash_balance, exchange, default_product, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		a.ID, a.UserID, a.Label, a.MarketProfile, a.BaseCurrency,
		a.StartingBalance, a.CashBalance, a.Exchange, a.DefaultProduct, a.Status, a.CreatedAt, a.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("store: create paper account: %w", err)
	}
	if err := p.RecordPaperAccountEquitySnapshot(ctx, a.ID, a.UserID); err != nil {
		log.Printf("store: initial equity snapshot account=%s: %v", a.ID, err)
	}
	return nil
}

// GetPaperAccount returns a paper account by ID. Returns nil, nil when not found.
func (p *PGStore) GetPaperAccount(ctx context.Context, id string) (*PaperAccount, error) {
	a := &PaperAccount{}
	err := p.db.QueryRowContext(ctx, `
		SELECT id, user_id, label, market_profile, base_currency,
		       starting_balance, cash_balance, exchange, default_product, status, created_at, updated_at
		FROM paper_accounts WHERE id = $1`, id,
	).Scan(
		&a.ID, &a.UserID, &a.Label, &a.MarketProfile, &a.BaseCurrency,
		&a.StartingBalance, &a.CashBalance, &a.Exchange, &a.DefaultProduct, &a.Status, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get paper account: %w", err)
	}
	return a, nil
}

// ListPaperAccountsByUser returns all paper accounts owned by a user, newest first.
func (p *PGStore) ListPaperAccountsByUser(ctx context.Context, userID string) ([]*PaperAccount, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, user_id, label, market_profile, base_currency,
		       starting_balance, cash_balance, exchange, default_product, status, created_at, updated_at
		FROM paper_accounts WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list paper accounts: %w", err)
	}
	defer rows.Close()

	var out []*PaperAccount
	for rows.Next() {
		a := &PaperAccount{}
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.Label, &a.MarketProfile, &a.BaseCurrency,
			&a.StartingBalance, &a.CashBalance, &a.Exchange, &a.DefaultProduct, &a.Status, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan paper account: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountPaperOrdersByUserInMonth counts paper orders placed by a user in the
// given UTC calendar month (used for free-plan monthly trade quotas).
func (p *PGStore) CountPaperOrdersByUserInMonth(ctx context.Context, userID string, month time.Time) (int, error) {
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	var count int
	err := p.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM paper_orders po
		JOIN paper_accounts pa ON pa.id = po.paper_account_id
		WHERE pa.user_id = $1 AND po.created_at >= $2 AND po.created_at < $3`,
		userID, start, end,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count paper orders by user in month: %w", err)
	}
	return count, nil
}

// CountPaperAccountsByUser returns how many paper accounts a user owns.
// Paper accounts are solo-only (no org_id column) — see DestinationKind docs.
func (p *PGStore) CountPaperAccountsByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM paper_accounts WHERE user_id = $1`, userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count paper accounts: %w", err)
	}
	return count, nil
}

// ListPaperAccountsWithOpenPositions returns every paper account (across all
// users) that currently holds at least one non-flat position — the sweep
// list for the Phase 6 mark-to-market background job.
func (p *PGStore) ListPaperAccountsWithOpenPositions(ctx context.Context) ([]*PaperAccount, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT DISTINCT pa.id, pa.user_id, pa.label, pa.market_profile, pa.base_currency,
		       pa.starting_balance, pa.cash_balance, pa.exchange, pa.default_product, pa.status,
		       pa.created_at, pa.updated_at
		FROM paper_accounts pa
		JOIN paper_positions pp ON pp.paper_account_id = pa.id
		WHERE pp.quantity != 0`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list paper accounts with open positions: %w", err)
	}
	defer rows.Close()

	var out []*PaperAccount
	for rows.Next() {
		a := &PaperAccount{}
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.Label, &a.MarketProfile, &a.BaseCurrency,
			&a.StartingBalance, &a.CashBalance, &a.Exchange, &a.DefaultProduct, &a.Status, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan paper account: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdatePaperAccount updates the mutable fields of a paper account owned by
// a.UserID. Returns sql.ErrNoRows if no such account exists for that user.
func (p *PGStore) UpdatePaperAccount(ctx context.Context, a *PaperAccount) error {
	res, err := p.db.ExecContext(ctx, `
		UPDATE paper_accounts
		SET label = $3, default_product = $4, starting_balance = $5, cash_balance = $6, updated_at = now()
		WHERE id = $1 AND user_id = $2`,
		a.ID, a.UserID, a.Label, a.DefaultProduct, a.StartingBalance, a.CashBalance,
	)
	if err != nil {
		return fmt.Errorf("store: update paper account: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update paper account rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdatePaperAccountStatus sets a paper account's status (active|paused).
// Returns sql.ErrNoRows if no such account exists for that user.
func (p *PGStore) UpdatePaperAccountStatus(ctx context.Context, id, userID, status string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE paper_accounts SET status = $3, updated_at = now() WHERE id = $1 AND user_id = $2`,
		id, userID, status,
	)
	if err != nil {
		return fmt.Errorf("store: update paper account status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update paper account status rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ResetPaperAccount wipes a paper account's positions/orders/snapshots
// (paper_trades cascades from paper_orders' ON DELETE CASCADE FK) and resets
// cash_balance to starting_balance — or to newBalance, if non-nil, which lets
// a "blown" account be topped up in the same action. Unlike starting_balance
// edits via UpdatePaperAccount (blocked once any fill has moved cash, to
// protect the account's own Equity/P&L math), this doesn't need that guard:
// it deletes the history the math was protecting instead of leaving it
// out of sync. Runs as one transaction. Returns sql.ErrNoRows if no such
// account exists for that user.
func (p *PGStore) ResetPaperAccount(ctx context.Context, id, userID string, newBalance *float64) (*PaperAccount, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: begin paper account reset tx: %w", err)
	}
	defer tx.Rollback()

	a := &PaperAccount{}
	err = tx.QueryRowContext(ctx, `
		SELECT id, user_id, label, market_profile, base_currency,
		       starting_balance, cash_balance, exchange, default_product, status, created_at, updated_at
		FROM paper_accounts WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, userID,
	).Scan(
		&a.ID, &a.UserID, &a.Label, &a.MarketProfile, &a.BaseCurrency,
		&a.StartingBalance, &a.CashBalance, &a.Exchange, &a.DefaultProduct, &a.Status, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("store: reset paper account lookup: %w", err)
	}

	if newBalance != nil {
		a.StartingBalance = *newBalance
	}
	a.CashBalance = a.StartingBalance

	if _, err := tx.ExecContext(ctx, `DELETE FROM paper_positions WHERE paper_account_id = $1`, id); err != nil {
		return nil, fmt.Errorf("store: reset paper account: clear positions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM paper_orders WHERE paper_account_id = $1`, id); err != nil {
		return nil, fmt.Errorf("store: reset paper account: clear orders: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM paper_account_snapshots WHERE paper_account_id = $1`, id); err != nil {
		return nil, fmt.Errorf("store: reset paper account: clear snapshots: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE paper_accounts SET starting_balance = $2, cash_balance = $3, updated_at = now() WHERE id = $1`,
		id, a.StartingBalance, a.CashBalance,
	); err != nil {
		return nil, fmt.Errorf("store: reset paper account: update balances: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit paper account reset: %w", err)
	}
	a.UpdatedAt = time.Now()
	return a, nil
}

// DeletePaperAccount removes a paper account owned by userID.
// Returns sql.ErrNoRows if no such account exists for that user.
func (p *PGStore) DeletePaperAccount(ctx context.Context, id, userID string) error {
	res, err := p.db.ExecContext(ctx,
		`DELETE FROM paper_accounts WHERE id = $1 AND user_id = $2`, id, userID,
	)
	if err != nil {
		return fmt.Errorf("store: delete paper account: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete paper account rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ── Paper Orders ──────────────────────────────────────────────────────────────

// CreatePaperOrder inserts a new paper_orders row.
func (p *PGStore) CreatePaperOrder(ctx context.Context, o *PaperOrder) error {
	// pq mishandles nil []byte as empty bytea rather than SQL NULL for JSONB columns.
	var rawSignal interface{}
	if len(o.RawSignal) > 0 {
		rawSignal = o.RawSignal
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO paper_orders (
			id, user_id, paper_account_id, webhook_id, signal_id,
			symbol, exchange, side, order_type, product, quantity,
			requested_price, fill_price, status, reason, raw_signal,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		o.ID, o.UserID, o.PaperAccountID, o.WebhookID, o.SignalID,
		o.Symbol, o.Exchange, o.Side, o.OrderType, o.Product, o.Quantity,
		o.RequestedPrice, o.FillPrice, o.Status, o.Reason, rawSignal,
		o.CreatedAt, o.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("store: create paper order: %w", err)
	}
	return nil
}

// GetPaperOrder returns a paper order by ID. Returns nil, nil when not found.
func (p *PGStore) GetPaperOrder(ctx context.Context, id string) (*PaperOrder, error) {
	o := &PaperOrder{}
	err := p.db.QueryRowContext(ctx, `
		SELECT id, user_id, paper_account_id, webhook_id, signal_id,
		       symbol, exchange, side, order_type, product, quantity,
		       requested_price, fill_price, status, reason, raw_signal,
		       created_at, updated_at
		FROM paper_orders WHERE id = $1`, id,
	).Scan(
		&o.ID, &o.UserID, &o.PaperAccountID, &o.WebhookID, &o.SignalID,
		&o.Symbol, &o.Exchange, &o.Side, &o.OrderType, &o.Product, &o.Quantity,
		&o.RequestedPrice, &o.FillPrice, &o.Status, &o.Reason, &o.RawSignal,
		&o.CreatedAt, &o.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get paper order: %w", err)
	}
	return o, nil
}

// ListPaperOrdersByAccount returns paper orders for an account, newest first.
func (p *PGStore) ListPaperOrdersByAccount(ctx context.Context, accountID string) ([]*PaperOrder, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, user_id, paper_account_id, webhook_id, signal_id,
		       symbol, exchange, side, order_type, product, quantity,
		       requested_price, fill_price, status, reason, raw_signal,
		       created_at, updated_at
		FROM paper_orders WHERE paper_account_id = $1 ORDER BY created_at DESC`, accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list paper orders: %w", err)
	}
	defer rows.Close()

	var out []*PaperOrder
	for rows.Next() {
		o := &PaperOrder{}
		if err := rows.Scan(
			&o.ID, &o.UserID, &o.PaperAccountID, &o.WebhookID, &o.SignalID,
			&o.Symbol, &o.Exchange, &o.Side, &o.OrderType, &o.Product, &o.Quantity,
			&o.RequestedPrice, &o.FillPrice, &o.Status, &o.Reason, &o.RawSignal,
			&o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan paper order: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// PaperOrderWithPnL is a paper order enriched with the realized P&L of the
// trade it produced (nil for rejected or never-filled orders). Used for the
// Phase 4 dashboard's trade history, which needs both fills and rejections —
// paper_trades alone can't show rejections since it's fills-only.
type PaperOrderWithPnL struct {
	PaperOrder
	RealizedPnL *float64 `json:"realized_pnl,omitempty"`
}

// ListPaperOrdersWithPnLByAccount returns one page of order attempts for an
// account (filled and rejected), newest first, with realized P&L attached
// where the order resulted in a trade.
func (p *PGStore) ListPaperOrdersWithPnLByAccount(ctx context.Context, accountID string, limit, offset int) ([]*PaperOrderWithPnL, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT o.id, o.user_id, o.paper_account_id, o.webhook_id, o.signal_id,
		       o.symbol, o.exchange, o.side, o.order_type, o.product, o.quantity,
		       o.requested_price, o.fill_price, o.status, o.reason, o.raw_signal,
		       o.created_at, o.updated_at, t.realized_pnl
		FROM paper_orders o
		LEFT JOIN paper_trades t ON t.order_id = o.id
		WHERE o.paper_account_id = $1
		ORDER BY o.created_at DESC
		LIMIT $2 OFFSET $3`, accountID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list paper orders with pnl: %w", err)
	}
	defer rows.Close()

	var out []*PaperOrderWithPnL
	for rows.Next() {
		o := &PaperOrderWithPnL{}
		if err := rows.Scan(
			&o.ID, &o.UserID, &o.PaperAccountID, &o.WebhookID, &o.SignalID,
			&o.Symbol, &o.Exchange, &o.Side, &o.OrderType, &o.Product, &o.Quantity,
			&o.RequestedPrice, &o.FillPrice, &o.Status, &o.Reason, &o.RawSignal,
			&o.CreatedAt, &o.UpdatedAt, &o.RealizedPnL,
		); err != nil {
			return nil, fmt.Errorf("store: scan paper order with pnl: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// CountPaperOrdersByAccount returns the total number of orders for an
// account, regardless of page — used by ListPaperOrders to report a "total"
// alongside the current page so the frontend knows when to stop paginating.
func (p *PGStore) CountPaperOrdersByAccount(ctx context.Context, accountID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM paper_orders WHERE paper_account_id = $1`, accountID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count paper orders: %w", err)
	}
	return count, nil
}

// ListPaperOrdersForExport returns up to 10,000 orders for an account,
// newest first, optionally bounded by a created_at date range — used by CSV
// export (no destination/plan gating, matching how paper trading is
// plan-independent everywhere else).
func (p *PGStore) ListPaperOrdersForExport(ctx context.Context, accountID string, from, to *time.Time) ([]*PaperOrderWithPnL, error) {
	query := `
		SELECT o.id, o.user_id, o.paper_account_id, o.webhook_id, o.signal_id,
		       o.symbol, o.exchange, o.side, o.order_type, o.product, o.quantity,
		       o.requested_price, o.fill_price, o.status, o.reason, o.raw_signal,
		       o.created_at, o.updated_at, t.realized_pnl
		FROM paper_orders o
		LEFT JOIN paper_trades t ON t.order_id = o.id
		WHERE o.paper_account_id = $1`
	args := []any{accountID}
	if from != nil {
		args = append(args, *from)
		query += fmt.Sprintf(" AND o.created_at >= $%d", len(args))
	}
	if to != nil {
		args = append(args, *to)
		query += fmt.Sprintf(" AND o.created_at <= $%d", len(args))
	}
	query += " ORDER BY o.created_at DESC LIMIT 10000"

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list paper orders for export: %w", err)
	}
	defer rows.Close()

	var out []*PaperOrderWithPnL
	for rows.Next() {
		o := &PaperOrderWithPnL{}
		if err := rows.Scan(
			&o.ID, &o.UserID, &o.PaperAccountID, &o.WebhookID, &o.SignalID,
			&o.Symbol, &o.Exchange, &o.Side, &o.OrderType, &o.Product, &o.Quantity,
			&o.RequestedPrice, &o.FillPrice, &o.Status, &o.Reason, &o.RawSignal,
			&o.CreatedAt, &o.UpdatedAt, &o.RealizedPnL,
		); err != nil {
			return nil, fmt.Errorf("store: scan paper order for export: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ── Paper Positions ───────────────────────────────────────────────────────────

// CreatePaperPosition inserts a new paper_positions row.
func (p *PGStore) CreatePaperPosition(ctx context.Context, pos *PaperPosition) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO paper_positions (
			id, user_id, paper_account_id, symbol, exchange, product,
			quantity, avg_entry_price, last_price, unrealized_pnl,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		pos.ID, pos.UserID, pos.PaperAccountID, pos.Symbol, pos.Exchange, pos.Product,
		pos.Quantity, pos.AvgEntryPrice, pos.LastPrice, pos.UnrealizedPnL,
		pos.CreatedAt, pos.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("store: create paper position: %w", err)
	}
	return nil
}

// GetPaperPosition returns the position for an account+symbol+exchange+product.
// Returns nil, nil when not found.
func (p *PGStore) GetPaperPosition(ctx context.Context, accountID, exchange, symbol, product string) (*PaperPosition, error) {
	pos := &PaperPosition{}
	err := p.db.QueryRowContext(ctx, `
		SELECT id, user_id, paper_account_id, symbol, exchange, product,
		       quantity, avg_entry_price, last_price, unrealized_pnl,
		       created_at, updated_at
		FROM paper_positions
		WHERE paper_account_id = $1
		  AND UPPER(exchange) = UPPER($2)
		  AND UPPER(symbol) = UPPER($3)
		  AND product = $4`,
		accountID, exchange, symbol, product,
	).Scan(
		&pos.ID, &pos.UserID, &pos.PaperAccountID, &pos.Symbol, &pos.Exchange, &pos.Product,
		&pos.Quantity, &pos.AvgEntryPrice, &pos.LastPrice, &pos.UnrealizedPnL,
		&pos.CreatedAt, &pos.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get paper position: %w", err)
	}
	return pos, nil
}

// UpdatePaperPositionMark updates last_price/unrealized_pnl on an existing
// position from a PRICE_UPDATE signal — no order, trade, or cash movement.
func (p *PGStore) UpdatePaperPositionMark(ctx context.Context, pos *PaperPosition) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE paper_positions SET last_price = $1, unrealized_pnl = $2, updated_at = $3 WHERE id = $4`,
		pos.LastPrice, pos.UnrealizedPnL, pos.UpdatedAt, pos.ID,
	)
	if err != nil {
		return fmt.Errorf("store: update paper position mark: %w", err)
	}
	return nil
}

// ListPaperPositionsByAccount returns all positions for a paper account.
func (p *PGStore) ListPaperPositionsByAccount(ctx context.Context, accountID string) ([]*PaperPosition, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, user_id, paper_account_id, symbol, exchange, product,
		       quantity, avg_entry_price, last_price, unrealized_pnl,
		       created_at, updated_at
		FROM paper_positions WHERE paper_account_id = $1 ORDER BY symbol`, accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list paper positions: %w", err)
	}
	defer rows.Close()

	var out []*PaperPosition
	for rows.Next() {
		pos := &PaperPosition{}
		if err := rows.Scan(
			&pos.ID, &pos.UserID, &pos.PaperAccountID, &pos.Symbol, &pos.Exchange, &pos.Product,
			&pos.Quantity, &pos.AvgEntryPrice, &pos.LastPrice, &pos.UnrealizedPnL,
			&pos.CreatedAt, &pos.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan paper position: %w", err)
		}
		out = append(out, pos)
	}
	return out, rows.Err()
}

// OpenMISPaperPosition pairs an open MIS position with its paper account.
type OpenMISPaperPosition struct {
	Position *PaperPosition
	Account  *PaperAccount
}

// ListOpenMISPaperPositions returns every non-flat MIS position across all
// paper accounts (active or paused). Used by the MIS auto square-off job.
func (p *PGStore) ListOpenMISPaperPositions(ctx context.Context) ([]*OpenMISPaperPosition, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT
			pos.id, pos.user_id, pos.paper_account_id, pos.symbol, pos.exchange, pos.product,
			pos.quantity, pos.avg_entry_price, pos.last_price, pos.unrealized_pnl,
			pos.created_at, pos.updated_at,
			acct.id, acct.user_id, acct.label, acct.market_profile, acct.base_currency,
			acct.starting_balance, acct.cash_balance, acct.exchange, acct.default_product,
			acct.status, acct.created_at, acct.updated_at
		FROM paper_positions pos
		JOIN paper_accounts acct ON acct.id = pos.paper_account_id
		WHERE pos.product = 'MIS'
		  AND pos.quantity <> 0
		  AND acct.status <> 'closed'
		ORDER BY pos.paper_account_id, pos.symbol`)
	if err != nil {
		return nil, fmt.Errorf("store: list open MIS paper positions: %w", err)
	}
	defer rows.Close()

	var out []*OpenMISPaperPosition
	for rows.Next() {
		pos := &PaperPosition{}
		acct := &PaperAccount{}
		if err := rows.Scan(
			&pos.ID, &pos.UserID, &pos.PaperAccountID, &pos.Symbol, &pos.Exchange, &pos.Product,
			&pos.Quantity, &pos.AvgEntryPrice, &pos.LastPrice, &pos.UnrealizedPnL,
			&pos.CreatedAt, &pos.UpdatedAt,
			&acct.ID, &acct.UserID, &acct.Label, &acct.MarketProfile, &acct.BaseCurrency,
			&acct.StartingBalance, &acct.CashBalance, &acct.Exchange, &acct.DefaultProduct,
			&acct.Status, &acct.CreatedAt, &acct.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan open MIS paper position: %w", err)
		}
		out = append(out, &OpenMISPaperPosition{Position: pos, Account: acct})
	}
	return out, rows.Err()
}

// ── Paper Trades ──────────────────────────────────────────────────────────────

// CreatePaperTrade inserts a new paper_trades row (immutable fill history).
func (p *PGStore) CreatePaperTrade(ctx context.Context, t *PaperTrade) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO paper_trades (
			id, user_id, paper_account_id, order_id, symbol, exchange,
			side, quantity, price, realized_pnl, fees, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		t.ID, t.UserID, t.PaperAccountID, t.OrderID, t.Symbol, t.Exchange,
		t.Side, t.Quantity, t.Price, t.RealizedPnL, t.Fees, t.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("store: create paper trade: %w", err)
	}
	return nil
}

// ListPaperTradesByAccount returns executed paper trades for an account, newest first.
func (p *PGStore) ListPaperTradesByAccount(ctx context.Context, accountID string) ([]*PaperTrade, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, user_id, paper_account_id, order_id, symbol, exchange,
		       side, quantity, price, realized_pnl, fees, created_at
		FROM paper_trades WHERE paper_account_id = $1 ORDER BY created_at DESC`, accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list paper trades: %w", err)
	}
	defer rows.Close()

	var out []*PaperTrade
	for rows.Next() {
		t := &PaperTrade{}
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.PaperAccountID, &t.OrderID, &t.Symbol, &t.Exchange,
			&t.Side, &t.Quantity, &t.Price, &t.RealizedPnL, &t.Fees, &t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan paper trade: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ── Paper Account Snapshots ───────────────────────────────────────────────────

// RecordPaperAccountEquitySnapshot writes a snapshot from the live P&L rollup.
// Best-effort: callers log failures but do not fail the primary operation.
func (p *PGStore) RecordPaperAccountEquitySnapshot(ctx context.Context, accountID, userID string) error {
	summary, err := p.GetPaperAccountPnL(ctx, accountID)
	if err != nil {
		return err
	}
	if summary == nil {
		return fmt.Errorf("store: paper account not found")
	}
	return p.CreatePaperAccountSnapshot(ctx, &PaperAccountSnapshot{
		ID:             uuid.NewString(),
		UserID:         userID,
		PaperAccountID: accountID,
		Equity:         summary.Equity,
		CashBalance:    summary.CashBalance,
		UnrealizedPnL:  summary.UnrealizedPnL,
		RealizedPnL:    summary.RealizedPnL,
		CreatedAt:      time.Now().UTC(),
	})
}

// CreatePaperAccountSnapshot inserts a point-in-time equity/P&L snapshot.
func (p *PGStore) CreatePaperAccountSnapshot(ctx context.Context, s *PaperAccountSnapshot) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO paper_account_snapshots (
			id, user_id, paper_account_id, equity, cash_balance,
			unrealized_pnl, realized_pnl, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		s.ID, s.UserID, s.PaperAccountID, s.Equity, s.CashBalance,
		s.UnrealizedPnL, s.RealizedPnL, s.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("store: create paper account snapshot: %w", err)
	}
	return nil
}

// ListPaperAccountSnapshots returns the most recent snapshots for an account, newest first.
func (p *PGStore) ListPaperAccountSnapshots(ctx context.Context, accountID string, limit int) ([]*PaperAccountSnapshot, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, user_id, paper_account_id, equity, cash_balance,
		       unrealized_pnl, realized_pnl, created_at
		FROM paper_account_snapshots
		WHERE paper_account_id = $1 ORDER BY created_at DESC LIMIT $2`, accountID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list paper account snapshots: %w", err)
	}
	defer rows.Close()

	var out []*PaperAccountSnapshot
	for rows.Next() {
		s := &PaperAccountSnapshot{}
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.PaperAccountID, &s.Equity, &s.CashBalance,
			&s.UnrealizedPnL, &s.RealizedPnL, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan paper account snapshot: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
