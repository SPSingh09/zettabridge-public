package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"time"

	"github.com/SPSingh09/zettabridge/internal/platform/crypto"
)

const (
	BrokerModeMock = "mock"
	BrokerModeLive = "live"
)

// Config holds all runtime configuration loaded from environment variables.
// In production, inject these via AWS ECS task definitions or a .env loader.
type Config struct {
	Port                      string
	DatabaseURL               string
	RedisURL                  string
	JWTSecret                 string
	CORSOrigins               string
	WorkerCount               int
	QueueBuffer               int
	AESKey                    string // 32-byte hex key for AES-256-GCM credential encryption
	BootstrapAdminEmail       string // creates + promotes admin on startup if not exists
	BootstrapAdminPassword    string // password used when creating the bootstrap admin account
	DefaultOrgSeatLimit       int    // default seats when creating an organization
	OrgInviteTTLDays          int    // days until an org invite expires
	ClosedRegistration        bool   // when true, POST /v1/auth/register requires a valid invite_code
	PlatformInviteTTLDays     int    // days until a platform invite expires (default: 30)
	BrokerMode                string // mock | live
	EnabledAdapters           []string
	ExecutionAdapterMode      string // local | http (phase 1: local only)
	BrokerHTTPTimeoutSec      int
	BrokerHTTPGetRetries      int
	BrokerMT5BaseURL          string
	BrokerMT5DemoBaseURL      string
	BrokerZerodhaBaseURL      string
	BrokerZerodhaDemoBaseURL  string
	BrokerAngelBaseURL        string
	BrokerAngelDemoBaseURL    string
	BrokerDhanBaseURL         string
	BrokerDhanDemoBaseURL     string
	BrokerAngelClientLocalIP  string // SmartAPI X-ClientLocalIP (server egress; shared for all users)
	BrokerAngelClientPublicIP string // SmartAPI X-ClientPublicIP
	BrokerAngelMACAddress     string // SmartAPI X-MACAddress
	MetricsEnabled            bool
	MetricsToken              string // optional bearer token for GET /metrics
	EmailVerificationRequired bool
	EmailVerificationTTLHours int
	EmailProvider             string // log | smtp
	EmailFrom                 string
	AppPublicURL              string
	FrontendURL               string // FRONTEND_URL — base URL of the dashboard (for invite/email links); defaults to AppPublicURL
	SMTPHost                  string
	SMTPPort                  int
	SMTPUser                  string
	SMTPPass                  string
	SebiAlgoIDRequired        bool
	BrokerZerodhaAlgoID       string
	BrokerAngelAlgoID         string
	BrokerDhanAlgoID          string
	BillingEnabled            bool
	StripeSecretKey           string
	StripeWebhookSecret       string
	StripePricePaper          string
	StripePricePro            string
	StripePriceProPlus        string
	BillingPlanPriceLabels    map[string]string // optional display labels from BILLING_PLAN_PRICE_LABELS
	BillingSuccessURL         string // default: APP_PUBLIC_URL/billing/success?session_id={CHECKOUT_SESSION_ID}
	BillingCancelURL          string // default: APP_PUBLIC_URL/billing/cancel
	BillingPortalReturnURL    string // default: BILLING_CANCEL_URL or APP_PUBLIC_URL/billing/cancel

	// Zerodha Kite Connect OAuth (one app for all ZettaBridge users)
	ZerodhaAPIKey      string // ZERODHA_API_KEY — from developers.kite.trade
	ZerodhaAPISecret   string // ZERODHA_API_SECRET — never stored in DB
	ZerodhaCallbackURL string // ZERODHA_CALLBACK_URL — must match kite app redirect URL
	ZerodhaFrontendURL string // ZERODHA_FRONTEND_URL — where to redirect browser after OAuth

	// Zerodha Kite Publisher (browser-redirect order confirmation flow)
	ZerodhaPublisherEnabled     bool   // ZERODHA_PUBLISHER_ENABLED — gates publisher credential creation and execution
	ZerodhaPublisherCallbackURL string // ZERODHA_PUBLISHER_CALLBACK_URL — e.g. https://api.staging.zettabridge.net/v1/publisher/callback

	// Execution adapter URLs (phase 2+; HTTP transport in phase 3)
	ZBServiceToken     string
	PaperAdapterURL    string
	ZerodhaAdapterURL  string
	AngelAdapterURL    string
	DhanAdapterURL     string
	MT5AdapterURL      string
	CoreInternalURL    string // adapter → core internal API base

	TelegramBotToken string // TELEGRAM_BOT_TOKEN — empty disables Telegram alerts

	// Symbol request notifications (admin/support side) — both optional;
	// an empty value just skips that channel, the admin dashboard banner
	// always works regardless.
	SupportNotificationEmail string // SUPPORT_NOTIFICATION_EMAIL
	SupportTelegramChatID    string // SUPPORT_TELEGRAM_CHAT_ID

	// Paper Trading market data / mark-to-market (Phase 6)
	MarketDataProvider            string // MARKET_DATA_PROVIDER — signal | fyers (default: signal — no external feed until FYERS is connected via the admin page)
	MarketDataSnapshotIntervalSec int    // MARKET_DATA_SNAPSHOT_INTERVAL_SEC — background sweep cadence (default: 45, within the planned 30-60s range)

	// FYERS market-data-only OAuth app (platform-wide, admin-connected via
	// the admin dashboard — never used for order placement). AppID/SecretID
	// are static deploy-time app registration values; the actual
	// access/refresh tokens are obtained via admin OAuth login and stored
	// encrypted in the fyers_connection table, not here.
	FyersAppID           string // FYERS_APP_ID — FYERS "App ID" / client_id
	FyersSecretID        string // FYERS_SECRET_ID — FYERS "Secret ID", never exposed to the frontend
	FyersCallbackURL     string // FYERS_CALLBACK_URL — e.g. https://api.staging.zettabridge.net/v1/admin/fyers/callback
	FyersRefreshInterval int    // FYERS_REFRESH_INTERVAL_MIN — how often the background job attempts a silent token refresh (default: 60)

	// Zerodha Kite Connect fill polling for OAuth trades (submitted → filled/rejected).
	ZerodhaFillPollIntervalSec int // ZERODHA_FILL_POLL_INTERVAL_SEC — 0 disables (default: 10)
}

