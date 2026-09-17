package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	stripewebhook "github.com/stripe/stripe-go/v81/webhook"

	"github.com/SPSingh09/zettabridge/internal/billing"
	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/integrations/telegram"
	"github.com/SPSingh09/zettabridge/internal/marketdata"
	"github.com/SPSingh09/zettabridge/internal/modules/admin"
	authmod "github.com/SPSingh09/zettabridge/internal/modules/auth"
	billingmod "github.com/SPSingh09/zettabridge/internal/modules/billing"
	"github.com/SPSingh09/zettabridge/internal/modules/credentials"
	"github.com/SPSingh09/zettabridge/internal/modules/invites"
	"github.com/SPSingh09/zettabridge/internal/modules/internalapi"
	"github.com/SPSingh09/zettabridge/internal/modules/paperaccounts"
	"github.com/SPSingh09/zettabridge/internal/modules/pnl"
	pubmod "github.com/SPSingh09/zettabridge/internal/modules/publisher"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/modules/signals"
	"github.com/SPSingh09/zettabridge/internal/modules/symbolrequests"
	"github.com/SPSingh09/zettabridge/internal/modules/webhooks"
	"github.com/SPSingh09/zettabridge/internal/paperengine"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/platform/mailer"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/platform/queue"
	"github.com/SPSingh09/zettabridge/internal/platform/security"
	"github.com/SPSingh09/zettabridge/internal/store"
	httptransport "github.com/SPSingh09/zettabridge/internal/transport/http"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
	"github.com/SPSingh09/zettabridge/internal/transport/websocket"
)

// Handler wires together all dependencies for HTTP route handlers.
type Handler struct {
	// Legacy fields kept for test backward-compatibility (tests construct Handler directly).
	pg             *store.PGStore
	redis          *store.RedisStore
	queue          *queue.Queue
	cfg            *config.Config
	infra          *livebrokers.Infra
	mail           mailer.Sender
	tg             *telegram.Sender
	trades         *tradepush.Hub
	billing        *billing.Service
	stripeWebhooks *billing.WebhookProcessor

	// Module sub-handlers.
	base       *shared.Handler
	authH      *authmod.Handler
	signalsH   *signals.Handler
	webhooksH  *webhooks.Handler
	credsH     *credentials.Handler
	paperAcctH *paperaccounts.Handler
	pnlH       *pnl.Handler
	billingH   *billingmod.Handler
	invitesH   *invites.Handler
	adminH     *admin.Handler
	pubH       *pubmod.Handler
	symbolReqH *symbolrequests.Handler

	// Router owns the route registration logic.
	router *httptransport.Router
}

func New(pg *store.PGStore, redis *store.RedisStore, q *queue.Queue, cfg *config.Config, infra *livebrokers.Infra, trades *tradepush.Hub) *Handler {
	mail := mailer.New(cfg)
	tg := telegram.New(cfg.TelegramBotToken)

	var billingSvc *billing.Service
	var stripeWH *billing.WebhookProcessor
	if cfg.BillingEnabled {
		billingSvc = billing.NewService(cfg, pg, billing.NewStripeClient(cfg.StripeSecretKey))
		stripeWH = billing.NewWebhookProcessor(cfg, pg)
	}

	base := &shared.Handler{PG: pg, Redis: redis, Cfg: cfg}

	authH := &authmod.Handler{Handler: base, Mail: mail}
	signalsH := &signals.Handler{Handler: base, Queue: q, Trades: trades}
	webhooksH := &webhooks.Handler{Handler: base, Infra: infra}
	credsH := &credentials.Handler{Handler: base, Infra: infra}
	paperAcctH := &paperaccounts.Handler{Handler: base, PaperEngine: paperengine.New(pg), MarketData: marketDataFromQueue(q)}
	pnlH := &pnl.Handler{Handler: base}
	billingH := &billingmod.Handler{Handler: base, Billing: billingSvc, StripeWebhooks: stripeWH}
	invitesH := &invites.Handler{Handler: base, Mail: mail, TG: tg}
	adminH := &admin.Handler{Handler: base, TG: tg, Billing: billingSvc, Mail: mail}
	pubH := &pubmod.Handler{Handler: base}
	symbolReqH := &symbolrequests.Handler{Handler: base, Mail: mail, TG: tg}
	internalH := &internalapi.Handler{Handler: base}

	router := httptransport.NewRouter(cfg, pg, redis, authH, signalsH, webhooksH, credsH, paperAcctH, pnlH, billingH, invitesH, adminH, pubH, symbolReqH, internalH)

	return &Handler{
		pg:             pg,
		redis:          redis,
		queue:          q,
		cfg:            cfg,
		infra:          infra,
		mail:           mail,
		tg:             tg,
		trades:         trades,
		billing:        billingSvc,
		stripeWebhooks: stripeWH,
		base:           base,
		router:         router,
		authH:          authH,
		signalsH:       signalsH,
		webhooksH:      webhooksH,
		credsH:         credsH,
		paperAcctH:     paperAcctH,
		pnlH:           pnlH,
		billingH:       billingH,
		invitesH:       invitesH,
		adminH:         adminH,
		pubH:           pubH,
		symbolReqH:     symbolReqH,
	}
}

