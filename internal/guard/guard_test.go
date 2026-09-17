package guard

import (
	"errors"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/store"
)

func testWebhook() *store.Webhook {
	return &store.Webhook{
		Symbol:         "EURUSD",
		AllowedSymbols: []string{"EURUSD"},
		SLPoints:       20,
		TPPoints:       30,
		AllowedActions: DefaultAllowedActions(),
		Timezone:       "UTC",
	}
}

func TestValidateAction(t *testing.T) {
	wh := testWebhook()
	wh.AllowedActions = []string{"BUY", "SELL"}
	if err := ValidateAction(wh, "CLOSE"); !errors.Is(err, ErrActionNotAllowed) {
		t.Fatalf("expected ErrActionNotAllowed, got %v", err)
	}
}

func TestValidateOrderTypeMismatch(t *testing.T) {
	wh := testWebhook()
	wh.DefaultOrderType = "LIMIT"
	if err := ValidateOrderType(wh, "MARKET"); !errors.Is(err, ErrOrderTypeNotAllowed) {
		t.Fatalf("expected ErrOrderTypeNotAllowed, got %v", err)
	}

	wh.DefaultOrderType = "MARKET"
	if err := ValidateOrderType(wh, "LIMIT"); !errors.Is(err, ErrOrderTypeNotAllowed) {
		t.Fatalf("expected ErrOrderTypeNotAllowed, got %v", err)
	}
}

func TestValidateOrderTypeMatchOrAbsent(t *testing.T) {
	wh := testWebhook()
	wh.DefaultOrderType = "LIMIT"
	if err := ValidateOrderType(wh, "limit"); err != nil {
		t.Fatalf("expected case-insensitive match to pass, got %v", err)
	}
	if err := ValidateOrderType(wh, ""); err != nil {
		t.Fatalf("expected absent order_type to pass, got %v", err)
	}
}

func TestValidateOrderTypeUnsetWebhookRejectsMismatch(t *testing.T) {
	wh := testWebhook()
	if err := ValidateOrderType(wh, "MARKET"); !errors.Is(err, ErrOrderTypeNotAllowed) {
		t.Fatalf("expected rejection when webhook unset (LIMIT) and signal MARKET, got %v", err)
	}
}

func TestWebhookOrderType(t *testing.T) {
	if got := WebhookOrderType(nil); got != "LIMIT" {
		t.Fatalf("nil webhook defaults to LIMIT, got %q", got)
	}
	if got := WebhookOrderType(&store.Webhook{}); got != "LIMIT" {
		t.Fatalf("empty default_order_type defaults to LIMIT, got %q", got)
	}
	if got := WebhookOrderType(&store.Webhook{DefaultOrderType: "MARKET"}); got != "MARKET" {
		t.Fatalf("configured MARKET, got %q", got)
	}
}

func TestValidateOrderTypeInvalidValue(t *testing.T) {
	wh := testWebhook()
	wh.DefaultOrderType = "LIMIT"
	if err := ValidateOrderType(wh, "STOP"); !errors.Is(err, ErrOrderTypeNotAllowed) {
		t.Fatalf("expected ErrOrderTypeNotAllowed for invalid value, got %v", err)
	}
}