func Load() *Config {
	brokerMode := normalizeBrokerMode(getEnv("BROKER_MODE", BrokerModeMock))
	sebiRequired := brokerMode == BrokerModeLive
	if v := os.Getenv("SEBI_ALGO_ID_REQUIRED"); v != "" {
		sebiRequired = getEnvBool("SEBI_ALGO_ID_REQUIRED", sebiRequired)
	}
	return &Config{
		Port:                      getEnv("PORT", "8080"),
		DatabaseURL:               getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/zettabridge?sslmode=disable"),
		RedisURL:                  getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:                 getEnv("JWT_SECRET", "change-me-in-production-min-32-chars!!"),
		CORSOrigins:               getEnv("CORS_ORIGINS", "http://localhost:3000"),
		WorkerCount:               getEnvInt("WORKER_COUNT", 20),
		QueueBuffer:               getEnvInt("QUEUE_BUFFER", 500),
		AESKey:                    getEnv("AES_KEY", "00000000000000000000000000000000"), // 32 hex bytes
		BootstrapAdminEmail:       getEnv("BOOTSTRAP_ADMIN_EMAIL", ""),
		BootstrapAdminPassword:    getEnv("BOOTSTRAP_ADMIN_PASSWORD", ""),
		DefaultOrgSeatLimit:       getEnvInt("DEFAULT_ORG_SEAT_LIMIT", 5),
		OrgInviteTTLDays:          getEnvInt("ORG_INVITE_TTL_DAYS", 7),
		ClosedRegistration:        getEnvBool("CLOSED_REGISTRATION", false),
		PlatformInviteTTLDays:     getEnvInt("PLATFORM_INVITE_TTL_DAYS", 30),
		BrokerMode:                brokerMode,
		EnabledAdapters:           parseEnabledAdapters(getEnv("ENABLED_ADAPTERS", "paper,zerodha,angel,dhan,mt5")),
		ExecutionAdapterMode:      strings.ToLower(getEnv("EXECUTION_ADAPTER_MODE", "local")),
		BrokerHTTPTimeoutSec:      getEnvInt("BROKER_HTTP_TIMEOUT_SEC", 8),
		BrokerHTTPGetRetries:      getEnvInt("BROKER_HTTP_GET_RETRIES", 2),
		BrokerMT5BaseURL:          getEnv("BROKER_MT5_BASE_URL", ""),
		BrokerMT5DemoBaseURL:      getEnv("BROKER_MT5_DEMO_BASE_URL", ""),
		BrokerZerodhaBaseURL:      getEnv("BROKER_ZERODHA_BASE_URL", ""),
		BrokerZerodhaDemoBaseURL:  getEnv("BROKER_ZERODHA_DEMO_BASE_URL", ""),
		BrokerAngelBaseURL:        getEnv("BROKER_ANGEL_BASE_URL", ""),
		BrokerAngelDemoBaseURL:    getEnv("BROKER_ANGEL_DEMO_BASE_URL", ""),
		BrokerDhanBaseURL:         getEnv("BROKER_DHAN_BASE_URL", ""),
		BrokerDhanDemoBaseURL:     getEnv("BROKER_DHAN_DEMO_BASE_URL", ""),
		BrokerAngelClientLocalIP:  getEnv("BROKER_ANGEL_CLIENT_LOCAL_IP", "127.0.0.1"),
		BrokerAngelClientPublicIP: getEnv("BROKER_ANGEL_CLIENT_PUBLIC_IP", "127.0.0.1"),
		BrokerAngelMACAddress:     getEnv("BROKER_ANGEL_MAC_ADDRESS", "00:00:00:00:00:00"),
		MetricsEnabled:            getEnvBool("METRICS_ENABLED", true),
		MetricsToken:              getEnv("METRICS_TOKEN", ""),
		EmailVerificationRequired: getEnvBool("EMAIL_VERIFICATION_REQUIRED", true),
		EmailVerificationTTLHours: getEnvInt("EMAIL_VERIFICATION_TTL_HOURS", 24),
		EmailProvider:             getEnv("EMAIL_PROVIDER", "log"),
		EmailFrom:                 getEnv("EMAIL_FROM", "noreply@localhost"),
		AppPublicURL:              strings.TrimRight(getEnv("APP_PUBLIC_URL", "http://localhost:8080"), "/"),
		FrontendURL:               strings.TrimRight(getEnv("FRONTEND_URL", getEnv("APP_PUBLIC_URL", "http://localhost:3000")), "/"),
		SMTPHost:                  getEnv("SMTP_HOST", ""),
		SMTPPort:                  getEnvInt("SMTP_PORT", 587),
		SMTPUser:                  getEnv("SMTP_USER", ""),
		SMTPPass:                  getEnv("SMTP_PASS", ""),
		SebiAlgoIDRequired:        sebiRequired,
		BrokerZerodhaAlgoID:       getEnv("BROKER_ZERODHA_ALGO_ID", ""),
		BrokerAngelAlgoID:         getEnv("BROKER_ANGEL_ALGO_ID", ""),
		BrokerDhanAlgoID:          getEnv("BROKER_DHAN_ALGO_ID", ""),
		BillingEnabled:            getEnvBool("BILLING_ENABLED", false),
		StripeSecretKey:           getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret:       getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripePricePaper:          getEnv("STRIPE_PRICE_PAPER", ""),
		StripePricePro:            getEnv("STRIPE_PRICE_PRO", getEnv("STRIPE_PRICE_INDIVIDUAL", "")),
		StripePriceProPlus:        getEnv("STRIPE_PRICE_PRO_PLUS", ""),
		BillingPlanPriceLabels:    parsePlanPriceLabels(getEnv("BILLING_PLAN_PRICE_LABELS", "")),
		BillingSuccessURL:         getEnv("BILLING_SUCCESS_URL", ""),
		BillingCancelURL:          getEnv("BILLING_CANCEL_URL", ""),
		BillingPortalReturnURL:    getEnv("BILLING_PORTAL_RETURN_URL", ""),

		ZerodhaAPIKey:      getEnv("ZERODHA_API_KEY", ""),
		ZerodhaAPISecret:   getEnv("ZERODHA_API_SECRET", ""),
		ZerodhaCallbackURL: getEnv("ZERODHA_CALLBACK_URL", ""),
		ZerodhaFrontendURL: getEnv("ZERODHA_FRONTEND_URL", ""),

		ZerodhaPublisherEnabled:     getEnvBool("ZERODHA_PUBLISHER_ENABLED", false),
		ZerodhaPublisherCallbackURL: getEnv("ZERODHA_PUBLISHER_CALLBACK_URL", ""),

		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),

		ZBServiceToken:    getEnv("ZB_SERVICE_TOKEN", ""),
		PaperAdapterURL:   getEnv("PAPER_ADAPTER_URL", ""),
		ZerodhaAdapterURL: getEnv("ZERODHA_ADAPTER_URL", "http://localhost:8092"),
		AngelAdapterURL:   getEnv("ANGEL_ADAPTER_URL", ""),
		DhanAdapterURL:    getEnv("DHAN_ADAPTER_URL", ""),
		MT5AdapterURL:     getEnv("MT5_ADAPTER_URL", ""),
		CoreInternalURL:   getEnv("CORE_INTERNAL_URL", "http://localhost:8080"),

		SupportNotificationEmail: getEnv("SUPPORT_NOTIFICATION_EMAIL", ""),
		SupportTelegramChatID:    getEnv("SUPPORT_TELEGRAM_CHAT_ID", ""),

		MarketDataProvider:            getEnv("MARKET_DATA_PROVIDER", "signal"),
		MarketDataSnapshotIntervalSec: getEnvInt("MARKET_DATA_SNAPSHOT_INTERVAL_SEC", 45),
		FyersAppID:                    getEnv("FYERS_APP_ID", ""),
		FyersSecretID:                 getEnv("FYERS_SECRET_ID", ""),
		FyersCallbackURL:              getEnv("FYERS_CALLBACK_URL", ""),
		FyersRefreshInterval:          getEnvInt("FYERS_REFRESH_INTERVAL_MIN", 60),
		ZerodhaFillPollIntervalSec:    getEnvInt("ZERODHA_FILL_POLL_INTERVAL_SEC", 10),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

// EmailVerificationTTL returns verification link lifetime.
func (c *Config) EmailVerificationTTL() time.Duration {
	h := c.EmailVerificationTTLHours
	if h <= 0 {
		h = 24
	}
	return time.Duration(h) * time.Hour
}

// BrokerHTTPTimeout returns the outbound broker HTTP client timeout.
func (c *Config) BrokerHTTPTimeout() time.Duration {
	if c.BrokerHTTPTimeoutSec <= 0 {
		return 8 * time.Second
	}
	return time.Duration(c.BrokerHTTPTimeoutSec) * time.Second
}

// HTTPGetRetries returns retry count for idempotent broker GETs only.
func (c *Config) HTTPGetRetries() int {
	if c.BrokerHTTPGetRetries < 0 {
		return 0
	}
	return c.BrokerHTTPGetRetries
}

// AESKeyBytes returns the decoded AES-256 encryption key.
func (c *Config) AESKeyBytes() ([]byte, error) {
	return credenc.ParseKey(c.AESKey)
}

func normalizeBrokerMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}

