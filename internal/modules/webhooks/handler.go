package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	zerodhasettings "github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/settings"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/platform/crypto"
	"github.com/SPSingh09/zettabridge/internal/platform/security/webhooktoken"
	"github.com/SPSingh09/zettabridge/internal/risk"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

// Handler handles webhook CRUD and trade management.
type Handler struct {
	*shared.Handler
	Infra *livebrokers.Infra
}

// ── Webhook guard settings ────────────────────────────────────────────────────

type webhookGuardInput struct {
	AllowedActions  []string
	AllowedSymbols  []string
	MaxLotSize      *float64
	RateLimitPerSec *int
	RateLimitPerMin *int
	DedupWindowSec  *int
	Timezone        *string
	TradingHours    *store.TradingSchedule
	RequiredComment *string
}

func (in webhookGuardInput) usesAdvancedGuards() bool {
	if in.RateLimitPerSec != nil && *in.RateLimitPerSec > 0 {
		return true
	}
	if in.DedupWindowSec != nil && *in.DedupWindowSec > 0 {
		return true
	}
	if in.TradingHours != nil && len(*in.TradingHours) > 0 {
		return true
	}
	if in.Timezone != nil && strings.TrimSpace(*in.Timezone) != "" && *in.Timezone != "UTC" {
		return true
	}
	return false
}

// ApplyGuardDefaults sets default guard values on a webhook.
func ApplyGuardDefaults(wh *store.Webhook) {
	wh.AllowedActions = guard.DefaultAllowedActions()
	wh.AllowedSymbols = []string{}
	wh.RateLimitPerMin = 60
	wh.DedupWindowSec = 2
	wh.Timezone = "UTC"
	wh.TradingHours = nil
}

// resolvePaperDestination looks up and validates a paper account for use as a
// webhook's execution destination, writing the HTTP response and returning a
// non-nil error if it can't be used. Shared by CreateWebhook and
// UpdateWebhook, which both need identical validation.
func (h *Handler) resolvePaperDestination(c *fiber.Ctx, scope store.ResourceScope, paperAccountID string) (*store.PaperAccount, error) {
	pa, err := h.PG.GetPaperAccount(c.Context(), paperAccountID)
	if err != nil {
		return nil, shared.Fail(c, fiber.StatusInternalServerError, "paper account lookup failed")
	}
	if !shared.PaperAccountUsableForWebhook(scope, pa) {
		return nil, shared.Fail(c, fiber.StatusNotFound, "paper account not found")
	}
	return pa, nil
}

// credentialPausedMessage returns a user-facing reason a webhook cannot be
// created or switched to use a paused credential — distinguishing the
// admin-disabled-execution-mode case from an ordinary manual pause. Shared
// by CreateWebhook and UpdateWebhook.
func credentialPausedMessage(cred *store.BrokerCredential) string {
	if cred.AdminDisabled {
		modeLabel := "OAuth"
		if cred.ExecutionMode == "publisher" {
			modeLabel = "Kite Publisher"
		}
		return fmt.Sprintf("Zerodha %s is currently disabled by the ZettaBridge admin — this credential cannot be used for a webhook until it is re-enabled", modeLabel)
	}
	return "this credential is paused; resume it before using it for a webhook"
}

// validatePublisherAllowedActions returns an error if any of the requested
// allowed_actions include CLOSE, which is unsupported in Kite Publisher mode.
func validatePublisherAllowedActions(actions []string) error {
	for _, a := range actions {
		if strings.ToUpper(a) == "CLOSE" {
			return fmt.Errorf("CLOSE is not supported in Kite Publisher mode — allowed_actions may only include BUY and SELL")
		}
	}
	return nil
}

func normalizeWebhookOrderType(requested string) (string, error) {
	ot := strings.ToUpper(strings.TrimSpace(requested))
	if ot == "" {
		return "LIMIT", nil
	}
	if ot != "MARKET" && ot != "LIMIT" {
		return "", fmt.Errorf("default_order_type must be MARKET or LIMIT")
	}
	return ot, nil
}

func resolveWebhookOrderType(cred *store.BrokerCredential, requested string) (string, error) {
	if cred != nil && cred.ExecutionMode == "publisher" {
		return "LIMIT", nil
	}
	return normalizeWebhookOrderType(requested)
}

