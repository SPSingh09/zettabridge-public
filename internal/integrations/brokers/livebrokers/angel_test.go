package livebrokers

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers/httpclient"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func testAngelBroker(t *testing.T, srv *httptest.Server) *AngelBroker {
	t.Helper()
	cred := &store.BrokerCredential{BrokerType: "angel", AccountMode: "live"}
	infra := &Infra{
		HTTP: httpclient.New(time.Second, 0),
		URLs: URLs{AngelLive: srv.URL},
		Angel: AngelHeaders{
			ClientLocalIP:  "10.0.0.1",
			ClientPublicIP: "203.0.113.10",
			MACAddress:     "00:00:00:00:00:00",
		},
		SymbolTokens: nil, // direct searchScrip in tests
	}
	b, err := NewWithParsed(cred, brokercreds.Parsed{
		BrokerType: "angel",
		APIKey:     "api-key",
		ClientCode: "CLIENT1",
		JWT:        "jwt-token",
	}, infra, nil)
	if err != nil {
		t.Fatalf("NewWithParsed: %v", err)
	}
	return b.(*AngelBroker)
}

func angelOK(w http.ResponseWriter, data interface{}) {
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": true, "message": "SUCCESS", "errorcode": "", "data": data,
	})
}

func TestAngelPlaceMarketOrder(t *testing.T) {
	var gotOrder map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "searchScrip"):
			angelOK(w, []map[string]string{
				{"exchange": "NSE", "tradingsymbol": "RELIANCE-EQ", "symboltoken": "2885"},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "placeOrder"):
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotOrder = body
			if body["ordertype"] != "MARKET" {
				t.Errorf("ordertype=%q", body["ordertype"])
			}
			if body["stoploss"] != "0" || body["squareoff"] != "0" {
				t.Errorf("bracket fields should be zero: %#v", body)
			}
			angelOK(w, map[string]string{"orderid": "ANG123"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testAngelBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:   "BUY",
		Symbol:   "RELIANCE",
		Exchange: "NSE",
		Product:  "MIS",
		Quantity: 5,
		AlgoID:   "ANGALGO1",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if result.OrderID != "ANG123" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
	if gotOrder["symboltoken"] != "2885" || gotOrder["producttype"] != "INTRADAY" {
		t.Fatalf("order payload=%#v", gotOrder)
	}
	if gotOrder["ordertag"] != "ANGALGO1" {
		t.Fatalf("ordertag=%q", gotOrder["ordertag"])
	}
}

func TestAngelCloseLongPosition(t *testing.T) {
	var placedTxn string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "searchScrip"):
			angelOK(w, []map[string]string{
				{"exchange": "NSE", "tradingsymbol": "RELIANCE-EQ", "symboltoken": "2885"},
			})
		case strings.HasSuffix(r.URL.Path, "getPosition"):
			angelOK(w, []map[string]interface{}{
				{
					"tradingsymbol": "RELIANCE-EQ",
					"exchange":      "NSE",
					"producttype":   "INTRADAY",
					"netqty":        "10",
					"symboltoken":   "2885",
				},
			})
		case strings.HasSuffix(r.URL.Path, "placeOrder"):
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			placedTxn = body["transactiontype"]
			angelOK(w, map[string]string{"orderid": "ANG-CLOSE"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testAngelBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:   "CLOSE",
		Symbol:   "RELIANCE",
		Exchange: "NSE",
		Product:  "MIS",
	})
	if err != nil {
		t.Fatalf("CLOSE: %v", err)
	}
	if placedTxn != "SELL" {
		t.Fatalf("txn=%q want SELL", placedTxn)
	}
	if result.OrderID != "ANG-CLOSE" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
}

func TestAngelCloseNoPosition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "searchScrip"):
			angelOK(w, []map[string]string{
				{"exchange": "NSE", "tradingsymbol": "RELIANCE-EQ", "symboltoken": "2885"},
			})
		case strings.HasSuffix(r.URL.Path, "getPosition"):
			angelOK(w, []interface{}{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testAngelBroker(t, srv)
	_, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action: "CLOSE", Symbol: "RELIANCE", Exchange: "NSE", Product: "MIS",
	})
	if err == nil {
		t.Fatal("expected no position")
	}
	if brokererr.CodeOf(err) != brokererr.CodeNoPosition {
		t.Fatalf("code=%q", brokererr.CodeOf(err))
	}
}

func TestAngelAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":false,"message":"Invalid Token","errorcode":"AG8001"}`))
	}))
	defer srv.Close()

	b := testAngelBroker(t, srv)
	_, err := b.GetAccountEquity(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
	if brokererr.CodeOf(err) != brokererr.CodeAuthFailed {
		t.Fatalf("code=%q", brokererr.CodeOf(err))
	}
}

