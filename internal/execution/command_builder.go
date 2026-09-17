package execution

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// BuildLiveCommand maps a domain execution request to the adapter wire format.
func BuildLiveCommand(req domain.ExecutionRequest, user *store.User, cred *store.BrokerCredential, idempotencyKey string) ExecutionCommand {
	userPlan := plan.PlanFree
	if user != nil {
		userPlan = plan.EffectivePlan(user.Plan)
	}
	cmd := ExecutionCommand{
		RequestID:      req.SignalID,
		IdempotencyKey: idempotencyKey,
		Tenant: Tenant{
			UserID: req.UserID,
			Plan:   userPlan,
		},
		Destination: Destination{
			Kind:          "live_broker",
			Broker:        cred.BrokerType,
			CredentialID:  cred.ID,
			ExecutionMode: cred.ExecutionMode,
		},
		Order: OrderPayload{
			Symbol:        req.Symbol,
			Exchange:      req.Exchange,
			Side:          req.Side,
			OrderType:     req.OrderType,
			Product:       req.Product,
			Quantity:      req.Quantity,
			Price:         req.Price,
			SLPoints:      req.SLPoints,
			TPPoints:      req.TPPoints,
			Comment:       req.Comment,
			FixedLotSize:  req.FixedLotSize,
			MaxRiskPct:    req.MaxRiskPct,
			MaxLotSize:    req.MaxLotSize,
			MarketProfile: req.MarketProfile,
			TickSize:      req.TickSize,
			LotSize:       req.LotSize,
			SlippageBps:   req.SlippageBps,
			FeeBps:        req.FeeBps,
		},
		Metadata: map[string]string{
			"webhook_id": req.WebhookID,
		},
	}
	if cred.EncryptedCreds != "" {
		cmd.Credentials = &CredentialBlob{
			EncryptedBlob: cred.EncryptedCreds,
			CredentialID:  cred.ID,
		}
	}
	return cmd
}

// BuildPaperCommand maps a paper execution request to the adapter wire format.
func BuildPaperCommand(req domain.ExecutionRequest, user *store.User, idempotencyKey string) ExecutionCommand {
	userPlan := plan.PlanFree
	if user != nil {
		userPlan = plan.EffectivePlan(user.Plan)
	}
	return ExecutionCommand{
		RequestID:      req.SignalID,
		IdempotencyKey: idempotencyKey,
		Tenant: Tenant{
			UserID: req.UserID,
			Plan:   userPlan,
		},
		Destination: Destination{
			Kind:           "paper_trading",
			PaperAccountID: req.AccountID,
		},
		Order: OrderPayload{
			Symbol:        req.Symbol,
			Exchange:      req.Exchange,
			Side:          req.Side,
			OrderType:     req.OrderType,
			Product:       req.Product,
			Quantity:      req.Quantity,
			Price:         req.Price,
			MarketProfile: req.MarketProfile,
			TickSize:      req.TickSize,
			LotSize:       req.LotSize,
			SlippageBps:   req.SlippageBps,
			FeeBps:        req.FeeBps,
		},
		Metadata: map[string]string{
			"webhook_id": req.WebhookID,
		},
	}
}

// ExecutionError wraps a remote execution failure with broker type for metrics tagging.
type ExecutionError struct {
	Err        error
	BrokerType string
}

func (e *ExecutionError) Error() string { return e.Err.Error() }
func (e *ExecutionError) Unwrap() error { return e.Err }

// OutcomeToResult maps adapter wire outcome to domain.ExecutionResult.
func OutcomeToResult(out ExecutionOutcome) (*domain.ExecutionResult, error) {
	if out.ErrorCode != "" || out.ErrorMessage != "" {
		code := brokererr.Code(out.ErrorCode)
		if code == "" {
			code = brokererr.CodeInternal
		}
		msg := out.ErrorMessage
		if msg == "" {
			msg = brokererr.PublicMessage(code)
		}
		return nil, &ExecutionError{
			Err:        brokererr.New(code, msg),
			BrokerType: out.BrokerType,
		}
	}
	return &domain.ExecutionResult{
		OrderID:    out.BrokerOrderID,
		Status:     out.Status,
		FillPrice:  out.FillPrice,
		Quantity:   out.Quantity,
		OrderType:  out.OrderType,
		AlgoID:     out.AlgoID,
		BrokerType: out.BrokerType,
		SLPrice:    out.SLPrice,
		TPPrice:    out.TPPrice,
		Handoff:    out.Handoff,
	}, nil
}
