package shared

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// Handler holds shared infrastructure available to all module handlers.
type Handler struct {
	PG    *store.PGStore
	Redis *store.RedisStore
	Cfg   *config.Config
}

// ── Scope ─────────────────────────────────────────────────────────────────────

func (h *Handler) ResolveResourceScope(c *fiber.Ctx) (store.ResourceScope, error) {
	uid := middleware.UserID(c)
	return store.ResourceScope{UserID: uid, InOrg: false}, nil
}

func WebhookAccessible(scope store.ResourceScope, wh *store.Webhook) bool {
	if wh == nil {
		return false
	}
	return wh.UserID == scope.UserID
}

func WebhookMutable(scope store.ResourceScope, wh *store.Webhook) bool {
	if !WebhookAccessible(scope, wh) {
		return false
	}
	return wh.UserID == scope.UserID
}

func CredentialAccessible(scope store.ResourceScope, bc *store.BrokerCredential) bool {
	if bc == nil {
		return false
	}
	return bc.UserID == scope.UserID
}

func CredentialMutable(scope store.ResourceScope, bc *store.BrokerCredential) bool {
	return CredentialAccessible(scope, bc)
}

func CredentialUsableForWebhook(scope store.ResourceScope, bc *store.BrokerCredential) bool {
	if bc == nil {
		return false
	}
	return bc.UserID == scope.UserID
}

// PaperAccountUsableForWebhook reports whether pa may be used as a webhook's execution destination in scope.
func PaperAccountUsableForWebhook(scope store.ResourceScope, pa *store.PaperAccount) bool {
	if pa == nil {
		return false
	}
	return pa.UserID == scope.UserID
}

func (h *Handler) RequireWebhookAccess(c *fiber.Ctx, id string, mutate bool) (*store.Webhook, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, Fail(c, fiber.StatusBadRequest, "invalid webhook id")
	}
	scope, err := h.ResolveResourceScope(c)
	if err != nil {
		return nil, Fail(c, fiber.StatusInternalServerError, "scope lookup failed")
	}
	wh, err := h.PG.GetWebhookByID(c.Context(), id)
	if err != nil {
		return nil, Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	ok := WebhookMutable(scope, wh)
	if !mutate {
		ok = WebhookAccessible(scope, wh)
	}
	if !ok {
		return nil, Fail(c, fiber.StatusNotFound, "webhook not found")
	}
	return wh, nil
}

func (h *Handler) RequireCredentialAccess(c *fiber.Ctx, id string, mutate bool) (*store.BrokerCredential, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, Fail(c, fiber.StatusBadRequest, "invalid credential id")
	}
	scope, err := h.ResolveResourceScope(c)
	if err != nil {
		return nil, Fail(c, fiber.StatusInternalServerError, "scope lookup failed")
	}
	bc, err := h.PG.GetBrokerCred(c.Context(), id)
	if err != nil {
		return nil, Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	ok := CredentialMutable(scope, bc)
	if !mutate {
		ok = CredentialAccessible(scope, bc)
	}
	if !ok {
		return nil, Fail(c, fiber.StatusNotFound, "credential not found")
	}
	return bc, nil
}

func OrgIDPtr(orgID string) *string {
	if orgID == "" {
		return nil
	}
	return &orgID
}

// ── Membership ────────────────────────────────────────────────────────────────

func (h *Handler) InvalidateWebhookTokens(ctx context.Context, tokens []string) {
	for _, token := range tokens {
		if token != "" {
			_ = h.Redis.InvalidateWebhook(ctx, token)
		}
	}
}

func (h *Handler) EffectiveUserPlan(c *fiber.Ctx, uid string) (string, error) {
	user, err := h.PG.GetUserByID(c.Context(), uid)
	if err != nil || user == nil {
		return "", err
	}
	return plan.EffectivePlan(user.Plan), nil
}

func (h *Handler) UserPlan(c *fiber.Ctx, uid string) (string, error) {
	return h.EffectiveUserPlan(c, uid)
}

func (h *Handler) LoadUserWithOrgContext(c *fiber.Ctx, uid string) (*store.User, error) {
	user, err := h.PG.GetUserByID(c.Context(), uid)
	if err != nil || user == nil {
		return nil, err
	}
	return user, nil
}

// ── Plan limits ───────────────────────────────────────────────────────────────

