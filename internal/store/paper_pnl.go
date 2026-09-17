package store

import (
	"context"
	"database/sql"
	"time"
)

// PaperPnLSummary is a Phase 4 dashboard rollup for a single paper account,
// extended for the account detail page's Account Summary / Trading
// Performance / Risk / Diagnostics sections. Equity/TotalPnL are computed
// from RealizedPnL+UnrealizedPnL-TotalFees rather than reconciled against
// CashBalance — see internal/paperengine for why these are equivalent.
// TotalFees (added alongside the market-profile slippage_bps/fee_bps
// settings) is already netted into TotalPnL/Equity, so callers don't need to
// subtract it again — it's also surfaced on its own for a "Total Fees" stat.
//
// Fields populated here (GetPaperAccountPnL) vs. by the caller:
// LinkedWebhooks/Signals*/WebhookFillRate/DuplicateSuppressed/
// LastWebhookSignalAt/MarketProfileName/TradingSession/MarketDataProvider
// are populated by internal/modules/paperaccounts.Handler.GetPaperAccountPnL
// instead — they need a webhook list, the market_profiles row, and (for the
// provider name) the *marketdata.Registry, none of which this package can
// depend on without an import cycle (guard and marketdata both import store).
// Every other field is self-contained here.
type PaperPnLSummary struct {
	PaperAccountID  string         `json:"paper_account_id"`
	StartingBalance float64        `json:"starting_balance"`
	CashBalance     float64        `json:"cash_balance"`
	Equity          float64        `json:"equity"`
	RealizedPnL     float64        `json:"realized_pnl"`
	UnrealizedPnL   float64        `json:"unrealized_pnl"`
	TotalPnL        float64        `json:"total_pnl"`
	TradeCount      int            `json:"trade_count"`
	ByStatus        map[string]int `json:"by_status"`
	Notional        float64        `json:"notional"`
	FillRate        *float64       `json:"fill_rate,omitempty"`
	WinRate         *float64       `json:"win_rate,omitempty"`
	TotalFees       float64        `json:"total_fees"`

	// Account Health — first thing a trader checks before placing more trades.
	AccountHealthStatus string  `json:"account_health_status"` // healthy | paused | warning
	BuyingPower         float64 `json:"buying_power"`
	UsedMargin          float64 `json:"used_margin"`

	// Today's Performance — day boundary computed in the account's market
	// profile timezone (falls back to UTC if the profile/timezone can't be
	// resolved), not the server's local time.
	TodayPnL      *float64 `json:"today_pnl,omitempty"`
	TodayTrades   int      `json:"today_trades"`
	TodayWinRate  *float64 `json:"today_win_rate,omitempty"`
	TodayFees     float64  `json:"today_fees"`
	TodayNotional float64  `json:"today_notional"`

	// Trade Distribution — order-level (paper_orders), not fill-level.
	BuyOrders   int `json:"buy_orders"`
	SellOrders  int `json:"sell_orders"`
	CloseOrders int `json:"close_orders"`
	Filled      int `json:"filled"`
	Rejected    int `json:"rejected"`
	Cancelled   int `json:"cancelled"`
	// DuplicateSuppressed is populated by the handler from ingest_log
	// (dedup happens before a paper_orders row ever exists).
	DuplicateSuppressed int `json:"duplicate_suppressed"`

	// Win/Loss distribution — from paper_trades.realized_pnl, exact (the
	// fill engine computes it directly, unlike live trades which have to
	// approximate a "win" via fill_price vs average entry price).
	LargestWin   *float64 `json:"largest_win,omitempty"`
	LargestLoss  *float64 `json:"largest_loss,omitempty"`
	AverageWin   *float64 `json:"average_win,omitempty"`
	AverageLoss  *float64 `json:"average_loss,omitempty"`
	GrossProfit  float64  `json:"gross_profit"`
	GrossLoss    float64  `json:"gross_loss"`
	ProfitFactor *float64 `json:"profit_factor,omitempty"`
	Expectancy   *float64 `json:"expectancy,omitempty"`

	// Drawdown — from paper_account_snapshots, written on every fill and by
	// the periodic mark-to-market sweep (internal/marketdata/snapshotjob.go),
	// so this isn't fill-only/sparse — only gaps while flat with no open
	// position (equity can't move then anyway, so that's not a real gap).
	PeakEquity      float64 `json:"peak_equity"`
	CurrentDrawdown float64 `json:"current_drawdown"`
	MaxDrawdown     float64 `json:"max_drawdown"`

	// Exposure & positions — current snapshot from paper_positions.
	Exposure        float64  `json:"exposure"`
	ExposurePct     *float64 `json:"exposure_pct,omitempty"`
	LargestPosition float64  `json:"largest_position"`
	LongPositions   int      `json:"long_positions"`
	ShortPositions  int      `json:"short_positions"`
	OpenPositions   int      `json:"open_positions"`
	// OpenOrders is expected to always read 0 today: the paper engine
	// resolves every order synchronously (immediate fill or reject), so
	// there's no pending/in-flight order state yet. Included for schema
	// completeness / future-proofing if that ever changes.
	OpenOrders int `json:"open_orders"`

	// Risk summary — open risk is aggregate unrealized loss on open positions;
	// today_risk is the sum of losing trade magnitudes closed today.
	OpenRisk  float64 `json:"open_risk"`
	TodayRisk float64 `json:"today_risk"`

	// Recent Activity proxy — last time any position's mark/fill touched
	// this account. Populated here (paper_positions is store-internal);
	// LastWebhookSignalAt (a different timestamp — last ingest, not last
	// fill/mark) is populated by the handler from webhook diagnostics.
	LastMarketUpdateAt *time.Time `json:"last_market_update_at,omitempty"`

	// Diagnostics — aggregated across every webhook routed to this account
	// (there's no uniqueness constraint, so there can be more than one).
	// Populated by the handler.
	LinkedWebhooks      []PaperLinkedWebhook `json:"linked_webhooks,omitempty"`
	SignalsReceived     int                  `json:"signals_received"`
	SignalsAccepted     int                  `json:"signals_accepted"`
	SignalsRejected     int                  `json:"signals_rejected"`
	WebhookFillRate     *float64             `json:"webhook_fill_rate,omitempty"`
	LastWebhookSignalAt *time.Time           `json:"last_webhook_signal_at,omitempty"`

	// Market info — populated by the handler.
	MarketProfileName  string `json:"market_profile_name,omitempty"`
	TradingSession     string `json:"trading_session,omitempty"` // "open" | "closed"
	MarketDataProvider string `json:"market_data_provider,omitempty"`
}