func (h *Handler) enforceWebhookOrderType(ctx context.Context, wh *store.Webhook) error {
	var cred *store.BrokerCredential
	if wh.BrokerCredID != nil && *wh.BrokerCredID != "" {
		var err error
		cred, err = h.PG.GetBrokerCred(ctx, *wh.BrokerCredID)
		if err != nil {
			return err
		}
	}
	ot, err := resolveWebhookOrderType(cred, wh.DefaultOrderType)
	if err != nil {
		return err
	}
	wh.DefaultOrderType = ot
	return nil
}

// ApplyGuardInput applies guard input values to a webhook.
func ApplyGuardInput(wh *store.Webhook, in webhookGuardInput) error {
	if len(in.AllowedActions) > 0 {
		if err := guard.ValidateAllowedActions(in.AllowedActions); err != nil {
			return err
		}
		wh.AllowedActions = in.AllowedActions
	}
	if in.AllowedSymbols != nil {
		wh.AllowedSymbols = in.AllowedSymbols
	}
	if in.MaxLotSize != nil {
		if *in.MaxLotSize < 0 {
			return errInvalidMaxLotSize
		}
		wh.MaxLotSize = *in.MaxLotSize
	}
	if in.RateLimitPerSec != nil {
		if *in.RateLimitPerSec < 0 {
			return errInvalidRateLimit
		}
		wh.RateLimitPerSec = *in.RateLimitPerSec
	}
	if in.RateLimitPerMin != nil {
		if *in.RateLimitPerMin < 0 {
			return errInvalidRateLimitPerMin
		}
		wh.RateLimitPerMin = *in.RateLimitPerMin
	}
	if in.DedupWindowSec != nil {
		if *in.DedupWindowSec < 0 || *in.DedupWindowSec > 60 {
			return errInvalidDedupWindow
		}
		wh.DedupWindowSec = *in.DedupWindowSec
	}
	if in.Timezone != nil {
		tz := strings.TrimSpace(*in.Timezone)
		if tz == "" {
			tz = "UTC"
		}
		if _, err := time.LoadLocation(tz); err != nil {
			return errInvalidTimezone
		}
		wh.Timezone = tz
	}
	if in.TradingHours != nil {
		if len(*in.TradingHours) == 0 {
			wh.TradingHours = nil
		} else {
			sched := *in.TradingHours
			wh.TradingHours = &sched
		}
	}
	if in.RequiredComment != nil {
		wh.RequiredComment = *in.RequiredComment
	}
	return nil
}

// EnforceAdvancedGuardsPlan checks whether the user's plan allows advanced
// guard features (rate limits, dedup window, trading hours).
// Paper trading webhooks are exempt — Paper Trading is its own flat tier,
// independent of the live-trading plan ladder, so none of its configuration
// is gated by plan (see DEMO_EXECUTION_PLAN.md's Business Rule correction).
func EnforceAdvancedGuardsPlan(userPlan string, in webhookGuardInput, isPaperDestination bool) error {
	if isPaperDestination {
		return nil
	}
	if !in.usesAdvancedGuards() {
		return nil
	}
	return plan.CanConfigureAdvancedWebhookGuards(userPlan)
}

var (
	errInvalidMaxLotSize      = planErr("max_lot_size must be >= 0")
	errInvalidRateLimit       = planErr("rate_limit_per_sec must be >= 0")
	errInvalidRateLimitPerMin = planErr("rate_limit_per_min must be >= 0")
	errInvalidDedupWindow     = planErr("dedup_window_sec must be between 0 and 60")
	errInvalidTimezone        = planErr("invalid timezone")
)

type planErr string

func (e planErr) Error() string { return string(e) }

// ── Immutable webhook fields ──────────────────────────────────────────────────

// immutableWebhookFields cannot be changed via PUT /webhooks/:id.
// token uses POST /webhooks/:id/rotate-token; status uses pause/resume endpoints.
var immutableWebhookFields = []string{"id", "user_id", "created_at", "token", "status"}

// RejectImmutableWebhookFields returns the field name and error if any immutable field is present.
func RejectImmutableWebhookFields(body []byte) (string, error) {
	if len(body) == 0 {
		return "", nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", err
	}
	for _, field := range immutableWebhookFields {
		if _, ok := raw[field]; ok {
			return field, fmt.Errorf("%s cannot be updated", field)
		}
	}
	return "", nil
}

// ── Webhook CRUD ──────────────────────────────────────────────────────────────

