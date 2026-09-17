package signals

import (
	"encoding/json"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestRoutePaperIngest_LegacyOAuthShape(t *testing.T) {
	body := []byte(`{"action":"BUY","symbol":"SBIN","exchange":"NSE","product":"MIS","price":842.5,"lot":3,"order_type":"LIMIT","comment":"paper-secret-1"}`)
	route, sig, _, err := routePaperIngest(body)
	if err != nil {
		t.Fatal(err)
	}
	if route != paperRouteOrder || sig.Action != "BUY" || sig.Lot != 3 || sig.OrderType != "LIMIT" {
		t.Fatalf("route=%v sig=%+v", route, sig)
	}
}

func TestRoutePaperIngest_DuplicateOrderType_UserScript(t *testing.T) {
	// User's Pine script mistakenly puts ORDER_SIGNAL in order_type, then LIMIT again.
	body := []byte(`{"action":"BUY","symbol":"SBIN","exchange":"NSE","product":"MIS","price":842.5,"lot":3,"order_type":"ORDER_SIGNAL","order_type":"LIMIT","comment":"paper-secret-1"}`)
	route, sig, _, err := routePaperIngest(body)
	if err != nil {
		t.Fatal(err)
	}
	if route != paperRouteOrder {
		t.Fatalf("route=%v want order", route)
	}
	if sig.OrderType != "LIMIT" {
		t.Fatalf("order_type=%q want LIMIT", sig.OrderType)
	}
	wh := &store.Webhook{DefaultOrderType: "LIMIT"}
	if err := guard.ValidateOrderType(wh, sig.OrderType); err != nil {
		t.Fatalf("ValidateOrderType: %v", err)
	}
}

func TestRoutePaperIngest_ORDER_SIGNALOnlyInOrderType(t *testing.T) {
	body := []byte(`{"action":"BUY","symbol":"SBIN","lot":3,"price":100,"order_type":"ORDER_SIGNAL","comment":"x"}`)
	route, sig, _, err := routePaperIngest(body)
	if err != nil {
		t.Fatal(err)
	}
	if route != paperRouteOrder {
		t.Fatalf("route=%v", route)
	}
	if sig.OrderType != "" {
		t.Fatalf("order_type=%q want cleared", sig.OrderType)
	}
	wh := &store.Webhook{DefaultOrderType: "LIMIT"}
	if err := guard.ValidateOrderType(wh, sig.OrderType); err != nil {
		t.Fatalf("ValidateOrderType: %v", err)
	}
}

func TestRoutePaperIngest_TypedORDER_SIGNAL(t *testing.T) {
	body := []byte(`{"type":"ORDER_SIGNAL","action":"BUY","symbol":"SBIN","quantity":3,"price":100,"order_type":"LIMIT","comment":"x"}`)
	route, sig, _, err := routePaperIngest(body)
	if err != nil {
		t.Fatal(err)
	}
	if route != paperRouteOrder || sig.Lot != 3 || sig.OrderType != "LIMIT" {
		t.Fatalf("route=%v sig=%+v err=%v", route, sig, err)
	}
}

func TestNormalizePaperTypeFields_MisplacedTypeOnly(t *testing.T) {
	body := []byte(`{"action":"BUY","symbol":"SBIN","order_type":"ORDER_SIGNAL","lot":1,"price":1}`)
	var raw store.PaperSignalPayload
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	normalizePaperTypeFields(body, &raw)
	if raw.Type != "ORDER_SIGNAL" {
		t.Fatalf("type=%q want ORDER_SIGNAL", raw.Type)
	}
	if raw.OrderType != "" {
		t.Fatalf("order_type=%q want empty", raw.OrderType)
	}
}
