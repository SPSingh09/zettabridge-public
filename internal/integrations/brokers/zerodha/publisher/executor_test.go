package publisher

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// ── mock store ──────────────────────────────────────────────────────────────

type mockStore struct {
	orders []store.PublisherOrder
	events []store.PublisherOrderEvent
}

func (m *mockStore) InsertPublisherOrder(_ context.Context, o *store.PublisherOrder) error {
	m.orders = append(m.orders, *o)
	return nil
}

func (m *mockStore) InsertPublisherOrderEvent(_ context.Context, e *store.PublisherOrderEvent) error {
	m.events = append(m.events, *e)
	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func newTestExecutor(ms *mockStore) *Executor {
	cred := &store.BrokerCredential{
		ID:            "cred-1",
		UserID:        "user-1",
		BrokerType:    "zerodha",
		ExecutionMode: "publisher",
		Exchange:      "NSE",
		Product:       "MIS",
	}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: "testapikey"}
	return NewExecutor(cred, parsed, ms, "test-secret", "https://api.example.com/v1/publisher/callback")
}

func limitBuyReq() *domain.PlaceRequest {
	return &domain.PlaceRequest{
		Action:        "BUY",
		Symbol:        "RELIANCE",
		Exchange:      "NSE",
		Product:       "MIS",
		Quantity:      10,
		QuantityUnit:  domain.QuantityShares,
		OrderType:     domain.OrderMarket,
		EntryExecType: "LIMIT",
		Price:         2500.00,
		BrokerType:    "zerodha",
	}
}

func marketBuyReq() *domain.PlaceRequest {
	return &domain.PlaceRequest{
		Action:        "BUY",
		Symbol:        "RELIANCE",
		Exchange:      "NSE",
		Product:       "MIS",
		Quantity:      10,
		QuantityUnit:  domain.QuantityShares,
		OrderType:     domain.OrderMarket,
		EntryExecType: "MARKET",
		BrokerType:    "zerodha",
	}
}

// ── PlaceOrder tests ─────────────────────────────────────────────────────────

