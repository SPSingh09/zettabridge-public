package queue

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/marketdata"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// processPaperOrder routes a signal to the Paper Trading Engine instead of a
// live broker. It reuses the trade row already built by process() (UserID,
// WebhookID, Signal, Symbol, SignalKey, Comment, CreatedAt) and the same
// insertTrade path (metrics/notifications/websocket), so paper trades show up
// in the same webhook trade history as live trades.
func (q *Queue) processPaperOrder(ctx context.Context, wh *store.Webhook, sig *store.SignalPayload, params guard.TradeParams, trade *store.Trade) {
	reject := func(code, reason string) {
		trade.Status = domain.StatusRejected
		trade.Error = reason
		trade.ErrorCode = code
		q.finalizeTrade(ctx, trade, "paper")
	}

	if q.orchestrator == nil || q.orchestrator.PaperRunner() == nil {
		reject("paper_not_configured", "paper trading engine not configured")
		return
	}

	account, err := q.pg.GetPaperAccount(ctx, *wh.PaperAccountID)
	if err != nil || account == nil {
		reject("paper_account_not_found", "paper account not found")
		return
	}
	if account.Status != "active" {
		reject("paper_account_inactive", fmt.Sprintf("paper account is %s", account.Status))
		return
	}
	applyTradeSignalMeta(trade, wh, sig, params, account.DefaultProduct)

	user, err := q.pg.GetUserByID(ctx, wh.UserID)
	if err != nil || user == nil {
		reject(string(brokererr.CodeInternal), "user not found")
		return
	}
	if reason, quotaHit := q.enforcePaperTradeQuota(ctx, user); reason != "" {
		if quotaHit {
			q.notifyPaperQuotaExceeded(ctx, user)
		}
		reject("paper_quota_exceeded", reason)
		return
	}

	instrument, err := q.pg.GetInstrument(ctx, account.MarketProfile, account.Exchange, params.Symbol)
	if err != nil {
		reject(string(brokererr.CodeInternal), "instrument lookup failed")
		return
	}
	if instrument == nil || !instrument.Active {
		reject("instrument_not_available", fmt.Sprintf(
			"%s:%s is not available for paper trading",
			store.NormalizePaperExchange(account.Exchange),
			store.NormalizePaperSymbol(params.Symbol),
		))
		return
	}

	profile, err := q.pg.GetMarketProfileByCode(ctx, account.MarketProfile)
	if err != nil {
		reject(string(brokererr.CodeInternal), "market profile lookup failed")
		return
	}
	if profile == nil {
		reject("market_profile_not_found", fmt.Sprintf("unknown market profile %q", account.MarketProfile))
		return
	}

	if reason := q.checkPaperMarketHours(ctx, account); reason != "" {
		reject("outside_trading_hours", reason)
		return
	}

	lot, err := guard.ResolveQuantity(wh, params.Lot)
	if err != nil {
		reject(guard.ErrCode(err), err.Error())
		return
	}
	lot, err = guard.ApplyMaxLot(wh, lot)
	if err != nil {
		reject(guard.ErrCode(err), err.Error())
		return
	}
	trade.LotSize = lot

	orderType := guard.WebhookOrderType(wh)

	symbol := store.NormalizePaperSymbol(params.Symbol)
	if instrument != nil && strings.TrimSpace(instrument.Symbol) != "" {
		symbol = store.NormalizePaperSymbol(instrument.Symbol)
	}
	exchange := store.NormalizePaperExchange(account.Exchange)

	fillPrice := params.Price
	if fillPrice <= 0 && orderType == string(domain.OrderTypeMarket) {
		if q.mdRegistry != nil {
			var err error
			fillPrice, err = q.mdRegistry.ResolveMarketFillPrice(ctx, q.pg, exchange, symbol)
			if err != nil && !errors.Is(err, marketdata.ErrNoQuote) {
				log.Printf("worker: paper live quote lookup failed symbol=%s: %v", symbol, err)
				reject("quote_unavailable", "live quote lookup failed — check FYERS connection in admin settings")
				return
			}
		}
		if fillPrice <= 0 {
			provider := marketdata.ProviderSignal
			if q.mdRegistry != nil {
				provider = q.mdRegistry.ActiveProviderName(ctx, q.pg)
			}
			if provider == marketdata.ProviderFyers {
				reject("quote_unavailable", fmt.Sprintf("no live quote available for %s — reconnect FYERS in admin settings", symbol))
			} else {
				reject("price_required", "price is required for paper MARKET order until live data feed is enabled")
			}
			return
		}
	}

	product := strings.ToUpper(strings.TrimSpace(params.Product))
	if product == "" {
		product = strings.ToUpper(strings.TrimSpace(account.DefaultProduct))
	}

	req := domain.ExecutionRequest{
		AccountID:     account.ID,
		UserID:        wh.UserID,
		WebhookID:     wh.ID,
		SignalID:      trade.ID,
		MarketProfile: account.MarketProfile,
		Exchange:      exchange,
		Symbol:        symbol,
		Side:          sig.Action,
		OrderType:     orderType,
		Product:       product,
		Quantity:      lot,
		Price:         fillPrice,
		TickSize:      instrument.TickSize,
		LotSize:       instrument.LotSize,
		SlippageBps:   profile.SlippageBps,
		FeeBps:        profile.FeeBps,
	}

	result, err := q.orchestrator.ExecutePaper(ctx, req)
	if err != nil {
		code, msg := brokererr.TradeFields(err)
		reject(code, msg)
		return
	}

	if result.Status == string(domain.PaperOrderStatusFilled) {
		trade.Status = domain.StatusFilled
	} else {
		trade.Status = domain.StatusRejected
		trade.Error = result.Reason
		trade.ErrorCode = classifyPaperEngineReason(result.Reason)
	}
	trade.BrokerOrder = result.OrderID
	trade.FillPrice = result.FillPrice
	log.Printf("worker: paper %s webhook=%s order=%s symbol=%s action=%s",
		trade.Status, wh.ID, result.OrderID, params.Symbol, sig.Action)
	q.finalizeTrade(ctx, trade, "paper")
}