// PaperLinkedWebhook is a minimal webhook reference for the Diagnostics
// section's "which webhook(s) feed this account" list.
type PaperLinkedWebhook struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

// GetPaperAccountPnL computes the Phase 4+ dashboard metrics for a paper
// account. Returns nil, nil if the account does not exist.
func (p *PGStore) GetPaperAccountPnL(ctx context.Context, accountID string) (*PaperPnLSummary, error) {
	account, err := p.GetPaperAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, nil
	}

	summary := &PaperPnLSummary{
		PaperAccountID:  accountID,
		StartingBalance: account.StartingBalance,
		CashBalance:     account.CashBalance,
		ByStatus:        map[string]int{},
	}

	// (a) Order status + side breakdown -> fill rate + trade distribution.
	// paper_orders.status/side are uppercase (FILLED/REJECTED/CANCELLED,
	// BUY/SELL/CLOSE), unlike live trades' lowercase equivalents.
	rows, err := p.db.QueryContext(ctx,
		`SELECT status, side, COUNT(*)::int FROM paper_orders WHERE paper_account_id=$1 GROUP BY status, side`, accountID)
	if err != nil {
		return nil, err
	}
	var filled, rejected int
	for rows.Next() {
		var status, side string
		var count int
		if err := rows.Scan(&status, &side, &count); err != nil {
			rows.Close()
			return nil, err
		}
		summary.ByStatus[status] += count
		switch status {
		case "FILLED":
			filled += count
			summary.Filled += count
		case "REJECTED":
			rejected += count
			summary.Rejected += count
		case "CANCELLED":
			summary.Cancelled += count
		}
		switch side {
		case "BUY":
			summary.BuyOrders += count
		case "SELL":
			summary.SellOrders += count
		case "CLOSE":
			summary.CloseOrders += count
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	// OpenOrders: anything not in a terminal state — see doc comment above.
	for status, count := range summary.ByStatus {
		if status != "FILLED" && status != "REJECTED" && status != "CANCELLED" {
			summary.OpenOrders += count
		}
	}
	if filled+rejected > 0 {
		rate := float64(filled) / float64(filled+rejected)
		summary.FillRate = &rate
	}

	// (b) Trade aggregates -> trade count, notional, realized P&L, fees,
	// win/loss distribution, profit factor, expectancy.
	var tradeCount, wins, losses int
	var notional, realizedPnL, totalFees, grossProfit, grossLoss float64
	var largestWin, largestLoss, avgWin, avgLoss sql.NullFloat64
	err = p.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::int,
		       COALESCE(SUM(quantity * price), 0),
		       COALESCE(SUM(realized_pnl), 0),
		       COALESCE(SUM(fees), 0),
		       COUNT(*) FILTER (WHERE realized_pnl > 0)::int,
		       COUNT(*) FILTER (WHERE realized_pnl < 0)::int,
		       COALESCE(SUM(realized_pnl) FILTER (WHERE realized_pnl > 0), 0),
		       COALESCE(SUM(realized_pnl) FILTER (WHERE realized_pnl < 0), 0),
		       MAX(realized_pnl) FILTER (WHERE realized_pnl > 0),
		       MIN(realized_pnl) FILTER (WHERE realized_pnl < 0),
		       AVG(realized_pnl) FILTER (WHERE realized_pnl > 0),
		       AVG(realized_pnl) FILTER (WHERE realized_pnl < 0)
		FROM paper_trades WHERE paper_account_id=$1`, accountID,
	).Scan(&tradeCount, &notional, &realizedPnL, &totalFees, &wins, &losses,
		&grossProfit, &grossLoss, &largestWin, &largestLoss, &avgWin, &avgLoss)
	if err != nil {
		return nil, err
	}
	summary.TradeCount = tradeCount
	summary.Notional = notional
	summary.RealizedPnL = realizedPnL
	summary.TotalFees = totalFees
	summary.GrossProfit = grossProfit
	summary.GrossLoss = grossLoss
	if largestWin.Valid {
		summary.LargestWin = &largestWin.Float64
	}
	if largestLoss.Valid {
		summary.LargestLoss = &largestLoss.Float64
	}
	if avgWin.Valid {
		summary.AverageWin = &avgWin.Float64
	}
	if avgLoss.Valid {
		summary.AverageLoss = &avgLoss.Float64
	}
	var winRate float64
	if wins+losses > 0 {
		winRate = float64(wins) / float64(wins+losses)
		summary.WinRate = &winRate
	}
	if grossLoss < 0 {
		pf := grossProfit / -grossLoss
		summary.ProfitFactor = &pf
	}
	if (wins+losses) > 0 && avgWin.Valid && avgLoss.Valid {
		exp := winRate*avgWin.Float64 + (1-winRate)*avgLoss.Float64
		summary.Expectancy = &exp
	}

	// (c) Open position mark-to-market -> unrealized P&L, exposure, largest
	// position, long/short counts, open risk, last mark/fill time.
	var unrealizedPnL, exposure, largestPosition, openRisk sql.NullFloat64
	var longs, shorts, openPositions int
	var lastMarketUpdate sql.NullTime
	err = p.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(unrealized_pnl), 0),
			COALESCE(SUM(ABS(quantity) * last_price), 0),
			MAX(ABS(quantity) * last_price),
			COUNT(*) FILTER (WHERE quantity > 0)::int,
			COUNT(*) FILTER (WHERE quantity < 0)::int,
			COUNT(*) FILTER (WHERE quantity <> 0)::int,
			COALESCE(SUM(-unrealized_pnl) FILTER (WHERE quantity <> 0 AND unrealized_pnl < 0), 0),
			MAX(updated_at)
		FROM paper_positions WHERE paper_account_id=$1`, accountID,
	).Scan(&unrealizedPnL, &exposure, &largestPosition, &longs, &shorts, &openPositions, &openRisk, &lastMarketUpdate)
	if err != nil {
		return nil, err
	}
	summary.UnrealizedPnL = unrealizedPnL.Float64
	summary.Exposure = exposure.Float64
	if largestPosition.Valid {
		summary.LargestPosition = largestPosition.Float64
	}
	summary.LongPositions = longs
	summary.ShortPositions = shorts
	summary.OpenPositions = openPositions
	summary.OpenRisk = openRisk.Float64
	if lastMarketUpdate.Valid {
		summary.LastMarketUpdateAt = &lastMarketUpdate.Time
	}

	summary.UsedMargin = summary.Exposure
	summary.BuyingPower = summary.CashBalance

	summary.TotalPnL = summary.RealizedPnL + summary.UnrealizedPnL - summary.TotalFees
	summary.Equity = summary.StartingBalance + summary.TotalPnL
	if summary.Equity > 0 {
		pct := summary.Exposure / summary.Equity
		summary.ExposurePct = &pct
	}

	// (d) Drawdown — running peak over the account's equity snapshot
	// history, oldest to newest.
	peak, current, maxDD := 0.0, 0.0, 0.0
	snapRows, err := p.db.QueryContext(ctx,
		`SELECT equity FROM paper_account_snapshots WHERE paper_account_id=$1 ORDER BY created_at ASC`, accountID)
	if err != nil {
		return nil, err
	}
	haveSnapshot := false
	for snapRows.Next() {
		var eq float64
		if err := snapRows.Scan(&eq); err != nil {
			snapRows.Close()
			return nil, err
		}
		haveSnapshot = true
		if eq > peak {
			peak = eq
		}
		if dd := peak - eq; dd > maxDD {
			maxDD = dd
		}
		current = eq
	}
	if err := snapRows.Err(); err != nil {
		snapRows.Close()
		return nil, err
	}
	snapRows.Close()
	if haveSnapshot {
		summary.PeakEquity = peak
		summary.MaxDrawdown = maxDD
		summary.CurrentDrawdown = peak - current
	} else {
		// No snapshot yet (brand-new account) — starting balance is the
		// only equity point that exists.
		summary.PeakEquity = summary.Equity
	}

	// (e) Today's Performance — day boundary in the account's market
	// profile timezone, falling back to UTC if unresolvable. A config gap
	// here shouldn't block the rest of the summary, so errors are swallowed
	// (matches guard.CheckMarketHours' fail-open convention).
	loc := time.UTC
	if profile, err := p.GetMarketProfileByCode(ctx, account.MarketProfile); err == nil && profile != nil && profile.Timezone != "" {
		if l, err := time.LoadLocation(profile.Timezone); err == nil {
			loc = l
		}
	}
	now := time.Now().In(loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC()

	var todayTrades, todayWins, todayLosses int
	var todayNotional, todayPnL, todayFees, todayRisk float64
	err = p.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::int,
		       COALESCE(SUM(quantity * price), 0),
		       COALESCE(SUM(realized_pnl), 0),
		       COALESCE(SUM(fees), 0),
		       COUNT(*) FILTER (WHERE realized_pnl > 0)::int,
		       COUNT(*) FILTER (WHERE realized_pnl < 0)::int,
		       COALESCE(SUM(ABS(realized_pnl)) FILTER (WHERE realized_pnl < 0), 0)
		FROM paper_trades WHERE paper_account_id=$1 AND created_at >= $2`, accountID, startOfDay,
	).Scan(&todayTrades, &todayNotional, &todayPnL, &todayFees, &todayWins, &todayLosses, &todayRisk)
	if err != nil {
		return nil, err
	}
	summary.TodayTrades = todayTrades
	summary.TodayNotional = todayNotional
	summary.TodayFees = todayFees
	if todayTrades > 0 {
		pnl := todayPnL - todayFees
		summary.TodayPnL = &pnl
	}
	if todayWins+todayLosses > 0 {
		rate := float64(todayWins) / float64(todayWins+todayLosses)
		summary.TodayWinRate = &rate
	}
	summary.TodayRisk = todayRisk

	switch account.Status {
	case "paused":
		summary.AccountHealthStatus = "paused"
	default:
		summary.AccountHealthStatus = "healthy"
		if summary.StartingBalance > 0 && summary.MaxDrawdown/summary.StartingBalance > 0.10 {
			summary.AccountHealthStatus = "warning"
		} else if summary.ExposurePct != nil && *summary.ExposurePct > 1 {
			summary.AccountHealthStatus = "warning"
		}
	}

	return summary, nil
}
