package execution

import (
	"testing"

	"github.com/SPSingh09/zettabridge/internal/domain"
)

func TestExecutionCommandToPaperRequest(t *testing.T) {
	cmd := ExecutionCommand{
		RequestID: "trade-1",
		Tenant:    Tenant{UserID: "u1", Plan: "free"},
		Destination: Destination{
			Kind:           "paper_trading",
			PaperAccountID: "pa-1",
		},
		Order: OrderPayload{
			Symbol:   "RELIANCE",
			Exchange: "NSE",
			Side:     "BUY",
			Quantity: 1,
		},
		Metadata: map[string]string{"webhook_id": "wh-1"},
	}
	req := cmd.ToExecutionRequest()
	if req.AccountID != "pa-1" {
		t.Fatalf("account_id=%q want pa-1", req.AccountID)
	}
	if req.WebhookID != "wh-1" || req.UserID != "u1" {
		t.Fatalf("unexpected mapping: %+v", req)
	}
}

func TestOutcomeFromResult(t *testing.T) {
	out := OutcomeFromResult("t1", &domain.ExecutionResult{
		OrderID: "ord-1",
		Status:  domain.StatusSubmitted,
		Quantity: 2,
	}, "zerodha")
	if out.RequestID != "t1" || out.BrokerOrderID != "ord-1" || out.Quantity != 2 {
		t.Fatalf("unexpected outcome: %+v", out)
	}
}
