package queue

import (
	"strings"

	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// applyTradeSignalMeta records the user-facing order type (MARKET/LIMIT/BRACKET)
// and product from the resolved signal + webhook before persisting the trade row.
func applyTradeSignalMeta(trade *store.Trade, wh *store.Webhook, sig *store.SignalPayload, params guard.TradeParams, productFallback string) {
	if trade == nil {
		return
	}
	trade.OrderType = resolveTradeOrderType(wh, sig, params)
	product := strings.ToUpper(strings.TrimSpace(params.Product))
	if product == "" {
		product = strings.ToUpper(strings.TrimSpace(productFallback))
	}
	trade.Product = product
}

func resolveTradeOrderType(wh *store.Webhook, _ *store.SignalPayload, params guard.TradeParams) string {
	if params.SLPts > 0 || params.TPPts > 0 {
		return "BRACKET"
	}
	return guard.WebhookOrderType(wh)
}