func (h *Handler) GetWebhook(c *fiber.Ctx) error {
	id := c.Params("id")
	wh, err := h.RequireWebhookAccess(c, id, false)
	if err != nil {
		return err
	}
	return shared.Ok(c, wh)
}

func (h *Handler) ListWebhooks(c *fiber.Ctx) error {
	scope, err := h.ResolveResourceScope(c)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "scope lookup failed")
	}
	whs, err := h.PG.ListWebhooksForScope(c.Context(), scope)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list webhooks")
	}
	if whs == nil {
		whs = []*store.Webhook{}
	}
	return shared.Ok(c, whs)
}

func (h *Handler) CreateWebhook(c *fiber.Ctx) error {
	scope, err := h.ResolveResourceScope(c)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "scope lookup failed")
	}
	if scope.InOrg && !domain.CanCreateWebhook(scope.OrgRole) {
		return shared.Fail(c, fiber.StatusForbidden, domain.ErrInsufficientRole.Error())
	}

	var body struct {
		Label            string                 `json:"label"`
		BrokerCredID     string                 `json:"broker_cred_id"`
		PaperAccountID   string                 `json:"paper_account_id"`
		Symbol           string                 `json:"symbol"`
		LotSize          float64                `json:"lot_size"`
		MaxRiskPct       float64                `json:"max_risk_pct"`
		SLPoints         int                    `json:"sl_points"`
		TPPoints         int                    `json:"tp_points"`
		DefaultOrderType string                 `json:"default_order_type"`
		AllowedActions   []string               `json:"allowed_actions"`
		AllowedSymbols   []string               `json:"allowed_symbols"`
		MaxLotSize       float64                `json:"max_lot_size"`
		RateLimitPerSec  *int                   `json:"rate_limit_per_sec"`
		RateLimitPerMin  *int                   `json:"rate_limit_per_min"`
		DedupWindowSec   *int                   `json:"dedup_window_sec"`
		Timezone         string                 `json:"timezone"`
		TradingHours     *store.TradingSchedule `json:"trading_hours"`
		RequiredComment  string                 `json:"required_comment"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	if body.BrokerCredID == "" && body.PaperAccountID == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "either broker_cred_id or paper_account_id is required")
	}
	if body.BrokerCredID != "" && body.PaperAccountID != "" {
		return shared.Fail(c, fiber.StatusBadRequest, "only one of broker_cred_id or paper_account_id may be specified")
	}

	user, err := h.PG.GetUserByID(c.Context(), scope.UserID)
	if err != nil || user == nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user")
	}

	maxRiskPct := risk.NormalizeMaxRiskPct(body.MaxRiskPct)
	if err := risk.ValidateMaxRiskPct(maxRiskPct); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, err.Error())
	}

	raw, tokenHash, err := webhooktoken.Generate()
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not generate token")
	}
	wh := &store.Webhook{
		ID:         uuid.New().String(),
		UserID:     scope.UserID,
		CreatedBy:  scope.UserID,
		TokenHash:  tokenHash,
		RawToken:   raw,
		Label:      body.Label,
		Status:     "active",
		LotSize:    body.LotSize,
		MaxRiskPct: maxRiskPct,
		SLPoints:   body.SLPoints,
		TPPoints:   body.TPPoints,
		CreatedAt:  time.Now().UTC(),
	}
	if scope.InOrg {
		wh.OrgID = shared.OrgIDPtr(scope.OrgID)
	}

	var cred *store.BrokerCredential
	if body.PaperAccountID != "" {
		if _, err := h.resolvePaperDestination(c, scope, body.PaperAccountID); err != nil {
			return err
		}
		// Paper and live webhooks have separate per-plan caps.
		if err := h.EnforceWebhookLimit(c, scope, user, true); err != nil {
			return shared.PlanError(c, err)
		}
		wh.PaperAccountID = &body.PaperAccountID
	} else {
		cred, err = h.PG.GetBrokerCred(c.Context(), body.BrokerCredID)
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "credential lookup failed")
		}
		if !shared.CredentialUsableForWebhook(scope, cred) {
			return shared.Fail(c, fiber.StatusNotFound, "credential not found")
		}
		if cred.Status == "paused" {
			return shared.Fail(c, fiber.StatusBadRequest, credentialPausedMessage(cred))
		}
		if err := h.EnforceWebhookLimit(c, scope, user, false); err != nil {
			return shared.PlanError(c, err)
		}
		wh.BrokerCredID = &body.BrokerCredID
	}

	userPlan, err := h.UserPlan(c, scope.UserID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user plan")
	}
	if ot, err := resolveWebhookOrderType(cred, body.DefaultOrderType); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, err.Error())
	} else {
		wh.DefaultOrderType = ot
	}
	allowedSymbols, err := guard.NormalizeAllowedSymbols(body.AllowedSymbols)
	if err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, err.Error())
	}
	wh.AllowedSymbols = allowedSymbols
	wh.Symbol = allowedSymbols[0]
	ApplyGuardDefaults(wh)
	wh.AllowedSymbols = allowedSymbols
	// Publisher webhooks default to BUY/SELL only; CLOSE handoffs are unsupported.
	if cred != nil && cred.ExecutionMode == "publisher" {
		wh.AllowedActions = []string{"BUY", "SELL"}
		if err := validatePublisherAllowedActions(body.AllowedActions); err != nil {
			return shared.Fail(c, fiber.StatusBadRequest, err.Error())
		}
	}
	guardIn := webhookGuardInput{
		AllowedActions: body.AllowedActions,
		AllowedSymbols: allowedSymbols,
		TradingHours:   body.TradingHours,
	}
	if body.MaxLotSize > 0 {
		v := body.MaxLotSize
		guardIn.MaxLotSize = &v
	}
	guardIn.RateLimitPerSec = body.RateLimitPerSec
	guardIn.RateLimitPerMin = body.RateLimitPerMin
	guardIn.DedupWindowSec = body.DedupWindowSec
	if tz := strings.TrimSpace(body.Timezone); tz != "" {
		guardIn.Timezone = &tz
	}
	if rc := strings.TrimSpace(body.RequiredComment); rc != "" {
		guardIn.RequiredComment = &rc
	}
	if err := EnforceAdvancedGuardsPlan(userPlan, guardIn, body.PaperAccountID != ""); err != nil {
		return shared.PlanError(c, err)
	}
	if err := ApplyGuardInput(wh, guardIn); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, err.Error())
	}

	if err := h.PG.CreateWebhook(c.Context(), wh); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not create webhook")
	}
	h.LogUserAudit(c, wh.UserID, "webhook_created", map[string]any{"webhook_id": wh.ID, "label": wh.Label})
	return shared.Created(c, wh)
}

func (h *Handler) UpdateWebhook(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	id := c.Params("id")

	existing, err := h.PG.GetWebhookByID(c.Context(), id)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if existing == nil || existing.UserID != uid {
		return shared.Fail(c, fiber.StatusNotFound, "webhook not found")
	}

	if field, err := RejectImmutableWebhookFields(c.Body()); err != nil {
		if field != "" {
			return shared.Fail(c, fiber.StatusBadRequest, err.Error())
		}
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}

	var body struct {
		Label            *string                `json:"label"`
		BrokerCredID     *string                `json:"broker_cred_id"`
		PaperAccountID   *string                `json:"paper_account_id"`
		Symbol           *string                `json:"symbol"`
		LotSize          *float64               `json:"lot_size"`
		MaxRiskPct       *float64               `json:"max_risk_pct"`
		SLPoints         *int                   `json:"sl_points"`
		TPPoints         *int                   `json:"tp_points"`
		DefaultOrderType *string                `json:"default_order_type"`
		AllowedActions   []string               `json:"allowed_actions"`
		AllowedSymbols   []string               `json:"allowed_symbols"`
		MaxLotSize       *float64               `json:"max_lot_size"`
		RateLimitPerSec  *int                   `json:"rate_limit_per_sec"`
		RateLimitPerMin  *int                   `json:"rate_limit_per_min"`
		DedupWindowSec   *int                   `json:"dedup_window_sec"`
		Timezone         *string                `json:"timezone"`
		TradingHours     *store.TradingSchedule `json:"trading_hours"`
		RequiredComment  *string                `json:"required_comment"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	if body.Label == nil && body.BrokerCredID == nil && body.PaperAccountID == nil && body.Symbol == nil &&
		body.LotSize == nil && body.MaxRiskPct == nil && body.SLPoints == nil && body.TPPoints == nil &&
		body.DefaultOrderType == nil && body.AllowedActions == nil && body.AllowedSymbols == nil &&
		body.MaxLotSize == nil && body.RateLimitPerSec == nil && body.RateLimitPerMin == nil && body.DedupWindowSec == nil &&
		body.Timezone == nil && body.TradingHours == nil && body.RequiredComment == nil {
		return shared.Fail(c, fiber.StatusBadRequest, "at least one field required")
	}
	if body.BrokerCredID != nil && body.PaperAccountID != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "only one of broker_cred_id or paper_account_id may be specified")
	}

	userPlan, err := h.UserPlan(c, uid)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user plan")
	}
	user, err := h.PG.GetUserByID(c.Context(), uid)
	if err != nil || user == nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user")
	}

	guardIn := webhookGuardInput{
		AllowedActions:  body.AllowedActions,
		AllowedSymbols:  body.AllowedSymbols,
		MaxLotSize:      body.MaxLotSize,
		RateLimitPerSec: body.RateLimitPerSec,
		RateLimitPerMin: body.RateLimitPerMin,
		DedupWindowSec:  body.DedupWindowSec,
		Timezone:        body.Timezone,
		TradingHours:    body.TradingHours,
		RequiredComment: body.RequiredComment,
	}
	// Resolve the destination the request will end up with (it may be switching
	// this same call) so the guard-plan check below exempts paper webhooks correctly.
	isPaperDestination := existing.PaperAccountID != nil
	if body.BrokerCredID != nil {
		isPaperDestination = false
	} else if body.PaperAccountID != nil {
		isPaperDestination = true
	}
	if err := EnforceAdvancedGuardsPlan(userPlan, guardIn, isPaperDestination); err != nil {
		return shared.PlanError(c, err)
	}
	// Publisher credentials: reject if caller is trying to set CLOSE in allowed_actions.
	// Not applicable to paper destinations — there is no publisher-style credential to check.
	if len(body.AllowedActions) > 0 {
		var credID string
		switch {
		case body.BrokerCredID != nil && *body.BrokerCredID != "":
			credID = *body.BrokerCredID
		case body.BrokerCredID == nil && body.PaperAccountID == nil && existing.BrokerCredID != nil:
			credID = *existing.BrokerCredID
		}
		if credID != "" {
			if existingCred, err := h.PG.GetBrokerCred(c.Context(), credID); err == nil && existingCred != nil && existingCred.ExecutionMode == "publisher" {
				if err := validatePublisherAllowedActions(body.AllowedActions); err != nil {
					return shared.Fail(c, fiber.StatusBadRequest, err.Error())
				}
			}
		}
	}

	if body.Label != nil {
		existing.Label = *body.Label
	}
	if body.LotSize != nil {
		existing.LotSize = *body.LotSize
	}
	if body.MaxRiskPct != nil {
		if err := risk.ValidateMaxRiskPct(*body.MaxRiskPct); err != nil {
			return shared.Fail(c, fiber.StatusBadRequest, err.Error())
		}
		existing.MaxRiskPct = *body.MaxRiskPct
	}
	if body.SLPoints != nil {
		existing.SLPoints = *body.SLPoints
	}
	if body.TPPoints != nil {
		existing.TPPoints = *body.TPPoints
	}
	if body.DefaultOrderType != nil {
		ot, err := normalizeWebhookOrderType(*body.DefaultOrderType)
		if err != nil {
			return shared.Fail(c, fiber.StatusBadRequest, err.Error())
		}
		existing.DefaultOrderType = ot
	}
	if body.AllowedSymbols != nil {
		allowedSymbols, err := guard.NormalizeAllowedSymbols(body.AllowedSymbols)
		if err != nil {
			return shared.Fail(c, fiber.StatusBadRequest, err.Error())
		}
		existing.AllowedSymbols = allowedSymbols
		existing.Symbol = allowedSymbols[0]
	}
	if body.BrokerCredID != nil {
		if *body.BrokerCredID == "" {
			return shared.Fail(c, fiber.StatusBadRequest, "broker_cred_id cannot be empty")
		}
		cred, err := h.PG.GetBrokerCred(c.Context(), *body.BrokerCredID)
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "could not load broker credential")
		}
		if cred == nil || cred.UserID != uid {
			return shared.Fail(c, fiber.StatusNotFound, "broker credential not found")
		}
		if cred.Status == "paused" {
			return shared.Fail(c, fiber.StatusBadRequest, credentialPausedMessage(cred))
		}
		if err := plan.CanUseCredential(user.Plan, cred); err != nil {
			return shared.PlanError(c, err)
		}
		// No MaxWebhooks count check needed here: paper and live webhooks
		// share one combined pool per plan, and switching an *existing*
		// webhook's destination doesn't change the user's total webhook
		// count (every webhook has exactly one of BrokerCredID/PaperAccountID
		// set, so this one was already counted before the switch). The count
		// only needs enforcing on genuine creation (CreateWebhook).
		existing.BrokerCredID = body.BrokerCredID
		existing.PaperAccountID = nil
	}
	if body.PaperAccountID != nil {
		if *body.PaperAccountID == "" {
			return shared.Fail(c, fiber.StatusBadRequest, "paper_account_id cannot be empty")
		}
		scope, err := h.ResolveResourceScope(c)
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "scope lookup failed")
		}
		if _, err := h.resolvePaperDestination(c, scope, *body.PaperAccountID); err != nil {
			return err
		}
		// Same reasoning as the live-switch branch above — no count check needed.
		existing.PaperAccountID = body.PaperAccountID
		existing.BrokerCredID = nil
	}

	if err := ApplyGuardInput(existing, guardIn); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, err.Error())
	}

	if err := h.enforceWebhookOrderType(c.Context(), existing); err != nil {
		if err.Error() == "default_order_type must be MARKET or LIMIT" {
			return shared.Fail(c, fiber.StatusBadRequest, err.Error())
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not resolve order type")
	}

	if err := h.PG.UpdateWebhook(c.Context(), existing); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update webhook")
	}
	_ = h.Redis.InvalidateWebhook(c.Context(), existing.TokenHash)
	return shared.Ok(c, existing)
}

