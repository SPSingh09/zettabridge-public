package httptransport

import (
	"errors"
	"log"
	"strings"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/config"
	"github.com/SPSingh09/zettabridge/internal/modules/admin"
	authmod "github.com/SPSingh09/zettabridge/internal/modules/auth"
	billingmod "github.com/SPSingh09/zettabridge/internal/modules/billing"
	"github.com/SPSingh09/zettabridge/internal/modules/credentials"
	"github.com/SPSingh09/zettabridge/internal/modules/invites"
	"github.com/SPSingh09/zettabridge/internal/modules/internalapi"
	"github.com/SPSingh09/zettabridge/internal/modules/paperaccounts"
	"github.com/SPSingh09/zettabridge/internal/modules/pnl"
	pubmod "github.com/SPSingh09/zettabridge/internal/modules/publisher"
	"github.com/SPSingh09/zettabridge/internal/modules/signals"
	"github.com/SPSingh09/zettabridge/internal/modules/symbolrequests"
	"github.com/SPSingh09/zettabridge/internal/modules/webhooks"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

// Router owns the module handlers and registers all HTTP routes.
type Router struct {
	AuthH      *authmod.Handler
	SignalsH   *signals.Handler
	WebhooksH  *webhooks.Handler
	CredsH     *credentials.Handler
	PaperAcctH *paperaccounts.Handler
	PnlH       *pnl.Handler
	BillingH   *billingmod.Handler
	InvitesH   *invites.Handler
	AdminH     *admin.Handler
	PubH       *pubmod.Handler
	SymbolReqH *symbolrequests.Handler
	InternalH  *internalapi.Handler
	pg         *store.PGStore
	redis      *store.RedisStore
	cfg        *config.Config
}

// NewRouter constructs a Router from pre-built module handlers.
func NewRouter(
	cfg *config.Config,
	pg *store.PGStore,
	redis *store.RedisStore,
	authH *authmod.Handler,
	signalsH *signals.Handler,
	webhooksH *webhooks.Handler,
	credsH *credentials.Handler,
	paperAcctH *paperaccounts.Handler,
	pnlH *pnl.Handler,
	billingH *billingmod.Handler,
	invitesH *invites.Handler,
	adminH *admin.Handler,
	pubH *pubmod.Handler,
	symbolReqH *symbolrequests.Handler,
	internalH *internalapi.Handler,
) *Router {
	return &Router{
		AuthH:      authH,
		SignalsH:   signalsH,
		WebhooksH:  webhooksH,
		CredsH:     credsH,
		PaperAcctH: paperAcctH,
		PnlH:       pnlH,
		BillingH:   billingH,
		InvitesH:   invitesH,
		AdminH:     adminH,
		PubH:       pubH,
		SymbolReqH: symbolReqH,
		InternalH:  internalH,
		pg:         pg,
		redis:      redis,
		cfg:        cfg,
	}
}

// Register mounts all API routes onto the Fiber app.
func (r *Router) Register(app *fiber.App) {
	// Public
	app.Get("/healthz", r.health)
	app.Get("/metrics", r.AdminH.ScrapeMetrics)
	app.Post("/v1/auth/register", r.AuthH.Register)
	app.Post("/v1/auth/login", r.AuthH.Login)
	app.Get("/v1/auth/verify-email", r.AuthH.VerifyEmail)
	app.Post("/v1/auth/resend-verification", r.AuthH.ResendVerification)
	app.Get("/v1/invites/:token", r.InvitesH.PreviewInvite)
	app.Post("/v1/invites/accept", r.InvitesH.AcceptInvite)

	// Zerodha Kite OAuth callback — always public (browser redirect, no JWT/service token).
	// In http adapter mode the exec-* host also serves this path; registering it on Core
	// lets users who registered the api.* callback URL in their Kite app still complete OAuth.
	app.Get("/v1/credentials/zerodha/callback", r.CredsH.ZerodhaCallback)

	// Publisher + legacy in-process adapter callbacks when sidecars are not used.
	if !strings.EqualFold(r.cfg.ExecutionAdapterMode, "http") {
		app.Get("/v1/publisher/callback", r.PubH.Callback)
	}

	// FYERS OAuth callback — public, browser redirect from FYERS (no JWT).
	app.Get("/v1/admin/fyers/callback", r.AdminH.AdminFyersCallback)

	// Webhook signal ingestion — public but token-authenticated via URL path
	app.Post("/v1/webhook/:token", r.SignalsH.IngestSignal)

	// Stripe billing webhooks — public, signature-verified
	app.Post("/v1/webhooks/stripe", r.BillingH.PostStripeWebhook)

	// Adapter → Core internal API (service token)
	internal := app.Group("/v1/internal", middleware.RequireServiceToken(r.cfg.ZBServiceToken))
	internal.Post("/execution-events", r.InternalH.PostExecutionEvent)
	internal.Put("/credentials/:id/session", r.InternalH.PutCredentialSession)

	// Live trade stream (JWT via Authorization header or ?token=) — before /v1 auth group
	app.Get("/v1/ws/trades", r.SignalsH.WsTradesUpgrade, websocket.New(r.SignalsH.WsTrades))

	// Protected — requires valid JWT
	protected := app.Group("/v1", middleware.Auth(r.cfg.JWTSecret), middleware.CheckRevoked(r.redis))
	protected.Post("/auth/logout", r.AuthH.Logout)
	protected.Get("/me", r.AuthH.GetMe)
	protected.Get("/billing/plans", r.BillingH.GetBillingPlans)
	protected.Post("/billing/checkout", r.BillingH.PostBillingCheckout)
	protected.Post("/billing/portal", r.BillingH.PostBillingPortal)

	adminRoutes := protected.Group("/admin", middleware.RequirePlatformAdmin())
	adminRoutes.Get("/users", r.AdminH.AdminListUsers)
	adminRoutes.Put("/users/:id/plan", r.AdminH.AdminSetUserPlan)
	adminRoutes.Get("/users/:id", r.AdminH.AdminGetUser)
	adminRoutes.Patch("/users/:id", r.AdminH.AdminPatchUser)
	adminRoutes.Delete("/users/:id", r.AdminH.AdminDeleteUser)
	adminRoutes.Post("/invites", r.AdminH.AdminCreatePlatformInvite)
	adminRoutes.Get("/invites", r.AdminH.AdminListPlatformInvites)
	adminRoutes.Delete("/invites/:id", r.AdminH.AdminRevokePlatformInvite)
	adminRoutes.Get("/users/:id/audit", r.AdminH.AdminListUserAudit)
	adminRoutes.Get("/settings/market-data-provider", r.AdminH.AdminGetMarketDataProvider)
	adminRoutes.Put("/settings/market-data-provider", r.AdminH.AdminSetMarketDataProvider)
	adminRoutes.Get("/settings/market-data-interval", r.AdminH.AdminGetMarketDataInterval)
	adminRoutes.Put("/settings/market-data-interval", r.AdminH.AdminSetMarketDataInterval)
	adminRoutes.Get("/fyers/status", r.AdminH.AdminGetFyersStatus)
	adminRoutes.Get("/fyers/connect", r.AdminH.AdminFyersConnect)
	adminRoutes.Put("/fyers/pin", r.AdminH.AdminSetFyersPIN)
	adminRoutes.Get("/market-profiles", r.AdminH.AdminListMarketProfiles)
	adminRoutes.Patch("/market-profiles/:code", r.AdminH.AdminUpdateMarketProfile)
	adminRoutes.Get("/instruments", r.AdminH.AdminListInstruments)
	adminRoutes.Post("/instruments", r.AdminH.AdminCreateInstrument)
	adminRoutes.Patch("/instruments/:id", r.AdminH.AdminUpdateInstrument)
	adminRoutes.Delete("/instruments/:id", r.AdminH.AdminDeleteInstrument)
	adminRoutes.Get("/settings/market-hours-enforcement", r.AdminH.AdminGetMarketHoursEnforcement)
	adminRoutes.Put("/settings/market-hours-enforcement", r.AdminH.AdminSetMarketHoursEnforcement)
	adminRoutes.Get("/settings/zerodha-oauth-enabled", r.AdminH.AdminGetZerodhaOAuthEnabled)
	adminRoutes.Put("/settings/zerodha-oauth-enabled", r.AdminH.AdminSetZerodhaOAuthEnabled)
	adminRoutes.Get("/settings/zerodha-publisher-enabled", r.AdminH.AdminGetZerodhaPublisherEnabled)
	adminRoutes.Put("/settings/zerodha-publisher-enabled", r.AdminH.AdminSetZerodhaPublisherEnabled)
	adminRoutes.Get("/symbol-requests", r.AdminH.AdminListSymbolRequests)
	adminRoutes.Post("/symbol-requests/:id/accept", r.AdminH.AdminAcceptSymbolRequest)
	adminRoutes.Post("/symbol-requests/:id/reject", r.AdminH.AdminRejectSymbolRequest)

	// Webhooks CRUD
	protected.Get("/webhooks", r.WebhooksH.ListWebhooks)
	protected.Get("/webhooks/:id", r.WebhooksH.GetWebhook)
	protected.Post("/webhooks", r.WebhooksH.CreateWebhook)
	protected.Put("/webhooks/:id", r.WebhooksH.UpdateWebhook)
	protected.Post("/webhooks/:id/rotate-token", r.WebhooksH.RotateWebhookToken)
	protected.Put("/webhooks/:id/pause", r.WebhooksH.PauseWebhook)
	protected.Put("/webhooks/:id/resume", r.WebhooksH.ResumeWebhook)
	protected.Delete("/webhooks/:id", r.WebhooksH.DeleteWebhook)
	protected.Get("/webhooks/:id/trades", r.WebhooksH.ListTrades)
	protected.Get("/webhooks/:id/trades/export", r.PnlH.ExportTrades)
	protected.Delete("/webhooks/:id/trades/:tradeId", r.WebhooksH.CancelTrade)
	protected.Get("/webhooks/:id/pnl", r.PnlH.GetWebhookPnL)
	protected.Get("/trades/:id", r.WebhooksH.GetTrade)

	// Notifications (in-app)
	protected.Get("/notifications", r.AdminH.ListNotifications)
	protected.Get("/notifications/unread-count", r.AdminH.CountUnreadNotifications)
	protected.Patch("/notifications/:id/read", r.AdminH.MarkNotificationRead)
	protected.Post("/notifications/read-all", r.AdminH.MarkAllNotificationsRead)

	// Telegram alert settings
	protected.Get("/settings/telegram", r.AdminH.GetTelegramSettings)
	protected.Patch("/settings/telegram", r.AdminH.UpdateTelegramSettings)

	// Broker credentials CRUD
	protected.Get("/brokers/enabled", r.CredsH.ListEnabledBrokers)
	protected.Get("/credentials", r.CredsH.ListCredentials)
	protected.Post("/credentials", r.CredsH.CreateCredential)
	protected.Put("/credentials/:id", r.CredsH.UpdateCredential)
	protected.Put("/credentials/:id/pause", r.CredsH.PauseCredential)
	protected.Put("/credentials/:id/resume", r.CredsH.ResumeCredential)
	protected.Post("/credentials/:id/verify", r.CredsH.VerifyCredential)
	protected.Delete("/credentials/:id", r.CredsH.DeleteCredential)

	protected.Get("/credentials/zerodha/connect", r.CredsH.ZerodhaConnect)
	protected.Get("/credentials/zerodha/status", r.CredsH.ZerodhaStatus)
	protected.Get("/credentials/zerodha/connect-info", r.CredsH.ZerodhaConnectInfo)

	// Paper trading accounts CRUD
	protected.Get("/market-profiles", r.PaperAcctH.ListActiveMarketProfiles)
	protected.Get("/paper-accounts", r.PaperAcctH.ListPaperAccounts)
	protected.Post("/paper-accounts", r.PaperAcctH.CreatePaperAccount)
	protected.Get("/paper-accounts/:id", r.PaperAcctH.GetPaperAccount)
	protected.Patch("/paper-accounts/:id", r.PaperAcctH.UpdatePaperAccount)
	protected.Put("/paper-accounts/:id/pause", r.PaperAcctH.PausePaperAccount)
	protected.Put("/paper-accounts/:id/resume", r.PaperAcctH.ResumePaperAccount)
	protected.Put("/paper-accounts/:id/reset", r.PaperAcctH.ResetPaperAccount)
	protected.Delete("/paper-accounts/:id", r.PaperAcctH.DeletePaperAccount)
	protected.Get("/paper-accounts/:id/positions", r.PaperAcctH.ListPaperPositions)
	protected.Post("/paper-accounts/:id/positions/close", r.PaperAcctH.ClosePaperPosition)
	protected.Get("/paper-accounts/:id/orders", r.PaperAcctH.ListPaperOrders)
	protected.Get("/paper-accounts/:id/orders/export", r.PaperAcctH.ExportPaperOrders)
	protected.Get("/paper-accounts/:id/pnl", r.PaperAcctH.GetPaperAccountPnL)
	protected.Get("/paper-accounts/:id/snapshots", r.PaperAcctH.ListPaperAccountSnapshots)

	// Symbol requests
	protected.Post("/symbol-requests", r.SymbolReqH.CreateSymbolRequest)
	protected.Get("/symbol-requests", r.SymbolReqH.ListMySymbolRequests)

	// Kite Publisher orders — JWT required
	protected.Get("/publisher/orders/:id", r.PubH.GetPublisherOrder)
	protected.Post("/publisher/orders/:id/mark-submitted", r.PubH.MarkPublisherOrderSubmitted)
}

// ErrorHandler is the global Fiber error handler — wire it via fiber.Config.ErrorHandler.
func ErrorHandler(c *fiber.Ctx, err error) error {
	if len(c.Response().Body()) > 0 {
		return nil
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		_ = c.Status(fe.Code).JSON(fiber.Map{"error": fe.Message})
		return fiber.NewError(fe.Code, fe.Message)
	}
	log.Printf("unhandled error: %v", err)
	_ = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	return fiber.NewError(fiber.StatusInternalServerError, "internal server error")
}

func (r *Router) health(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok", "time": time.Now().UTC()})
}
