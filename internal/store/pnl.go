package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/SPSingh09/zettabridge/internal/pnl"
)

// PnLSummary is a v1 rollup for dashboard P&L panels. The fields below
// ByStatus are only populated for GetWebhookPnL (a single webhook's
// front-door funnel doesn't have an org-level equivalent) — see
// enrichWebhookSummary in webhook_summary.go.
type PnLSummary struct {
	WebhookID    string         `json:"webhook_id,omitempty"`
	OrgID        string         `json:"org_id,omitempty"`
	TradeCount   int            `json:"trade_count"`
	ByStatus     map[string]int `json:"by_status"`
	Notional     float64        `json:"notional"`
	PricedTrades int            `json:"priced_trades"`
	FillRate     *float64       `json:"fill_rate,omitempty"`
	WinRate      *float64       `json:"win_rate,omitempty"`

	// Signal Processing — from ingest_log, the front-door funnel. No
	// omitempty on these: 0 is a real, meaningful count (e.g. "0 duplicates
	// suppressed"), not "unknown" — only the *rates/timestamps below use a
	// nil pointer to mean "not computed".
	TotalSignals        int `json:"total_signals"`
	AcceptedSignals     int `json:"accepted_signals"`
	RejectedSignals     int `json:"rejected_signals"`
	DuplicateSuppressed int `json:"duplicate_suppressed"`
	RateLimitedSignals  int `json:"rate_limited_signals"`
	PausedSignals       int `json:"paused_signals"`
	QueueFullSignals    int `json:"queue_full_signals"`

	// Broker Execution — from trades.
	SubmittedToBroker   int      `json:"submitted_to_broker"`
	Filled              int      `json:"filled"`
	BrokerRejected      int      `json:"broker_rejected"`
	ZettaBridgeRejected int      `json:"zettabridge_rejected"`
	Cancelled           int      `json:"cancelled"`
	PendingConfirmation int      `json:"pending_confirmation"`
	AvgLatencyMs        *float64 `json:"avg_latency_ms,omitempty"`
	BrokerSuccessRate   *float64 `json:"broker_success_rate,omitempty"`

	// Orders by side (BUY/SELL/CLOSE), across all trades regardless of outcome.
	BuyOrders   int `json:"buy_orders"`
	SellOrders  int `json:"sell_orders"`
	CloseOrders int `json:"close_orders"`

	// Risk & Validation — human-readable rejection reason -> count.
	RejectionReasons map[string]int `json:"rejection_reasons,omitempty"`

	// Recent Activity.
	LastSignalAt         *time.Time `json:"last_signal_at,omitempty"`
	LastBrokerResponseAt *time.Time `json:"last_broker_response_at,omitempty"`
	LastRejectionReason  string     `json:"last_rejection_reason,omitempty"`
	LastRejectionAt      *time.Time `json:"last_rejection_at,omitempty"`

	// Broker Health — populated by the pnl handler (needs a credential
	// lookup, not derivable from trades/ingest_log alone).
	BrokerType              string `json:"broker_type,omitempty"`
	ExecutionMode           string `json:"execution_mode,omitempty"`
	ZerodhaConnectionStatus string `json:"zerodha_connection_status,omitempty"` // valid | expires_soon | expired; only set for zerodha user_api_oauth

	// Paper trading destination context — populated by the pnl handler for
	// webhooks routed to a paper account (no broker credential exists, so
	// BrokerType/ExecutionMode above stay empty for these).
	PaperAccountLabel string `json:"paper_account_label,omitempty"`
	MarketProfileName string `json:"market_profile_name,omitempty"`
}

func (p *PGStore) GetWebhookPnL(ctx context.Context, webhookID string) (*PnLSummary, error) {
	summary, err := p.aggregatePnL(ctx,
		`FROM trades WHERE webhook_id=$1`, webhookID, "status", "lot_size", "fill_price")
	if err != nil {
		return nil, err
	}
	summary.WebhookID = webhookID
	if summary.PricedTrades > 0 {
		rows, err := p.listPnLRows(ctx,
			`SELECT signal, symbol, lot_size, fill_price, status
			 FROM trades
			 WHERE webhook_id=$1 AND fill_price > 0 AND status = 'filled'
			 ORDER BY created_at ASC`, webhookID)
		if err != nil {
			return nil, err
		}
		summary.WinRate = pnl.WinRate(rows)
	}
	if err := p.enrichWebhookSummary(ctx, webhookID, summary); err != nil {
		return nil, err
	}
	return summary, nil
}

func (p *PGStore) GetOrgPnL(ctx context.Context, orgID string) (*PnLSummary, error) {
	summary, err := p.aggregatePnL(ctx,
		`FROM trades t
		 INNER JOIN webhooks w ON w.id = t.webhook_id
		 WHERE w.org_id=$1`,
		orgID,
		"t.status", "t.lot_size", "t.fill_price")
	if err != nil {
		return nil, err
	}
	summary.OrgID = orgID
	if summary.PricedTrades > 0 {
		rows, err := p.listPnLRows(ctx,
			`SELECT t.signal, t.symbol, t.lot_size, t.fill_price, t.status
			 FROM trades t
			 INNER JOIN webhooks w ON w.id = t.webhook_id
			 WHERE w.org_id=$1 AND t.fill_price > 0 AND t.status = 'filled'
			 ORDER BY t.created_at ASC`, orgID)
		if err != nil {
			return nil, err
		}
		summary.WinRate = pnl.WinRate(rows)
	}
	return summary, nil
}

func (p *PGStore) aggregatePnL(ctx context.Context, fromClause, id, statusCol, lotCol, priceCol string) (*PnLSummary, error) {
	q := `SELECT ` + statusCol + `,
		COUNT(*)::int,
		COALESCE(SUM(` + lotCol + ` * ` + priceCol + `) FILTER (WHERE ` + priceCol + ` > 0), 0),
		COUNT(*) FILTER (WHERE ` + priceCol + ` > 0)::int
	` + fromClause + `
	GROUP BY ` + statusCol

	rows, err := p.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := &PnLSummary{ByStatus: map[string]int{}}
	var filled, rejected int
	for rows.Next() {
		var status string
		var count, priced int
		var notional float64
		if err := rows.Scan(&status, &count, &notional, &priced); err != nil {
			return nil, err
		}
		summary.TradeCount += count
		summary.Notional += notional
		summary.PricedTrades += priced
		summary.ByStatus[status] = count
		if status == "filled" {
			filled = count
		} else if status == "rejected" {
			rejected = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if filled+rejected > 0 {
		rate := float64(filled) / float64(filled+rejected)
		summary.FillRate = &rate
	}
	return summary, nil
}

func (p *PGStore) listPnLRows(ctx context.Context, q string, id string) ([]pnl.Row, error) {
	rows, err := p.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pnl.Row
	for rows.Next() {
		var r pnl.Row
		var lot sql.NullFloat64
		if err := rows.Scan(&r.Signal, &r.Symbol, &lot, &r.FillPrice, &r.Status); err != nil {
			return nil, err
		}
		if lot.Valid {
			r.LotSize = lot.Float64
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
