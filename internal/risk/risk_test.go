package risk

import (
	"errors"
	"testing"
)

func TestValidateMaxRiskPct(t *testing.T) {
	if err := ValidateMaxRiskPct(1.0); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if err := ValidateMaxRiskPct(0); !errors.Is(err, ErrInvalidMaxRiskPct) {
		t.Fatalf("expected ErrInvalidMaxRiskPct, got %v", err)
	}
	if err := ValidateMaxRiskPct(100.01); !errors.Is(err, ErrInvalidMaxRiskPct) {
		t.Fatalf("expected ErrInvalidMaxRiskPct, got %v", err)
	}
}

func TestNormalizeMaxRiskPct(t *testing.T) {
	if got := NormalizeMaxRiskPct(0); got != DefaultMaxRiskPct {
		t.Fatalf("expected default %v, got %v", DefaultMaxRiskPct, got)
	}
	if got := NormalizeMaxRiskPct(2.5); got != 2.5 {
		t.Fatalf("expected 2.5, got %v", got)
	}
}

func TestCalcLotSizeForex(t *testing.T) {
	// $10,000 equity, 1% risk = $100; 20 pip SL → $100 / (20 * $10) = 0.5 lots
	got, err := CalcLotSize(10000, 1.0, 20, "mt5_cloud")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.5 {
		t.Fatalf("expected 0.5 lots, got %v", got)
	}
}

func TestCalcLotSizeMinimum(t *testing.T) {
	got, err := CalcLotSize(100, 0.01, 500, "mt5_cloud")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != MinLotSize {
		t.Fatalf("expected min lot %v, got %v", MinLotSize, got)
	}
}

func TestResolveLotRiskBased(t *testing.T) {
	got, err := ResolveLot(0, 0, 10000, 1.0, 20, "mt5_cloud")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.5 {
		t.Fatalf("expected risk-based 0.5, got %v", got)
	}
}

func TestResolveLotSignalOverride(t *testing.T) {
	got, err := ResolveLot(0.25, 0, 10000, 1.0, 20, "mt5_cloud")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.25 {
		t.Fatalf("expected signal lot 0.25, got %v", got)
	}
}

func TestResolveLotFixedLotWins(t *testing.T) {
	got, err := ResolveLot(0.25, 0.01, 10000, 1.0, 20, "mt5_cloud")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.01 {
		t.Fatalf("expected fixed lot 0.01, got %v", got)
	}
}

func TestResolveLotFallback(t *testing.T) {
	got, err := ResolveLot(0, 0.02, 0, 0, 20, "mt5_cloud")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.02 {
		t.Fatalf("expected fixed lot 0.02, got %v", got)
	}
}
