package handler

// OpenAPI endpoint stubs — never called; annotations are parsed by `make swagger`.

// ── Monitoring ──────────────────────────────────────────────────────────────

// healthzDoc godoc
// @Summary     Health check
// @Description Returns server status and current UTC timestamp. Used by ECS target group health checks.
// @Tags        monitoring
// @Produce     json
// @Success     200 {object} map[string]string "status=ok"
// @Router      /healthz [get]
func healthzDoc() {}

// metricsDoc godoc
// @Summary     Prometheus metrics
// @Description Exposes Prometheus metrics in text format. Requires Bearer token if METRICS_TOKEN is set.
// @Tags        monitoring
// @Produce     plain
// @Security    BearerAuth
// @Success     200 {string} string "Prometheus text format"
// @Failure     401 {object} ErrorResponse
// @Router      /metrics [get]
func metricsDoc() {}

// ── Auth ────────────────────────────────────────────────────────────────────

// authRegisterDoc godoc
// @Summary     Register a new user
// @Description Creates an account with email and password. Closed registration requires a platform invite code.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body RegisterRequest true "Registration payload"
// @Success     201 {object} APIResponse{data=RegisterResponse}
// @Failure     400 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Failure     409 {object} ErrorResponse
// @Router      /v1/auth/register [post]
func authRegisterDoc() {}

// authLoginDoc godoc
// @Summary     Login
// @Description Returns a JWT access token (5 hour TTL). Email must be verified when EMAIL_VERIFICATION_REQUIRED is enabled.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body LoginRequest true "Credentials"
// @Success     200 {object} APIResponse{data=LoginResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/auth/login [post]
func authLoginDoc() {}

// authLogoutDoc godoc
// @Summary     Logout
// @Description Revokes the current JWT access token.
// @Tags        auth
// @Security    BearerAuth
// @Success     204 "No content"
// @Failure     401 {object} ErrorResponse
// @Router      /v1/auth/logout [post]
func authLogoutDoc() {}

// authVerifyEmailDoc godoc
// @Summary     Verify email address
// @Description Confirms email ownership using the token from the verification email link.
// @Tags        auth
// @Produce     json
// @Param       token query string true "Verification token"
// @Success     200 {object} APIResponse
// @Failure     400 {object} ErrorResponse
// @Router      /v1/auth/verify-email [get]
func authVerifyEmailDoc() {}

// authResendVerificationDoc godoc
// @Summary     Resend verification email
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body VerifyEmailRequest true "Email address"
// @Success     204 "No content"
// @Failure     400 {object} ErrorResponse
// @Failure     429 {object} ErrorResponse
// @Router      /v1/auth/resend-verification [post]
func authResendVerificationDoc() {}

// ── Account ─────────────────────────────────────────────────────────────────

