package handler

// Swagger request/response types for OpenAPI generation.
// These structs mirror public API shapes; handlers may use inline structs with equivalent JSON tags.

// APIResponse wraps successful JSON payloads.
type APIResponse struct {
	Data interface{} `json:"data"`
}

// ErrorResponse is returned for all error paths.
type ErrorResponse struct {
	Error     string `json:"error" example:"invalid or missing token"`
	ErrorCode string `json:"error_code,omitempty" example:"account_suspended"`
}

// ── Auth ──────────────────────────────────────────────────────────────────────

type RegisterRequest struct {
	Email      string `json:"email" example:"trader@example.com"`
	Password   string `json:"password" example:"SecurePass123!"`
	InviteCode string `json:"invite_code,omitempty" example:"inv_abc123"`
}

type RegisterResponse struct {
	ID                        string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Email                     string `json:"email" example:"trader@example.com"`
	EmailVerificationRequired bool   `json:"email_verification_required,omitempty" example:"true"`
}

type LoginRequest struct {
	Email    string `json:"email" example:"trader@example.com"`
	Password string `json:"password" example:"SecurePass123!"`
}

type LoginResponse struct {
	Token     string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	ExpiresIn int    `json:"expires_in" example:"18000"`
	OrgID     string `json:"org_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440001"`
	OrgRole   string `json:"org_role,omitempty" example:"owner"`
}

type VerifyEmailRequest struct {
	Email string `json:"email" example:"trader@example.com"`
}

type MeResponse struct {
	ID                      string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Email                   string `json:"email" example:"trader@example.com"`
	Plan                    string `json:"plan" enums:"free,paper,pro,pro_plus" example:"pro"`
	Role                    string `json:"role" enums:"user,admin" example:"user"`
	Status                  string `json:"status" enums:"active,suspended" example:"active"`
	EmailVerified           bool   `json:"email_verified" example:"true"`
	CreatedAt               string `json:"created_at" example:"2026-06-23T10:00:00Z"`
	HasAutoPausedItems      bool   `json:"has_auto_paused_items" example:"false"`
	LiveTradingAllowed      bool   `json:"live_trading_allowed" example:"true"`
	MaxWebhooks             int    `json:"max_webhooks" example:"9"`
	MaxPaperWebhooks        int    `json:"max_paper_webhooks" example:"8"`
	MaxLiveWebhooks         int    `json:"max_live_webhooks" example:"1"`
	MaxBrokerCreds          int    `json:"max_broker_creds" example:"1"`
	MaxPaperAccounts        int    `json:"max_paper_accounts" example:"5"`
	OrdersPerSec            *int   `json:"orders_per_sec" example:"10"`
	OrdersPerSecEnforced    int    `json:"orders_per_sec_enforced" example:"10"`
	PaperWebhooksUsed       int    `json:"paper_webhooks_used" example:"1"`
	LiveWebhooksUsed        int    `json:"live_webhooks_used" example:"0"`
	PaperTradesUsedMonth    int    `json:"paper_trades_used_this_month" example:"42"`
	PaperTradeQuotaExceeded bool   `json:"paper_trade_quota_exceeded" example:"false"`
	MaxPaperTradesPerMonth  *int   `json:"max_paper_trades_per_month"`
}

// ── Billing ───────────────────────────────────────────────────────────────────

type CheckoutRequest struct {
	Plan string `json:"plan" enums:"paper,pro,pro_plus" example:"pro"`
}

type CheckoutResponse struct {
	URL string `json:"url" example:"https://checkout.stripe.com/pay/cs_test_..."`
}

type PortalResponse struct {
	URL string `json:"url" example:"https://billing.stripe.com/session/..."`
}

