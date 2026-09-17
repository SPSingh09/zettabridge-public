package publisher

import "encoding/json"

const kiteBasketURL = "https://kite.zerodha.com/connect/basket"

// EnsureMarketProtection patches a basket JSON string so that every MARKET
// order has a non-zero market_protection value. This is needed for orders
// stored before the market_protection field was introduced.
func EnsureMarketProtection(basketJSON string) string {
	var items []BasketItem
	if err := json.Unmarshal([]byte(basketJSON), &items); err != nil {
		return basketJSON
	}
	patched := false
	for i := range items {
		if items[i].OrderType == "MARKET" && items[i].MarketProtection <= 0 {
			items[i].MarketProtection = 0.5
			patched = true
		}
	}
	if !patched {
		return basketJSON
	}
	b, err := json.Marshal(items)
	if err != nil {
		return basketJSON
	}
	return string(b)
}

// BasketItem is one order inside a Kite Publisher basket.
type BasketItem struct {
	Exchange         string  `json:"exchange"`
	TradingSymbol    string  `json:"tradingsymbol"`
	TransactionType  string  `json:"transaction_type"` // BUY | SELL
	Quantity         int     `json:"quantity"`
	OrderType        string  `json:"order_type"` // MARKET | LIMIT
	Product          string  `json:"product"`    // MIS | CNC | NRML
	Price            float64 `json:"price,omitempty"`
	MarketProtection float64 `json:"market_protection,omitempty"` // % protection for MARKET orders; required by Kite API
	Variety          string  `json:"variety"`                     // regular (v1 only)
}

// buildBasketJSON serialises a basket to the JSON string Kite expects in the `data` POST field.
func buildBasketJSON(items []BasketItem) (string, error) {
	b, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
