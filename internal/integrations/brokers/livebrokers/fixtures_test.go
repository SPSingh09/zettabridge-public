package livebrokers

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers/httpclient"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(testdataPath(name))
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return b
}

func writeFixtureResponse(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func TestGoldenZerodhaPlaceOrder(t *testing.T) {
	orderBody := loadFixture(t, "zerodha_order_success.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/orders/regular" {
			writeFixtureResponse(w, http.StatusOK, orderBody)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action: "BUY", Symbol: "RELIANCE", Exchange: "NSE", Product: "MIS", Quantity: 5,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if result.OrderID != "Z-GOLDEN-001" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
}

func TestGoldenZerodhaGetEquity(t *testing.T) {
	marginsBody := loadFixture(t, "zerodha_margins_success.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/user/margins" {
			writeFixtureResponse(w, http.StatusOK, marginsBody)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	eq, err := b.GetAccountEquity(context.Background())
	if err != nil {
		t.Fatalf("GetAccountEquity: %v", err)
	}
	if eq != 250000.50 {
		t.Fatalf("equity=%v", eq)
	}
}

func TestGoldenZerodhaAuthError(t *testing.T) {
	errBody := loadFixture(t, "zerodha_auth_error.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFixtureResponse(w, http.StatusForbidden, errBody)
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

func TestGoldenMT5PlaceOrder(t *testing.T) {
	tradeBody := loadFixture(t, "mt5_trade_success.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/trade") {
			writeFixtureResponse(w, http.StatusOK, tradeBody)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	b := testMT5Broker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action: "BUY", Symbol: "EURUSD", Quantity: 0.1,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if result.OrderID != "MT5-GOLDEN-001" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
}

func TestGoldenMT5GetEquity(t *testing.T) {
	equityBody := loadFixture(t, "mt5_equity_success.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "account-information") {
			writeFixtureResponse(w, http.StatusOK, equityBody)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	b := testMT5Broker(t, srv)
	eq, err := b.GetAccountEquity(context.Background())
	if err != nil {
		t.Fatalf("GetAccountEquity: %v", err)
	}
	if eq != 12500.75 {
		t.Fatalf("equity=%v", eq)
	}
}

func TestGoldenAngelPlaceOrder(t *testing.T) {
	searchBody := loadFixture(t, "angel_search_scrip.json")
	placeBody := loadFixture(t, "angel_place_success.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "searchScrip"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(searchBody)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "placeOrder"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(placeBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testAngelBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action: "BUY", Symbol: "RELIANCE", Exchange: "NSE", Product: "MIS", Quantity: 1,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if result.OrderID != "ANG-GOLDEN-001" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
}

func TestGoldenDhanPlaceOrder(t *testing.T) {
	csv := loadFixture(t, "dhan_instrument.csv")
	orderBody := loadFixture(t, "dhan_order_success.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/instrument/"):
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write(csv)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/orders":
			writeFixtureResponse(w, http.StatusOK, orderBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := testDhanBroker(t, srv)
	result, err := b.PlaceOrder(context.Background(), &domain.PlaceRequest{
		Action: "BUY", Symbol: "RELIANCE", Exchange: "NSE", Product: "MIS", Quantity: 10,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if result.OrderID != "DH-GOLDEN-001" {
		t.Fatalf("orderID=%q", result.OrderID)
	}
}

func TestGoldenDhanGetEquity(t *testing.T) {
	fundBody := loadFixture(t, "dhan_fundlimit_success.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/fundlimit" {
			writeFixtureResponse(w, http.StatusOK, fundBody)
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
	if eq != 50000.25 {
		t.Fatalf("equity=%v", eq)
	}
}

func TestGoldenBrokerErrParsing(t *testing.T) {
	tests := []struct {
		fixture string
		status  int
		want    brokererr.Code
	}{
		{"zerodha_auth_error.json", http.StatusForbidden, brokererr.CodeAuthFailed},
		{"mt5_auth_error.json", http.StatusUnauthorized, brokererr.CodeAuthFailed},
		{"angel_auth_error.json", http.StatusUnauthorized, brokererr.CodeAuthFailed},
		{"dhan_auth_error.json", http.StatusUnauthorized, brokererr.CodeAuthFailed},
	}
	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			body := loadFixture(t, tc.fixture)
			rec := httptest.NewRecorder()
			rec.WriteHeader(tc.status)
			_, _ = rec.Write(body)
			err := brokerErrFromStatus(rec.Result())
			if brokererr.CodeOf(err) != tc.want {
				t.Fatalf("code=%q want %q", brokererr.CodeOf(err), tc.want)
			}
		})
	}
}

// Ensure golden JSON files remain valid JSON (except CSV).
func TestGoldenFixturesValidJSON(t *testing.T) {
	names := []string{
		"zerodha_order_success.json", "zerodha_margins_success.json", "zerodha_auth_error.json",
		"mt5_trade_success.json", "mt5_equity_success.json", "mt5_auth_error.json",
		"angel_search_scrip.json", "angel_place_success.json", "angel_auth_error.json",
		"dhan_order_success.json", "dhan_fundlimit_success.json", "dhan_auth_error.json",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if !json.Valid(loadFixture(t, name)) {
				t.Fatal("invalid JSON")
			}
		})
	}
}

// compile-time check that test helpers still build infra correctly.
func TestGoldenInfraDefaults(t *testing.T) {
	infra := &Infra{
		HTTP: httpclient.New(time.Second, 0),
		URLs: URLs{ZerodhaLive: "http://example.com"},
	}
	cred := &store.BrokerCredential{BrokerType: "zerodha", AccountMode: "live"}
	b, err := NewWithParsed(cred, brokercreds.Parsed{BrokerType: "zerodha", APIKey: "k", AccessToken: "t"}, infra, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b == nil {
		t.Fatal("nil broker")
	}
}
