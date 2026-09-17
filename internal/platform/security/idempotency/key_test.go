package idempotency

import (
	"testing"

	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestSignalKeyStableWithComment(t *testing.T) {
	wh := &store.Webhook{ID: "wh-1", Symbol: "EURUSD", LotSize: 0.01}
	sig := &store.SignalPayload{Action: "BUY", Comment: "tv-1"}

	k1 := SignalKey("wh-1", wh, sig)
	k2 := SignalKey("wh-1", wh, sig)
	if k1 == "" || k1 != k2 {
		t.Fatalf("expected stable key, got %q and %q", k1, k2)
	}
}

func TestSignalKeyDiffersByAction(t *testing.T) {
	wh := &store.Webhook{ID: "wh-1", Symbol: "EURUSD", LotSize: 0.01}
	comment := "same"

	buy := SignalKey("wh-1", wh, &store.SignalPayload{Action: "BUY", Comment: comment})
	sell := SignalKey("wh-1", wh, &store.SignalPayload{Action: "SELL", Comment: comment})
	if buy == sell {
		t.Fatal("expected different keys for BUY vs SELL")
	}
}

func TestSignalKeyWithoutCommentUsesPayload(t *testing.T) {
	wh := &store.Webhook{ID: "wh-1", Symbol: "RELIANCE", LotSize: 1}
	sig := &store.SignalPayload{Action: "BUY", Symbol: "RELIANCE", Lot: 2}

	k1 := SignalKey("wh-1", wh, sig)
	k2 := SignalKey("wh-1", wh, &store.SignalPayload{Action: "BUY", Symbol: "RELIANCE", Lot: 2})
	if k1 != k2 {
		t.Fatalf("expected same canonical key: %q vs %q", k1, k2)
	}

	k3 := SignalKey("wh-1", wh, &store.SignalPayload{Action: "BUY", Symbol: "RELIANCE", Lot: 3})
	if k1 == k3 {
		t.Fatal("expected different keys when lot differs")
	}
}

func TestSignalKeyIsConsistentForSamePayload(t *testing.T) {
	wh1 := &store.Webhook{ID: "wh-1", Symbol: "EURUSD", LotSize: 0.02}
	wh2 := &store.Webhook{ID: "wh-1", Symbol: "EURUSD", LotSize: 0.02}
	sig := &store.SignalPayload{Action: "BUY", Symbol: "EURUSD", Lot: 0.01, Comment: "tv"}

	k1 := SignalKey("wh-1", wh1, sig)
	k2 := SignalKey("wh-1", wh2, sig)
	if k1 != k2 {
		t.Fatalf("same webhook and signal should produce same key: %q vs %q", k1, k2)
	}
}

func TestWindowDisabledByDefault(t *testing.T) {
	if Enabled(&store.Webhook{}) {
		t.Fatal("expected dedup disabled when dedup_window_sec is 0")
	}
	if WindowSec(&store.Webhook{DedupWindowSec: 300}) != 300 {
		t.Fatal("expected 300s window")
	}
}
