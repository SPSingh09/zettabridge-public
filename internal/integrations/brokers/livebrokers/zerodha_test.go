package livebrokers

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers/httpclient"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func testZerodhaBroker(t *testing.T, srv *httptest.Server) *ZerodhaBroker {
	t.Helper()
	cred := &store.BrokerCredential{BrokerType: "zerodha", AccountMode: "live"}
	infra := &Infra{
		HTTP: httpclient.New(time.Second, 0),
		URLs: URLs{ZerodhaLive: srv.URL},
	}
	b, err := NewWithParsed(cred, brokercreds.Parsed{
		BrokerType:  "zerodha",
		APIKey:      "apikey",
		AccessToken: "access",
	}, infra, nil)
	if err != nil {
		t.Fatalf("NewWithParsed: %v", err)
	}
	return b.(*ZerodhaBroker)
}

func TestZerodhaPlaceMarketOrder(t *testing.T) {
	var gotTag string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/orders/regular" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.FormValue("order_type") != "MARKET" {
			t.Errorf("order_type=%v want MARKET", r.FormValue("order_type"))
		}
		if r.FormValue("transaction_type") != "BUY" {
			t.Errorf("transaction_type=%v", r.FormValue("transaction_type"))
		}
		gotTag = r.FormValue("tag")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]string{"order_id": "Z123"},
		})
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:    "BUY",
		Symbol:    "RELIANCE",
		Exchange:  "NSE",
		Product:   "MIS",
		Quantity:  10,
		AlgoID:    "ALGO123",
		OrderType: domain.OrderMarket,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if result.OrderID != "Z123" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
	if gotTag != "ALGO123" {
		t.Fatalf("tag=%q want ALGO123", gotTag)
	}
}

func TestZerodhaCoverOrder(t *testing.T) {
	var gotTriggerPrice float64
	var gotVariety string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/quote":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"NSE:RELIANCE": map[string]interface{}{"last_price": 2500.0},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/orders/co":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			gotVariety = r.FormValue("order_type")
			gotTriggerPrice, _ = strconv.ParseFloat(r.FormValue("trigger_price"), 64)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]string{"order_id": "ZCO-1"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:    "BUY",
		Symbol:    "RELIANCE",
		Exchange:  "NSE",
		Product:   "MIS",
		Quantity:  10,
		OrderType: domain.OrderBracket,
		SLPoints:  20,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if result.OrderID != "ZCO-1" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
	if gotVariety != "MARKET" {
		t.Fatalf("order_type=%q want MARKET", gotVariety)
	}
	// ltp=2500, BUY → trigger_price = 2500 - 20 = 2480
	if gotTriggerPrice != 2480.0 {
		t.Fatalf("trigger_price=%v want 2480", gotTriggerPrice)
	}
	if result.SLPrice != 2480.0 {
		t.Fatalf("SLPrice=%v want 2480", result.SLPrice)
	}
}

func TestZerodhaCoverOrderTPIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/quote":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"NSE:RELIANCE": map[string]interface{}{"last_price": 2500.0},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/orders/co":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			if r.FormValue("target_price") != "" {
				t.Error("cover order must not include target_price")
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]string{"order_id": "ZCO-2"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	_, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:    "BUY",
		Symbol:    "RELIANCE",
		Exchange:  "NSE",
		Product:   "MIS",
		Quantity:  5,
		OrderType: domain.OrderBracket,
		SLPoints:  20,
		TPPoints:  30,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
}

func TestZerodhaCloseLongPosition(t *testing.T) {
	var placedTxn string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/portfolio/positions":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"net": []map[string]interface{}{
						{
							"tradingsymbol": "RELIANCE",
							"exchange":      "NSE",
							"product":       "MIS",
							"quantity":      15,
						},
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/orders/regular":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			placedTxn = r.FormValue("transaction_type")
			if r.FormValue("order_type") != "MARKET" {
				t.Errorf("order_type=%v", r.FormValue("order_type"))
			}
			if qty, _ := strconv.Atoi(r.FormValue("quantity")); qty != 15 {
				t.Errorf("quantity=%v", r.FormValue("quantity"))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]string{"order_id": "Z-CLOSE"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:   "CLOSE",
		Symbol:   "RELIANCE",
		Exchange: "NSE",
		Product:  "MIS",
	})
	if err != nil {
		t.Fatalf("PlaceOrder CLOSE: %v", err)
	}
	if placedTxn != "SELL" {
		t.Fatalf("close txn=%q want SELL", placedTxn)
	}
	if result.OrderID != "Z-CLOSE" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
}

