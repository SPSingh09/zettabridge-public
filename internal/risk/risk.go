package risk

import (
	"errors"
	"fmt"
	"math"
)

const (
	DefaultMaxRiskPct = 1.0
	MinMaxRiskPct     = 0.01
	MaxMaxRiskPct     = 100.0
	MinLotSize        = 0.01 // minimum lot size for MT5/forex
	MinShares         = 1.0  // minimum quantity for Indian brokers (1 share)
)

// MinQtyForBroker returns the minimum tradeable quantity unit for a broker type.
// Indian brokers count in shares; MT5/forex counts in lots.
func MinQtyForBroker(brokerType string) float64 {
	switch brokerType {
	case "zerodha", "angel", "dhan":
		return MinShares
	default:
		return MinLotSize
	}
}

var (
	ErrInvalidMaxRiskPct = errors.New("max_risk_pct must be between 0.01 and 100")
	ErrInvalidSLPoints   = errors.New("sl_points must be positive for risk-based sizing")
)

// ValidateMaxRiskPct checks that v is within the allowed percentage range.
func ValidateMaxRiskPct(v float64) error {
	if v < MinMaxRiskPct || v > MaxMaxRiskPct {
		return ErrInvalidMaxRiskPct
	}
	return nil
}

// NormalizeMaxRiskPct returns the default when v is unset (<= 0).
func NormalizeMaxRiskPct(v float64) float64 {
	if v <= 0 {
		return DefaultMaxRiskPct
	}
	return v
}

// CalcLotSize derives position size from account equity, risk percentage, and stop distance.
// slPts is treated as pips for forex (mt5_cloud) and as per-share stop distance for Indian brokers.
func CalcLotSize(equity, maxRiskPct float64, slPts int, brokerType string) (float64, error) {
	if maxRiskPct <= 0 {
		return 0, nil
	}
	if slPts <= 0 {
		return 0, ErrInvalidSLPoints
	}

	riskAmount := equity * (maxRiskPct / 100)
	var lots float64

	switch brokerType {
	case "mt5_cloud", "mock":
		// Standard-lot forex: ~$10 per pip for USD-denominated majors.
		const pipValuePerLot = 10.0
		lots = riskAmount / (float64(slPts) * pipValuePerLot)
		lots = math.Floor(lots*100) / 100
		if lots < MinLotSize {
			lots = MinLotSize
		}
	default:
		// Indian brokers: slPts = stop distance in INR per share; quantity is in shares.
		shares := riskAmount / float64(slPts)
		lots = math.Floor(shares) // round down to whole shares
		if lots < MinShares {
			lots = MinShares
		}
	}
	return lots, nil
}

// ResolveLot picks the effective lot size for a trade.
// Precedence: webhook fixed lot > explicit signal lot > risk-based sizing.
func ResolveLot(signalLot, fixedLot, equity, maxRiskPct float64, slPts int, brokerType string) (float64, error) {
	if fixedLot > 0 {
		return fixedLot, nil
	}
	if signalLot > 0 {
		return signalLot, nil
	}
	if maxRiskPct > 0 && equity > 0 {
		calculated, err := CalcLotSize(equity, maxRiskPct, slPts, brokerType)
		if err != nil {
			return 0, err
		}
		if calculated > 0 {
			return calculated, nil
		}
	}
	if fixedLot > 0 {
		return fixedLot, nil
	}
	return MinLotSize, fmt.Errorf("could not resolve lot size")
}
