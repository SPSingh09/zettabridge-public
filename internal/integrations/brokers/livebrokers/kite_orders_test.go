package livebrokers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers/httpclient"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestMapKiteStatus(t *testing.T) {
	tests := []struct {
		kite     string
		want     string
		terminal bool
	}{
		{"COMPLETE", domain.StatusFilled, true},
		{"REJECTED", domain.StatusRejected, true},
		{"CANCELLED", "cancelled", true},
		{"OPEN", domain.StatusSubmitted, false},
		{"TRIGGER PENDING", domain.StatusSubmitted, false},
	}
	for _, tc := range tests {
		got, term := MapKiteStatus(tc.kite)
		if got != tc.want || term != tc.terminal {
			t.Fatalf("MapKiteStatus(%q) = (%q, %v), want (%q, %v)", tc.kite, got, term, tc.want, tc.terminal)
		}
	}
}

func TestFetchTodayOrders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/orders" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"order_id": "111", "status": "OPEN", "average_price": 0},
				{"order_id": "222", "status": "COMPLETE", "average_price": 2500.5},
			},
		})
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	orders, err := b.FetchTodayOrders(context.Background())
	if err != nil {
		t.Fatalf("FetchTodayOrders: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("len=%d want 2", len(orders))
	}
	if orders["222"].AveragePrice != 2500.5 {
		t.Fatalf("avg=%v", orders["222"].AveragePrice)
	}
}

func TestFetchOrderHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/orders/333" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"order_id": "333", "status": "OPEN", "average_price": 0},
				{"order_id": "333", "status": "COMPLETE", "average_price": 109.4, "status_message": ""},
			},
		})
	}))
	defer srv.Close()

	b := testZerodhaBroker(t, srv)
	snap, err := b.FetchOrderHistory(context.Background(), "333")
	if err != nil {
		t.Fatalf("FetchOrderHistory: %v", err)
	}
	if snap.Status != "COMPLETE" || snap.AveragePrice != 109.4 {
		t.Fatalf("snap=%+v", snap)
	}
}

func TestApplyKiteSnapshot(t *testing.T) {
	status, price, msg, ok := ApplyKiteSnapshot(KiteOrderSnapshot{Status: "COMPLETE", AveragePrice: 100})
	if !ok || status != domain.StatusFilled || price != 100 || msg != "" {
		t.Fatalf("got (%q, %v, %q, %v)", status, price, msg, ok)
	}
	status, _, msg, ok = ApplyKiteSnapshot(KiteOrderSnapshot{Status: "REJECTED", StatusMessage: "margin shortfall"})
	if !ok || status != domain.StatusRejected || msg != "margin shortfall" {
		t.Fatalf("rejected snap=%+v", status)
	}
	_, _, _, ok = ApplyKiteSnapshot(KiteOrderSnapshot{Status: "OPEN"})
	if ok {
		t.Fatal("OPEN should not apply")
	}
}

func TestFetchOrderHistoryNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cred := &store.BrokerCredential{BrokerType: "zerodha", AccountMode: "live"}
	infra := &Infra{
		HTTP: httpclient.New(time.Second, 0),
		URLs: URLs{ZerodhaLive: srv.URL},
	}
	b, err := NewWithParsed(cred, brokercreds.Parsed{BrokerType: "zerodha", APIKey: "k", AccessToken: "t"}, infra, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.(*ZerodhaBroker).FetchOrderHistory(context.Background(), "missing")
	if err == nil || brokererr.CodeOf(err) != brokererr.CodeOrderNotFound {
		t.Fatalf("err=%v", err)
	}
}
