package symboltoken

import (
	"fmt"
	"strings"
	"time"
)

// DefaultTTL is how long resolved Angel/Dhan symbol tokens are cached (live adapters).
const DefaultTTL = 24 * time.Hour

// CacheKey is the Redis key for broker+exchange+symbol → instrument token.
// Example: symtoken:angel:NSE:RELIANCE
func CacheKey(brokerType, exchange, symbol string) string {
	bt := strings.ToLower(strings.TrimSpace(brokerType))
	if bt == "" {
		bt = "default"
	}
	ex := strings.ToUpper(strings.TrimSpace(exchange))
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	return fmt.Sprintf("symtoken:%s:%s:%s", bt, ex, sym)
}
