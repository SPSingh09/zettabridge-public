package domain

import "context"

const (
	StatusSubmitted           = "submitted"
	StatusFilled              = "filled"
	StatusRejected            = "rejected"
	StatusPendingConfirmation = "pending_confirmation" // Publisher: awaiting user action on Kite

	QuantityLots   = "lots"
	QuantityShares = "shares"

	OrderMarket  = "market"
	OrderBracket = "bracket"
)

// ExecutionMode describes how a broker credential places orders.
type ExecutionMode string

const (
	ExecutionModeDirectAPI    ExecutionMode = "direct_api"    // Angel, Dhan, MT5
	ExecutionModeUserAPIOAuth ExecutionMode = "user_api_oauth" // Zerodha Kite Connect API
	ExecutionModePublisher    ExecutionMode = "publisher"      // Zerodha Kite Publisher (browser handoff)
)

// DestinationKind distinguishes ZettaBridge's own broker-free Paper Trading
// Engine (Phase 1+) from a live broker connection. It is unrelated to
// ExecutionMode above (which describes how a *live* broker credential places
// orders) and to plan.AccountDemo/AccountLive (which selects a broker's own
// sandbox vs production API) — a paper trading account never reaches broker
// code at all. Not to be confused with the ExecutionDestination interface
// below, which is the thing that actually executes an order.
type DestinationKind string

const (
	DestinationKindPaperTrading DestinationKind = "paper_trading"
	DestinationKindLiveBroker   DestinationKind = "live_broker"
)

// HandoffResult carries the POST-form fields needed for the browser to submit
// a Kite basket order. Only set when ExecutionMode = publisher.
type HandoffResult struct {
	Provider         string            // "zerodha"
	Method           string            // "POST_FORM"
	Action           string            // "https://kite.zerodha.com/connect/basket"
	Fields           map[string]string // api_key + data (basket JSON)
	PublisherOrderID string
}

// PlaceRequest is the normalized order sent to broker adapters after maporder.Build.
type PlaceRequest struct {
	Action       string // BUY | SELL | CLOSE
	Symbol       string // user symbol (e.g. RELIANCE)
	Exchange     string // NSE | BSE (Indian); empty for MT5
	Product      string // MIS | CNC | NRML (Indian)
	Quantity     float64
	QuantityUnit string // lots | shares
	SLPoints     int
	TPPoints     int
	Comment      string
	AlgoID       string // exchange-assigned SEBI algo ID (Indian live brokers)
	OrderType    string // market | bracket (SL/TP as bracket/CO when supported)
	BrokerType   string

	// Entry execution for Indian brokers: "MARKET" (with market_protection) or "LIMIT".
	EntryExecType    string  // MARKET | LIMIT
	MarketProtection float64 // protection % for MARKET; slippage % for LIMIT when no signal price (0 = exact LTP)
	Price            float64 // explicit LIMIT entry price from signal; 0 = broker fetches LTP ± slippage
}

// OrderResult is returned after a broker accepts or fills an order.
// For Publisher mode: Status = StatusPendingConfirmation, OrderID = publisherOrderID, Handoff != nil.
type OrderResult struct {
	Status    string // submitted | filled | rejected | pending_confirmation
	OrderID   string
	FillPrice float64
	SLPrice   float64        // actual SL price used (0 if not a bracket order)
	TPPrice   float64        // actual TP price used (0 if not a bracket order)
	Handoff   *HandoffResult // non-nil only for publisher mode
}

// CancelRequest is sent to broker adapters to cancel a pending order.
type CancelRequest struct {
	OrderID    string
	Variety    string // broker-specific variant ("NORMAL" for Angel, "regular" for Zerodha); empty = use broker default
	BrokerType string
}

// Broker places orders, cancels pending orders, and reports account equity for risk sizing.
type Broker interface {
	PlaceOrder(ctx context.Context, req *PlaceRequest) (*OrderResult, error)
	CancelOrder(ctx context.Context, req *CancelRequest) error
	GetAccountEquity(ctx context.Context) (float64, error)
}

// Order is deprecated; use PlaceRequest. Kept as alias for gradual migration in tests.
type Order = PlaceRequest