func marketDataFromQueue(q *queue.Queue) *marketdata.Registry {
	if q == nil {
		return nil
	}
	return q.MarketDataRegistry()
}

// Register mounts all routes onto the Fiber app.
func (h *Handler) Register(app *fiber.App) {
	h.router.Register(app)
}

// ── Core helpers ──────────────────────────────────────────────────────────────

func ok(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": data})
}

func created(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": data})
}

func fail(c *fiber.Ctx, status int, msg string) error {
	_ = c.Status(status).JSON(fiber.Map{"error": msg})
	return fiber.NewError(status, msg)
}

func planError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, plan.ErrLiveNotAllowed),
		errors.Is(err, plan.ErrWebhookLimitReached),
		errors.Is(err, plan.ErrBrokerLimitReached),
		errors.Is(err, plan.ErrAuditLogsNotAllowed),
		errors.Is(err, plan.ErrAdvancedWebhookGuardsNotAllowed):
		return fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, plan.ErrInvalidAccountMode):
		return fail(c, fiber.StatusBadRequest, err.Error())
	default:
		return fail(c, fiber.StatusBadRequest, err.Error())
	}
}

// ErrorHandler is a global Fiber error handler.
func ErrorHandler(c *fiber.Ctx, err error) error {
	if len(c.Response().Body()) > 0 {
		return nil
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return fail(c, fe.Code, fe.Message)
	}
	log.Printf("unhandled error: %v", err)
	return fail(c, fiber.StatusInternalServerError, "internal server error")
}

func (h *Handler) health(c *fiber.Ctx) error {
	return ok(c, fiber.Map{"status": "ok", "time": time.Now().UTC()})
}

// ── Backward-compatibility wrappers for test files in this package ────────────

