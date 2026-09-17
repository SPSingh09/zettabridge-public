package livebrokers

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers/httpclient"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func testMT5Broker(t *testing.T, srv *httptest.Server) *MT5Broker {
	t.Helper()
	cred := &store.BrokerCredential{BrokerType: "mt5_cloud", AccountMode: "live"}
	infra := &Infra{
		HTTP: httpclient.New(time.Second, 0),
		URLs: URLs{MT5Live: srv.URL},
	}
	b, err := NewWithParsed(cred, brokercreds.Parsed{
		BrokerType: "mt5_cloud",
		AuthToken:  "tok",
		AccountID:  "acct-1",
	}, infra, nil)
	if err != nil {
		t.Fatalf("NewWithParsed: %v", err)
	}
	return b.(*MT5Broker)
}

func TestMT5PlaceOrderBuy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/users/current/accounts/acct-1/trade" {
			http.NotFound(w, r)
			return
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["actionType"] != "ORDER_TYPE_BUY" {
			t.Errorf("actionType=%v", body["actionType"])
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"orderId": "ord-123"})
	}))
	defer srv.Close()

	b := testMT5Broker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:   "BUY",
		Symbol:   "EURUSD",
		Quantity: 0.1,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if result.OrderID != "ord-123" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
}

func TestMT5CloseSymbolOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["actionType"] != "POSITIONS_CLOSE_SYMBOL" {
			t.Errorf("actionType=%v want POSITIONS_CLOSE_SYMBOL", body["actionType"])
		}
		if body["symbol"] != "EURUSD" {
			t.Errorf("symbol=%v", body["symbol"])
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"orderId": "close-1"})
	}))
	defer srv.Close()

	b := testMT5Broker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action: "CLOSE",
		Symbol: "EURUSD",
	})
	if err != nil {
		t.Fatalf("PlaceOrder CLOSE: %v", err)
	}
	if result.OrderID != "close-1" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
}

func TestMT5AuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid auth-token"}`))
	}))
	defer srv.Close()

	b := testMT5Broker(t, srv)
	_, err := b.GetAccountEquity(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
	if brokererr.CodeOf(err) != brokererr.CodeAuthFailed {
		t.Fatalf("code=%q", brokererr.CodeOf(err))
	}
}

func TestMT5CloseNoPosition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"position closed"}`))
	}))
	defer srv.Close()

	b := testMT5Broker(t, srv)
	_, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action: "CLOSE",
		Symbol: "EURUSD",
	})
	if err == nil {
		t.Fatal("expected no position error")
	}
	if brokererr.CodeOf(err) != brokererr.CodeNoPosition {
		t.Fatalf("code=%q", brokererr.CodeOf(err))
	}
}

func TestMT5GetAccountEquity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/current/accounts/acct-1/account-information" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]float64{"equity": 5000})
	}))
	defer srv.Close()

	b := testMT5Broker(t, srv)
	eq, err := b.GetAccountEquity(context.Background())
	if err != nil {
		t.Fatalf("GetAccountEquity: %v", err)
	}
	if eq != 5000 {
		t.Fatalf("equity=%v", eq)
	}
}

func TestMT5OrderWithSLTP(t *testing.T) {
	var gotPayload map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/current/accounts/acct-1/symbols/EURUSD/current-price":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ask": 1.1000,
				"bid": 1.0998,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/users/current/accounts/acct-1/trade":
			_ = json.NewDecoder(r.Body).Decode(&gotPayload)
			_ = json.NewEncoder(w).Encode(map[string]string{"orderId": "MT5-SLTP-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testMT5Broker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:    "BUY",
		Symbol:    "EURUSD",
		Quantity:  1.0,
		OrderType: domain.OrderBracket,
		SLPoints:  20,
		TPPoints:  40,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if result.OrderID != "MT5-SLTP-1" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
	// ask=1.1000, BUY, SL=20 pips → stopLoss = 1.1000 - 20*0.0001 = 1.0980
	wantSL := 1.1000 - 20*0.0001
	if result.SLPrice != wantSL {
		t.Fatalf("SLPrice=%v want %v", result.SLPrice, wantSL)
	}
	// TP=40 pips → takeProfit = 1.1000 + 40*0.0001 = 1.1040
	wantTP := 1.1000 + 40*0.0001
	if result.TPPrice != wantTP {
		t.Fatalf("TPPrice=%v want %v", result.TPPrice, wantTP)
	}
	if _, ok := gotPayload["stopLoss"]; !ok {
		t.Error("stopLoss missing from MT5 payload")
	}
	if _, ok := gotPayload["takeProfit"]; !ok {
		t.Error("takeProfit missing from MT5 payload")
	}
}

func TestMT5OrderNoSLTP(t *testing.T) {
	var gotPayload map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/users/current/accounts/acct-1/trade" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		_ = json.NewEncoder(w).Encode(map[string]string{"orderId": "MT5-MKT-1"})
	}))
	defer srv.Close()

	b := testMT5Broker(t, srv)
	_, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:    "BUY",
		Symbol:    "EURUSD",
		Quantity:  1.0,
		OrderType: domain.OrderMarket,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if _, ok := gotPayload["stopLoss"]; ok {
		t.Error("stopLoss must not be in market order payload")
	}
	if _, ok := gotPayload["takeProfit"]; ok {
		t.Error("takeProfit must not be in market order payload")
	}
}

func TestMT5CancelOrder(t *testing.T) {
	var gotPayload map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		_ = json.NewEncoder(w).Encode(map[string]string{"orderId": "MT5ORD1"})
	}))
	defer srv.Close()

	b := testMT5Broker(t, srv)
	err := b.CancelOrder(context.Background(), &domain.CancelRequest{OrderID: "MT5ORD1"})
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if gotPayload["actionType"] != "ORDER_CANCEL" {
		t.Fatalf("actionType=%v want ORDER_CANCEL", gotPayload["actionType"])
	}
	if gotPayload["orderId"] != "MT5ORD1" {
		t.Fatalf("orderId=%v want MT5ORD1", gotPayload["orderId"])
	}
}

func TestMT5CancelOrderNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"order not found"}`))
	}))
	defer srv.Close()

	b := testMT5Broker(t, srv)
	err := b.CancelOrder(context.Background(), &domain.CancelRequest{OrderID: "MISSING"})
	if err == nil {
		t.Fatal("expected error")
	}
	if brokererr.CodeOf(err) != brokererr.CodeOrderNotFound {
		t.Fatalf("code=%q want order_not_found", brokererr.CodeOf(err))
	}
}