func (h *Handler) RotateWebhookToken(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	id := c.Params("id")

	existing, err := h.PG.GetWebhookByID(c.Context(), id)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if existing == nil || existing.UserID != uid {
		return shared.Fail(c, fiber.StatusNotFound, "webhook not found")
	}

	oldHash := existing.TokenHash
	raw, tokenHash, err := webhooktoken.Generate()
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not generate token")
	}

	if err := h.PG.RotateWebhookToken(c.Context(), id, uid, tokenHash); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not rotate token")
	}

	_ = h.Redis.InvalidateWebhook(c.Context(), oldHash)
	existing.TokenHash = tokenHash
	existing.RawToken = raw
	h.LogUserAudit(c, uid, "webhook_token_rotated", map[string]any{"webhook_id": id})
	return shared.Ok(c, existing)
}

func (h *Handler) PauseWebhook(c *fiber.Ctx) error {
	return h.SetWebhookStatus(c, "paused")
}

func (h *Handler) ResumeWebhook(c *fiber.Ctx) error {
	return h.SetWebhookStatus(c, "active")
}

func (h *Handler) SetWebhookStatus(c *fiber.Ctx, status string) error {
	id := c.Params("id")
	wh, err := h.RequireWebhookAccess(c, id, true)
	if err != nil {
		return err
	}

	if status == "active" {
		if wh.AdminDisabled {
			return shared.Fail(c, fiber.StatusForbidden,
				"this webhook's execution mode is currently disabled by the ZettaBridge admin — it cannot be resumed until it is re-enabled")
		}
		uid := middleware.UserID(c)
		user, err := h.PG.GetUserByID(c.Context(), uid)
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "could not load user")
		}
		scope, err := h.ResolveResourceScope(c)
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "scope lookup failed")
		}

		// Paper webhooks have no broker credential to validate, so the
		// credential-specific checks below are skipped for them — but the
		// resume-limit check still applies to both kinds: paper and live
		// webhooks share one combined active-webhook pool per plan.
		if wh.BrokerCredID != nil {
			// Validate associated credential before allowing the webhook to go active.
			cred, err := h.PG.GetBrokerCred(c.Context(), *wh.BrokerCredID)
			if err != nil || !shared.CredentialUsableForWebhook(scope, cred) {
				return shared.Fail(c, fiber.StatusBadRequest, "webhook is configured with a missing or inaccessible credential; update the webhook before resuming")
			}
			if cred.AdminDisabled {
				modeLabel := "OAuth"
				if cred.ExecutionMode == "publisher" {
					modeLabel = "Kite Publisher"
				}
				return shared.Fail(c, fiber.StatusForbidden,
					fmt.Sprintf("Zerodha %s is currently disabled by the ZettaBridge admin — resume the credential once it is re-enabled", modeLabel))
			}
			if cred.Status == "paused" {
				return shared.Fail(c, fiber.StatusBadRequest, "the credential associated with this webhook is paused; please resume the credential first")
			}
			// Block resuming a webhook that uses a live credential when the plan no longer allows live trading.
			if err := plan.CanUseCredential(user.Plan, cred); err != nil {
				return shared.PlanError(c, err)
			}
		}
		if err := h.EnforceWebhookResumeLimit(c, scope, user, wh.PaperAccountID != nil); err != nil {
			return err
		}
	}

	if err := h.PG.UpdateWebhookStatus(c.Context(), id, status); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "update failed")
	}
	if status == "active" {
		_ = h.PG.ClearWebhookAutoPaused(c.Context(), id)
	}
	_ = h.Redis.InvalidateWebhook(c.Context(), wh.TokenHash)
	return shared.Ok(c, fiber.Map{"status": status})
}