type PlanOffer struct {
	Plan                    string `json:"plan" example:"pro"`
	Name                    string `json:"name" example:"Pro"`
	PriceLabel              string `json:"price_label,omitempty" example:"₹999/mo"`
	MaxPaperAccounts        int    `json:"max_paper_accounts" example:"5"`
	MaxPaperWebhooks        int    `json:"max_paper_webhooks" example:"8"`
	MaxLiveWebhooks         int    `json:"max_live_webhooks" example:"1"`
	MaxBrokers              int    `json:"max_brokers" example:"1"`
	MaxWebhooks             int    `json:"max_webhooks" example:"9"`
	MaxPaperTradesPerMonth  *int   `json:"max_paper_trades_per_month"`
	OrdersPerSec            *int   `json:"orders_per_sec" example:"10"`
	LiveTrading             bool   `json:"live_trading" example:"true"`
	AuditLogs               bool   `json:"audit_logs" example:"true"`
	MultiProductCredentials bool   `json:"multi_product_credentials" example:"false"`
}

type BillingCatalog struct {
	BillingEnabled bool        `json:"billing_enabled" example:"true"`
	Plans          []PlanOffer `json:"plans"`
}

// ── Admin ───────────────────────────────────────────────────────────────────

type AdminPatchUserRequest struct {
	Status        string `json:"status,omitempty" enums:"active,suspended" example:"suspended"`
	EmailVerified *bool  `json:"email_verified,omitempty" example:"true"`
}

type AdminSetPlanRequest struct {
	Plan string `json:"plan" enums:"free,paper,pro,pro_plus" example:"pro"`
}

type AdminUserResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Plan          string `json:"plan"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	EmailVerified bool   `json:"email_verified"`
	BillingSource string `json:"billing_source"`
	CreatedAt     string `json:"created_at"`
}

type AdminPlatformInviteRequest struct {
	Email string `json:"email,omitempty" example:"trader@example.com"`
	Note  string `json:"note,omitempty" example:"beta invite"`
}

// ── Credentials ─────────────────────────────────────────────────────────────

type EnabledBrokersResponse struct {
	Brokers []string `json:"brokers" example:"paper,zerodha"`
}

type CreateCredentialRequest struct {
	BrokerType       string   `json:"broker_type" enums:"zerodha,angel,dhan,mt5_cloud,mock" example:"zerodha"`
	AccountLabel     string   `json:"account_label" example:"Zerodha Main"`
	AccountMode      string   `json:"account_mode" enums:"live" example:"live"`
	Exchange         string   `json:"exchange" example:"NSE"`
	Product          string   `json:"product,omitempty" example:"MIS"`
	Products         []string `json:"products,omitempty" example:"MIS,CNC"`
	AlgoID           string   `json:"algo_id,omitempty" example:"ALGO12345"`
	MarketProtection float64  `json:"market_protection,omitempty" example:"2"`
	RawCreds         string   `json:"raw_creds" example:"api_key:access_token"`
	ExecutionMode    string   `json:"execution_mode,omitempty" enums:"user_api_oauth,publisher,direct_api" example:"user_api_oauth"`
}

type CredentialResponse struct {
	ID            string `json:"id" example:"550e8400-e29b-41d4-a716-446655440003"`
	UserID        string `json:"user_id"`
	BrokerType    string `json:"broker_type" enums:"zerodha,angel,dhan,mt5_cloud,mock" example:"zerodha"`
	AccountLabel  string `json:"account_label" example:"Zerodha Main"`
	AccountMode   string `json:"account_mode" enums:"live" example:"live"`
	Exchange      string `json:"exchange" example:"NSE"`
	Product       string `json:"product" example:"MIS"`
	OrderType     string `json:"order_type" enums:"MARKET,LIMIT" example:"MARKET"`
	Status        string `json:"status" enums:"active,paused" example:"active"`
	AutoPaused    bool   `json:"auto_paused" example:"false"`
	AdminDisabled bool   `json:"admin_disabled" example:"false"`
}

type VerifyCredentialResponse struct {
	Valid   bool   `json:"valid" example:"true"`
	Status  string `json:"status" example:"active"`
	Message string `json:"message" example:"credentials verified"`
}

// ── Webhooks ──────────────────────────────────────────────────────────────────

type CreateWebhookRequest struct {
	Label            string   `json:"label" example:"Nifty Long Strategy"`
	BrokerCredID     string   `json:"broker_cred_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440003"`
	PaperAccountID   string   `json:"paper_account_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440010"`
	Symbol           string   `json:"symbol,omitempty" example:"NIFTY"`
	LotSize          float64  `json:"lot_size,omitempty" example:"1"`
	MaxRiskPct       float64  `json:"max_risk_pct,omitempty" example:"2"`
	SLPoints         int      `json:"sl_points,omitempty" example:"50"`
	TPPoints         int      `json:"tp_points,omitempty" example:"100"`
	DefaultOrderType string   `json:"default_order_type,omitempty" example:"MARKET"`
	AllowedActions   []string `json:"allowed_actions,omitempty" example:"BUY,SELL,CLOSE"`
	AllowedSymbols   []string `json:"allowed_symbols,omitempty"`
	MaxLotSize       float64  `json:"max_lot_size,omitempty" example:"5"`
	RateLimitPerSec  *int     `json:"rate_limit_per_sec,omitempty" example:"0"`
	RateLimitPerMin  *int     `json:"rate_limit_per_min,omitempty" example:"60"`
	DedupWindowSec   *int     `json:"dedup_window_sec,omitempty" example:"300"`
	Timezone         string   `json:"timezone,omitempty" example:"Asia/Kolkata"`
	RequiredComment  string   `json:"required_comment,omitempty" example:"strategy_v2"`
}

type WebhookResponse struct {
	ID              string   `json:"id" example:"550e8400-e29b-41d4-a716-446655440002"`
	UserID          string   `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Label           string   `json:"label" example:"Nifty Long Strategy"`
	Token           string   `json:"token" example:"3f2504e0-4f89-11d3-9a0c-0305e82c3301"`
	Status          string   `json:"status" enums:"active,paused" example:"active"`
	BrokerCredID    *string  `json:"broker_cred_id,omitempty"`
	PaperAccountID  *string  `json:"paper_account_id,omitempty"`
	Symbol          string   `json:"symbol" example:"NIFTY"`
	LotSize         float64  `json:"lot_size" example:"1"`
	AllowedActions  []string `json:"allowed_actions"`
	AllowedSymbols  []string `json:"allowed_symbols"`
	DedupWindowSec  int      `json:"dedup_window_sec" example:"300"`
	RateLimitPerSec int      `json:"rate_limit_per_sec" example:"0"`
	RateLimitPerMin int      `json:"rate_limit_per_min" example:"60"`
	AutoPaused      bool     `json:"auto_paused" example:"false"`
	AdminDisabled   bool     `json:"admin_disabled" example:"false"`
	CreatedAt       string   `json:"created_at" example:"2026-06-23T10:00:00Z"`
}