// classifyPaperEngineReason maps a paperengine validateOrder() free-text
// reason to a stable code for the webhook Summary's rejection breakdown.
// The engine only returns a string (internal/paperengine/fill.go), so this
// is necessarily substring-based — kept narrow and defaulting to
// "invalid_order" so a wording change there degrades gracefully instead of
// silently mis-labeling.
func classifyPaperEngineReason(reason string) string {
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "tick size"):
		return "invalid_price"
	case strings.Contains(lower, "lot size"):
		return "quantity_required"
	case strings.Contains(lower, "price is required"):
		return "price_required"
	default:
		return "invalid_order"
	}
}

// checkPaperMarketHours returns a non-empty rejection reason when Phase 7's
// market-hours enforcement is on (the platform_settings default when unset)
// and the account's market profile is currently outside its trading hours.
// A missing/unreadable platform setting or market profile row fails open
// (no rejection) — a config gap shouldn't block every paper order.
func (q *Queue) checkPaperMarketHours(ctx context.Context, account *store.PaperAccount) string {
	return q.checkPaperMarketHoursAt(ctx, account, time.Now().UTC())
}

// checkPaperMarketHoursAt is checkPaperMarketHours with an injectable clock,
// so tests don't depend on the wall-clock time the suite happens to run at.
// Delegates to guard.CheckMarketHours, shared with the
// paperaccounts.Handler.ClosePaperPosition HTTP path.
func (q *Queue) checkPaperMarketHoursAt(ctx context.Context, account *store.PaperAccount, now time.Time) string {
	return guard.CheckMarketHours(ctx, q.pg, account.MarketProfile, now)
}