// meDoc godoc
// @Summary     Current user profile
// @Description Returns profile, effective plan, and usage counters (webhooks, paper trades).
// @Tags        account
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse{data=MeResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/me [get]
func meDoc() {}

// ── Invites ─────────────────────────────────────────────────────────────────

// invitePreviewDoc godoc
// @Summary     Preview platform invite
// @Tags        auth
// @Produce     json
// @Param       token path string true "Invite token"
// @Success     200 {object} APIResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/invites/{token} [get]
func invitePreviewDoc() {}

// inviteAcceptDoc godoc
// @Summary     Accept platform invite
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body AcceptInviteRequest true "Invite acceptance"
// @Success     200 {object} APIResponse
// @Failure     400 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/invites/accept [post]
func inviteAcceptDoc() {}

// ── Signals ─────────────────────────────────────────────────────────────────

// ingestSignalDoc godoc
// @Summary     Ingest trade signal
// @Description Receives a TradingView webhook signal and enqueues it for execution. Returns 202 before broker is called. Duplicate signals within dedup_window_sec return 409 with deduplicated=true.
// @Tags        signals
// @Accept      json
// @Produce     json
// @Param       token path string true "Webhook ingest token (UUID v4)"
// @Param       body body SignalRequest true "Trade signal payload"
// @Success     202 {object} IngestAcceptedResponse
// @Success     409 {object} IngestDedupResponse "Duplicate within dedup window"
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Failure     429 {object} ErrorResponse
// @Failure     503 {object} ErrorResponse
// @Router      /v1/webhook/{token} [post]
func ingestSignalDoc() {}

// ── Streaming ─────────────────────────────────────────────────────────────────

// wsTradesDoc godoc
// @Summary     Live trade WebSocket
// @Description Streams real-time trade updates. Pass JWT as query param ?token= because headers cannot be set during WebSocket upgrade.
// @Tags        streaming
// @Param       token query string true "JWT access token"
// @Success     101 "Switching Protocols"
// @Failure     401 {object} ErrorResponse
// @Router      /v1/ws/trades [get]
func wsTradesDoc() {}

// ── Webhooks ──────────────────────────────────────────────────────────────────

// listWebhooksDoc godoc
// @Summary     List webhooks
// @Description Returns all webhooks owned by the authenticated user.
// @Tags        webhooks
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse{data=[]WebhookResponse}
// @Failure     401 {object} ErrorResponse
// @Router      /v1/webhooks [get]
func listWebhooksDoc() {}

// createWebhookDoc godoc
// @Summary     Create webhook
// @Description Creates a webhook with a unique ingest token. Set either broker_cred_id (live) or paper_account_id (paper). Plan limits apply per tier (free, paper, pro, pro_plus).
// @Tags        webhooks
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body CreateWebhookRequest true "Webhook configuration"
// @Success     201 {object} APIResponse{data=WebhookResponse}
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/webhooks [post]
func createWebhookDoc() {}

// getWebhookDoc godoc
// @Summary     Get webhook
// @Tags        webhooks
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Webhook ID"
// @Success     200 {object} APIResponse{data=WebhookResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/webhooks/{id} [get]
func getWebhookDoc() {}

// updateWebhookDoc godoc
// @Summary     Update webhook
// @Tags        webhooks
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Webhook ID"
// @Param       body body CreateWebhookRequest true "Fields to update"
// @Success     200 {object} APIResponse{data=WebhookResponse}
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/webhooks/{id} [put]
func updateWebhookDoc() {}

// deleteWebhookDoc godoc
// @Summary     Delete webhook
// @Tags        webhooks
// @Security    BearerAuth
// @Param       id path string true "Webhook ID"
// @Success     204 "No content"
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/webhooks/{id} [delete]
func deleteWebhookDoc() {}

// pauseWebhookDoc godoc
// @Summary     Pause webhook
// @Tags        webhooks
// @Security    BearerAuth
// @Param       id path string true "Webhook ID"
// @Success     200 {object} APIResponse{data=WebhookResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/webhooks/{id}/pause [put]
func pauseWebhookDoc() {}

// resumeWebhookDoc godoc
// @Summary     Resume webhook
// @Tags        webhooks
// @Security    BearerAuth
// @Param       id path string true "Webhook ID"
// @Success     200 {object} APIResponse{data=WebhookResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/webhooks/{id}/resume [put]
func resumeWebhookDoc() {}

// rotateWebhookTokenDoc godoc
// @Summary     Rotate webhook ingest token
// @Description Generates a new ingest URL token; the old token is rejected immediately.
// @Tags        webhooks
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Webhook ID"
// @Success     200 {object} APIResponse{data=RotateTokenResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/webhooks/{id}/rotate-token [post]
func rotateWebhookTokenDoc() {}

// listWebhookTradesDoc godoc
// @Summary     List webhook trades
// @Tags        trades
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Webhook ID"
// @Param       limit query int false "Max rows (default 50)"
// @Success     200 {object} APIResponse{data=[]TradeResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/webhooks/{id}/trades [get]
func listWebhookTradesDoc() {}

// exportWebhookTradesDoc godoc
// @Summary     Export webhook trades CSV
// @Tags        trades
// @Produce     text/csv
// @Security    BearerAuth
// @Param       id path string true "Webhook ID"
// @Success     200 {string} string "CSV file"
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/webhooks/{id}/trades/export [get]
func exportWebhookTradesDoc() {}

// cancelTradeDoc godoc
// @Summary     Cancel trade
// @Description Attempts to cancel a broker-side order for the given trade.
// @Tags        trades
// @Security    BearerAuth
// @Param       id path string true "Webhook ID"
// @Param       tradeId path string true "Trade ID"
// @Success     200 {object} APIResponse{data=TradeResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/webhooks/{id}/trades/{tradeId} [delete]
func cancelTradeDoc() {}

// webhookPnLDoc godoc
// @Summary     Webhook P&L summary
// @Tags        trades
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Webhook ID"
// @Success     200 {object} APIResponse{data=PnLResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/webhooks/{id}/pnl [get]
func webhookPnLDoc() {}

// getTradeDoc godoc
// @Summary     Get trade detail
// @Tags        trades
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Trade ID"
// @Success     200 {object} APIResponse{data=TradeResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/trades/{id} [get]
func getTradeDoc() {}

// ── Credentials ───────────────────────────────────────────────────────────────

// listEnabledBrokersDoc godoc
// @Summary     List enabled brokers
// @Description Returns broker types enabled on this deployment (ENABLED_ADAPTERS).
// @Tags        credentials
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse{data=EnabledBrokersResponse}
// @Failure     401 {object} ErrorResponse
// @Router      /v1/brokers/enabled [get]
func listEnabledBrokersDoc() {}

// listCredentialsDoc godoc
// @Summary     List broker credentials
// @Description Never returns raw_creds or encrypted session material.
// @Tags        credentials
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse{data=[]CredentialResponse}
// @Failure     401 {object} ErrorResponse
// @Router      /v1/credentials [get]
func listCredentialsDoc() {}

// createCredentialDoc godoc
// @Summary     Create broker credential
// @Description Adds a live broker credential. account_mode must be live; requires a plan with live_trading_allowed.
// @Tags        credentials
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body CreateCredentialRequest true "Credential payload"
// @Success     201 {object} APIResponse{data=CredentialResponse}
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/credentials [post]
func createCredentialDoc() {}

// updateCredentialDoc godoc
// @Summary     Update broker credential
// @Tags        credentials
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Credential ID"
// @Param       body body CreateCredentialRequest true "Fields to update"
// @Success     200 {object} APIResponse{data=CredentialResponse}
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/credentials/{id} [put]
func updateCredentialDoc() {}

// deleteCredentialDoc godoc
// @Summary     Delete broker credential
// @Tags        credentials
// @Security    BearerAuth
// @Param       id path string true "Credential ID"
// @Success     204 "No content"
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/credentials/{id} [delete]
func deleteCredentialDoc() {}

// verifyCredentialDoc godoc
// @Summary     Verify broker credential
// @Description Probes the broker API to validate stored credentials.
// @Tags        credentials
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Credential ID"
// @Success     200 {object} APIResponse{data=VerifyCredentialResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/credentials/{id}/verify [post]
func verifyCredentialDoc() {}

// pauseCredentialDoc godoc
// @Summary     Pause broker credential
// @Tags        credentials
// @Security    BearerAuth
// @Param       id path string true "Credential ID"
// @Success     200 {object} APIResponse{data=CredentialResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/credentials/{id}/pause [put]
func pauseCredentialDoc() {}

// resumeCredentialDoc godoc
// @Summary     Resume broker credential
// @Tags        credentials
// @Security    BearerAuth
// @Param       id path string true "Credential ID"
// @Success     200 {object} APIResponse{data=CredentialResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/credentials/{id}/resume [put]
func resumeCredentialDoc() {}

// zerodhaConnectDoc godoc
// @Summary     Initiate Zerodha OAuth
// @Tags        credentials
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse "Redirect URL in data"
// @Failure     401 {object} ErrorResponse
// @Router      /v1/credentials/zerodha/connect [get]
func zerodhaConnectDoc() {}

// zerodhaConnectInfoDoc godoc
// @Summary     Zerodha connect metadata
// @Tags        credentials
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Router      /v1/credentials/zerodha/connect-info [get]
func zerodhaConnectInfoDoc() {}

// zerodhaStatusDoc godoc
// @Summary     Zerodha OAuth session status
// @Tags        credentials
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Router      /v1/credentials/zerodha/status [get]
func zerodhaStatusDoc() {}

// zerodhaCallbackDoc godoc
// @Summary     Zerodha OAuth callback
// @Description Browser redirect from Kite Connect; public endpoint.
// @Tags        credentials
// @Param       request_token query string true "Kite request token"
// @Param       state query string true "OAuth state"
// @Success     302 "Redirect to dashboard"
// @Router      /v1/credentials/zerodha/callback [get]
func zerodhaCallbackDoc() {}

// ── Paper accounts ────────────────────────────────────────────────────────────

// listMarketProfilesDoc godoc
// @Summary     List active market profiles
// @Tags        paper
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse{data=[]MarketProfileResponse}
// @Failure     401 {object} ErrorResponse
// @Router      /v1/market-profiles [get]
func listMarketProfilesDoc() {}

// listPaperAccountsDoc godoc
// @Summary     List paper accounts
// @Tags        paper
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse{data=[]PaperAccountResponse}
// @Failure     401 {object} ErrorResponse
// @Router      /v1/paper-accounts [get]
func listPaperAccountsDoc() {}

// createPaperAccountDoc godoc
// @Summary     Create paper account
// @Tags        paper
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body CreatePaperAccountRequest true "Paper account"
// @Success     201 {object} APIResponse{data=PaperAccountResponse}
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/paper-accounts [post]
func createPaperAccountDoc() {}

// getPaperAccountDoc godoc
// @Summary     Get paper account
// @Tags        paper
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Success     200 {object} APIResponse{data=PaperAccountResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id} [get]
func getPaperAccountDoc() {}

// updatePaperAccountDoc godoc
// @Summary     Update paper account
// @Tags        paper
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Param       body body CreatePaperAccountRequest true "Fields to update"
// @Success     200 {object} APIResponse{data=PaperAccountResponse}
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id} [patch]
func updatePaperAccountDoc() {}

