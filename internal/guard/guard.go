package guard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SPSingh09/zettabridge/internal/store"
)

var (
	ErrActionNotAllowed    = errors.New("action not allowed for this webhook")
	ErrSymbolNotAllowed    = errors.New("symbol not allowed for this webhook")
	ErrSymbolRequired      = errors.New("symbol is required in signal")
	ErrQuantityRequired    = errors.New("quantity is required in signal when webhook default quantity is 0")
	ErrCommentRequired     = errors.New("comment does not match required value")
	ErrOutsideTradingHours = errors.New("outside trading hours")
	ErrLotExceedsMax       = errors.New("lot size exceeds max_lot_size")
	ErrOrderTypeNotAllowed = errors.New("order_type not allowed for this webhook")
)

// ErrCode maps a guard validation error to a stable error code string, used
// to populate trade.ErrorCode so the webhook Summary view can distinguish
// ZettaBridge-side validation rejections from broker rejections regardless
// of whether the check ran at HTTP ingest or in the worker's re-validation.
func ErrCode(err error) string {
	switch {
	case errors.Is(err, ErrCommentRequired):
		return "invalid_secret"
	case errors.Is(err, ErrActionNotAllowed):
		return "action_not_allowed"
	case errors.Is(err, ErrSymbolNotAllowed):
		return "symbol_not_allowed"
	case errors.Is(err, ErrSymbolRequired):
		return "symbol_required"
	case errors.Is(err, ErrQuantityRequired):
		return "quantity_required"
	case errors.Is(err, ErrLotExceedsMax):
		return "lot_exceeds_max"
	case errors.Is(err, ErrOutsideTradingHours):
		return "outside_trading_hours"
	case errors.Is(err, ErrOrderTypeNotAllowed):
		return "order_type_not_allowed"
	default:
		return "gateway_rejected"
	}
}

// MarketHoursEnforcementSettingKey is the platform_settings key controlling
// whether Paper Trading orders are rejected outside their market profile's
// trading hours (Phase 7). Enabled by default when unset — live market data
// (FYERS) only flows during real market hours, so this is normally on;
// disable it for signal-only testing outside those hours.
const MarketHoursEnforcementSettingKey = "paper_market_hours_enforcement"

// MarketHoursStore is the narrow read set CheckMarketHours needs — satisfied
// by *store.PGStore directly, and structurally by queue.TradeStore (a
// superset interface), so both queue/paper.go and paperaccounts/handler.go
// can share this one check.
type MarketHoursStore interface {
	GetPlatformSetting(ctx context.Context, key string) (value string, ok bool, err error)
	GetMarketProfileByCode(ctx context.Context, code string) (*store.MarketProfile, error)
}

// CheckMarketHours returns a non-empty rejection reason when Phase 7's
// market-hours enforcement is on (the platform_settings default when unset)
// and marketProfileCode's trading hours don't cover now. A missing/unreadable
// platform setting or market profile row fails open (no rejection) — a
// config gap shouldn't block every paper order.
func CheckMarketHours(ctx context.Context, pg MarketHoursStore, marketProfileCode string, now time.Time) string {
	enabled := true
	if v, ok, err := pg.GetPlatformSetting(ctx, MarketHoursEnforcementSettingKey); err == nil && ok {
		enabled = v == "true"
	}
	if !enabled {
		return ""
	}

	profile, err := pg.GetMarketProfileByCode(ctx, marketProfileCode)
	if err != nil || profile == nil {
		return ""
	}
	if !WithinSchedule(profile.TradingDays, profile.Timezone, now) {
		return fmt.Sprintf("outside trading hours for %s (%s)", profile.Name, profile.Timezone)
	}
	return ""
}

var validActions = map[string]struct{}{
	"BUY": {}, "SELL": {}, "CLOSE": {},
}

var weekdayKeys = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// TradeParams are the resolved execution parameters after applying signal overrides.
type TradeParams struct {
	Symbol  string
	Lot     float64
	SLPts   int
	TPPts   int
	Price   float64 // explicit LIMIT entry price from signal; 0 = broker fetches LTP
	Product string  // MIS | CNC | NRML — required when credential enables multiple products
}

// DefaultAllowedActions returns the backward-compatible action set.
func DefaultAllowedActions() []string {
	return []string{"BUY", "SELL", "CLOSE"}
}