func TestAngelGetAccountEquity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "getRMS") {
			angelOK(w, map[string]string{"availablecash": "15000.50"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	b := testAngelBroker(t, srv)
	eq, err := b.GetAccountEquity(context.Background())
	if err != nil {
		t.Fatalf("GetAccountEquity: %v", err)
	}
	if eq != 15000.50 {
		t.Fatalf("equity=%v", eq)
	}
}

func TestPickAngelSearchResult(t *testing.T) {
	token, ts, err := pickAngelSearchResult("RELIANCE", []angelSearchRow{
		{TradingSymbol: "RELIANCE-EQ", SymbolToken: "2885"},
		{TradingSymbol: "RELIANCE-BE", SymbolToken: "1111"},
	})
	if err != nil || token != "2885" || ts != "RELIANCE-EQ" {
		t.Fatalf("got token=%q ts=%q err=%v", token, ts, err)
	}
}

func TestAngelHeadersSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-ClientPublicIP") != "203.0.113.10" {
			t.Errorf("public IP header=%q", r.Header.Get("X-ClientPublicIP"))
		}
		if r.Header.Get("X-PrivateKey") != "api-key" {
			t.Errorf("private key header=%q", r.Header.Get("X-PrivateKey"))
		}
		angelOK(w, map[string]string{"availablecash": "1000"})
	}))
	defer srv.Close()

	b := testAngelBroker(t, srv)
	if _, err := b.GetAccountEquity(context.Background()); err != nil {
		t.Fatalf("GetAccountEquity: %v", err)
	}
}

func TestAngelROBOOrder(t *testing.T) {
	var gotVariety, gotOrderType string
	var gotStoploss, gotSquareoff string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/rest/secure/angelbroking/order/v1/searchScrip":
			angelOK(w, []map[string]interface{}{
				{"exchange": "NSE", "tradingsymbol": "RELIANCE-EQ", "symboltoken": "2885"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/rest/secure/angelbroking/market-data/v1/quote/real-time":
			angelOK(w, map[string]interface{}{
				"fetched": []map[string]interface{}{
					{"ltp": 2500.0},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/rest/secure/angelbroking/order/v1/placeOrder":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotVariety, _ = body["variety"].(string)
			gotOrderType, _ = body["ordertype"].(string)
			gotStoploss, _ = body["stoploss"].(string)
			gotSquareoff, _ = body["squareoff"].(string)
			angelOK(w, map[string]string{"orderid": "AROBO-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testAngelBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:    "BUY",
		Symbol:    "RELIANCE",
		Exchange:  "NSE",
		Product:   "MIS",
		Quantity:  5,
		OrderType: domain.OrderBracket,
		SLPoints:  30,
		TPPoints:  50,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if result.OrderID != "AROBO-1" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
	if gotVariety != "ROBO" {
		t.Fatalf("variety=%q want ROBO", gotVariety)
	}
	if gotOrderType != "MARKET" {
		t.Fatalf("ordertype=%q want MARKET", gotOrderType)
	}
	// ltp=2500, BUY, SLPoints=30 → slAbs=30 → stoploss="30.00"
	if gotStoploss != "30.00" {
		t.Fatalf("stoploss=%q want 30.00", gotStoploss)
	}
	// TPPoints=50 → tpAbs=50 → squareoff="50.00"
	if gotSquareoff != "50.00" {
		t.Fatalf("squareoff=%q want 50.00", gotSquareoff)
	}
	// SLPrice = ltp - SLPoints = 2500 - 30 = 2470
	if result.SLPrice != 2470.0 {
		t.Fatalf("SLPrice=%v want 2470", result.SLPrice)
	}
	// TPPrice = ltp + TPPoints = 2500 + 50 = 2550
	if result.TPPrice != 2550.0 {
		t.Fatalf("TPPrice=%v want 2550", result.TPPrice)
	}
}

func TestAngelCancelOrder(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "cancelOrder") {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			angelOK(w, map[string]string{"orderid": "ANG123"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	b := testAngelBroker(t, srv)
	err := b.CancelOrder(context.Background(), &domain.CancelRequest{OrderID: "ANG123"})
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if gotBody["orderid"] != "ANG123" {
		t.Fatalf("orderid=%q want ANG123", gotBody["orderid"])
	}
	if gotBody["variety"] != "NORMAL" {
		t.Fatalf("variety=%q want NORMAL", gotBody["variety"])
	}
}

func TestAngelCancelOrderBrokerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": false, "message": "Order not found", "errorcode": "AG8002",
		})
	}))
	defer srv.Close()

	b := testAngelBroker(t, srv)
	err := b.CancelOrder(context.Background(), &domain.CancelRequest{OrderID: "BADORDER"})
	if err == nil {
		t.Fatal("expected error")
	}
}
