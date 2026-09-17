package execution

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
)

// ExecutionCommand is the wire format Core sends to execution adapters.
type ExecutionCommand struct {
	RequestID      string            `json:"request_id"`
	IdempotencyKey string            `json:"idempotency_key"`
	Tenant         Tenant            `json:"tenant"`
	Destination    Destination       `json:"destination"`
	Order          OrderPayload      `json:"order"`
	Credentials    *CredentialBlob   `json:"credentials,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type Tenant struct {
	UserID string `json:"user_id"`
	Plan   string `json:"plan"`
}

type Destination struct {
	Kind           string `json:"kind"` // paper_trading | live_broker
	Broker         string `json:"broker,omitempty"`
	CredentialID   string `json:"credential_id,omitempty"`
	PaperAccountID string `json:"paper_account_id,omitempty"`
	ExecutionMode  string `json:"execution_mode,omitempty"`
}

type OrderPayload struct {
	Symbol           string  `json:"symbol"`
	Exchange         string  `json:"exchange"`
	Side             string  `json:"side"`
	OrderType        string  `json:"order_type"`
	Product          string  `json:"product"`
	Quantity         float64 `json:"quantity"`
	Price            float64 `json:"price"`
	SLPoints         int     `json:"sl_points"`
	TPPoints         int     `json:"tp_points"`
	Comment          string  `json:"comment"`
	AlgoID           string  `json:"algo_id"`
	FixedLotSize     float64 `json:"fixed_lot_size"`
	MaxRiskPct       float64 `json:"max_risk_pct"`
	MaxLotSize       float64 `json:"max_lot_size"`
	MarketProfile    string  `json:"market_profile,omitempty"`
	TickSize         float64 `json:"tick_size,omitempty"`
	LotSize          float64 `json:"lot_size,omitempty"`
	SlippageBps      float64 `json:"slippage_bps,omitempty"`
	FeeBps           float64 `json:"fee_bps,omitempty"`
}

type CredentialBlob struct {
	EncryptedBlob string `json:"encrypted_blob"`
	CredentialID  string `json:"credential_id"`
}

// ExecutionOutcome is returned synchronously from adapter PlaceOrder.
type ExecutionOutcome struct {
	RequestID     string                 `json:"request_id"`
	Status        string                 `json:"status"`
	BrokerOrderID string                 `json:"broker_order_id,omitempty"`
	FillPrice     float64                `json:"fill_price,omitempty"`
	Quantity      float64                `json:"quantity,omitempty"`
	OrderType     string                 `json:"order_type,omitempty"`
	AlgoID        string                 `json:"algo_id,omitempty"`
	BrokerType    string                 `json:"broker_type,omitempty"`
	SLPrice       float64                `json:"sl_price,omitempty"`
	TPPrice       float64                `json:"tp_price,omitempty"`
	ErrorCode     string                 `json:"error_code,omitempty"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	Handoff       *domain.HandoffResult  `json:"handoff,omitempty"`
}

type CancelCommand struct {
	RequestID    string `json:"request_id"`
	Broker       string `json:"broker"`
	CredentialID string `json:"credential_id"`
	OrderID      string `json:"order_id"`
	Variety      string `json:"variety,omitempty"`
}

// ExecutionEvent is posted asynchronously from adapter to Core.
type ExecutionEvent struct {
	RequestID     string  `json:"request_id"`
	Event         string  `json:"event"`
	BrokerOrderID string  `json:"broker_order_id,omitempty"`
	FillPrice     float64 `json:"fill_price,omitempty"`
	OccurredAt    string  `json:"occurred_at,omitempty"`
}

// SessionUpdate is sent from adapter to Core after OAuth token exchange.
type SessionUpdate struct {
	EncryptedCreds string `json:"encrypted_creds"`
}

// ToExecutionRequest maps a wire command to the domain execution request.
func (cmd ExecutionCommand) ToExecutionRequest() domain.ExecutionRequest {
	req := domain.ExecutionRequest{
		AccountID:    cmd.Destination.CredentialID,
		WebhookID:    cmd.Metadata["webhook_id"],
		SignalID:     cmd.RequestID,
		UserID:       cmd.Tenant.UserID,
		MarketProfile: cmd.Order.MarketProfile,
		Exchange:     cmd.Order.Exchange,
		Symbol:       cmd.Order.Symbol,
		Side:         cmd.Order.Side,
		OrderType:    cmd.Order.OrderType,
		Product:      cmd.Order.Product,
		Quantity:     cmd.Order.Quantity,
		Price:        cmd.Order.Price,
		Comment:      cmd.Order.Comment,
		SLPoints:     cmd.Order.SLPoints,
		TPPoints:     cmd.Order.TPPoints,
		FixedLotSize: cmd.Order.FixedLotSize,
		MaxRiskPct:   cmd.Order.MaxRiskPct,
		MaxLotSize:   cmd.Order.MaxLotSize,
		TickSize:     cmd.Order.TickSize,
		LotSize:      cmd.Order.LotSize,
		SlippageBps:  cmd.Order.SlippageBps,
		FeeBps:       cmd.Order.FeeBps,
	}
	if cmd.Destination.Kind == "paper_trading" {
		req.AccountID = cmd.Destination.PaperAccountID
	}
	return req
}

// OutcomeFromResult maps domain.ExecutionResult to wire outcome.
func OutcomeFromResult(requestID string, r *domain.ExecutionResult, brokerType string) ExecutionOutcome {
	if r == nil {
		return ExecutionOutcome{RequestID: requestID, Status: domain.StatusRejected}
	}
	return ExecutionOutcome{
		RequestID:     requestID,
		Status:        r.Status,
		BrokerOrderID: r.OrderID,
		FillPrice:     r.FillPrice,
		Quantity:      r.Quantity,
		OrderType:     r.OrderType,
		AlgoID:        r.AlgoID,
		BrokerType:    brokerType,
		SLPrice:       r.SLPrice,
		TPPrice:       r.TPPrice,
		Handoff:       r.Handoff,
	}
}
