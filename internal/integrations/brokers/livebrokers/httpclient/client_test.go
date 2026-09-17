package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoRetriesGETOn503(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(2*time.Second, 2)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/equity", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.Do(context.Background(), "mt5_cloud", "get_equity", req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if calls.Load() != 3 {
		t.Fatalf("want 3 GET attempts, got %d", calls.Load())
	}
}

func TestDoDoesNotRetryPOST(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(2*time.Second, 3)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/orders", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.Do(context.Background(), "zerodha", "place_order", req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if calls.Load() != 1 {
		t.Fatalf("want 1 POST attempt, got %d", calls.Load())
	}
}

func TestRedactHeaderValue(t *testing.T) {
	if !RedactHeaderValue("Authorization") || !RedactHeaderValue("auth-token") {
		t.Fatal("expected sensitive headers to redact")
	}
	if RedactHeaderValue("Content-Type") {
		t.Fatal("content-type should not redact")
	}
}
