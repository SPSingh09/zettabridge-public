package signals

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/store"
)

var errPaperIngestTypeRequired = errors.New(`type must be "ORDER_SIGNAL" or "PRICE_UPDATE"`)

type paperIngestRoute int

const (
	paperRouteOrder paperIngestRoute = iota
	paperRoutePriceUpdate
)

// routePaperIngest interprets a paper webhook body. It accepts:
//   - typed payloads with "type":"ORDER_SIGNAL" | "PRICE_UPDATE"
//   - legacy live-webhook shape (action, lot, price, …) with no type field
//   - the common script mistake of putting ORDER_SIGNAL in order_type instead of type
func routePaperIngest(body []byte) (paperIngestRoute, *store.SignalPayload, *store.PaperSignalPayload, error) {
	var raw store.PaperSignalPayload
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, nil, nil, err
	}
	normalizePaperTypeFields(body, &raw)

	switch strings.ToUpper(strings.TrimSpace(raw.Type)) {
	case string(domain.SignalTypePriceUpdate):
		return paperRoutePriceUpdate, nil, &raw, nil

	case string(domain.SignalTypeOrderSignal):
		return paperRouteOrder, paperOrderFromPayload(&raw), &raw, nil

	case "":
		var sig store.SignalPayload
		if err := json.Unmarshal(body, &sig); err != nil {
			return 0, nil, nil, err
		}
		if priceUpdate, fixed := normalizeMisplacedOrderType(body, &sig); fixed {
			if priceUpdate {
				pr := &store.PaperSignalPayload{
					Type:    string(domain.SignalTypePriceUpdate),
					Symbol:  sig.Symbol,
					Price:   sig.Price,
					Comment: sig.Comment,
				}
				return paperRoutePriceUpdate, nil, pr, nil
			}
		}
		if strings.TrimSpace(sig.Action) == "" {
			return 0, nil, nil, errPaperIngestTypeRequired
		}
		return paperRouteOrder, &sig, nil, nil

	default:
		return 0, paperOrderFromPayload(&raw), &raw, errPaperIngestTypeRequired
	}
}

// normalizePaperTypeFields fixes scripts that set "order_type":"ORDER_SIGNAL"
// (or PRICE_UPDATE) instead of "type". When duplicate order_type keys exist,
// Go keeps the last value — extractRealOrderType recovers LIMIT/MARKET from the
// same body when present.
func normalizePaperTypeFields(body []byte, raw *store.PaperSignalPayload) {
	if raw == nil || strings.TrimSpace(raw.Type) != "" {
		return
	}
	switch strings.ToUpper(strings.TrimSpace(raw.OrderType)) {
	case string(domain.SignalTypeOrderSignal):
		raw.Type = string(domain.SignalTypeOrderSignal)
		raw.OrderType = extractRealOrderType(body)
	case string(domain.SignalTypePriceUpdate):
		raw.Type = string(domain.SignalTypePriceUpdate)
		raw.OrderType = ""
	}
}

func extractRealOrderType(body []byte) string {
	var sig store.SignalPayload
	if err := json.Unmarshal(body, &sig); err != nil {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(sig.OrderType)) {
	case "MARKET", "LIMIT":
		return sig.OrderType
	default:
		return ""
	}
}

// normalizeMisplacedOrderType clears order_type when a script used it for the
// paper signal type instead of MARKET/LIMIT. Returns (priceUpdate, fixed).
func normalizeMisplacedOrderType(body []byte, sig *store.SignalPayload) (priceUpdate, fixed bool) {
	if sig == nil {
		return false, false
	}
	switch strings.ToUpper(strings.TrimSpace(sig.OrderType)) {
	case string(domain.SignalTypeOrderSignal):
		sig.OrderType = extractRealOrderType(body)
		return false, true
	case string(domain.SignalTypePriceUpdate):
		sig.OrderType = ""
		return true, true
	default:
		return false, false
	}
}
