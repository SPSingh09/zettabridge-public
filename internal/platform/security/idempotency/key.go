package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/store"
)

const DefaultWindowSec = 300

type canonicalPayload struct {
	Action string  `json:"action"`
	Symbol string  `json:"symbol"`
	Lot    float64 `json:"lot"`
	SLPts  int     `json:"sl_pts,omitempty"`
	TPPts  int     `json:"tp_pts,omitempty"`
}

// WindowSec returns the dedup window for a webhook (0 = disabled).
func WindowSec(wh *store.Webhook) int {
	if wh == nil {
		return 0
	}
	return wh.DedupWindowSec
}

// Window returns the dedup duration for a webhook (0 = disabled).
func Window(wh *store.Webhook) time.Duration {
	sec := WindowSec(wh)
	if sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

// Enabled reports whether signal dedup is active for the webhook.
func Enabled(wh *store.Webhook) bool {
	return WindowSec(wh) > 0
}

// SignalKey derives a stable idempotency key from the inbound payload (not guard-resolved params).
// Uses raw signal fields with webhook defaults so Redis-cached webhooks produce the same key as Postgres loads.
func SignalKey(webhookID string, wh *store.Webhook, sig *store.SignalPayload) string {
	symbol := strings.TrimSpace(sig.Symbol)
	if symbol == "" {
		if syms := guard.EffectiveAllowedSymbols(wh); len(syms) == 1 {
			symbol = syms[0]
		} else if len(syms) > 0 {
			symbol = syms[0]
		}
	}
	lot := sig.Lot
	if lot <= 0 && wh.LotSize > 0 {
		lot = wh.LotSize
	}
	sl := sig.SLPts
	if sl <= 0 {
		sl = wh.SLPoints
	}
	tp := sig.TPPts
	if tp <= 0 {
		tp = wh.TPPoints
	}

	comment := strings.TrimSpace(sig.Comment)
	if comment != "" {
		return hashHex(fmt.Sprintf("%s|%s|%s|%g|%s", webhookID, sig.Action, symbol, lot, comment))
	}
	body, err := json.Marshal(canonicalPayload{
		Action: sig.Action,
		Symbol: symbol,
		Lot:    lot,
		SLPts:  sl,
		TPPts:  tp,
	})
	if err != nil {
		return hashHex(webhookID + "|" + sig.Action + "|" + symbol)
	}
	return hashHex(webhookID + "|" + string(body))
}

func hashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
