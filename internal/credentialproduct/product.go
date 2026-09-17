package credentialproduct

import (
	"fmt"
	"sort"
	"strings"
)

var valid = map[string]struct{}{
	"MIS":  {},
	"CNC":  {},
	"NRML": {},
}

var order = []string{"MIS", "CNC", "NRML"}

// Parse splits a stored credential product field into normalized product codes.
// Accepts a single code or comma-separated list (e.g. "MIS,CNC,NRML").
func Parse(stored string) []string {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return nil
	}
	parts := strings.Split(stored, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, raw := range parts {
		p := strings.ToUpper(strings.TrimSpace(raw))
		if p == "" {
			continue
		}
		if _, ok := valid[p]; !ok {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sortByCanonical(out)
	return out
}

// Serialize stores a product list in the credential product column.
func Serialize(products []string) (string, error) {
	normalized, err := NormalizeList(products)
	if err != nil {
		return "", err
	}
	if len(normalized) == 0 {
		return "MIS", nil
	}
	return strings.Join(normalized, ","), nil
}

// NormalizeList validates, deduplicates, and orders product codes.
func NormalizeList(products []string) ([]string, error) {
	if len(products) == 0 {
		return []string{"MIS"}, nil
	}
	seen := make(map[string]struct{}, len(products))
	out := make([]string, 0, len(products))
	for _, raw := range products {
		p := strings.ToUpper(strings.TrimSpace(raw))
		if p == "" {
			continue
		}
		if _, ok := valid[p]; !ok {
			return nil, fmt.Errorf("product must be MIS, CNC, or NRML")
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one product is required")
	}
	sortByCanonical(out)
	return out, nil
}

// Default returns the primary product (first in canonical order) for legacy callers.
func Default(stored string) string {
	products := Parse(stored)
	if len(products) == 0 {
		return "MIS"
	}
	return products[0]
}

// NormalizeSettings validates exchange + products for Indian broker credentials.
func NormalizeSettings(exchange string, products []string, allowMultiple bool) (string, string, error) {
	ex := strings.TrimSpace(exchange)
	if ex == "" {
		return "", "", fmt.Errorf("exchange is required for Indian brokers (NSE or BSE)")
	}
	ex = strings.ToUpper(ex)
	if ex != "NSE" && ex != "BSE" {
		return "", "", fmt.Errorf("exchange must be NSE or BSE")
	}

	normalized, err := NormalizeList(products)
	if err != nil {
		return "", "", err
	}
	if !allowMultiple && len(normalized) > 1 {
		return "", "", fmt.Errorf("your plan allows one product per broker account; upgrade to Pro Plus for multiple products")
	}

	stored, err := Serialize(normalized)
	if err != nil {
		return "", "", err
	}
	return ex, stored, nil
}

// ValidateStored checks a persisted product field (single or comma-separated).
func ValidateStored(exchange, stored string) error {
	ex := strings.ToUpper(strings.TrimSpace(exchange))
	if ex != "NSE" && ex != "BSE" {
		return fmt.Errorf("exchange must be NSE or BSE")
	}
	raw := strings.TrimSpace(stored)
	if raw == "" {
		return nil
	}
	if strings.Contains(raw, ",") {
		if _, err := NormalizeList(Parse(raw)); err != nil {
			return err
		}
		return nil
	}
	p := strings.ToUpper(raw)
	if _, ok := valid[p]; !ok {
		return fmt.Errorf("product must be MIS, CNC, or NRML")
	}
	return nil
}

// ResolveOrderProduct picks the broker product for an order.
// When multiple products are enabled, the signal must include product.
func ResolveOrderProduct(stored, signalProduct string) (string, error) {
	allowed := Parse(stored)
	if len(allowed) == 0 {
		return "MIS", nil
	}

	sig := strings.ToUpper(strings.TrimSpace(signalProduct))
	if sig != "" {
		for _, p := range allowed {
			if p == sig {
				return sig, nil
			}
		}
		return "", fmt.Errorf("product %q is not enabled on this broker account (allowed: %s)", sig, strings.Join(allowed, ", "))
	}

	if len(allowed) == 1 {
		return allowed[0], nil
	}
	return "", fmt.Errorf("product is required in the signal when multiple products are enabled on the broker account")
}

func sortByCanonical(products []string) {
	sort.Slice(products, func(i, j int) bool {
		return canonicalIndex(products[i]) < canonicalIndex(products[j])
	})
}

func canonicalIndex(p string) int {
	for i, code := range order {
		if code == p {
			return i
		}
	}
	return len(order)
}