func TestNormalizeAllowedSymbols(t *testing.T) {
	got, err := NormalizeAllowedSymbols([]string{" reliance ", "INFY", "reliance"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "RELIANCE" || got[1] != "INFY" {
		t.Fatalf("got %#v", got)
	}
	_, err = NormalizeAllowedSymbols(nil)
	if err == nil {
		t.Fatal("expected error for empty allowed symbols")
	}
}

func TestEffectiveAllowedSymbolsLegacySymbol(t *testing.T) {
	wh := &store.Webhook{Symbol: "EURUSD"}
	if len(EffectiveAllowedSymbols(wh)) != 1 || EffectiveAllowedSymbols(wh)[0] != "EURUSD" {
		t.Fatalf("legacy webhook symbol fallback failed")
	}
	wh.AllowedSymbols = []string{"GBPUSD", "EURUSD"}
	if len(EffectiveAllowedSymbols(wh)) != 2 {
		t.Fatalf("expected whitelist length 2")
	}
}

func TestResolveParamsSignalWinsWhenProvided(t *testing.T) {
	wh := testWebhook()
	p := ResolveParams(wh, &store.SignalPayload{
		Symbol: "GBPUSD",
		Lot:    0.5,
		SLPts:  99,
		TPPts:  99,
	})
	if p.Symbol != "GBPUSD" {
		t.Fatalf("expected signal symbol GBPUSD, got %q", p.Symbol)
	}
	if p.SLPts != 99 || p.TPPts != 99 {
		t.Fatalf("expected signal SL=99 TP=99, got sl=%d tp=%d", p.SLPts, p.TPPts)
	}
	if p.Lot != 0.5 {
		t.Fatalf("expected signal lot 0.5, got %v", p.Lot)
	}
}

func TestResolveQuantityFixedLotWins(t *testing.T) {
	wh := &store.Webhook{LotSize: 5}
	got, err := ResolveQuantity(wh, 1)
	if err != nil || got != 5 {
		t.Fatalf("expected webhook lot 5, got %v err=%v", got, err)
	}
}

func TestResolveQuantityRequiresSignalWhenDefaultZero(t *testing.T) {
	wh := &store.Webhook{LotSize: 0}
	_, err := ResolveQuantity(wh, 0)
	if !errors.Is(err, ErrQuantityRequired) {
		t.Fatalf("expected ErrQuantityRequired, got %v", err)
	}
	got, err := ResolveQuantity(wh, 2)
	if err != nil || got != 2 {
		t.Fatalf("expected signal lot 2, got %v err=%v", got, err)
	}
}

func TestResolveParamsUsesSingleAllowedSymbol(t *testing.T) {
	wh := &store.Webhook{AllowedSymbols: []string{"RELIANCE"}, LotSize: 0, SLPoints: 0, TPPoints: 0}
	p := ResolveParams(wh, &store.SignalPayload{Lot: 2.0, SLPts: 15, TPPts: 25})
	if p.Symbol != "RELIANCE" || p.Lot != 2.0 || p.SLPts != 15 || p.TPPts != 25 {
		t.Fatalf("expected single allowed symbol default, got: %+v", p)
	}
}

func TestResolveParamsFallsBackToWebhookDefaults(t *testing.T) {
	wh := testWebhook()
	p := ResolveParams(wh, &store.SignalPayload{Action: "BUY", Symbol: "EURUSD"})
	if p.Symbol != "EURUSD" || p.SLPts != 20 || p.TPPts != 30 {
		t.Fatalf("expected webhook defaults when signal omits params, got: %+v", p)
	}
}

func TestValidateSymbolRejected(t *testing.T) {
	wh := testWebhook()
	if err := ValidateSymbol(wh, "GBPUSD"); !errors.Is(err, ErrSymbolNotAllowed) {
		t.Fatalf("expected ErrSymbolNotAllowed, got %v", err)
	}
}

func TestApplyMaxLot(t *testing.T) {
	wh := testWebhook()
	wh.MaxLotSize = 0.1
	_, err := ApplyMaxLot(wh, 0.5)
	if !errors.Is(err, ErrLotExceedsMax) {
		t.Fatalf("expected ErrLotExceedsMax, got %v", err)
	}
	got, err := ApplyMaxLot(wh, 0.05)
	if err != nil || got != 0.05 {
		t.Fatalf("expected 0.05, got %v err=%v", got, err)
	}
}

func TestWithinTradingHours(t *testing.T) {
	wh := testWebhook()
	schedule := store.TradingSchedule{
		"mon": {{Start: "09:00", End: "17:00"}},
	}
	wh.TradingHours = &schedule
	wh.Timezone = "UTC"

	mon10 := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	if !WithinTradingHours(wh, mon10) {
		t.Fatal("expected inside window")
	}
	mon20 := time.Date(2026, 6, 8, 20, 0, 0, 0, time.UTC)
	if WithinTradingHours(wh, mon20) {
		t.Fatal("expected outside window")
	}
}
