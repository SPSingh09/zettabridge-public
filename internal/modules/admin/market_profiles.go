package admin

import (
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

// ── Market profiles (Phase 7) ─────────────────────────────────────────────────

// AdminListMarketProfiles returns every market profile, active first.
func (h *Handler) AdminListMarketProfiles(c *fiber.Ctx) error {
	profiles, err := h.PG.ListMarketProfiles(c.Context())
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list market profiles")
	}
	if profiles == nil {
		profiles = []*store.MarketProfile{}
	}
	return shared.Ok(c, profiles)
}

// AdminUpdateMarketProfile patches an existing market profile by code.
// Market profiles are curated/pre-configured (added via migration, not
// created ad hoc by admins — there is deliberately no AdminCreateMarketProfile).
// Only base_currency, timezone, active, slippage_bps, and fee_bps are
// editable here — name and code are the profile's stable identity and are
// never accepted from this body.
func (h *Handler) AdminUpdateMarketProfile(c *fiber.Ctx) error {
	code := c.Params("code")
	existing, err := h.PG.GetMarketProfileByCode(c.Context(), code)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if existing == nil {
		return shared.Fail(c, fiber.StatusNotFound, "market profile not found")
	}

	var body struct {
		BaseCurrency *string  `json:"base_currency"`
		Timezone     *string  `json:"timezone"`
		Active       *bool    `json:"active"`
		SlippageBps  *float64 `json:"slippage_bps"`
		FeeBps       *float64 `json:"fee_bps"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	if body.BaseCurrency != nil {
		bc := strings.TrimSpace(*body.BaseCurrency)
		if bc == "" {
			return shared.Fail(c, fiber.StatusBadRequest, "base_currency cannot be empty")
		}
		existing.BaseCurrency = bc
	}
	if body.Timezone != nil {
		tz := strings.TrimSpace(*body.Timezone)
		if _, err := time.LoadLocation(tz); err != nil {
			return shared.Fail(c, fiber.StatusBadRequest, "invalid timezone: "+tz)
		}
		existing.Timezone = tz
	}
	if body.Active != nil {
		existing.Active = *body.Active
	}
	if body.SlippageBps != nil {
		if *body.SlippageBps < 0 || *body.SlippageBps > 1000 {
			return shared.Fail(c, fiber.StatusBadRequest, "slippage_bps must be between 0 and 1000")
		}
		existing.SlippageBps = *body.SlippageBps
	}
	if body.FeeBps != nil {
		if *body.FeeBps < 0 || *body.FeeBps > 1000 {
			return shared.Fail(c, fiber.StatusBadRequest, "fee_bps must be between 0 and 1000")
		}
		existing.FeeBps = *body.FeeBps
	}

	if err := h.PG.UpdateMarketProfile(c.Context(), existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return shared.Fail(c, fiber.StatusNotFound, "market profile not found")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update market profile")
	}
	h.LogUserAudit(c, middleware.UserID(c), "market_profile_updated", map[string]any{"code": code})
	return shared.Ok(c, existing)
}

// ── Instruments (Phase 7 symbol master) ───────────────────────────────────────

// AdminListInstruments returns instruments, optionally filtered by
// ?market_profile=<code>.
func (h *Handler) AdminListInstruments(c *fiber.Ctx) error {
	profileCode := c.Query("market_profile")
	instruments, err := h.PG.ListInstruments(c.Context(), profileCode)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list instruments")
	}
	if instruments == nil {
		instruments = []*store.Instrument{}
	}
	return shared.Ok(c, instruments)
}

// AdminCreateInstrument adds a symbol to a market profile's instrument master.
func (h *Handler) AdminCreateInstrument(c *fiber.Ctx) error {
	var body struct {
		MarketProfileCode string  `json:"market_profile_code"`
		Exchange          string  `json:"exchange"`
		Symbol            string  `json:"symbol"`
		Name              string  `json:"name"`
		InstrumentType    string  `json:"instrument_type"`
		TickSize          float64 `json:"tick_size"`
		LotSize           float64 `json:"lot_size"`
		Active            *bool   `json:"active"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	profileCode := strings.TrimSpace(body.MarketProfileCode)
	exchange := strings.ToUpper(strings.TrimSpace(body.Exchange))
	symbol := strings.ToUpper(strings.TrimSpace(body.Symbol))
	name := strings.TrimSpace(body.Name)
	if profileCode == "" || exchange == "" || symbol == "" || name == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "market_profile_code, exchange, symbol, and name are required")
	}
	profile, err := h.PG.GetMarketProfileByCode(c.Context(), profileCode)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if profile == nil {
		return shared.Fail(c, fiber.StatusBadRequest, "unknown market_profile_code: "+profileCode)
	}
	instrumentType := strings.ToUpper(strings.TrimSpace(body.InstrumentType))
	if instrumentType == "" {
		instrumentType = "EQUITY"
	}
	switch instrumentType {
	case "EQUITY", "INDEX", "FUTURES", "OPTIONS":
	default:
		return shared.Fail(c, fiber.StatusBadRequest, "instrument_type must be one of: EQUITY, INDEX, FUTURES, OPTIONS")
	}
	tickSize := body.TickSize
	if tickSize <= 0 {
		tickSize = 0.05
	}
	lotSize := body.LotSize
	if lotSize <= 0 {
		lotSize = 1
	}
	active := true
	if body.Active != nil {
		active = *body.Active
	}

	now := time.Now()
	i := &store.Instrument{
		ID: uuid.New().String(), MarketProfileCode: profileCode, Exchange: exchange, Symbol: symbol,
		Name: name, InstrumentType: instrumentType, TickSize: tickSize, LotSize: lotSize, Active: active,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := h.PG.CreateInstrument(c.Context(), i); err != nil {
		if isUniqueViolation(err) {
			return shared.Fail(c, fiber.StatusConflict, "this symbol already exists for this market profile/exchange")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not create instrument")
	}
	h.LogUserAudit(c, middleware.UserID(c), "instrument_created", map[string]any{"symbol": symbol, "market_profile_code": profileCode})

	// Auto-resolve any outstanding symbol requests this instrument satisfies
	// — regardless of whether the admin got here via the pending-requests
	// banner's "Accept" action or added the symbol independently. This is
	// the single source of truth for a request reaching "resolved".
	resolved, rerr := h.PG.ResolveSymbolRequestsByInstrument(c.Context(), profileCode, symbol, i.ID)
	if rerr != nil {
		log.Printf("symbol_request: auto-resolve failed profile=%s symbol=%s: %v", profileCode, symbol, rerr)
	} else {
		for _, r := range resolved {
			notifyUserSymbolRequestStatus(c.Context(), h, r.UserID, r.Symbol, "resolved", "")
		}
	}

	return shared.Created(c, i)
}

// AdminUpdateInstrument patches an instrument's editable fields by ID.
func (h *Handler) AdminUpdateInstrument(c *fiber.Ctx) error {
	id := c.Params("id")

	var body struct {
		Name           *string  `json:"name"`
		InstrumentType *string  `json:"instrument_type"`
		TickSize       *float64 `json:"tick_size"`
		LotSize        *float64 `json:"lot_size"`
		Active         *bool    `json:"active"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}

	existing, err := h.PG.GetInstrumentByID(c.Context(), id)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if existing == nil {
		return shared.Fail(c, fiber.StatusNotFound, "instrument not found")
	}

	if body.Name != nil {
		existing.Name = strings.TrimSpace(*body.Name)
	}
	if body.InstrumentType != nil {
		t := strings.ToUpper(strings.TrimSpace(*body.InstrumentType))
		switch t {
		case "EQUITY", "INDEX", "FUTURES", "OPTIONS":
		default:
			return shared.Fail(c, fiber.StatusBadRequest, "instrument_type must be one of: EQUITY, INDEX, FUTURES, OPTIONS")
		}
		existing.InstrumentType = t
	}
	if body.TickSize != nil {
		if *body.TickSize <= 0 {
			return shared.Fail(c, fiber.StatusBadRequest, "tick_size must be greater than zero")
		}
		existing.TickSize = *body.TickSize
	}
	if body.LotSize != nil {
		if *body.LotSize <= 0 {
			return shared.Fail(c, fiber.StatusBadRequest, "lot_size must be greater than zero")
		}
		existing.LotSize = *body.LotSize
	}
	if body.Active != nil {
		existing.Active = *body.Active
	}

	if err := h.PG.UpdateInstrument(c.Context(), existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return shared.Fail(c, fiber.StatusNotFound, "instrument not found")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update instrument")
	}
	h.LogUserAudit(c, middleware.UserID(c), "instrument_updated", map[string]any{"id": id})
	return shared.Ok(c, existing)
}

// AdminDeleteInstrument permanently removes a symbol from the instrument
// master. Prefer deactivating (AdminUpdateInstrument with active:false) for
// a symbol that has ever been traded, since paper_orders/paper_trades don't
// reference instruments by ID — deleting doesn't touch trade history, but
// it does drop tick/lot-size context for any past order in the audit trail.
func (h *Handler) AdminDeleteInstrument(c *fiber.Ctx) error {
	id := c.Params("id")
	existing, err := h.PG.GetInstrumentByID(c.Context(), id)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if existing == nil {
		return shared.Fail(c, fiber.StatusNotFound, "instrument not found")
	}
	if err := h.PG.DeleteInstrument(c.Context(), id); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not delete instrument")
	}
	h.LogUserAudit(c, middleware.UserID(c), "instrument_deleted", map[string]any{"id": id, "symbol": existing.Symbol})
	return c.SendStatus(fiber.StatusNoContent)
}

// ── Market hours enforcement toggle (Phase 7) ─────────────────────────────────

// AdminGetMarketHoursEnforcement reports whether Paper Trading orders are
// rejected outside their market profile's trading hours. Defaults to true
// when the platform_settings row doesn't exist yet.
func (h *Handler) AdminGetMarketHoursEnforcement(c *fiber.Ctx) error {
	v, ok, err := h.PG.GetPlatformSetting(c.Context(), guard.MarketHoursEnforcementSettingKey)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load setting")
	}
	enabled := true
	if ok {
		enabled = v == "true"
	}
	return shared.Ok(c, fiber.Map{"enabled": enabled})
}

// AdminSetMarketHoursEnforcement toggles market-hours enforcement at runtime
// — the paper order path re-reads this on every signal, no restart needed.
func (h *Handler) AdminSetMarketHoursEnforcement(c *fiber.Ctx) error {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil || body.Enabled == nil {
		return shared.Fail(c, fiber.StatusBadRequest, "enabled (boolean) is required")
	}
	value := "false"
	if *body.Enabled {
		value = "true"
	}
	adminID := middleware.UserID(c)
	if err := h.PG.SetPlatformSetting(c.Context(), guard.MarketHoursEnforcementSettingKey, value, adminID); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update setting")
	}
	h.LogUserAudit(c, adminID, "market_hours_enforcement_changed", map[string]any{"enabled": *body.Enabled})
	return shared.Ok(c, fiber.Map{"enabled": *body.Enabled})
}

// isUniqueViolation reports whether err looks like a Postgres unique
// constraint violation (pq error code 23505). Matched by message substring
// to avoid importing the pq driver package just for this.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}
