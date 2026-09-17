package pnl_test

import (
	"math"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/pnl"
)

func TestWinRateNilWithoutCloses(t *testing.T) {
	if got := pnl.WinRate([]pnl.Row{
		{Signal: "BUY", Symbol: "RELIANCE", LotSize: 1, FillPrice: 100, Status: "filled"},
	}); got != nil {
		t.Fatalf("expected nil win rate, got %v", *got)
	}
}

func TestWinRateFIFO(t *testing.T) {
	rate := pnl.WinRate([]pnl.Row{
		{Signal: "BUY", Symbol: "RELIANCE", LotSize: 1, FillPrice: 100, Status: "filled"},
		{Signal: "SELL", Symbol: "RELIANCE", LotSize: 1, FillPrice: 110, Status: "filled"},
		{Signal: "BUY", Symbol: "RELIANCE", LotSize: 1, FillPrice: 100, Status: "filled"},
		{Signal: "SELL", Symbol: "RELIANCE", LotSize: 1, FillPrice: 90, Status: "filled"},
	})
	if rate == nil {
		t.Fatal("expected win rate")
	}
	if math.Abs(*rate-0.5) > 0.001 {
		t.Fatalf("win rate want 0.5 got %v", *rate)
	}
}