type RotateTokenResponse struct {
	Token string `json:"token" example:"3f2504e0-4f89-11d3-9a0c-0305e82c3302"`
}

// ── Signals ───────────────────────────────────────────────────────────────────

type SignalRequest struct {
	Action   string  `json:"action" enums:"BUY,SELL,CLOSE" example:"BUY"`
	Symbol   string  `json:"symbol" example:"NIFTY"`
	Quantity float64 `json:"quantity,omitempty" example:"1"`
	Price    float64 `json:"price,omitempty" example:"22500.5"`
	Comment  string  `json:"comment,omitempty" example:"strategy_v2_long"`
}

type IngestAcceptedResponse struct {
	Status       string `json:"status" example:"accepted"`
	RequestID    string `json:"request_id" example:"550e8400-e29b-41d4-a716-446655440004"`
	Queued       bool   `json:"queued" example:"true"`
	Deduplicated bool   `json:"deduplicated" example:"false"`
	Message      string `json:"message" example:"Webhook accepted for processing."`
}

type IngestDedupResponse struct {
	RequestID    string `json:"request_id" example:"550e8400-e29b-41d4-a716-446655440004"`
	Queued       bool   `json:"queued" example:"false"`
	Deduplicated bool   `json:"deduplicated" example:"true"`
}

// ── Trades ────────────────────────────────────────────────────────────────────