func (h *Handler) EnrichMePlanFields(c *fiber.Ctx, data fiber.Map, user *store.User) {
	effective := plan.EffectivePlan(user.Plan)
	limits := plan.LimitsFor(effective)
	display, enforced, credCap := plan.OrderRateLimits(effective)
	data["orders_per_sec"] = display
	data["orders_per_sec_enforced"] = enforced
	data["broker_cred_orders_per_sec"] = credCap

	data["max_webhooks"] = limits.MaxPaperWebhooks + limits.MaxLiveWebhooks
	data["max_paper_webhooks"] = limits.MaxPaperWebhooks
	data["max_live_webhooks"] = limits.MaxLiveWebhooks
	data["max_broker_creds"] = limits.MaxBrokers
	data["max_paper_accounts"] = limits.MaxPaperAccounts
	data["live_trading_allowed"] = limits.LiveAllowed

	if limits.MaxPaperTradesPerMonth > 0 {
		data["max_paper_trades_per_month"] = limits.MaxPaperTradesPerMonth
	} else {
		data["max_paper_trades_per_month"] = nil
	}

	ctx := c.Context()
	paperWH, _ := h.PG.CountPaperWebhooksByUser(ctx, user.ID)
	liveWH, _ := h.PG.CountLiveWebhooksByUser(ctx, user.ID)
	data["paper_webhooks_used"] = paperWH
	data["live_webhooks_used"] = liveWH

	paperTrades, _ := h.PG.CountPaperOrdersByUserInMonth(ctx, user.ID, time.Now().UTC())
	data["paper_trades_used_this_month"] = paperTrades
	quotaExceeded := !plan.PaperTradesUnlimited(effective) && paperTrades >= limits.MaxPaperTradesPerMonth
	data["paper_trade_quota_exceeded"] = quotaExceeded
}

// ── Subscription limits ───────────────────────────────────────────────────────

func (h *Handler) EnforceWebhookLimit(c *fiber.Ctx, scope store.ResourceScope, user *store.User, paper bool) error {
	effective := plan.EffectivePlan(user.Plan)
	limits := plan.LimitsFor(effective)
	var max int
	var countFn func(context.Context, string) (int, error)
	if paper {
		max = limits.MaxPaperWebhooks
		countFn = h.PG.CountPaperWebhooksByUser
	} else {
		if !limits.LiveAllowed {
			return plan.ErrLiveNotAllowed
		}
		max = limits.MaxLiveWebhooks
		countFn = h.PG.CountLiveWebhooksByUser
	}

	count, err := countFn(c.Context(), scope.UserID)
	if err != nil {
		return Fail(c, fiber.StatusInternalServerError, "could not count webhooks")
	}
	if err := plan.CanAddWebhook(max, count); err != nil {
		return err
	}
	return nil
}

func (h *Handler) EnforceBrokerLimit(c *fiber.Ctx, scope store.ResourceScope, user *store.User) error {
	effective := plan.EffectivePlan(user.Plan)
	max := plan.MaxBrokers(effective)

	count, err := h.PG.CountSoloBrokerCredsByUser(c.Context(), scope.UserID)
	if err != nil {
		return Fail(c, fiber.StatusInternalServerError, "could not count broker credentials")
	}
	if err := plan.CanAddBroker(max, count); err != nil {
		return err
	}
	return nil
}

func SubscriptionLimitsForMe(user *store.User) (maxWebhooks, maxBrokers int) {
	effective := plan.EffectivePlan(user.Plan)
	limits := plan.LimitsFor(effective)
	return limits.MaxPaperWebhooks + limits.MaxLiveWebhooks, limits.MaxBrokers
}

func (h *Handler) EnforceWebhookResumeLimit(c *fiber.Ctx, scope store.ResourceScope, user *store.User, paper bool) error {
	effective := plan.EffectivePlan(user.Plan)
	limits := plan.LimitsFor(effective)
	var max int
	var countFn func(context.Context, string) (int, error)
	if paper {
		max = limits.MaxPaperWebhooks
		countFn = h.PG.CountActivePaperWebhooksByUser
	} else {
		if !limits.LiveAllowed {
			return Fail(c, fiber.StatusPaymentRequired, "your plan does not include live trading webhooks")
		}
		max = limits.MaxLiveWebhooks
		countFn = h.PG.CountActiveLiveWebhooksByUser
	}

	count, err := countFn(c.Context(), scope.UserID)
	if err != nil {
		return Fail(c, fiber.StatusInternalServerError, "could not count webhooks")
	}
	if count >= max {
		kind := "live"
		if paper {
			kind = "paper"
		}
		return Fail(c, fiber.StatusPaymentRequired,
			fmt.Sprintf("your %s plan allows %d active %s webhook(s); upgrade to activate more", user.Plan, max, kind))
	}
	return nil
}

func (h *Handler) EnforceCredentialResumeLimit(c *fiber.Ctx, scope store.ResourceScope, user *store.User) error {
	effective := plan.EffectivePlan(user.Plan)
	max := plan.MaxBrokers(effective)

	count, err := h.PG.CountActiveCredentialsByUser(c.Context(), scope.UserID)
	if err != nil {
		return Fail(c, fiber.StatusInternalServerError, "could not count credentials")
	}
	if count >= max {
		return Fail(c, fiber.StatusPaymentRequired,
			fmt.Sprintf("your %s plan allows %d active credential(s); upgrade to activate more", user.Plan, max))
	}
	return nil
}

// ── Audit ─────────────────────────────────────────────────────────────────────

func (h *Handler) LogUserAudit(c *fiber.Ctx, userID, action string, metadata map[string]any) {
	h.PG.InsertUserAudit(c.Context(), userID, action, c.IP(), metadata)
}

func (h *Handler) LogIngest(ctx context.Context, ip, webhookID, requestID, outcome, errMsg string) {
	h.PG.InsertIngestLog(ctx, webhookID, requestID, outcome, errMsg, ip)
}

func (h *Handler) LogOrgAudit(c *fiber.Ctx, orgID, action, targetUserID string, metadata map[string]any) {}
