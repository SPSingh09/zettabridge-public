// Package paperaccounts is the HTTP API for ZettaBridge's broker-free Paper
// Trading accounts (see internal/paperengine). Solo-only for now — paper
// accounts have no org_id column, unlike broker credentials.
package paperaccounts

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/marketdata"
	"github.com/SPSingh09/zettabridge/internal/modules/pnl"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/paperengine"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

// Handler handles paper trading account CRUD.
type Handler struct {
	*shared.Handler
	// PaperEngine executes CLOSE requests for ClosePaperPosition — a second
	// instance of the same stateless engine the queue worker uses for
	// webhook-triggered fills (internal/paperengine.New just wraps the store).
	PaperEngine domain.ExecutionDestination
	// MarketData reports the active LTP provider for the Diagnostics
	// section — the same registry instance the queue worker uses to price
	// paper MARKET orders (internal/handler/handler.go wires it via
	// queue.Queue.MarketDataRegistry()). Nil-safe: ActiveProviderName
	// returns "" on a nil receiver.
	MarketData *marketdata.Registry
}

func isValidExchange(v string) bool {
	return v == "NSE" || v == "BSE"
}

func isValidProduct(v string) bool {
	return v == "MIS" || v == "CNC" || v == "NRML"
}

// ListActiveMarketProfiles returns market profiles available for paper
// account creation (Phase 7). Any authenticated user can call this — the
// admin-only CRUD for managing profiles lives in the admin module.
func (h *Handler) ListActiveMarketProfiles(c *fiber.Ctx) error {
	profiles, err := h.PG.ListMarketProfiles(c.Context())
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list market profiles")
	}
	active := make([]*store.MarketProfile, 0, len(profiles))
	for _, p := range profiles {
		if p.Active {
			active = append(active, p)
		}
	}
	return shared.Ok(c, active)
}

// ListPaperAccounts returns the caller's paper accounts.
func (h *Handler) ListPaperAccounts(c *fiber.Ctx) error {
	userID := middleware.UserID(c)
	accounts, err := h.PG.ListPaperAccountsByUser(c.Context(), userID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list paper accounts")
	}
	if accounts == nil {
		accounts = []*store.PaperAccount{}
	}
	return shared.Ok(c, accounts)
}