type TradeResponse struct {
	ID          string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440004"`
	WebhookID   string  `json:"webhook_id"`
	Symbol      string  `json:"symbol" example:"NIFTY"`
	Signal      string  `json:"signal" example:"BUY"`
	Status      string  `json:"status" enums:"queued,submitted,placed,filled,rejected,cancelled" example:"filled"`
	BrokerOrder string  `json:"broker_order,omitempty" example:"220613000000001"`
	FillPrice   float64 `json:"fill_price,omitempty" example:"22450.25"`
	AlgoID      string  `json:"algo_id,omitempty" example:"ALGO12345"`
	ErrorCode   string  `json:"error_code,omitempty" example:"algo_id_required"`
	CreatedAt   string  `json:"created_at" example:"2026-06-23T10:00:00Z"`
}

type PnLResponse struct {
	TotalPnL    float64 `json:"total_pnl" example:"1520.5"`
	TotalVolume float64 `json:"total_volume" example:"45000"`
	TradeCount  int     `json:"trade_count" example:"42"`
	WinRate     float64 `json:"win_rate" example:"0.62"`
}

// ── Paper accounts ──────────────────────────────────────────────────────────

type CreatePaperAccountRequest struct {
	Label           string  `json:"label" example:"Practice NIFTY"`
	StartingBalance float64 `json:"starting_balance" example:"100000"`
	Exchange        string  `json:"exchange,omitempty" example:"NSE"`
	DefaultProduct  string  `json:"default_product,omitempty" example:"MIS"`
	MarketProfile   string  `json:"market_profile,omitempty" example:"indian_equity"`
}

type PaperAccountResponse struct {
	ID              string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440010"`
	Label           string  `json:"label" example:"Practice NIFTY"`
	MarketProfile   string  `json:"market_profile" example:"indian_equity"`
	StartingBalance float64 `json:"starting_balance" example:"100000"`
	CashBalance     float64 `json:"cash_balance" example:"98500"`
	Exchange        string  `json:"exchange" example:"NSE"`
	DefaultProduct  string  `json:"default_product" example:"MIS"`
	Status          string  `json:"status" enums:"active,paused" example:"active"`
}

type ClosePaperPositionRequest struct {
	Symbol  string `json:"symbol" example:"NIFTY"`
	Product string `json:"product,omitempty" example:"MIS"`
}

type MarketProfileResponse struct {
	Code         string `json:"code" example:"indian_equity"`
	Name         string `json:"name" example:"Indian Equity"`
	BaseCurrency string `json:"base_currency" example:"INR"`
}

// ── Internal (adapter→Core) ───────────────────────────────────────────────────

type ExecutionEventRequest struct {
	RequestID     string  `json:"request_id" example:"550e8400-e29b-41d4-a716-446655440004"`
	Event         string  `json:"event" enums:"submitted,filled,rejected,cancelled" example:"filled"`
	BrokerOrderID string  `json:"broker_order_id,omitempty" example:"220613000000001"`
	FillPrice     float64 `json:"fill_price,omitempty" example:"22450.25"`
	OccurredAt    string  `json:"occurred_at,omitempty" example:"2026-06-23T10:00:01Z"`
}

type SessionUpdateRequest struct {
	EncryptedCreds string `json:"encrypted_creds" example:"base64-ciphertext"`
}

// ── Notifications & Telegram ─────────────────────────────────────────────────

type TelegramSettingsRequest struct {
	Enabled  *bool  `json:"enabled,omitempty" example:"true"`
	ChatID   string `json:"chat_id,omitempty" example:"123456789"`
	Username string `json:"username,omitempty" example:"mybot"`
}

type TelegramSettingsResponse struct {
	Enabled  bool   `json:"enabled" example:"false"`
	ChatID   string `json:"chat_id,omitempty"`
	Username string `json:"username,omitempty"`
}

// ── Symbol requests ─────────────────────────────────────────────────────────

type CreateSymbolRequest struct {
	Symbol   string `json:"symbol" example:"BANKNIFTY"`
	Exchange string `json:"exchange,omitempty" example:"NSE"`
	Note     string `json:"note,omitempty" example:"Need for weekly strategy"`
}

type AcceptInviteRequest struct {
	Token    string `json:"token" example:"inv_abc123"`
	Password string `json:"password,omitempty" example:"SecurePass123!"`
	Email    string `json:"email,omitempty" example:"trader@example.com"`
}