// deletePaperAccountDoc godoc
// @Summary     Delete paper account
// @Tags        paper
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Success     204 "No content"
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id} [delete]
func deletePaperAccountDoc() {}

// pausePaperAccountDoc godoc
// @Summary     Pause paper account
// @Tags        paper
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Success     200 {object} APIResponse{data=PaperAccountResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id}/pause [put]
func pausePaperAccountDoc() {}

// resumePaperAccountDoc godoc
// @Summary     Resume paper account
// @Tags        paper
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Success     200 {object} APIResponse{data=PaperAccountResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id}/resume [put]
func resumePaperAccountDoc() {}

// resetPaperAccountDoc godoc
// @Summary     Reset paper account
// @Description Resets cash balance and clears open positions.
// @Tags        paper
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Success     200 {object} APIResponse{data=PaperAccountResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id}/reset [put]
func resetPaperAccountDoc() {}

// listPaperPositionsDoc godoc
// @Summary     List paper positions
// @Tags        paper
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id}/positions [get]
func listPaperPositionsDoc() {}

// closePaperPositionDoc godoc
// @Summary     Close paper position
// @Tags        paper
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Param       body body ClosePaperPositionRequest true "Position to close"
// @Success     200 {object} APIResponse
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id}/positions/close [post]
func closePaperPositionDoc() {}