func (h *Handler) DeleteWebhook(c *fiber.Ctx) error {
	id := c.Params("id")
	wh, err := h.RequireWebhookAccess(c, id, true)
	if err != nil {
		return err
	}
	if err := h.PG.DeleteWebhook(c.Context(), id); err != nil {
		log.Printf("delete webhook failed id=%s: %v", id, err)
		return shared.Fail(c, fiber.StatusInternalServerError, "delete failed")
	}
	_ = h.Redis.InvalidateWebhook(c.Context(), wh.TokenHash)
	h.LogUserAudit(c, wh.UserID, "webhook_deleted", map[string]any{"webhook_id": id, "label": wh.Label})
	return c.SendStatus(fiber.StatusNoContent)
}

// ── Trades ────────────────────────────────────────────────────────────────────

func (h *Handler) ListTrades(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	userPlan, err := h.UserPlan(c, uid)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user plan")
	}

	webhookID := c.Params("id")
	wh, err := h.RequireWebhookAccess(c, webhookID, false)
	if err != nil {
		return err
	}

	// Free users get a short recent-trade window for basic feedback (did my webhook fire?).
	// Full history (100 rows, export) remains an Individual+ feature — except for paper
	// webhooks, which always get the full window since Paper Trading is a flat,
	// plan-independent tier (see EnforceAdvancedGuardsPlan for the same exemption).
	limit := 100
	if !plan.LimitsFor(userPlan).AuditLogs && wh.PaperAccountID == nil {
		limit = 20
	}

	trades, err := h.PG.ListTradesByWebhook(c.Context(), webhookID, limit)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list trades")
	}
	if trades == nil {
		trades = []*store.Trade{}
	}
	return shared.Ok(c, trades)
}