// ValidateAllowedActions ensures each action is known and non-empty.
func ValidateAllowedActions(actions []string) error {
	if len(actions) == 0 {
		return errors.New("allowed_actions must not be empty")
	}
	for _, a := range actions {
		if _, ok := validActions[a]; !ok {
			return fmt.Errorf("invalid action %q in allowed_actions", a)
		}
	}
	return nil
}

// NormalizeAllowedSymbols uppercases, trims, and deduplicates webhook symbol whitelist entries.
func NormalizeAllowedSymbols(symbols []string) ([]string, error) {
	seen := make(map[string]struct{}, len(symbols))
	out := make([]string, 0, len(symbols))
	for _, raw := range symbols {
		s := strings.ToUpper(strings.TrimSpace(raw))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, errors.New("allowed_symbols must include at least one symbol")
	}
	return out, nil
}

// EffectiveAllowedSymbols returns the webhook's allowed symbol list.
// Legacy rows with only webhook.symbol are supported until migrated.
func EffectiveAllowedSymbols(wh *store.Webhook) []string {
	if len(wh.AllowedSymbols) > 0 {
		return wh.AllowedSymbols
	}
	if s := strings.TrimSpace(wh.Symbol); s != "" {
		return []string{strings.ToUpper(s)}
	}
	return nil
}

// ValidateSymbol checks the resolved symbol against allowed_symbols (Option A).
func ValidateSymbol(wh *store.Webhook, symbol string) error {
	if symbolAllowed(wh, symbol) {
		return nil
	}
	return symbolNotAllowedError(symbol)
}

// symbolNotAllowedError builds an actionable rejection message naming the
// symbol that was tried, wrapping the stable ErrSymbolNotAllowed sentinel so
// errors.Is(err, ErrSymbolNotAllowed) still matches (see gatewayErrCode in
// modules/signals/handler.go).
func symbolNotAllowedError(symbol string) error {
	return fmt.Errorf("%w — %q is not in this webhook's allowed symbols list", ErrSymbolNotAllowed, symbol)
}

func symbolAllowed(wh *store.Webhook, symbol string) bool {
	symbol = strings.TrimSpace(symbol)
	for _, s := range EffectiveAllowedSymbols(wh) {
		if strings.EqualFold(strings.TrimSpace(s), symbol) {
			return true
		}
	}
	return false
}

// ResolveParams produces effective trade parameters. Signal values win for SL/TP
// when provided. Symbol comes from the signal, or the sole allowed symbol when
// the whitelist has exactly one entry. Quantity is resolved via ResolveQuantity.
func ResolveParams(wh *store.Webhook, sig *store.SignalPayload) TradeParams {
	p := TradeParams{
		Price:   sig.Price,
		Lot:     sig.Lot,
		Product: strings.ToUpper(strings.TrimSpace(sig.Product)),
	}

	if sig.Symbol != "" {
		p.Symbol = strings.TrimSpace(sig.Symbol)
	} else if syms := EffectiveAllowedSymbols(wh); len(syms) == 1 {
		p.Symbol = syms[0]
	}

	if sig.SLPts > 0 {
		p.SLPts = sig.SLPts
	} else {
		p.SLPts = wh.SLPoints
	}

	if sig.TPPts > 0 {
		p.TPPts = sig.TPPts
	} else {
		p.TPPts = wh.TPPoints
	}

	return p
}

// ResolveQuantity applies webhook default quantity rules.
// Non-zero webhook lot_size always wins; zero requires a positive signal quantity.
func ResolveQuantity(wh *store.Webhook, signalLot float64) (float64, error) {
	if wh.LotSize > 0 {
		return wh.LotSize, nil
	}
	if signalLot > 0 {
		return signalLot, nil
	}
	return 0, ErrQuantityRequired
}

// ValidateIngest runs hot-path checks before enqueueing a signal.
func ValidateIngest(wh *store.Webhook, sig *store.SignalPayload, now time.Time) error {
	if err := ValidateAction(wh, sig.Action); err != nil {
		return err
	}
	if wh.RequiredComment != "" && sig.Comment != wh.RequiredComment {
		return ErrCommentRequired
	}
	if !WithinTradingHours(wh, now) {
		return ErrOutsideTradingHours
	}
	if err := ValidateOrderType(wh, sig.OrderType); err != nil {
		return err
	}
	return nil
}

// NormalizeEntryOrderType returns MARKET, LIMIT, or "" for unknown/empty input.
func NormalizeEntryOrderType(v string) string {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "MARKET":
		return "MARKET"
	case "LIMIT":
		return "LIMIT"
	default:
		return ""
	}
}