// listPaperOrdersDoc godoc
// @Summary     List paper orders
// @Tags        paper
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id}/orders [get]
func listPaperOrdersDoc() {}

// exportPaperOrdersDoc godoc
// @Summary     Export paper orders CSV
// @Tags        paper
// @Produce     text/csv
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Success     200 {string} string "CSV file"
// @Failure     401 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id}/orders/export [get]
func exportPaperOrdersDoc() {}

// paperAccountPnLDoc godoc
// @Summary     Paper account P&L
// @Tags        paper
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Success     200 {object} APIResponse{data=PnLResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id}/pnl [get]
func paperAccountPnLDoc() {}

// listPaperSnapshotsDoc godoc
// @Summary     List paper account snapshots
// @Tags        paper
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Paper account ID"
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Router      /v1/paper-accounts/{id}/snapshots [get]
func listPaperSnapshotsDoc() {}

// ── Billing ───────────────────────────────────────────────────────────────────

// billingPlansDoc godoc
// @Summary     List billing plans
// @Description Returns plan matrix: free, paper, pro, pro_plus with limits.
// @Tags        billing
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse{data=BillingCatalog}
// @Failure     401 {object} ErrorResponse
// @Router      /v1/billing/plans [get]
func billingPlansDoc() {}

// billingCheckoutDoc godoc
// @Summary     Create Stripe Checkout session
// @Tags        billing
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body CheckoutRequest true "Target plan"
// @Success     200 {object} APIResponse{data=CheckoutResponse}
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     503 {object} ErrorResponse
// @Router      /v1/billing/checkout [post]
func billingCheckoutDoc() {}

