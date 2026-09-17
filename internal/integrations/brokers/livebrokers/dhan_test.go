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

const testInstrumentCSV = `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SM_SYMBOL_NAME,SEM_CUSTOM_SYMBOL
NSE,E,11536,TCS,TCS
NSE,E,2885,RELIANCE,RELIANCE`

func testDhanBroker(t *testing.T, srv *httptest.Server) *DhanBroker {
	t.Helper()
	cred := &store.BrokerCredential{BrokerType: "dhan", AccountMode: "live"}
	infra := &Infra{
		HTTP:         httpclient.New(time.Second, 0),
		URLs:         URLs{DhanLive: srv.URL},
		SymbolTokens: nil,
	}
	b, err := NewWithParsed(cred, brokercreds.Parsed{
		BrokerType:  "dhan",
		ClientID:    "1000000001",
		AccessToken: "dhan-jwt",
	}, infra, nil)
	if err != nil {
		t.Fatalf("NewWithParsed: %v", err)
	}
	return b.(*DhanBroker)
}

func TestDhanPlaceMarketOrder(t *testing.T) {
	var gotOrder map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/instrument/"):
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(testInstrumentCSV))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/orders":
			_ = json.NewDecoder(r.Body).Decode(&gotOrder)
			if gotOrder["orderType"] != "MARKET" {
				t.Errorf("orderType=%v", gotOrder["orderType"])
			}
			if gotOrder["productType"] != "INTRADAY" {
				t.Errorf("productType=%v", gotOrder["productType"])
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"orderId": "DH123", "orderStatus": "PENDING",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:   "BUY",
		Symbol:   "TCS",
		Exchange: "NSE",
		Product:  "MIS",
		Quantity: 10,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if result.OrderID != "DH123" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
	if gotOrder["securityId"] != "11536" {
		t.Fatalf("securityId=%v", gotOrder["securityId"])
	}
	if gotOrder["dhanClientId"] != "1000000001" {
		t.Fatalf("dhanClientId=%v", gotOrder["dhanClientId"])
	}
	if _, present := gotOrder["correlationId"]; present {
		t.Fatal("correlationId should be absent when AlgoID is empty")
	}
}

func TestDhanPlaceMarketOrderWithAlgoID(t *testing.T) {
	var gotOrder map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/instrument/"):
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(testInstrumentCSV))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/orders":
			_ = json.NewDecoder(r.Body).Decode(&gotOrder)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"orderId": "DH456", "orderStatus": "PENDING",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	_, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:   "BUY",
		Symbol:   "RELIANCE",
		Exchange: "NSE",
		Product:  "MIS",
		Quantity: 1,
		AlgoID:   "TESTALGO1",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if gotOrder["correlationId"] != "TESTALGO1" {
		t.Fatalf("correlationId=%v, want TESTALGO1", gotOrder["correlationId"])
	}
}

func TestDhanCloseLongPosition(t *testing.T) {
	var placedTxn string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/instrument/"):
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(testInstrumentCSV))
		case r.URL.Path == "/v2/positions":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"tradingSymbol":   "TCS",
					"securityId":      "11536",
					"exchangeSegment": "NSE_EQ",
					"productType":     "INTRADAY",
					"netQty":          8,
				},
			})
		case r.URL.Path == "/v2/orders":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			placedTxn, _ = body["transactionType"].(string)
			_ = json.NewEncoder(w).Encode(map[string]string{"orderId": "DH-CLOSE", "orderStatus": "PENDING"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action: "CLOSE", Symbol: "TCS", Exchange: "NSE", Product: "MIS",
	})
	if err != nil {
		t.Fatalf("CLOSE: %v", err)
	}
	if placedTxn != "SELL" {
		t.Fatalf("txn=%q want SELL", placedTxn)
	}
	if result.OrderID != "DH-CLOSE" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
}

func TestDhanCloseNoPosition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/instrument/"):
			_, _ = w.Write([]byte(testInstrumentCSV))
		case r.URL.Path == "/v2/positions":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	_, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action: "CLOSE", Symbol: "TCS", Exchange: "NSE", Product: "MIS",
	})
	if err == nil {
		t.Fatal("expected no position")
	}
	if brokererr.CodeOf(err) != brokererr.CodeNoPosition {
		t.Fatalf("code=%q", brokererr.CodeOf(err))
	}
}

func TestDhanAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "failure", "errorType": "AUTHENTICATION_ERROR",
			"errorCode": "AUTH001", "errorMessage": "Invalid or expired access token",
		})
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	_, err := b.GetAccountEquity(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
	if brokererr.CodeOf(err) != brokererr.CodeAuthFailed {
		t.Fatalf("code=%q", brokererr.CodeOf(err))
	}
}

func TestDhanGetAccountEquity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/fundlimit" {
			_ = json.NewEncoder(w).Encode(map[string]float64{"availabelBalance": 42000.5})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	eq, err := b.GetAccountEquity(context.Background())
	if err != nil {
		t.Fatalf("GetAccountEquity: %v", err)
	}
	if eq != 42000.5 {
		t.Fatalf("equity=%v", eq)
	}
}

func TestDhanFailureResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "failure", "errorCode": "E001", "errorMessage": "Invalid security ID",
		})
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	_, err := b.GetAccountEquity(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if brokererr.CodeOf(err) != brokererr.CodeInvalidSymbol {
		t.Fatalf("code=%q", brokererr.CodeOf(err))
	}
}

func TestDhanAccessTokenHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("access-token") != "dhan-jwt" {
			t.Errorf("access-token=%q", r.Header.Get("access-token"))
		}
		_ = json.NewEncoder(w).Encode(map[string]float64{"availabelBalance": 1000})
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	if _, err := b.GetAccountEquity(context.Background()); err != nil {
		t.Fatalf("GetAccountEquity: %v", err)
	}
}

func TestDhanBracketRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/instrument/"):
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(testInstrumentCSV))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	_, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:    "BUY",
		Symbol:    "RELIANCE",
		Exchange:  "NSE",
		Product:   "MIS",
		Quantity:  5,
		OrderType: domain.OrderBracket,
		SLPoints:  20,
	})
	if err == nil {
		t.Fatal("expected bracket_not_supported error")
	}
	if brokererr.CodeOf(err) != brokererr.CodeBracketNotSupported {
		t.Fatalf("code=%q want bracket_not_supported", brokererr.CodeOf(err))
	}
}

func TestDhanCancelOrder(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"orderId": "DHN123", "orderStatus": "CANCELLED"})
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	err := b.CancelOrder(context.Background(), &domain.CancelRequest{OrderID: "DHN123"})
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method=%q want DELETE", gotMethod)
	}
	if gotPath != "/v2/orders/DHN123" {
		t.Fatalf("path=%q want /v2/orders/DHN123", gotPath)
	}
}

func TestDhanCancelOrderNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":"failure","errorMessage":"order not found"}`))
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	err := b.CancelOrder(context.Background(), &domain.CancelRequest{OrderID: "MISSING"})
	if err == nil {
		t.Fatal("expected error")
	}
	if brokererr.CodeOf(err) != brokererr.CodeOrderNotFound {
		t.Fatalf("code=%q want order_not_found", brokererr.CodeOf(err))
	}
}
