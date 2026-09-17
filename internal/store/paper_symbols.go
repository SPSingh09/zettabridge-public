package store

import "strings"

// NormalizePaperSymbol uppercases and trims paper position/order symbol keys.
func NormalizePaperSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}

// NormalizePaperExchange uppercases and trims paper position/order exchange keys.
func NormalizePaperExchange(exchange string) string {
	return strings.ToUpper(strings.TrimSpace(exchange))
}