// CreatePaperAccount creates a new paper trading account for the caller.
// Only Indian Equity / INR is supported today (per DEMO_EXECUTION_PLAN.md MVP scope).
func (h *Handler) CreatePaperAccount(c *fiber.Ctx) error {
	userID := middleware.UserID(c)

	var body struct {
		Label           string  `json:"label"`
		StartingBalance float64 `json:"starting_balance"`
		Exchange        string  `json:"exchange"`
		DefaultProduct  string  `json:"default_product"`
		MarketProfile   string  `json:"market_profile"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}

	if body.StartingBalance <= 0 {
		return shared.Fail(c, fiber.StatusBadRequest, "starting_balance must be greater than zero")
	}
	if body.Exchange == "" {
		body.Exchange = "NSE"
	}
	if !isValidExchange(body.Exchange) {
		return shared.Fail(c, fiber.StatusBadRequest, "exchange must be NSE or BSE")
	}
	if body.DefaultProduct == "" {
		body.DefaultProduct = "MIS"
	}
	if !isValidProduct(body.DefaultProduct) {
		return shared.Fail(c, fiber.StatusBadRequest, "default_product must be MIS, CNC, or NRML")
	}
	profileCode := strings.TrimSpace(body.MarketProfile)
	if profileCode == "" {
		profileCode = "indian_equity"
	}
	profile, err := h.PG.GetMarketProfileByCode(c.Context(), profileCode)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "market profile lookup failed")
	}
	if profile == nil || !profile.Active {
		return shared.Fail(c, fiber.StatusBadRequest, "unknown or inactive market_profile: "+profileCode)
	}

	user, err := h.PG.GetUserByID(c.Context(), userID)
	if err != nil || user == nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user")
	}

	count, err := h.PG.CountPaperAccountsByUser(c.Context(), userID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not count paper accounts")
	}
	if err := plan.CanAddPaperAccount(plan.EffectivePlan(user.Plan), count); err != nil {
		return shared.PlanError(c, err)
	}

	now := time.Now()
	account := &store.PaperAccount{
		ID:              uuid.New().String(),
		UserID:          userID,
		Label:           strings.TrimSpace(body.Label),
		MarketProfile:   profile.Code,
		BaseCurrency:    profile.BaseCurrency,
		StartingBalance: body.StartingBalance,
		CashBalance:     body.StartingBalance,
		Exchange:        body.Exchange,
		DefaultProduct:  body.DefaultProduct,
		Status:          "active",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := h.PG.CreatePaperAccount(c.Context(), account); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not save paper account")
	}
	h.LogUserAudit(c, userID, "paper_account_created", map[string]any{"label": account.Label})
	return shared.Created(c, account)
}

// DeletePaperAccount removes a paper account owned by the caller.
func (h *Handler) DeletePaperAccount(c *fiber.Ctx) error {
	userID := middleware.UserID(c)
	id := c.Params("id")

	// Block deletion while a webhook still routes to this account — otherwise
	// the paper_account_id FK's ON DELETE SET NULL would leave a webhook with
	// no destination at all, tripping webhooks_one_destination_check and
	// surfacing as an opaque 500. Mirrors DeleteCredential's same check.
	whCount, err := h.PG.CountWebhooksByPaperAccount(c.Context(), id)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not check webhook usage")
	}
	if whCount > 0 {
		return shared.Fail(c, fiber.StatusConflict, "paper account is in use by one or more webhooks")
	}

	if err := h.PG.DeletePaperAccount(c.Context(), id, userID); err != nil {
		if err == sql.ErrNoRows {
			return shared.Fail(c, fiber.StatusNotFound, "paper account not found")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not delete paper account")
	}
	h.LogUserAudit(c, userID, "paper_account_deleted", map[string]any{"id": id})
	return c.SendStatus(fiber.StatusNoContent)
}

// UpdatePaperAccount patches the caller's own paper account. `label` and
// `default_product` are always editable. `starting_balance` is only
// editable while the account is still flat (cash_balance == starting_balance,
// i.e. no fill has ever moved cash) — changing it after that would silently
// invalidate the account's own P&L math (Equity = StartingBalance +
// RealizedPnL + UnrealizedPnL), so it's rejected with a clear reason instead
// of guessed at. `market_profile` and `exchange` are intentionally not
// editable here — they're the account's asset-class identity referenced by
// every instrument lookup; changing them mid-life silently reinterprets what
// the account trades going forward rather than corrupting past data, but
// that's still surprising enough to leave out of scope (start a new account
// instead).
func (h *Handler) UpdatePaperAccount(c *fiber.Ctx) error {
	userID := middleware.UserID(c)
	existing, err := h.requirePaperAccountAccess(c, c.Params("id"))
	if err != nil {
		return err
	}

	var body struct {
		Label           *string  `json:"label"`
		DefaultProduct  *string  `json:"default_product"`
		StartingBalance *float64 `json:"starting_balance"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}

	if body.Label != nil {
		existing.Label = strings.TrimSpace(*body.Label)
	}
	if body.DefaultProduct != nil {
		dp := strings.ToUpper(strings.TrimSpace(*body.DefaultProduct))
		if !isValidProduct(dp) {
			return shared.Fail(c, fiber.StatusBadRequest, "default_product must be MIS, CNC, or NRML")
		}
		existing.DefaultProduct = dp
	}
	if body.StartingBalance != nil {
		if existing.CashBalance != existing.StartingBalance {
			return shared.Fail(c, fiber.StatusConflict, "starting_balance can only be changed before the account has any fills")
		}
		if *body.StartingBalance <= 0 {
			return shared.Fail(c, fiber.StatusBadRequest, "starting_balance must be greater than zero")
		}
		existing.StartingBalance = *body.StartingBalance
		existing.CashBalance = *body.StartingBalance
	}

	if err := h.PG.UpdatePaperAccount(c.Context(), existing); err != nil {
		if err == sql.ErrNoRows {
			return shared.Fail(c, fiber.StatusNotFound, "paper account not found")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update paper account")
	}
	h.LogUserAudit(c, userID, "paper_account_updated", map[string]any{"id": existing.ID})
	return shared.Ok(c, existing)
}

// ResetPaperAccount wipes a paper account's positions/orders/trades/
// snapshots and resets cash_balance back to starting_balance — or to a new
// starting_balance, if provided, to top up an account the user "blew" rather
// than making them create a whole new one. This is the escape hatch for
// starting_balance being otherwise locked once any fill exists (see
// UpdatePaperAccount): instead of trying to edit the balance while leaving
// history around to go stale, reset deletes the history so there's nothing
// left to protect. Irreversible — the frontend should confirm before calling
// this the same way it confirms account deletion.
func (h *Handler) ResetPaperAccount(c *fiber.Ctx) error {
	userID := middleware.UserID(c)
	id := c.Params("id")
	if _, err := h.requirePaperAccountAccess(c, id); err != nil {
		return err
	}

	var body struct {
		StartingBalance *float64 `json:"starting_balance"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&body); err != nil {
			return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
		}
	}
	if body.StartingBalance != nil && *body.StartingBalance <= 0 {
		return shared.Fail(c, fiber.StatusBadRequest, "starting_balance must be greater than zero")
	}

	account, err := h.PG.ResetPaperAccount(c.Context(), id, userID, body.StartingBalance)
	if err != nil {
		if err == sql.ErrNoRows {
			return shared.Fail(c, fiber.StatusNotFound, "paper account not found")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not reset paper account")
	}
	h.LogUserAudit(c, userID, "paper_account_reset", map[string]any{"id": account.ID, "starting_balance": account.StartingBalance})
	return shared.Ok(c, account)
}

// ClosePaperPosition closes an open position at its last known mark price —
// a one-click "market close" from the Positions card, equivalent to sending
// a CLOSE webhook signal but synchronous and requiring no webhook. Side and
// quantity are computed by the paper engine itself from the existing
// position (see paperengine.resolveCloseSide); the caller only names the symbol.
func (h *Handler) ClosePaperPosition(c *fiber.Ctx) error {
	userID := middleware.UserID(c)
	account, err := h.requirePaperAccountAccess(c, c.Params("id"))
	if err != nil {
		return err
	}
	if account.Status != "active" {
		return shared.Fail(c, fiber.StatusConflict, fmt.Sprintf("paper account is %s", account.Status))
	}

	var body struct {
		Symbol string `json:"symbol"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	symbol := strings.ToUpper(strings.TrimSpace(body.Symbol))
	if symbol == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "symbol is required")
	}

	position, err := h.PG.GetPaperPosition(c.Context(), account.ID, account.Exchange, symbol, account.DefaultProduct)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "position lookup failed")
	}
	if position == nil || position.Quantity == 0 {
		return shared.Fail(c, fiber.StatusConflict, fmt.Sprintf("no open position for %q", symbol))
	}

	result, err := paperengine.FlattenPosition(
		c.Context(), h.PaperEngine, h.MarketData, h.PG, account, position, uuid.New().String(), true,
	)
	if err != nil {
		return shared.Fail(c, fiber.StatusConflict, err.Error())
	}
	metrics.RecordTrade(result.Status, "", "paper")
	if result.Status != string(domain.PaperOrderStatusFilled) {
		return shared.Fail(c, fiber.StatusConflict, result.Reason)
	}

	h.LogUserAudit(c, userID, "paper_position_closed", map[string]any{"account_id": account.ID, "symbol": symbol})
	return shared.Ok(c, fiber.Map{
		"status":       result.Status,
		"fill_price":   result.FillPrice,
		"realized_pnl": result.RealizedPnL,
	})
}