// billingPortalDoc godoc
// @Summary     Create Stripe Customer Portal session
// @Tags        billing
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse{data=PortalResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     503 {object} ErrorResponse
// @Router      /v1/billing/portal [post]
func billingPortalDoc() {}

// stripeWebhookDoc godoc
// @Summary     Stripe webhook handler
// @Description Receives Stripe billing events. Verified via Stripe-Signature header.
// @Tags        billing
// @Accept      json
// @Success     200 "OK"
// @Failure     400 {object} ErrorResponse
// @Router      /v1/webhooks/stripe [post]
func stripeWebhookDoc() {}

// ── Internal ──────────────────────────────────────────────────────────────────

// postExecutionEventDoc godoc
// @Summary     Report execution event
// @Description Adapter→Core callback when order status changes. Requires service token.
// @Tags        internal
// @Accept      json
// @Produce     json
// @Security    ServiceToken
// @Param       body body ExecutionEventRequest true "Execution event"
// @Success     200 {object} APIResponse
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/internal/execution-events [post]
func postExecutionEventDoc() {}

// putCredentialSessionDoc godoc
// @Summary     Update broker session
// @Description Adapter→Core callback after OAuth token exchange. Requires service token.
// @Tags        internal
// @Accept      json
// @Produce     json
// @Security    ServiceToken
// @Param       id path string true "Credential ID"
// @Param       body body SessionUpdateRequest true "Encrypted session"
// @Success     200 {object} APIResponse
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/internal/credentials/{id}/session [put]
func putCredentialSessionDoc() {}

// ── Notifications ─────────────────────────────────────────────────────────────

// listNotificationsDoc godoc
// @Summary     List notifications
// @Tags        notifications
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Router      /v1/notifications [get]
func listNotificationsDoc() {}

// unreadNotificationsDoc godoc
// @Summary     Unread notification count
// @Tags        notifications
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Router      /v1/notifications/unread-count [get]
func unreadNotificationsDoc() {}

// markNotificationReadDoc godoc
// @Summary     Mark notification read
// @Tags        notifications
// @Security    BearerAuth
// @Param       id path string true "Notification ID"
// @Success     204 "No content"
// @Failure     401 {object} ErrorResponse
// @Router      /v1/notifications/{id}/read [patch]
func markNotificationReadDoc() {}

// markAllNotificationsReadDoc godoc
// @Summary     Mark all notifications read
// @Tags        notifications
// @Security    BearerAuth
// @Success     204 "No content"
// @Failure     401 {object} ErrorResponse
// @Router      /v1/notifications/read-all [post]
func markAllNotificationsReadDoc() {}

// getTelegramSettingsDoc godoc
// @Summary     Get Telegram alert settings
// @Tags        notifications
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse{data=TelegramSettingsResponse}
// @Failure     401 {object} ErrorResponse
// @Router      /v1/settings/telegram [get]
func getTelegramSettingsDoc() {}

// updateTelegramSettingsDoc godoc
// @Summary     Update Telegram alert settings
// @Tags        notifications
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body TelegramSettingsRequest true "Telegram settings"
// @Success     200 {object} APIResponse{data=TelegramSettingsResponse}
// @Failure     401 {object} ErrorResponse
// @Router      /v1/settings/telegram [patch]
func updateTelegramSettingsDoc() {}

// ── Symbol requests ───────────────────────────────────────────────────────────

// createSymbolRequestDoc godoc
// @Summary     Request a new tradable symbol
// @Tags        symbols
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body CreateSymbolRequest true "Symbol request"
// @Success     201 {object} APIResponse
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Router      /v1/symbol-requests [post]
func createSymbolRequestDoc() {}

// listSymbolRequestsDoc godoc
// @Summary     List my symbol requests
// @Tags        symbols
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Router      /v1/symbol-requests [get]
func listSymbolRequestsDoc() {}

// getPublisherOrderDoc godoc
// @Summary     Get Kite Publisher order status
// @Tags        trades
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Publisher order ID"
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/publisher/orders/{id} [get]
func getPublisherOrderDoc() {}

// ── Admin ─────────────────────────────────────────────────────────────────────

// adminListUsersDoc godoc
// @Summary     List all users
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse{data=[]AdminUserResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/users [get]
func adminListUsersDoc() {}

// adminGetUserDoc godoc
// @Summary     Get user detail
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "User ID"
// @Success     200 {object} APIResponse{data=AdminUserResponse}
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Failure     404 {object} ErrorResponse
// @Router      /v1/admin/users/{id} [get]
func adminGetUserDoc() {}

// adminPatchUserDoc godoc
// @Summary     Patch user
// @Tags        admin
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "User ID"
// @Param       body body AdminPatchUserRequest true "Fields to patch"
// @Success     200 {object} APIResponse{data=AdminUserResponse}
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/users/{id} [patch]
func adminPatchUserDoc() {}

// adminDeleteUserDoc godoc
// @Summary     Delete user
// @Tags        admin
// @Security    BearerAuth
// @Param       id path string true "User ID"
// @Success     204 "No content"
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/users/{id} [delete]
func adminDeleteUserDoc() {}

// adminSetUserPlanDoc godoc
// @Summary     Set user plan
// @Description Sets billing_source to admin. Plans: free, paper, pro, pro_plus.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "User ID"
// @Param       body body AdminSetPlanRequest true "Target plan"
// @Success     200 {object} APIResponse{data=AdminUserResponse}
// @Failure     400 {object} ErrorResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/users/{id}/plan [put]
func adminSetUserPlanDoc() {}

// adminListUserAuditDoc godoc
// @Summary     List user audit log
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "User ID"
// @Param       limit query int false "Max entries (default 50)"
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/users/{id}/audit [get]
func adminListUserAuditDoc() {}

// adminCreateInviteDoc godoc
// @Summary     Create platform invite
// @Tags        admin
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body AdminPlatformInviteRequest true "Invite"
// @Success     201 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/invites [post]
func adminCreateInviteDoc() {}

// adminListInvitesDoc godoc
// @Summary     List platform invites
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/invites [get]
func adminListInvitesDoc() {}

// adminRevokeInviteDoc godoc
// @Summary     Revoke platform invite
// @Tags        admin
// @Security    BearerAuth
// @Param       id path string true "Invite ID"
// @Success     204 "No content"
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/invites/{id} [delete]
func adminRevokeInviteDoc() {}

// adminGetMarketDataProviderDoc godoc
// @Summary     Get market data provider setting
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/settings/market-data-provider [get]
func adminGetMarketDataProviderDoc() {}

// adminSetMarketDataProviderDoc godoc
// @Summary     Set market data provider
// @Tags        admin
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/settings/market-data-provider [put]
func adminSetMarketDataProviderDoc() {}

// adminGetMarketDataIntervalDoc godoc
// @Summary     Get market data interval setting
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/settings/market-data-interval [get]
func adminGetMarketDataIntervalDoc() {}

// adminSetMarketDataIntervalDoc godoc
// @Summary     Set market data interval
// @Tags        admin
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/settings/market-data-interval [put]
func adminSetMarketDataIntervalDoc() {}

// adminGetFyersStatusDoc godoc
// @Summary     FYERS admin connection status
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/fyers/status [get]
func adminGetFyersStatusDoc() {}

// adminFyersConnectDoc godoc
// @Summary     Initiate FYERS admin OAuth
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/fyers/connect [get]
func adminFyersConnectDoc() {}

// adminFyersCallbackDoc godoc
// @Summary     FYERS OAuth callback
// @Description Browser redirect from FYERS; public endpoint.
// @Tags        admin
// @Success     302 "Redirect"
// @Router      /v1/admin/fyers/callback [get]
func adminFyersCallbackDoc() {}

// adminSetFyersPINDoc godoc
// @Summary     Set FYERS PIN
// @Tags        admin
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/fyers/pin [put]
func adminSetFyersPINDoc() {}

// adminListMarketProfilesDoc godoc
// @Summary     List market profiles (admin)
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/market-profiles [get]
func adminListMarketProfilesDoc() {}

// adminUpdateMarketProfileDoc godoc
// @Summary     Update market profile
// @Tags        admin
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       code path string true "Market profile code"
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/market-profiles/{code} [patch]
func adminUpdateMarketProfileDoc() {}

// adminListInstrumentsDoc godoc
// @Summary     List instruments
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/instruments [get]
func adminListInstrumentsDoc() {}

// adminCreateInstrumentDoc godoc
// @Summary     Create instrument
// @Tags        admin
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Success     201 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/instruments [post]
func adminCreateInstrumentDoc() {}

// adminUpdateInstrumentDoc godoc
// @Summary     Update instrument
// @Tags        admin
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Instrument ID"
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/instruments/{id} [patch]
func adminUpdateInstrumentDoc() {}

// adminDeleteInstrumentDoc godoc
// @Summary     Delete instrument
// @Tags        admin
// @Security    BearerAuth
// @Param       id path string true "Instrument ID"
// @Success     204 "No content"
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/instruments/{id} [delete]
func adminDeleteInstrumentDoc() {}

// adminGetMarketHoursEnforcementDoc godoc
// @Summary     Get market hours enforcement setting
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/settings/market-hours-enforcement [get]
func adminGetMarketHoursEnforcementDoc() {}

// adminSetMarketHoursEnforcementDoc godoc
// @Summary     Set market hours enforcement
// @Tags        admin
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/settings/market-hours-enforcement [put]
func adminSetMarketHoursEnforcementDoc() {}

// adminListSymbolRequestsDoc godoc
// @Summary     List symbol requests (admin queue)
// @Tags        admin
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/symbol-requests [get]
func adminListSymbolRequestsDoc() {}

// adminAcceptSymbolRequestDoc godoc
// @Summary     Accept symbol request
// @Tags        admin
// @Security    BearerAuth
// @Param       id path string true "Symbol request ID"
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/symbol-requests/{id}/accept [post]
func adminAcceptSymbolRequestDoc() {}

// adminRejectSymbolRequestDoc godoc
// @Summary     Reject symbol request
// @Tags        admin
// @Security    BearerAuth
// @Param       id path string true "Symbol request ID"
// @Success     200 {object} APIResponse
// @Failure     401 {object} ErrorResponse
// @Failure     403 {object} ErrorResponse
// @Router      /v1/admin/symbol-requests/{id}/reject [post]
func adminRejectSymbolRequestDoc() {}