func parseEnabledAdapters(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"paper", "zerodha", "angel", "dhan", "mt5"}
	}
	return out
}

// AdapterPublicURL returns this adapter's public base URL for broker callback registration.
func (c *Config) AdapterPublicURL() string {
	if v := os.Getenv("ADAPTER_PUBLIC_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return strings.TrimRight(c.AppPublicURL, "/")
}

// IsValidBrokerMode reports whether mode is mock or live (case-insensitive).
func IsValidBrokerMode(mode string) bool {
	switch normalizeBrokerMode(mode) {
	case BrokerModeMock, BrokerModeLive:
		return true
	default:
		return false
	}
}

// Validate checks required production configuration.
func (c *Config) Validate() error {
	c.BrokerMode = normalizeBrokerMode(c.BrokerMode)
	if !IsValidBrokerMode(c.BrokerMode) {
		return fmt.Errorf("BROKER_MODE must be %q or %q, got %q", BrokerModeMock, BrokerModeLive, c.BrokerMode)
	}

	key, err := c.AESKeyBytes()
	if err != nil {
		return err
	}
	if os.Getenv("APP_ENV") == "production" && credenc.IsWeakKey(key) {
		return fmt.Errorf("%w", credenc.ErrWeakKey)
	}
	if c.BillingEnabled {
		if c.StripeSecretKey == "" {
			return fmt.Errorf("STRIPE_SECRET_KEY is required when BILLING_ENABLED=true")
		}
		if c.StripeWebhookSecret == "" {
			return fmt.Errorf("STRIPE_WEBHOOK_SECRET is required when BILLING_ENABLED=true")
		}
		if !strings.HasPrefix(c.AppPublicURL, "https://") {
			return fmt.Errorf("APP_PUBLIC_URL must use https when BILLING_ENABLED=true")
		}
	}
	return c.validateExecutionAdapters()
}

func (c *Config) validateExecutionAdapters() error {
	mode := strings.ToLower(strings.TrimSpace(c.ExecutionAdapterMode))
	if mode != "http" {
		return nil
	}
	enabled := make(map[string]struct{}, len(c.EnabledAdapters))
	for _, name := range c.EnabledAdapters {
		enabled[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	if _, ok := enabled["zerodha"]; ok && strings.TrimSpace(c.ZerodhaAdapterURL) == "" {
		return fmt.Errorf("ZERODHA_ADAPTER_URL is required when zerodha is enabled and EXECUTION_ADAPTER_MODE=http")
	}
	if _, ok := enabled["angel"]; ok && strings.TrimSpace(c.AngelAdapterURL) == "" {
		return fmt.Errorf("ANGEL_ADAPTER_URL is required when angel is enabled and EXECUTION_ADAPTER_MODE=http")
	}
	if _, ok := enabled["dhan"]; ok && strings.TrimSpace(c.DhanAdapterURL) == "" {
		return fmt.Errorf("DHAN_ADAPTER_URL is required when dhan is enabled and EXECUTION_ADAPTER_MODE=http")
	}
	if _, ok := enabled["mt5"]; ok && strings.TrimSpace(c.MT5AdapterURL) == "" {
		return fmt.Errorf("MT5_ADAPTER_URL is required when mt5 is enabled and EXECUTION_ADAPTER_MODE=http")
	}
	return nil
}

// ZerodhaOAuthCallbackURL returns the Kite Connect redirect URL for the active deployment.
func (c *Config) ZerodhaOAuthCallbackURL() string {
	if v := strings.TrimSpace(c.ZerodhaCallbackURL); v != "" {
		return v
	}
	if strings.EqualFold(c.ExecutionAdapterMode, "http") && strings.TrimSpace(c.ZerodhaAdapterURL) != "" {
		return strings.TrimRight(c.ZerodhaAdapterURL, "/") + "/v1/credentials/zerodha/callback"
	}
	return c.AppPublicURL + "/v1/credentials/zerodha/callback"
}

// EffectiveDashboardURL returns the browser-facing dashboard base URL for post-callback redirects.
func (c *Config) EffectiveDashboardURL() string {
	if v := strings.TrimRight(strings.TrimSpace(c.ZerodhaFrontendURL), "/"); v != "" {
		return v
	}
	if v := strings.TrimRight(strings.TrimSpace(c.FrontendURL), "/"); v != "" {
		return v
	}
	if v := strings.TrimRight(strings.TrimSpace(c.AppPublicURL), "/"); v != "" {
		return v
	}
	return "http://localhost:3000"
}

// EffectiveZerodhaPublisherCallbackURL returns the Kite Publisher basket redirect URL.
func (c *Config) EffectiveZerodhaPublisherCallbackURL() string {
	if v := strings.TrimSpace(c.ZerodhaPublisherCallbackURL); v != "" {
		return v
	}
	if strings.EqualFold(c.ExecutionAdapterMode, "http") && strings.TrimSpace(c.ZerodhaAdapterURL) != "" {
		return strings.TrimRight(c.ZerodhaAdapterURL, "/") + "/v1/publisher/callback"
	}
	return c.AppPublicURL + "/v1/publisher/callback"
}

// EffectivePaperAdapterURL returns the remote paper adapter base URL, or empty
// when paper runs cohosted in Core (no sidecar).
func (c *Config) EffectivePaperAdapterURL() string {
	return strings.TrimRight(strings.TrimSpace(c.PaperAdapterURL), "/")
}

// PlanPriceLabel returns an optional display price for a plan from deployment config.
// Pricing is controlled outside the codebase via BILLING_PLAN_PRICE_LABELS.
func (c *Config) PlanPriceLabel(planName string) string {
	if c == nil || len(c.BillingPlanPriceLabels) == 0 {
		return ""
	}
	return c.BillingPlanPriceLabels[planName]
}

// parsePlanPriceLabels reads BILLING_PLAN_PRICE_LABELS as comma-separated plan=label pairs.
// Example: "paper=$9/mo,pro=$29/mo,pro_plus=$79/mo"
func parsePlanPriceLabels(raw string) map[string]string {
	out := map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	return out
}
