package marketdata

import "strings"

// SymbolKey returns a case-insensitive lookup key for exchange+symbol pairs.
func SymbolKey(exchange, symbol string) string {
	return strings.ToUpper(strings.TrimSpace(exchange)) + "|" + strings.ToUpper(strings.TrimSpace(symbol))
}