// WebhookOrderType returns the webhook's configured entry order type (MARKET or
// LIMIT). Empty/unset default_order_type is treated as LIMIT — the same default
// used by ValidateOrderType and live/paper execution.
func WebhookOrderType(wh *store.Webhook) string {
	if wh != nil {
		if ot := NormalizeEntryOrderType(wh.DefaultOrderType); ot != "" {
			return ot
		}
	}
	return "LIMIT"
}

// ValidateOrderType enforces that a signal's optional order_type field
// matches the webhook's configured default_order_type — a webhook set to
// LIMIT only ever places LIMIT orders, one set to MARKET only ever places
// MARKET orders. The actual order type used always comes from
// WebhookOrderType (maporder.Build, queue/paper.go); this check exists
// purely so a mismatched order_type in the signal is rejected with a clear
// reason instead of being silently ignored and overridden. An empty/absent
// order_type is fine — it just means the signal didn't specify one.
func ValidateOrderType(wh *store.Webhook, signalOrderType string) error {
	requested := strings.ToUpper(strings.TrimSpace(signalOrderType))
	if requested == "" {
		return nil
	}
	if requested != "MARKET" && requested != "LIMIT" {
		return fmt.Errorf("%w — order_type must be MARKET or LIMIT, got %q", ErrOrderTypeNotAllowed, signalOrderType)
	}
	configured := WebhookOrderType(wh)
	if requested != configured {
		return fmt.Errorf("%w — signal requested %s but this webhook is configured for %s", ErrOrderTypeNotAllowed, requested, configured)
	}
	return nil
}

// ValidateAction checks the signal action against the webhook allowed_actions list.
func ValidateAction(wh *store.Webhook, action string) error {
	actions := wh.AllowedActions
	if len(actions) == 0 {
		actions = DefaultAllowedActions()
	}
	for _, a := range actions {
		if a == action {
			return nil
		}
	}
	return ErrActionNotAllowed
}

// ApplyMaxLot caps or rejects lot sizes above max_lot_size. Zero max = no cap.
func ApplyMaxLot(wh *store.Webhook, lot float64) (float64, error) {
	return ApplyMaxLotSize(wh.MaxLotSize, lot)
}

// ApplyMaxLotSize is ApplyMaxLot's underlying check, taking the cap directly
// instead of a *store.Webhook — used by queue.liveBrokerDestination, which
// only has the cap value (via domain.ExecutionRequest.MaxLotSize), not a
// webhook object.
func ApplyMaxLotSize(maxLotSize, lot float64) (float64, error) {
	if maxLotSize <= 0 {
		return lot, nil
	}
	if lot > maxLotSize {
		return 0, fmt.Errorf("%w (%.4f > %.4f)", ErrLotExceedsMax, lot, maxLotSize)
	}
	return lot, nil
}

// WithinTradingHours returns true when trading_hours is unset (24/7) or now falls in a window.
func WithinTradingHours(wh *store.Webhook, now time.Time) bool {
	return WithinSchedule(wh.TradingHours, wh.Timezone, now)
}

// WithinSchedule is the reusable core of WithinTradingHours — returns true
// when schedule is unset (24/7) or now falls within one of its windows.
// Used by webhook trading-hours (per-webhook, live+paper) and by paper
// trading's market_profiles-based market-hours enforcement (Phase 7).
func WithinSchedule(schedule *store.TradingSchedule, timezone string, now time.Time) bool {
	if schedule == nil || len(*schedule) == 0 {
		return true
	}

	tz := timezone
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	local := now.In(loc)
	dayKey := weekdayKeys[int(local.Weekday())]

	windows, ok := (*schedule)[dayKey]
	if !ok || len(windows) == 0 {
		return false
	}

	nowMins := local.Hour()*60 + local.Minute()
	for _, w := range windows {
		start, ok1 := parseHHMM(w.Start)
		end, ok2 := parseHHMM(w.End)
		if !ok1 || !ok2 {
			continue
		}
		if start <= end {
			if nowMins >= start && nowMins <= end {
				return true
			}
		} else {
			// overnight window e.g. 22:00–02:00
			if nowMins >= start || nowMins <= end {
				return true
			}
		}
	}
	return false
}

func parseHHMM(s string) (int, bool) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, false
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil || h < 0 || h > 23 {
		return 0, false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}
