package fyers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/marketdata"
)

func TestGetBatchLTP_NoAccessToken_ReturnsErrNoQuote(t *testing.T) {
	p := New("APP123")
	_, err := p.GetBatchLTP(context.Background(), []marketdata.Symbol{{Exchange: "NSE", Symbol: "SBIN"}})
	if err != marketdata.ErrNoQuote {
		t.Fatalf("expected ErrNoQuote before SetAccessToken, got %v", err)
	}
}

func TestGetBatchLTP_ParsesRealResponseShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "APP123:tok-abc" {
			t.Errorf("unexpected Authorization header: %q", got)
		}
		if got := r.URL.Query().Get("symbols"); got != "NSE:SBIN-EQ,NSE:HDFCBANK-EQ" {
			t.Errorf("unexpected symbols param: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"s":"ok","d":[
			{"n":"NSE:SBIN-EQ","v":{"lp":825.5}},
			{"n":"NSE:HDFCBANK-EQ","v":{"lp":1650.25}}
		]}`))
	}))
	defer server.Close()

	quotesURL = server.URL
	defer func() { quotesURL = "https://api-t1.fyers.in/data/quotes" }()

	p := New("APP123")
	p.SetAccessToken("tok-abc")

	quotes, err := p.GetBatchLTP(context.Background(), []marketdata.Symbol{
		{Exchange: "NSE", Symbol: "SBIN"},
		{Exchange: "NSE", Symbol: "HDFCBANK"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("expected 2 quotes, got %d", len(quotes))
	}
	byLTP := map[string]float64{}
	for _, q := range quotes {
		byLTP[q.Symbol.Symbol] = q.LTP
	}
	if byLTP["SBIN"] != 825.5 {
		t.Fatalf("expected SBIN LTP 825.5, got %v", byLTP["SBIN"])
	}
	if byLTP["HDFCBANK"] != 1650.25 {
		t.Fatalf("expected HDFCBANK LTP 1650.25, got %v", byLTP["HDFCBANK"])
	}
}

func TestGetLTP_SingleSymbol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"s":"ok","d":[{"n":"NSE:SBIN-EQ","v":{"lp":825.5}}]}`))
	}))
	defer server.Close()
	quotesURL = server.URL
	defer func() { quotesURL = "https://api-t1.fyers.in/data/quotes" }()

	p := New("APP123")
	p.SetAccessToken("tok-abc")

	q, err := p.GetLTP(context.Background(), marketdata.Symbol{Exchange: "NSE", Symbol: "SBIN"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.LTP != 825.5 {
		t.Fatalf("expected LTP 825.5, got %v", q.LTP)
	}
}

func TestGetBatchLTP_APIErrorEnvelope_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"s":"error","code":-99,"message":"invalid auth token"}`))
	}))
	defer server.Close()
	quotesURL = server.URL
	defer func() { quotesURL = "https://api-t1.fyers.in/data/quotes" }()

	p := New("APP123")
	p.SetAccessToken("expired-tok")

	_, err := p.GetBatchLTP(context.Background(), []marketdata.Symbol{{Exchange: "NSE", Symbol: "SBIN"}})
	if err == nil {
		t.Fatal("expected an error for a FYERS error envelope, got nil")
	}
}
