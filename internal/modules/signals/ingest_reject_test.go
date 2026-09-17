package signals

import (
	"testing"

	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestBuildGatewayRejectedTrade_WithSignal(t *testing.T) {
	paperID := "paper-1"
	wh := &store.Webhook{
		ID:             "wh-1",
		UserID:         "u1",
		PaperAccountID: &paperID,
		DefaultOrderType: "LIMIT",
	}
	sig := &store.SignalPayload{
		Action:    "BUY",
		Symbol:    "SBIN",
		Lot:       3,
		Product:   "MIS",
		OrderType: "LIMIT",
		Comment:   "secret",
	}

	tr := buildGatewayRejectedTrade(wh, sig, "req-1", "order_type_not_allowed", "order_type not allowed for this webhook")
	if tr == nil {
		t.Fatal("nil trade")
	}
	if tr.Status != "rejected" || tr.ErrorCode != "order_type_not_allowed" {
		t.Fatalf("trade=%+v", tr)
	}
	if tr.Symbol != "SBIN" || tr.LotSize != 3 || tr.Product != "MIS" || tr.OrderType != "LIMIT" {
		t.Fatalf("fields not copied: %+v", tr)
	}
}

func TestBuildGatewayRejectedTrade_Unparsed(t *testing.T) {
	wh := &store.Webhook{ID: "wh-1", UserID: "u1"}
	tr := buildGatewayRejectedTrade(wh, nil, "req-2", errCodeInvalidPayload, "invalid payload")
	if tr == nil {
		t.Fatal("nil trade")
	}
	if tr.Status != "rejected" || tr.ErrorCode != errCodeInvalidPayload || tr.Error != "invalid payload" {
		t.Fatalf("trade=%+v", tr)
	}
	if tr.Signal != "—" || tr.Symbol != "—" {
		t.Fatalf("expected placeholder signal/symbol, got signal=%q symbol=%q", tr.Signal, tr.Symbol)
	}
}

func TestIngestBrokerType_Paper(t *testing.T) {
	paperID := "paper-1"
	wh := &store.Webhook{PaperAccountID: &paperID}
	if got := ingestBrokerType(nil, nil, wh); got != "paper" {
		t.Fatalf("brokerType=%q want paper", got)
	}
}
