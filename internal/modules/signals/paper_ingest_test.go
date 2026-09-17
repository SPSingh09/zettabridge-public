package signals

import (
	"encoding/json"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestPaperOrderFromPayload_QuantityPreferred(t *testing.T) {
	sig := paperOrderFromPayload(&store.PaperSignalPayload{
		Action:   "buy",
		Symbol:   "SBIN",
		Quantity: 5,
		Lot:      3,
		Product:  "MIS",
		Price:    100,
	})
	if sig.Action != "BUY" || sig.Lot != 5 || sig.Product != "MIS" {
		t.Fatalf("unexpected sig: %+v", sig)
	}
}

func TestPaperOrderFromPayload_LotFallback(t *testing.T) {
	sig := paperOrderFromPayload(&store.PaperSignalPayload{
		Action: "SELL",
		Symbol: "RELIANCE",
		Lot:    3,
		Price:  2500,
	})
	if sig.Lot != 3 {
		t.Fatalf("lot=%v want 3", sig.Lot)
	}
}

func TestLegacyFlatPayloadUnmarshal(t *testing.T) {
	body := []byte(`{"action":"BUY","symbol":"SBIN","exchange":"NSE","product":"MIS","price":842.5,"lot":3,"order_type":"LIMIT","comment":"paper-secret-1"}`)

	var typed store.PaperSignalPayload
	if err := json.Unmarshal(body, &typed); err != nil {
		t.Fatal(err)
	}
	if typed.Type != "" {
		t.Fatalf("type=%q want empty for legacy payload", typed.Type)
	}

	var sig store.SignalPayload
	if err := json.Unmarshal(body, &sig); err != nil {
		t.Fatal(err)
	}
	if sig.Action != "BUY" || sig.Lot != 3 || sig.Product != "MIS" || sig.OrderType != "LIMIT" {
		t.Fatalf("unexpected legacy sig: %+v", sig)
	}
}