// scrapeMetrics keeps metrics_test.go working.
func (h *Handler) scrapeMetrics(c *fiber.Ctx) error {
	if !h.cfg.MetricsEnabled {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if token := h.cfg.MetricsToken; token != "" {
		if middleware.BearerToken(c) != token {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
	}
	mf, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("metrics gather failed")
	}
	c.Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w := c.Response().BodyWriter()
	enc := expfmt.NewEncoder(w, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, family := range mf {
		if err := enc.Encode(family); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("metrics encode failed")
		}
	}
	return nil
}

// getBillingPlans keeps billing_test.go working.
func (h *Handler) getBillingPlans(c *fiber.Ctx) error {
	if h.billingH != nil {
		return h.billingH.GetBillingPlans(c)
	}
	return ok(c, billing.CatalogFromConfig(h.cfg))
}

// postBillingCheckout keeps billing_test.go working.
func (h *Handler) postBillingCheckout(c *fiber.Ctx) error {
	if h.billingH != nil {
		return h.billingH.PostBillingCheckout(c)
	}
	return fail(c, fiber.StatusServiceUnavailable, billing.ErrBillingDisabled.Error())
}

// postStripeWebhook keeps stripe_webhook_test.go working.
func (h *Handler) postStripeWebhook(c *fiber.Ctx) error {
	if !h.cfg.BillingEnabled || h.stripeWebhooks == nil {
		metrics.RecordBillingWebhook(metrics.BillingWebhookDisabled)
		return fail(c, fiber.StatusServiceUnavailable, "billing is not enabled")
	}
	sig := c.Get("Stripe-Signature")
	if sig == "" {
		metrics.RecordBillingWebhook(metrics.BillingWebhookSignatureInvalid)
		return fail(c, fiber.StatusBadRequest, "missing Stripe-Signature header")
	}
	event, err := stripewebhook.ConstructEventWithOptions(c.Body(), sig, h.cfg.StripeWebhookSecret, stripewebhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		metrics.RecordBillingWebhook(metrics.BillingWebhookSignatureInvalid)
		return fail(c, fiber.StatusBadRequest, "invalid webhook signature")
	}
	if err := h.stripeWebhooks.Handle(c.Context(), event); err != nil {
		log.Printf("stripe webhook %s (%s): %v", event.ID, event.Type, err)
		metrics.RecordBillingWebhook(metrics.BillingWebhookError)
		return fail(c, fiber.StatusInternalServerError, "webhook processing failed")
	}
	metrics.RecordBillingWebhook(metrics.BillingWebhookOK)
	return c.SendStatus(fiber.StatusOK)
}

// wsTradesUpgrade keeps ws_trades_integration_test.go working.
func (h *Handler) wsTradesUpgrade(c *fiber.Ctx) error {
	if h.signalsH != nil {
		return h.signalsH.WsTradesUpgrade(c)
	}
	sh := &signals.Handler{Handler: &shared.Handler{PG: h.pg, Redis: h.redis, Cfg: h.cfg}}
	return sh.WsTradesUpgrade(c)
}

// wsTrades keeps ws_trades_integration_test.go working.
func (h *Handler) wsTrades(conn *websocket.Conn) {
	if h.signalsH != nil {
		h.signalsH.WsTrades(conn)
		return
	}
	sh := &signals.Handler{Handler: &shared.Handler{PG: h.pg, Redis: h.redis, Cfg: h.cfg}, Trades: h.trades}
	sh.WsTrades(conn)
}

// verificationURL keeps auth_email_test.go working.
func (h *Handler) verificationURL(rawToken string) string {
	return h.cfg.AppPublicURL + "/v1/auth/verify-email?token=" + rawToken
}

// ── Package-level backward-compat functions (used by test files) ──────────────

// normalizeAccountEmail keeps auth_email_test.go working.
func normalizeAccountEmail(email string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil {
		return "", err
	}
	return strings.ToLower(addr.Address), nil
}

// loginBlockedByEmailVerification keeps auth_email_test.go working.
func loginBlockedByEmailVerification(cfg *config.Config, user *store.User) bool {
	return cfg.EmailVerificationRequired && user != nil && !user.EmailVerified()
}

// userEmailVerifiedFields keeps auth_email_test.go working.
func userEmailVerifiedFields(user *store.User) (verified bool, verifiedAt interface{}) {
	if user == nil || user.EmailVerifiedAt == nil {
		return false, nil
	}
	return true, user.EmailVerifiedAt
}

// webhookAccessible keeps org_scope_test.go working.
func webhookAccessible(scope store.ResourceScope, wh *store.Webhook) bool {
	return shared.WebhookAccessible(scope, wh)
}

// credentialAccessible keeps org_scope_test.go working.
func credentialAccessible(scope store.ResourceScope, bc *store.BrokerCredential) bool {
	return shared.CredentialAccessible(scope, bc)
}

// rejectImmutableWebhookFields keeps handler_test.go working.
func rejectImmutableWebhookFields(body []byte) (string, error) {
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

var immutableWebhookFields = []string{"id", "user_id", "created_at", "token", "status"}

// middlewareBearerOrQuery keeps ws_trades_query_test.go working.
func middlewareBearerOrQuery(c *fiber.Ctx) string {
	return signals.MiddlewareBearerOrQuery(c)
}

// credentialVerifyHint keeps credential_verify_test.go working.
func credentialVerifyHint(brokerType string, err error) string {
	return credentials.CredentialVerifyHint(brokerType, err)
}

// credentialVerifyFailure keeps credential_verify_test.go working.
func credentialVerifyFailure(brokerType, errMsg string, err error) map[string]interface{} {
	return credentials.CredentialVerifyFailure(brokerType, errMsg, err)
}

// Suppress "declared but not used" for packages only referenced in test-compat functions.
var _ = brokererr.CodeAuthFailed
var _ = security.ValidatePassword