func TestPlaceOrder_LimitBuyHappyPath(t *testing.T) {
	ms := &mockStore{}
	ex := newTestExecutor(ms)

	result, err := ex.PlaceOrder(context.Background(), limitBuyReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != domain.StatusPendingConfirmation {
		t.Errorf("status: want %q, got %q", domain.StatusPendingConfirmation, result.Status)
	}
	if result.OrderID == "" {
		t.Error("OrderID must not be empty")
	}
	if result.Handoff == nil {
		t.Fatal("Handoff must not be nil for publisher mode")
	}
	if result.Handoff.Provider != "zerodha" {
		t.Errorf("Handoff.Provider: want zerodha, got %q", result.Handoff.Provider)
	}
	if result.Handoff.Method != "POST_FORM" {
		t.Errorf("Handoff.Method: want POST_FORM, got %q", result.Handoff.Method)
	}
	if result.Handoff.Action != kiteBasketURL {
		t.Errorf("Handoff.Action: want %q, got %q", kiteBasketURL, result.Handoff.Action)
	}
	if result.Handoff.Fields["api_key"] != "testapikey" {
		t.Errorf("Handoff api_key: want testapikey, got %q", result.Handoff.Fields["api_key"])
	}

	var items []BasketItem
	if err := json.Unmarshal([]byte(result.Handoff.Fields["data"]), &items); err != nil {
		t.Fatalf("basket data is not valid JSON: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 basket item, got %d", len(items))
	}
	item := items[0]
	if item.TradingSymbol != "RELIANCE" {
		t.Errorf("TradingSymbol: want RELIANCE, got %q", item.TradingSymbol)
	}
	if item.TransactionType != "BUY" {
		t.Errorf("TransactionType: want BUY, got %q", item.TransactionType)
	}
	if item.Quantity != 10 {
		t.Errorf("Quantity: want 10, got %d", item.Quantity)
	}
	if item.OrderType != "LIMIT" {
		t.Errorf("OrderType: want LIMIT, got %q", item.OrderType)
	}
	if item.Price != 2500.00 {
		t.Errorf("Price: want 2500.00, got %v", item.Price)
	}
	if item.Variety != "regular" {
		t.Errorf("Variety: want regular, got %q", item.Variety)
	}

	if len(ms.orders) != 1 {
		t.Fatalf("expected 1 publisher order inserted, got %d", len(ms.orders))
	}
	po := ms.orders[0]
	if po.Status != "created" {
		t.Errorf("publisher order status: want created, got %q", po.Status)
	}
	if po.ExpiresAt.Before(time.Now().Add(29 * time.Minute)) {
		t.Error("publisher order expires_at should be ~30 min from now")
	}
	if len(ms.events) != 1 || ms.events[0].EventType != "BASKET_CREATED" {
		t.Errorf("expected BASKET_CREATED event, got %+v", ms.events)
	}
}

// Kite's basket page does not forward market_protection to its internal API,
// so MARKET orders always fail. Publisher mode only supports LIMIT orders.
func TestPlaceOrder_MarketBuy_Error(t *testing.T) {
	ms := &mockStore{}
	ex := newTestExecutor(ms)

	_, err := ex.PlaceOrder(context.Background(), marketBuyReq())
	if err == nil {
		t.Fatal("expected error for MARKET order in publisher mode (Kite basket does not support market_protection)")
	}
}

func TestPlaceOrder_LimitSell(t *testing.T) {
	ms := &mockStore{}
	ex := newTestExecutor(ms)

	req := limitBuyReq()
	req.Action = "SELL"

	result, err := ex.PlaceOrder(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var items []BasketItem
	json.Unmarshal([]byte(result.Handoff.Fields["data"]), &items)
	if items[0].TransactionType != "SELL" {
		t.Errorf("want SELL, got %q", items[0].TransactionType)
	}
}

func TestPlaceOrder_LimitBuy(t *testing.T) {
	ms := &mockStore{}
	ex := newTestExecutor(ms)

	req := marketBuyReq()
	req.EntryExecType = "LIMIT"
	req.Price = 2500.50

	result, err := ex.PlaceOrder(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var items []BasketItem
	json.Unmarshal([]byte(result.Handoff.Fields["data"]), &items)
	if items[0].OrderType != "LIMIT" {
		t.Errorf("OrderType: want LIMIT, got %q", items[0].OrderType)
	}
	if items[0].Price != 2500.50 {
		t.Errorf("Price: want 2500.50, got %v", items[0].Price)
	}
}

func TestPlaceOrder_LimitWithoutPrice_Error(t *testing.T) {
	ms := &mockStore{}
	ex := newTestExecutor(ms)

	req := marketBuyReq()
	req.EntryExecType = "LIMIT"
	req.Price = 0

	_, err := ex.PlaceOrder(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for LIMIT without price")
	}
}

func TestPlaceOrder_CloseAction_Error(t *testing.T) {
	ms := &mockStore{}
	ex := newTestExecutor(ms)

	req := marketBuyReq()
	req.Action = "CLOSE"

	_, err := ex.PlaceOrder(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for CLOSE action in publisher mode")
	}
}

func TestPlaceOrder_BracketOrder_Error(t *testing.T) {
	ms := &mockStore{}
	ex := newTestExecutor(ms)

	req := marketBuyReq()
	req.OrderType = domain.OrderBracket
	req.SLPoints = 10

	_, err := ex.PlaceOrder(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for bracket order in publisher mode")
	}
}

func TestPlaceOrder_InvalidExchange_Error(t *testing.T) {
	ms := &mockStore{}
	ex := newTestExecutor(ms)

	req := marketBuyReq()
	req.Exchange = "NFO"

	_, err := ex.PlaceOrder(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for NFO exchange in publisher mode")
	}
}

func TestPlaceOrder_MissingAPIKey_Error(t *testing.T) {
	ms := &mockStore{}
	cred := &store.BrokerCredential{ID: "c1", UserID: "u1", BrokerType: "zerodha", ExecutionMode: "publisher", Exchange: "NSE", Product: "MIS"}
	parsed := brokercreds.Parsed{BrokerType: "zerodha", APIKey: ""}
	ex := NewExecutor(cred, parsed, ms, "secret", "http://cb")

	_, err := ex.PlaceOrder(context.Background(), marketBuyReq())
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
}

// ── CancelOrder / GetAccountEquity ───────────────────────────────────────────

func TestCancelOrder_Unsupported(t *testing.T) {
	ex := newTestExecutor(&mockStore{})
	err := ex.CancelOrder(context.Background(), &domain.CancelRequest{OrderID: "123"})
	if err == nil {
		t.Fatal("expected error for CancelOrder in publisher mode")
	}
}

func TestGetAccountEquity_Unsupported(t *testing.T) {
	ex := newTestExecutor(&mockStore{})
	_, err := ex.GetAccountEquity(context.Background())
	if err == nil {
		t.Fatal("expected error for GetAccountEquity in publisher mode")
	}
}

// ── State round-trip ─────────────────────────────────────────────────────────

func TestState_RoundTrip(t *testing.T) {
	orderID := "pub-order-123"
	secret := "test-secret"

	state := GenerateState(orderID, secret)
	if state == "" {
		t.Fatal("GenerateState returned empty string")
	}

	got, err := ParseState(state, secret)
	if err != nil {
		t.Fatalf("ParseState error: %v", err)
	}
	if got != orderID {
		t.Errorf("ParseState: want %q, got %q", orderID, got)
	}
}

func TestState_InvalidSignature(t *testing.T) {
	state := GenerateState("order-1", "secret-a")
	_, err := ParseState(state, "secret-b")
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestState_Tampered(t *testing.T) {
	_, err := ParseState("not-valid-base64!!!", "secret")
	if err == nil {
		t.Fatal("expected error for tampered state")
	}
}