func TestZerodhaCloseNoPosition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/portfolio/positions" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{"net": []map[string]interface{}{}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	_, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:   "CLOSE",
		Symbol:   "RELIANCE",
		Exchange: "NSE",
		Product:  "MIS",
	})
	if err == nil {
		t.Fatal("expected no position error")
	}
	if brokererr.CodeOf(err) != brokererr.CodeNoPosition {
		t.Fatalf("code=%q", brokererr.CodeOf(err))
	}
}

func TestZerodhaAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Invalid api_key"}`))
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	_, err := b.GetAccountEquity(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
	if brokererr.CodeOf(err) != brokererr.CodeAuthFailed {
		t.Fatalf("code=%q", brokererr.CodeOf(err))
	}
}

// TestZerodhaLimitOrderQuotePermissionDenied covers a real-world Kite
// Connect gotcha: a LIMIT signal with no price triggers a /quote lookup to
// auto-price the order, and if the app's API key lacks market-data
// permission, Kite returns HTTP 403 with error_type=PermissionException —
// the same status Kite uses for a genuinely invalid/expired token. Before
// this fix, that always mapped to CodeAuthFailed, misleadingly implying the
// OAuth connection itself was broken.
func TestZerodhaLimitOrderQuotePermissionDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/quote") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"status":"error","message":"Insufficient permission for that call.","error_type":"PermissionException"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	_, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:        "BUY",
		Symbol:        "SBIN",
		Exchange:      "NSE",
		Product:       "MIS",
		Quantity:      2,
		EntryExecType: "LIMIT",
		// Price left at zero — forces the LTP auto-price lookup.
	})
	if err == nil {
		t.Fatal("expected permission-denied error")
	}
	if brokererr.CodeOf(err) != brokererr.CodePermissionDenied {
		t.Fatalf("code=%q, want %q", brokererr.CodeOf(err), brokererr.CodePermissionDenied)
	}
	if !strings.Contains(err.Error(), "auto-price this LIMIT order") {
		t.Fatalf("expected actionable context in error message, got: %v", err)
	}
}

func TestZerodhaCancelOrder(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data":   map[string]string{"order_id": "ZRD123"},
		})
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	err := b.CancelOrder(context.Background(), &domain.CancelRequest{OrderID: "ZRD123"})
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method=%q want DELETE", gotMethod)
	}
	if gotPath != "/orders/regular/ZRD123" {
		t.Fatalf("path=%q want /orders/regular/ZRD123", gotPath)
	}
}

func TestZerodhaLimitOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/quote":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"NSE:RELIANCE": map[string]interface{}{"last_price": 2500.0},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/orders/regular":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			if r.FormValue("order_type") != "LIMIT" {
				t.Errorf("order_type=%v want LIMIT", r.FormValue("order_type"))
			}
			if r.FormValue("price") == "" || r.FormValue("price") == "0.00" {
				t.Errorf("LIMIT order must have a price, got %q", r.FormValue("price"))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]string{"order_id": "Z-LIMIT"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:        "BUY",
		Symbol:        "RELIANCE",
		Exchange:      "NSE",
		Product:       "MIS",
		Quantity:      10,
		EntryExecType: "LIMIT",
		OrderType:     domain.OrderMarket,
	})
	if err != nil {
		t.Fatalf("PlaceOrder LIMIT: %v", err)
	}
	if result.OrderID != "Z-LIMIT" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
}

func TestZerodhaCloseWithLimitCredAlwaysMarket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/portfolio/positions":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"net": []map[string]interface{}{
						{
							"tradingsymbol": "RELIANCE",
							"exchange":      "NSE",
							"product":       "MIS",
							"quantity":      5,
						},
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/orders/regular":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			if r.FormValue("order_type") != "MARKET" {
				t.Errorf("CLOSE must use MARKET regardless of credential order_type; got %q", r.FormValue("order_type"))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]string{"order_id": "Z-CLOSE-MKT"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action:        "CLOSE",
		Symbol:        "RELIANCE",
		Exchange:      "NSE",
		Product:       "MIS",
		EntryExecType: "LIMIT", // credential has LIMIT; CLOSE must override to MARKET
	})
	if err != nil {
		t.Fatalf("PlaceOrder CLOSE: %v", err)
	}
	if result.OrderID != "Z-CLOSE-MKT" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
}

func TestZerodhaCancelOrderNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Order not found"}`))
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	err := b.CancelOrder(context.Background(), &domain.CancelRequest{OrderID: "MISSING"})
	if err == nil {
		t.Fatal("expected error")
	}
	if brokererr.CodeOf(err) != brokererr.CodeOrderNotFound {
		t.Fatalf("code=%q want order_not_found", brokererr.CodeOf(err))
	}
}