// PausePaperAccount stops the account from accepting any further paper
// signals (both ORDER_SIGNAL and PRICE_UPDATE — see the account.Status
// checks in internal/platform/queue/paper.go and
// internal/modules/signals/paper_ingest.go) until resumed. Existing
// positions/orders/trades are untouched, and the Phase 6 mark-to-market
// sweep keeps marking open positions regardless of pause state — pausing
// only blocks new trading activity, it isn't a freeze of displayed P&L.
func (h *Handler) PausePaperAccount(c *fiber.Ctx) error {
	return h.SetPaperAccountStatus(c, "paused")
}

// ResumePaperAccount reverses PausePaperAccount.
func (h *Handler) ResumePaperAccount(c *fiber.Ctx) error {
	return h.SetPaperAccountStatus(c, "active")
}

func (h *Handler) SetPaperAccountStatus(c *fiber.Ctx, status string) error {
	userID := middleware.UserID(c)
	existing, err := h.requirePaperAccountAccess(c, c.Params("id"))
	if err != nil {
		return err
	}
	if err := h.PG.UpdatePaperAccountStatus(c.Context(), existing.ID, userID, status); err != nil {
		if err == sql.ErrNoRows {
			return shared.Fail(c, fiber.StatusNotFound, "paper account not found")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update paper account status")
	}
	h.LogUserAudit(c, userID, "paper_account_"+status, map[string]any{"id": existing.ID})
	return shared.Ok(c, fiber.Map{"status": status})
}

// requirePaperAccountAccess loads a paper account and verifies it belongs to
// the caller, writing the HTTP response and returning a non-nil error if not.
// Shared by every Phase 4 read endpoint below.
func (h *Handler) requirePaperAccountAccess(c *fiber.Ctx, id string) (*store.PaperAccount, error) {
	account, err := h.PG.GetPaperAccount(c.Context(), id)
	if err != nil {
		return nil, shared.Fail(c, fiber.StatusInternalServerError, "paper account lookup failed")
	}
	if account == nil || account.UserID != middleware.UserID(c) {
		return nil, shared.Fail(c, fiber.StatusNotFound, "paper account not found")
	}
	return account, nil
}

// GetPaperAccount returns a single paper account owned by the caller.
func (h *Handler) GetPaperAccount(c *fiber.Ctx) error {
	account, err := h.requirePaperAccountAccess(c, c.Params("id"))
	if err != nil {
		return err
	}
	return shared.Ok(c, account)
}

// ListPaperPositions returns the open/closed positions for a paper account.
func (h *Handler) ListPaperPositions(c *fiber.Ctx) error {
	account, err := h.requirePaperAccountAccess(c, c.Params("id"))
	if err != nil {
		return err
	}
	positions, listErr := h.PG.ListPaperPositionsByAccount(c.Context(), account.ID)
	if listErr != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list paper positions")
	}
	if positions == nil {
		positions = []*store.PaperPosition{}
	}
	return shared.Ok(c, positions)
}

