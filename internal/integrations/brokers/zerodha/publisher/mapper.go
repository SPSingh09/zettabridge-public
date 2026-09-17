package publisher

import (
	"fmt"
	"math"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/domain"
)

// math is used for qty rounding below

// mapOrder converts a normalised PlaceRequest (already validated by maporder.Build)
// into a Kite basket item.
//
// By the time PlaceRequest reaches the publisher executor, maporder.Build has already:
//   - confirmed Exchange is NSE or BSE
//   - converted lot → shares (req.Quantity is integer share count)
//   - set EntryExecType to MARKET or LIMIT
func mapOrder(req *domain.PlaceRequest) (BasketItem, error) {
	exchange := strings.ToUpper(strings.TrimSpace(req.Exchange))
	if exchange != "NSE" && exchange != "BSE" {
		return BasketItem{}, fmt.Errorf("publisher only supports NSE/BSE exchange, got %q", req.Exchange)
	}

	product := strings.ToUpper(strings.TrimSpace(req.Product))
	if product != "MIS" && product != "CNC" && product != "NRML" {
		return BasketItem{}, fmt.Errorf("publisher only supports MIS/CNC/NRML product, got %q", req.Product)
	}

	txnType := "BUY"
	if strings.ToUpper(req.Action) == "SELL" {
		txnType = "SELL"
	}

	qty := int(math.Round(req.Quantity))
	if qty < 1 {
		return BasketItem{}, fmt.Errorf("quantity must be >= 1, got %v", req.Quantity)
	}

	execType := strings.ToUpper(req.EntryExecType)
	if execType != "LIMIT" {
		// Kite's basket page does not forward market_protection to its internal
		// order-placement API, so MARKET orders always fail with
		// "Market orders without market protection are not allowed via API".
		// Only LIMIT orders are supported in publisher basket mode.
		return BasketItem{}, fmt.Errorf(
			"publisher basket mode only supports LIMIT orders — set order_type = LIMIT and provide a price in your signal")
	}

	var price float64
	price = req.Price
	if price <= 0 {
		return BasketItem{}, fmt.Errorf("LIMIT order requires price > 0")
	}

	return BasketItem{
		Exchange:        exchange,
		TradingSymbol:   strings.ToUpper(strings.TrimSpace(req.Symbol)),
		TransactionType: txnType,
		Quantity:        qty,
		OrderType:       "LIMIT",
		Product:         product,
		Price:           price,
		Variety:         "regular",
	}, nil
}
