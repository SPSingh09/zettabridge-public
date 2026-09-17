package domain

import "context"

// Side is the signal action for an ExecutionRequest.
type Side string

const (
	SideBuy   Side = "BUY"
	SideSell  Side = "SELL"
	SideClose Side = "CLOSE"
)

// OrderType is the entry execution type for an ExecutionRequest.
type OrderType string

const (
	OrderTypeMarket OrderType = "MARKET"
	OrderTypeLimit  OrderType = "LIMIT"
)

// PaperOrderStatus is the terminal status of a paper order fill attempt.
// Matches the CHECK constraint values on paper_orders.status.
type PaperOrderStatus string

const (
	PaperOrderStatusFilled   PaperOrderStatus = "FILLED"
	PaperOrderStatusRejected PaperOrderStatus = "REJECTED"
)

// SignalType discriminates the two webhook event kinds accepted by
// paper-destination webhooks. Live-broker webhooks are unaffected — they
// keep the legacy flat action/lot/comment payload with no type field.
type SignalType string

const (
	// SignalTypeOrderSignal places or closes a paper order.
	SignalTypeOrderSignal SignalType = "ORDER_SIGNAL"
	// SignalTypePriceUpdate marks a symbol's last price for an open paper
	// position (unrealized P&L only — no order, trade, or cash movement).
	SignalTypePriceUpdate SignalType = "PRICE_UPDATE"
)

// UnrealizedPnL computes mark-to-market P&L for a signed position quantity
// (positive = long, negative = short) at lastPrice.
func UnrealizedPnL(quantity, avgEntryPrice, lastPrice float64) float64 {
	if quantity > 0 {
		return (lastPrice - avgEntryPrice) * quantity
	}
	if quantity < 0 {
		return (avgEntryPrice - lastPrice) * -quantity
	}
	return 0
}

// ExecutionRequest is the normalized order built from a webhook signal,
// destined for either the Paper Trading Engine or a live broker adapter.
type ExecutionRequest struct {
	AccountID     string // paper_account_id (paper) or broker_credential_id (live)
	WebhookID     string
	SignalID      string
	MarketProfile string // indian_equity | indian_fno | forex | crypto_spot
	Exchange      string
	Symbol        string
	Side          string // domain.SideBuy | SideSell | SideClose
	OrderType     string // domain.OrderTypeMarket | OrderTypeLimit
	Product       string
	Quantity      float64
	Price         float64 // signal price; required for MARKET, used as the v1 fill price for LIMIT
	RawSignal     []byte

	// TickSize/LotSize come from the instrument master (Phase 7) — 0 means
	// no instrument was found (callers should reject before Execute in that
	// case; these are validated here only as a defense-in-depth check).
	TickSize float64 // Price must be a multiple of this
	LotSize  float64 // Quantity must be a multiple of this (ignored for CLOSE — quantity is engine-computed there)

	// SlippageBps/FeeBps come from the market profile (simplified fee model —
	// see migration 042). Both 0 (default) means no friction, identical to
	// pre-fee-model behavior.
	SlippageBps float64
	FeeBps      float64

	// The following are live-broker-only concerns, ignored by
	// paperengine.Engine (always zero-valued for paper requests) — added so
	// live execution can dispatch through this same interface instead of
	// being special-cased inline in queue.process().
	UserID   string  // live: needed to re-derive plan/early-bird/org context
	OrgID    *string // live: enterprise org context; nil for solo
	Comment  string  // live: passed through to the broker order
	SLPoints int     // live: bracket order SL points
	TPPoints int     // live: bracket order TP points

	// FixedLotSize/MaxRiskPct/MaxLotSize are the webhook's lot-sizing config
	// (store.Webhook.LotSize/MaxRiskPct/MaxLotSize) — live resolves the final
	// order quantity itself (risk-based sizing via broker equity + the
	// webhook's max-lot cap), unlike paper where the caller pre-resolves
	// Quantity before calling Execute.
	FixedLotSize float64
	MaxRiskPct   float64
	MaxLotSize   float64
}

// ExecutionResult is returned after an ExecutionDestination processes a request.
type ExecutionResult struct {
	OrderID     string
	Status      string // PaperOrderStatusFilled|Rejected for paper; Submitted|Filled|Rejected|PendingConfirmation for live (see StatusSubmitted etc. in broker.go)
	FillPrice   float64
	RealizedPnL float64
	Reason      string // populated when Status == REJECTED

	// The following are live-broker-only, always zero-valued/empty for paper.
	Quantity   float64        // live: final resolved lot (after risk-sizing/ApplyMaxLot), for trade.LotSize
	OrderType  string         // live: the broker PlaceRequest's OrderType (domain.OrderMarket|OrderBracket — whether SL/TP made it a bracket order), for trade.OrderType
	SLPrice    float64        // live: actual SL price used (bracket orders)
	TPPrice    float64        // live: actual TP price used (bracket orders)
	AlgoID     string         // live: SEBI algo ID actually used
	BrokerType string         // live: cred.BrokerType, for the zettabridge_trades_total metric tag on success (failures carry this via executionError instead)
	Handoff    *HandoffResult // live: non-nil only for Zerodha Publisher mode
}

// ExecutionDestination executes a normalized order against a specific
// destination. PaperExecutionEngine is the only implementation for
// DestinationKindPaperTrading; DestinationKindLiveBroker requests are routed
// through the existing broker packages (brokerfactory, livebrokers) instead.
type ExecutionDestination interface {
	Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error)
}
