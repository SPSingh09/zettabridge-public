package queue

import (
	"testing"

	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestResolveTradeOrderType(t *testing.T) {
	wh := &store.Webhook{DefaultOrderType: "LIMIT"}
	sig := &store.SignalPayload{OrderType: "MARKET"}
	params := guard.TradeParams{}

	if got := resolveTradeOrderType(wh, sig, params); got != "LIMIT" {
		t.Fatalf("webhook configured order type wins: got %q", got)
	}

	params = guard.TradeParams{SLPts: 10}
	if got := resolveTradeOrderType(wh, sig, params); got != "BRACKET" {
		t.Fatalf("SL/TP -> BRACKET: got %q", got)
	}

	sig = &store.SignalPayload{}
	if got := resolveTradeOrderType(wh, sig, params); got != "BRACKET" {
		t.Fatalf("SL/TP without signal type: got %q", got)
	}

	params = guard.TradeParams{}
	if got := resolveTradeOrderType(wh, sig, params); got != "LIMIT" {
		t.Fatalf("webhook default: got %q", got)
	}

	wh = &store.Webhook{DefaultOrderType: "MARKET"}
	if got := resolveTradeOrderType(wh, sig, params); got != "MARKET" {
		t.Fatalf("webhook MARKET: got %q", got)
	}
}