func (h *Handler) CancelTrade(c *fiber.Ctx) error {
	webhookID := c.Params("id")
	tradeID := c.Params("tradeId")

	wh, err := h.RequireWebhookAccess(c, webhookID, false)
	if err != nil {
		return err
	}

	trade, err := h.PG.GetTradeByID(c.Context(), tradeID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not look up trade")
	}
	if trade == nil || trade.WebhookID != webhookID {
		return shared.Fail(c, fiber.StatusNotFound, "trade not found")
	}

	switch trade.Status {
	case "filled":
		return shared.Fail(c, fiber.StatusUnprocessableEntity, "order already filled — use CLOSE signal to exit position")
	case "rejected":
		return shared.Fail(c, fiber.StatusUnprocessableEntity, "order was rejected, nothing to cancel")
	case "cancelled":
		return shared.Fail(c, fiber.StatusUnprocessableEntity, "order already cancelled")
	case "queued":
		return shared.Fail(c, fiber.StatusUnprocessableEntity, "order queued but not yet submitted to broker")
	case "submitted":
		// proceed
	default:
		return shared.Fail(c, fiber.StatusUnprocessableEntity, "order cannot be cancelled in current state")
	}

	// Paper trades are always filled/rejected immediately (Phase 2 MVP fill rules)
	// and never reach "submitted", so wh.BrokerCredID is always set here in practice.
	if wh.BrokerCredID == nil {
		return shared.Fail(c, fiber.StatusUnprocessableEntity, "paper trades cannot be cancelled")
	}
	cred, err := h.PG.GetBrokerCred(c.Context(), *wh.BrokerCredID)
	if err != nil || cred == nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load broker credentials")
	}

	credKey, err := h.Cfg.AESKeyBytes()
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load encryption key")
	}
	plaintext, err := credenc.PlaintextOrDecrypt(cred.EncryptedCreds, credKey, cred.ID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not read credential")
	}
	parsed, err := brokercreds.Parse(cred.BrokerType, plaintext)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "invalid broker credential format")
	}

	uid := middleware.UserID(c)
	user, err := h.PG.GetUserByID(c.Context(), uid)
	if err != nil || user == nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user")
	}
	effectivePlan := plan.EffectivePlan(user.Plan)
	route, err := brokerfactory.ResolveExecution(h.Cfg.BrokerMode, effectivePlan, cred)
	if err != nil {
		return shared.PlanError(c, err)
	}

	execCred := route.CredentialForExecution(cred)
	execCred.EncryptedCreds = plaintext
	if execCred.BrokerType == "zerodha" {
		isPublisher := domain.ExecutionMode(execCred.ExecutionMode) == domain.ExecutionModePublisher
		var zerodhaOK bool
		if isPublisher {
			zerodhaOK = zerodhasettings.PublisherEnabled(c.Context(), h.PG)
		} else {
			zerodhaOK = zerodhasettings.OAuthEnabled(c.Context(), h.PG)
		}
		if !zerodhaOK {
			msg := "zerodha User OAuth API execution is currently disabled by the platform admin"
			if isPublisher {
				msg = "zerodha Kite Publisher execution is currently disabled by the platform admin"
			}
			return shared.Fail(c, fiber.StatusForbidden, msg)
		}
	}
	b, err := brokerfactory.New(route.Mode, h.Infra, execCred, parsed)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not initialize broker")
	}

	cancelReq := &domain.CancelRequest{
		OrderID:    trade.BrokerOrder,
		BrokerType: cred.BrokerType,
	}
	if cancelErr := b.CancelOrder(c.Context(), cancelReq); cancelErr != nil {
		code := brokererr.CodeOf(cancelErr)
		if code == brokererr.CodeOrderNotFound || code == brokererr.CodeAlreadyCancelled {
			// Order is gone from broker side — mark as cancelled in our system
		} else {
			msg := brokererr.PublicFrom(cancelErr)
			log.Printf("cancel_trade: broker cancel failed trade=%s code=%s err=%v", trade.ID, code, cancelErr)
			_ = h.PG.MarkTradeCancelled(c.Context(), trade.ID, msg)
			return shared.Fail(c, fiber.StatusBadGateway, "cancel failed: "+msg)
		}
	}

	if err := h.PG.MarkTradeCancelled(c.Context(), trade.ID, ""); err != nil {
		log.Printf("cancel_trade: could not persist cancel trade=%s: %v", trade.ID, err)
		return shared.Fail(c, fiber.StatusInternalServerError, "could not record cancel")
	}

	return shared.Ok(c, fiber.Map{
		"id":           trade.ID,
		"webhook_id":   trade.WebhookID,
		"broker_order": trade.BrokerOrder,
		"status":       "cancelled",
	})
}

// GetTrade returns a single trade by ID, verifying the caller owns the webhook it belongs to.
func (h *Handler) GetTrade(c *fiber.Ctx) error {
	tradeID := c.Params("id")

	trade, err := h.PG.GetTradeByIDWithPublisherMeta(c.Context(), tradeID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not look up trade")
	}
	if trade == nil {
		return shared.Fail(c, fiber.StatusNotFound, "trade not found")
	}

	if _, err := h.RequireWebhookAccess(c, trade.WebhookID, false); err != nil {
		return err
	}

	if trade.Status == "pending_confirmation" && trade.BrokerOrder != "" {
		if expired, err := h.PG.ExpirePublisherOrderIfStale(c.Context(), trade.BrokerOrder); err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "could not look up trade")
		} else if expired {
			trade, err = h.PG.GetTradeByIDWithPublisherMeta(c.Context(), tradeID)
			if err != nil {
				return shared.Fail(c, fiber.StatusInternalServerError, "could not look up trade")
			}
			if trade == nil {
				return shared.Fail(c, fiber.StatusNotFound, "trade not found")
			}
		}
	}

	return shared.Ok(c, trade)
}
