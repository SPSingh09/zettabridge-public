package paperengine

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// validateOrder checks side/order-type/price/instrument constraints are
// satisfied, returning a non-empty rejection reason if not. Once it returns
// "", req.Price > 0.
func validateOrder(req domain.ExecutionRequest) (rejectReason string) {
	switch req.Side {
	case string(domain.SideBuy), string(domain.SideSell), string(domain.SideClose):
	default:
		return fmt.Sprintf("unsupported side %q", req.Side)
	}

	switch req.OrderType {
	case string(domain.OrderTypeMarket):
		if req.Price <= 0 {
			return "price is required for paper MARKET order until live data feed is enabled"
		}
	case string(domain.OrderTypeLimit):
		if req.Price <= 0 {
			return "price is required for paper LIMIT order in v1"
		}
	default:
		return fmt.Sprintf("unsupported order type %q", req.OrderType)
	}

	// Phase 7: instrument tick/lot size — the caller (queue/paper.go) already
	// rejects unknown/inactive symbols before building this request, so
	// TickSize/LotSize are always > 0 here in production; a 0 just means
	// "no constraint" (harmless for tests that don't set them).
	if req.TickSize > 0 && !isMultiple(req.Price, req.TickSize) {
		return fmt.Sprintf("price %.4f is not a multiple of tick size %.4f", req.Price, req.TickSize)
	}
	if req.Side != string(domain.SideClose) && req.LotSize > 0 && !isMultiple(req.Quantity, req.LotSize) {
		return fmt.Sprintf("quantity %.4f is not a multiple of lot size %.4f", req.Quantity, req.LotSize)
	}
	return ""
}

// isMultiple reports whether value is an integer multiple of step, tolerant
// of float64 rounding error.
func isMultiple(value, step float64) bool {
	if step <= 0 {
		return true
	}
	ratio := value / step
	return math.Abs(ratio-math.Round(ratio)) < 1e-6
}

// resolveCloseSide translates a CLOSE signal into the concrete side and
// quantity needed to flatten the current position. ok is false when there is
// no open position to close.
func resolveCloseSide(currentQty float64) (side string, qty float64, ok bool) {
	if currentQty == 0 {
		return "", 0, false
	}
	if currentQty > 0 {
		return string(domain.SideSell), currentQty, true
	}
	return string(domain.SideBuy), -currentQty, true
}

// applyBuy applies a BUY fill to a signed position (positive = long, negative
// = short), returning the new quantity/average entry price and any realized
// P&L from reducing or flipping a short position.
func applyBuy(currentQty, currentAvg, qty, fillPrice float64) (newQty, newAvg, realizedPnL float64) {
	if currentQty >= 0 {
		newQty = currentQty + qty
		newAvg = (currentAvg*currentQty + fillPrice*qty) / newQty
		return newQty, newAvg, 0
	}
	shortQty := -currentQty
	if qty <= shortQty {
		realizedPnL = (currentAvg - fillPrice) * qty
		newQty = currentQty + qty
		newAvg = currentAvg
		if newQty == 0 {
			newAvg = 0
		}
		return newQty, newAvg, realizedPnL
	}
	realizedPnL = (currentAvg - fillPrice) * shortQty
	newQty = qty - shortQty
	newAvg = fillPrice
	return newQty, newAvg, realizedPnL
}

// applySell is the mirror of applyBuy for a SELL fill.
func applySell(currentQty, currentAvg, qty, fillPrice float64) (newQty, newAvg, realizedPnL float64) {
	if currentQty <= 0 {
		absCurrent := -currentQty
		newQty = currentQty - qty
		newAvg = (currentAvg*absCurrent + fillPrice*qty) / -newQty
		return newQty, newAvg, 0
	}
	longQty := currentQty
	if qty <= longQty {
		realizedPnL = (fillPrice - currentAvg) * qty
		newQty = currentQty - qty
		newAvg = currentAvg
		if newQty == 0 {
			newAvg = 0
		}
		return newQty, newAvg, realizedPnL
	}
	realizedPnL = (fillPrice - currentAvg) * longQty
	newQty = -(qty - longQty)
	newAvg = fillPrice
	return newQty, newAvg, realizedPnL
}

