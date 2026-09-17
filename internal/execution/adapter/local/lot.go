package local

import (
	"context"
	"log"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/risk"
)

// ResolveLot computes order quantity from signal lot, risk sizing, or fixed lot.
func ResolveLot(
	ctx context.Context,
	b domain.Broker,
	brokerType string,
	signalLot, fixedLot, maxRiskPct float64,
	slPts int,
) (float64, error) {
	if fixedLot > 0 {
		return fixedLot, nil
	}
	if signalLot > 0 {
		return signalLot, nil
	}

	var equity float64
	if maxRiskPct > 0 {
		var err error
		equity, err = b.GetAccountEquity(ctx)
		if err != nil {
			log.Printf("worker: equity lookup failed, using fixed lot: %v", err)
			if fixedLot > 0 {
				return fixedLot, nil
			}
			return risk.MinQtyForBroker(brokerType), nil
		}
	}

	lot, err := risk.ResolveLot(0, fixedLot, equity, maxRiskPct, slPts, brokerType)
	if err != nil {
		log.Printf("worker: risk sizing unavailable, using fixed lot: %v", err)
		if fixedLot > 0 {
			return fixedLot, nil
		}
		return risk.MinQtyForBroker(brokerType), nil
	}
	return lot, nil
}
