package publisher

import (
	"fmt"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/domain"
)

// validate runs publisher-specific pre-flight checks before basket construction.
// General order validation (symbol, exchange, product) is handled by maporder.Build
// before the executor is called; this layer catches publisher-only restrictions.
func validate(parsed brokercreds.Parsed, req *domain.PlaceRequest) error {
	if strings.TrimSpace(parsed.APIKey) == "" {
		return fmt.Errorf("publisher credential is missing api_key")
	}
	if strings.ToUpper(req.Action) == "CLOSE" {
		return fmt.Errorf("CLOSE is not supported in Kite Publisher mode. Send an explicit BUY or SELL signal with quantity and limit price.")
	}
	if req.OrderType == domain.OrderBracket {
		return fmt.Errorf("publisher mode does not support bracket/cover orders (v1)")
	}
	if req.SLPoints > 0 || req.TPPoints > 0 {
		return fmt.Errorf("publisher mode does not support SL/TP (v1)")
	}
	return nil
}