// computeFill applies the MVP fill and position-update rules. It is pure
// (no I/O) — store.ExecutePaperFill calls it while holding row locks.
func computeFill(now time.Time, account *store.PaperAccount, position *store.PaperPosition, req domain.ExecutionRequest) (*store.PaperFillOutcome, error) {
	baseOrder := &store.PaperOrder{
		ID:             uuid.NewString(),
		UserID:         account.UserID,
		PaperAccountID: req.AccountID,
		SignalID:       req.SignalID,
		Symbol:         req.Symbol,
		Exchange:       req.Exchange,
		Side:           req.Side,
		OrderType:      req.OrderType,
		Product:        req.Product,
		Quantity:       req.Quantity,
		RawSignal:      req.RawSignal,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if req.WebhookID != "" {
		baseOrder.WebhookID = &req.WebhookID
	}
	if req.Price > 0 {
		price := req.Price
		baseOrder.RequestedPrice = &price
	}

	reject := func(reason string) (*store.PaperFillOutcome, error) {
		baseOrder.Status = string(domain.PaperOrderStatusRejected)
		baseOrder.Reason = reason
		return &store.PaperFillOutcome{Order: baseOrder}, nil
	}

	if reason := validateOrder(req); reason != "" {
		return reject(reason)
	}
	fillPrice := req.Price

	currentQty, currentAvg := 0.0, 0.0
	if position != nil {
		currentQty = position.Quantity
		currentAvg = position.AvgEntryPrice
	}

	side := req.Side
	qty := req.Quantity
	if side == string(domain.SideClose) {
		s, q, ok := resolveCloseSide(currentQty)
		if !ok {
			return reject("No open position to close")
		}
		side, qty = s, q
	} else if qty <= 0 {
		return reject("quantity must be greater than zero")
	}

	// Slippage worsens the fill price in the adverse direction, simulating
	// market impact — applied after CLOSE resolves its concrete side, since
	// the adverse direction depends on whether this fill is ultimately a
	// BUY or a SELL. 0 (default) leaves fillPrice unchanged.
	if req.SlippageBps > 0 {
		switch side {
		case string(domain.SideBuy):
			fillPrice *= 1 + req.SlippageBps/10000
		case string(domain.SideSell):
			fillPrice *= 1 - req.SlippageBps/10000
		}
	}

	var newQty, newAvg, realizedPnL, cashDelta float64
	switch side {
	case string(domain.SideBuy):
		cashDelta = -fillPrice * qty
		newQty, newAvg, realizedPnL = applyBuy(currentQty, currentAvg, qty, fillPrice)
	case string(domain.SideSell):
		cashDelta = fillPrice * qty
		newQty, newAvg, realizedPnL = applySell(currentQty, currentAvg, qty, fillPrice)
	}

	// FeeBps is a simplified combined brokerage+taxes+charges estimate,
	// applied to notional (fillPrice * qty) and deducted from cash separately
	// from realizedPnL, which stays gross — matching how `fees` is already a
	// separate column from `realized_pnl` in the schema (net P&L is computed
	// at the summary layer, not baked into the per-trade realized_pnl number).
	fee := fillPrice * qty * req.FeeBps / 10000
	cashDelta -= fee

	baseOrder.Status = string(domain.PaperOrderStatusFilled)
	baseOrder.FillPrice = &fillPrice

	newPosition := &store.PaperPosition{
		UserID:         account.UserID,
		PaperAccountID: req.AccountID,
		Symbol:         req.Symbol,
		Exchange:       req.Exchange,
		Product:        req.Product,
		Quantity:       newQty,
		AvgEntryPrice:  newAvg,
		LastPrice:      fillPrice,
		UnrealizedPnL:  domain.UnrealizedPnL(newQty, newAvg, fillPrice),
		UpdatedAt:      now,
	}
	if position != nil {
		newPosition.ID = position.ID
		newPosition.CreatedAt = position.CreatedAt
	} else {
		newPosition.ID = uuid.NewString()
		newPosition.CreatedAt = now
	}

	trade := &store.PaperTrade{
		ID:             uuid.NewString(),
		UserID:         account.UserID,
		PaperAccountID: req.AccountID,
		OrderID:        baseOrder.ID,
		Symbol:         req.Symbol,
		Exchange:       req.Exchange,
		Side:           side,
		Quantity:       qty,
		Price:          fillPrice,
		RealizedPnL:    realizedPnL,
		Fees:           fee,
		CreatedAt:      now,
	}

	newCashBalance := account.CashBalance + cashDelta

	return &store.PaperFillOutcome{
		Order:          baseOrder,
		Position:       newPosition,
		Trade:          trade,
		NewCashBalance: &newCashBalance,
	}, nil
}