// ListPaperOrders returns the order/trade history for a paper account,
// including rejections, for the dashboard's "Trades" table.
func (h *Handler) ListPaperOrders(c *fiber.Ctx) error {
	account, err := h.requirePaperAccountAccess(c, c.Params("id"))
	if err != nil {
		return err
	}
	limit := c.QueryInt("limit", 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := c.QueryInt("offset", 0)
	if offset < 0 {
		offset = 0
	}
	orders, listErr := h.PG.ListPaperOrdersWithPnLByAccount(c.Context(), account.ID, limit, offset)
	if listErr != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list paper orders")
	}
	if orders == nil {
		orders = []*store.PaperOrderWithPnL{}
	}
	total, countErr := h.PG.CountPaperOrdersByAccount(c.Context(), account.ID)
	if countErr != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not count paper orders")
	}
	return shared.Ok(c, fiber.Map{"orders": orders, "total": total})
}

// ExportPaperOrders exports a paper account's order/trade history as CSV (or
// JSON via ?format=json), mirroring pnl.Handler.ExportTrades's shape. No plan
// gating — Paper Trading is plan-independent everywhere, including export.
func (h *Handler) ExportPaperOrders(c *fiber.Ctx) error {
	account, err := h.requirePaperAccountAccess(c, c.Params("id"))
	if err != nil {
		return err
	}

	from, to, err := pnl.ParseDateRange(c)
	if err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, err.Error())
	}

	orders, err := h.PG.ListPaperOrdersForExport(c.Context(), account.ID, from, to)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not fetch paper orders")
	}

	if c.Query("format", "csv") == "json" {
		if orders == nil {
			orders = []*store.PaperOrderWithPnL{}
		}
		return shared.Ok(c, orders)
	}

	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="paper-trades-%s.csv"`, account.ID))

	w := csv.NewWriter(c.Response().BodyWriter())
	_ = w.Write([]string{
		"id", "symbol", "exchange", "side", "order_type", "product",
		"quantity", "requested_price", "fill_price", "status", "reason",
		"realized_pnl", "created_at",
	})
	for _, o := range orders {
		requestedPrice, fillPrice, realizedPnL := "", "", ""
		if o.RequestedPrice != nil {
			requestedPrice = strconv.FormatFloat(*o.RequestedPrice, 'f', -1, 64)
		}
		if o.FillPrice != nil {
			fillPrice = strconv.FormatFloat(*o.FillPrice, 'f', -1, 64)
		}
		if o.RealizedPnL != nil {
			realizedPnL = strconv.FormatFloat(*o.RealizedPnL, 'f', -1, 64)
		}
		_ = w.Write([]string{
			o.ID, o.Symbol, o.Exchange, o.Side, o.OrderType, o.Product,
			strconv.FormatFloat(o.Quantity, 'f', -1, 64),
			requestedPrice, fillPrice, o.Status, o.Reason, realizedPnL,
			o.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	w.Flush()
	return nil
}

// GetPaperAccountPnL returns the computed dashboard metrics for a paper
// account. Beyond the self-contained store aggregation, this also merges in
// Diagnostics (webhook signal stats — needs the list of webhooks routed to
// this account) and Market info (profile name/session/data provider) —
// both need dependencies (guard, the webhook list, *marketdata.Registry)
// that internal/store can't import without a cycle.
func (h *Handler) GetPaperAccountPnL(c *fiber.Ctx) error {
	account, err := h.requirePaperAccountAccess(c, c.Params("id"))
	if err != nil {
		return err
	}
	summary, pnlErr := h.PG.GetPaperAccountPnL(c.Context(), account.ID)
	if pnlErr != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not compute paper account pnl")
	}

	if webhooks, err := h.PG.ListWebhooksByPaperAccount(c.Context(), account.ID); err == nil {
		ids := make([]string, len(webhooks))
		for i, wh := range webhooks {
			ids[i] = wh.ID
			summary.LinkedWebhooks = append(summary.LinkedWebhooks, store.PaperLinkedWebhook{
				ID: wh.ID, Label: wh.Label, Status: wh.Status,
			})
		}
		if diag, err := h.PG.GetPaperAccountWebhookDiagnostics(c.Context(), ids); err == nil {
			summary.SignalsReceived = diag.SignalsReceived
			summary.SignalsAccepted = diag.SignalsAccepted
			summary.SignalsRejected = diag.SignalsRejected
			summary.DuplicateSuppressed = diag.DuplicateSuppressed
			summary.WebhookFillRate = diag.FillRate
			summary.LastWebhookSignalAt = diag.LastSignalAt
		}
	}

	if profile, err := h.PG.GetMarketProfileByCode(c.Context(), account.MarketProfile); err == nil && profile != nil {
		summary.MarketProfileName = profile.Name
		if guard.WithinSchedule(profile.TradingDays, profile.Timezone, time.Now()) {
			summary.TradingSession = "open"
		} else {
			summary.TradingSession = "closed"
		}
	}
	if h.MarketData != nil {
		summary.MarketDataProvider = h.MarketData.ActiveProviderName(c.Context(), h.PG)
	}

	return shared.Ok(c, summary)
}

// ListPaperAccountSnapshots returns the equity/P&L history written by the
// Phase 6 mark-to-market background job, newest first.
func (h *Handler) ListPaperAccountSnapshots(c *fiber.Ctx) error {
	account, err := h.requirePaperAccountAccess(c, c.Params("id"))
	if err != nil {
		return err
	}
	limit := c.QueryInt("limit", 100)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	snapshots, listErr := h.PG.ListPaperAccountSnapshots(c.Context(), account.ID, limit)
	if listErr != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list paper account snapshots")
	}
	if snapshots == nil {
		snapshots = []*store.PaperAccountSnapshot{}
	}
	return shared.Ok(c, snapshots)
}
