package maporder

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestBuildMT5(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "mt5_cloud"}
	req, err := Build("", cred, guard.TradeParams{Symbol: "EURUSD", SLPts: 20, TPPts: 30}, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	if req.QuantityUnit != domain.QuantityLots || req.Exchange != "" {
		t.Fatalf("got %#v", req)
	}
}

func TestBuildIndianRequiresExchange(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", Product: "MIS"}
	_, err := Build("", cred, guard.TradeParams{Symbol: "RELIANCE"}, 1)
	if err == nil {
		t.Fatal("expected exchange required")
	}
}

func TestBuildIndianWithExchange(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", Exchange: "NSE", Product: "MIS"}
	req, err := Build("", cred, guard.TradeParams{Symbol: "RELIANCE"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if req.Exchange != "NSE" || req.Product != "MIS" || req.QuantityUnit != domain.QuantityShares {
		t.Fatalf("got %#v", req)
	}
}

func TestBuildRejectsForexOnIndian(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", Exchange: "NSE", Product: "MIS"}
	_, err := Build("", cred, guard.TradeParams{Symbol: "EURUSD"}, 0.01)
	if err == nil {
		t.Fatal("expected forex symbol rejected on Indian broker")
	}
	if brokererr.CodeOf(err) != brokererr.CodeInvalidSymbol {
		t.Fatalf("got code %q", brokererr.CodeOf(err))
	}
}

func TestBuildCloseIndian(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "dhan", Exchange: "NSE", Product: "MIS"}
	req, err := Build("", cred, guard.TradeParams{Symbol: "RELIANCE", SLPts: 10, TPPts: 15}, 1)
	if err != nil {
		t.Fatal(err)
	}
	WithAction(req, "CLOSE")
	if req.Action != "CLOSE" || req.Exchange != "NSE" || req.Product != "MIS" {
		t.Fatalf("got %#v", req)
	}
	if req.QuantityUnit != domain.QuantityShares || req.Quantity < 1 {
		t.Fatalf("CLOSE should carry share quantity: %#v", req)
	}
}

func TestBuildCloseMT5(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "mt5_cloud"}
	req, err := Build("", cred, guard.TradeParams{Symbol: "EURUSD"}, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	WithAction(req, "CLOSE")
	if req.Action != "CLOSE" || req.QuantityUnit != domain.QuantityLots || req.Quantity != 0.05 {
		t.Fatalf("got %#v", req)
	}
}

func TestBuildBracketCNCRejected(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", Exchange: "NSE", Product: "CNC"}
	_, err := Build("", cred, guard.TradeParams{Symbol: "RELIANCE", SLPts: 10}, 1)
	if err == nil {
		t.Fatal("expected bracket+CNC to be rejected")
	}
	if brokererr.CodeOf(err) != brokererr.CodeBracketNotSupported {
		t.Fatalf("got code %q", brokererr.CodeOf(err))
	}
}

func TestBuildBracketNRMLRejected(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", Exchange: "NSE", Product: "NRML"}
	_, err := Build("", cred, guard.TradeParams{Symbol: "RELIANCE", SLPts: 10}, 1)
	if err == nil {
		t.Fatal("expected bracket+NRML to be rejected")
	}
	if brokererr.CodeOf(err) != brokererr.CodeBracketNotSupported {
		t.Fatalf("got code %q", brokererr.CodeOf(err))
	}
}

func TestBuildOrderTypeLIMIT(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", Exchange: "NSE", Product: "MIS"}
	req, err := Build("LIMIT", cred, guard.TradeParams{Symbol: "RELIANCE"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if req.EntryExecType != "LIMIT" {
		t.Fatalf("EntryExecType=%q want LIMIT", req.EntryExecType)
	}
}

func TestBuildOrderTypeMARKET(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", Exchange: "NSE", Product: "MIS"}
	req, err := Build("MARKET", cred, guard.TradeParams{Symbol: "RELIANCE"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if req.EntryExecType != "MARKET" {
		t.Fatalf("EntryExecType=%q want MARKET", req.EntryExecType)
	}
}

func TestBuildOrderTypeEmptyDefaultsLIMIT(t *testing.T) {
	cred := &store.BrokerCredential{BrokerType: "zerodha", Exchange: "NSE", Product: "MIS", OrderType: "MARKET"}
	req, err := Build("", cred, guard.TradeParams{Symbol: "RELIANCE"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if req.EntryExecType != "LIMIT" {
		t.Fatalf("EntryExecType=%q want LIMIT (empty webhook order type defaults to LIMIT)", req.EntryExecType)
	}
}
