//go:build functional

package functionaltester

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/fasthttp/websocket"
)

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

type loginResult struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
	OrgID     string `json:"org_id,omitempty"`
	OrgRole   string `json:"org_role,omitempty"`
}

type meResult struct {
	ID                     string    `json:"id"`
	Email                  string    `json:"email"`
	Plan                   string    `json:"plan"`
	Role                   string    `json:"role"`
	Status                 string    `json:"status"`
	CreatedAt              time.Time `json:"created_at"`
	OrdersPerSec           *int      `json:"orders_per_sec"`
	OrdersPerSecEnforced   int       `json:"orders_per_sec_enforced"`
	BrokerCredOrdersPerSec int       `json:"broker_cred_orders_per_sec"`
	MaxWebhooks            int       `json:"max_webhooks"`
	MaxBrokerCreds         int       `json:"max_broker_creds"`
	BillingPlan            string    `json:"billing_plan,omitempty"`
}

type userSummary struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Plan          string `json:"plan"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	BillingSource string `json:"billing_source"`
	WebhookCount  int    `json:"webhook_count"`
	BrokerCount   int    `json:"broker_count"`
}

type adminUsersResult struct {
	Users      []userSummary `json:"users"`
	Total      int           `json:"total"`
	Page       int           `json:"page"`
	Limit      int           `json:"limit"`
	TotalPages int           `json:"total_pages"`
}

type credentialResult struct {
	ID           string `json:"id"`
	BrokerType   string `json:"broker_type"`
	Label        string `json:"label"`
	AccountLabel string `json:"account_label"`
	AccountMode  string `json:"account_mode"`
	Exchange     string `json:"exchange"`
	Product      string `json:"product"`
}

type verifyCredentialResult struct {
	Valid       bool   `json:"valid"`
	BrokerType  string `json:"broker_type"`
	AccountMode string `json:"account_mode"`
	Exchange    string `json:"exchange"`
	Product     string `json:"product"`
	Error       string `json:"error"`
	ErrorCode   string `json:"error_code"`
	Hint        string `json:"hint"`
}

type webhookResult struct {
	ID           string   `json:"id"`
	UserID       string   `json:"user_id"`
	OrgID        *string  `json:"org_id,omitempty"`
	CreatedBy    string   `json:"created_by"`
	Token        string   `json:"token"`
	Label        string   `json:"label"`
	Status       string   `json:"status"`
	BrokerCredID string   `json:"broker_cred_id"`
	Symbol       string   `json:"symbol"`
	LotSize      float64  `json:"lot_size"`
	MaxRiskPct   float64  `json:"max_risk_pct"`
	SLPoints     int      `json:"sl_points"`
	TPPoints     int      `json:"tp_points"`
	DedupWindowSec int    `json:"dedup_window_sec"`
	CreatedAt    string   `json:"created_at"`
}

type createWebhookRequest struct {
	Label          string   `json:"label"`
	BrokerCredID   string   `json:"broker_cred_id"`
	AllowedSymbols []string `json:"allowed_symbols"`
	LotSize        float64  `json:"lot_size,omitempty"`
	MaxRiskPct     float64  `json:"max_risk_pct,omitempty"`
	SLPoints       int      `json:"sl_points,omitempty"`
	TPPoints       int      `json:"tp_points,omitempty"`
}

func webhookSymbols(symbols ...string) []string {
	return symbols
}

type tradeResult struct {
	ID          string     `json:"id"`
	WebhookID   string     `json:"webhook_id"`
	Signal      string     `json:"signal"`
	Symbol      string     `json:"symbol"`
	LotSize     float64    `json:"lot_size"`
	BrokerOrder string     `json:"broker_order"`
	SignalKey   string     `json:"signal_key"`
	Status      string     `json:"status"`
	FillPrice   float64    `json:"fill_price"`
	OrderType   string     `json:"order_type"`
	SLPrice     *float64   `json:"sl_price,omitempty"`
	TPPrice     *float64   `json:"tp_price,omitempty"`
	Error       string     `json:"error"`
	ErrorCode   string     `json:"error_code"`
	Comment     string     `json:"comment"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	CancelError string     `json:"cancel_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type pnlResult struct {
	WebhookID    string             `json:"webhook_id,omitempty"`
	OrgID        string             `json:"org_id,omitempty"`
	TradeCount   int                `json:"trade_count"`
	ByStatus     map[string]int     `json:"by_status"`
	Notional     float64            `json:"notional"`
	PricedTrades int                `json:"priced_trades"`
	FillRate     *float64           `json:"fill_rate,omitempty"`
	WinRate      *float64           `json:"win_rate,omitempty"`
}

type wsTradeMessage struct {
	Type  string      `json:"type"`
	Trade tradeResult `json:"trade"`
}

type state struct {
	baseURL      string
	metricsToken string
	client       *http.Client

	adminEmail    string
	adminPassword string

	ownerEmail    string
	ownerPassword string
	tempEmail     string
	tempPassword  string

	adminToken string
	ownerToken string
	tempToken  string

	ownerID string
	tempID  string

	personalCredUpdateID string
	personalCredWebhookID string
	personalCredDeleteID string
	personalWebhookID         string
	personalWebhookToken      string
	personalWebhookOldToken   string
	personalWebhookDeleteID   string
	personalWebhookSymbol     string // EURUSD (mt5) or RELIANCE (zerodha-only deployments)

	// Platform invite (5C.1 closed beta)
	platformInviteID    string
	platformInviteToken string
	betaUserEmail       string
	betaUserPassword    string
	betaUserToken       string

	// Bracket order trade assertions (3.3)
	bracketWebhookID    string
	bracketWebhookToken string
	cncCredID           string
	cncWebhookID        string
	cncWebhookToken     string
}

func TestFunctionalAPI(t *testing.T) {
	s := newState()

	t.Run("health", func(t *testing.T) {
		status, raw := s.request(t, http.MethodGet, "/healthz", "", nil)
		if status != http.StatusOK {
			t.Fatalf("healthz: want 200 got %d body=%s", status, trimBody(raw))
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("healthz decode: %v body=%s", err, trimBody(raw))
		}
		if got["status"] != "ok" {
			t.Fatalf("healthz status mismatch: %#v", got)
		}
		s.assertMetricsEndpoint(t)
	})

	t.Run("auth and personal resources", func(t *testing.T) {
		s.registerOwner(t)
		s.loginOwner(t)
		s.assertMe(t, s.ownerToken, s.ownerEmail, "free", "user", false)
		s.assertFreePlanLimits(t) // §5 §16
		s.assertLiveCredentialBlockedOnFree(t)
		s.loginAdmin(t)
		s.stagingUpgradeForCredLimits(t)

		s.createPersonalCredential(t, "personal cred update", "zerodha", "owner-live", "api_key:mock_secret:mock_access_token", "NSE", "MIS", &s.personalCredUpdateID)
		s.assertCredentialCreateRequiresExchange(t)
		s.assertCredentialCreateInvalidFormat(t)
		s.verifyCredentialValid(t, s.personalCredUpdateID, "zerodha", "NSE", "MIS")
		s.assertIndianBrokerSignalSubmitted(t, s.personalCredUpdateID)
		s.assertForexSymbolRejectedOnIndianBroker(t, s.personalCredUpdateID)
		s.assertUpdateCredentialInvalidRawRejected(t, s.personalCredUpdateID)
		if s.stagingUsesSingleBrokerCred() {
			s.personalCredDeleteID = s.personalCredUpdateID
		} else {
			deleteBroker, deleteRaw := "dhan", "client_id:delete_secret"
			if !s.brokerEnabled("dhan") {
				deleteBroker, deleteRaw = "zerodha", "api_key:delete_key:delete_token"
			}
			s.createPersonalCredential(t, "personal cred delete", deleteBroker, "owner-delete", deleteRaw, "NSE", "MIS", &s.personalCredDeleteID)
			s.verifyCredentialValid(t, s.personalCredDeleteID, deleteBroker, "NSE", "MIS")
		}
		s.listCredentialsIncludesIndianMetadata(t, s.ownerToken)
		s.updatePersonalCredentialIndian(t)
		if s.brokerEnabled("angel") {
			s.verifyCredentialValid(t, s.personalCredUpdateID, "angel", "NSE", "MIS")
		} else {
			s.verifyCredentialValid(t, s.personalCredUpdateID, "zerodha", "NSE", "MIS")
		}

		s.createPersonalWebhookCredential(t)
		if s.brokerEnabled("mt5_cloud") {
			s.verifyMt5Credential(t, s.personalCredWebhookID)
		} else {
			s.verifyCredentialValid(t, s.personalCredWebhookID, "zerodha", "NSE", "MIS")
		}
		s.createPersonalWebhook(t)
		s.updatePersonalWebhook(t)
		s.rotatePersonalWebhookToken(t)
		s.assertUnknownWebhookTokenRejected(t, s.personalWebhookOldToken)
		s.assertWebhookImmutableFieldRejected(t)
		if !s.stagingUsesSingleBrokerCred() {
			s.createPersonalWebhookToDelete(t)
		}
		s.listWebhooksHasAtLeast(t, s.ownerToken, 1)
		s.pauseWebhook(t, s.personalWebhookID, s.ownerToken)
		s.assertPausedWebhookRejectsSignal(t, s.personalWebhookToken)
		s.resumeWebhook(t, s.personalWebhookID, s.ownerToken)
		s.assertSuspendedUserRejectsWebhookSignal(t)
		s.assertSignalDedup(t)
		if s.stagingLiveBrokers() {
			// Owner was upgraded to pro for cred limits — pnl is available (not free-tier blocked).
			var pnl pnlResult
			callJSON(s, t, http.MethodGet, "/v1/webhooks/"+s.personalWebhookID+"/pnl", s.ownerToken, nil, http.StatusOK, &pnl)
			if pnl.WebhookID != s.personalWebhookID {
				t.Fatalf("staging pnl webhook_id mismatch: got %q want %q", pnl.WebhookID, s.personalWebhookID)
			}
		}
		s.ingestSignalAndAssertTrade(t, s.personalWebhookToken, s.ownerToken, s.personalWebhookID, "/v1/webhooks/"+s.personalWebhookID+"/trades")
		if !s.stagingLiveBrokers() {
			s.assertWebhookPnL(t, s.ownerToken, s.personalWebhookID, 1)
			s.assertWebSocketTradePush(t, s.ownerToken, s.personalWebhookToken, s.personalWebhookID)
		}
		if !s.stagingLiveBrokers() {
			s.ingestSignalAccepted(t, s.personalWebhookToken, "SELL")
			s.ingestSignalAccepted(t, s.personalWebhookToken, "CLOSE")
		}
		// Delete no-trade webhook first, then webhook with trade history.
		if s.personalWebhookDeleteID != "" && s.personalWebhookDeleteID != s.personalWebhookID {
			s.deleteWebhookByID(t, s.personalWebhookDeleteID, s.ownerToken)
		}
		s.deleteWebhookByID(t, s.personalWebhookID, s.ownerToken)
		if !s.stagingUsesSingleBrokerCred() {
			s.deleteCredential(t, s.personalCredDeleteID, s.ownerToken, "/v1/credentials/"+s.personalCredDeleteID)
		}
		if s.personalCredWebhookID != s.personalCredUpdateID {
			s.deleteCredential(t, s.personalCredWebhookID, s.ownerToken, "/v1/credentials/"+s.personalCredWebhookID)
		}
	})

	t.Run("platform invites (closed beta)", func(t *testing.T) {
		s.stagingEnsureAdmin(t) // reuse auth token from auth_and_personal_resources
		s.assertPlatformInviteFlow(t)
	})

	t.Run("bracket orders (SL/TP)", func(t *testing.T) {
		s.loginOwner(t)
		s.stagingEnsureAdmin(t)
		s.stagingUpgradeForCredLimits(t)
		s.assertBracketOrderTrade(t)
		s.assertCNCBracketRejected(t)
	})

	t.Run("cancel order", func(t *testing.T) {
		if s.stagingLiveBrokers() {
			t.Skip("staging: skipping cancel_order — cannot guarantee a pending (submitted) order for real brokers")
		}
		s.loginOwner(t)
		s.stagingEnsureAdmin(t)
		s.stagingUpgradeForCredLimits(t)
		s.assertCancelOrderFlow(t)
	})

	t.Run("admin user management", func(t *testing.T) {
		s.registerTempUser(t)
		s.loginAdmin(t)
		s.assertAdminMe(t)
		s.assertAdminUsersListPlain(t)
		s.assertAdminUserListing(t)
		s.assertAdminUserDetailAndPatch(t)
		s.assertSessionLogout(t)
	})
}

func newState() *state {
	return &state{
		baseURL:      strings.TrimRight(env("FUNCTIONAL_TEST_BASE_URL", "http://localhost:8080"), "/"),
		metricsToken: env("FUNCTIONAL_TEST_METRICS_TOKEN", ""),
		client:       &http.Client{Timeout: 15 * time.Second},

		adminEmail:    env("FUNCTIONAL_TEST_ADMIN_EMAIL", "admin@example.com"),
		adminPassword: env("FUNCTIONAL_TEST_ADMIN_PASSWORD", "admin123"),

		ownerEmail:    uniqueEmail("owner"),
		ownerPassword: env("FUNCTIONAL_TEST_OWNER_PASSWORD", "owner12345"),
		tempEmail:     uniqueEmail("admin-target"),
		tempPassword:  env("FUNCTIONAL_TEST_TEMP_PASSWORD", "temp12345"),

		betaUserEmail:    uniqueEmail("beta"),
		betaUserPassword: "betapass123",
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (s *state) stagingLiveBrokers() bool {
	return os.Getenv("FUNCTIONAL_TEST_STAGING") == "1"
}

func (s *state) enabledAdapterSet() map[string]bool {
	raw := env("FUNCTIONAL_TEST_ENABLED_ADAPTERS", "paper,zerodha")
	set := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			set[part] = true
		}
	}
	return set
}

func (s *state) brokerEnabled(brokerType string) bool {
	set := s.enabledAdapterSet()
	switch brokerType {
	case "zerodha":
		return set["zerodha"]
	case "angel":
		return set["angel"]
	case "dhan":
		return set["dhan"]
	case "mt5_cloud":
		return set["mt5"]
	default:
		return false
	}
}

func (s *state) webhookTradeSymbol() string {
	if s.personalWebhookSymbol != "" {
		return s.personalWebhookSymbol
	}
	return "EURUSD"
}

// closedRegistration reports whether the server under test has CLOSED_REGISTRATION enabled.
// Set FUNCTIONAL_TEST_CLOSED_REGISTRATION=1 when the server enforces invite codes on registration.
func (s *state) closedRegistration() bool {
	return os.Getenv("FUNCTIONAL_TEST_CLOSED_REGISTRATION") == "1"
}

func (s *state) serverEnforcesClosedRegistration(t *testing.T) bool {
	t.Helper()
	if !s.closedRegistration() {
		return false
	}
	type regBody struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code"`
	}
	status, raw := s.request(t, http.MethodPost, "/v1/auth/register", "", regBody{
		Email:      uniqueEmail("close-reg-check"),
		Password:   "closedRegPass123",
		InviteCode: "inv_invalidtoken",
	})
	if status == http.StatusCreated {
		t.Log("server appears to allow open registration; skipping invite enforcement assertions")
		return false
	}
	if status == http.StatusBadRequest || status == http.StatusForbidden {
		return true
	}
	t.Fatalf("unexpected closed-registration probe response: want 400/403 or 201, got %d body=%s", status, trimBody(raw))
	return false
}

func (s *state) stagingIngestPause() {
	if s.stagingLiveBrokers() {
		time.Sleep(1100 * time.Millisecond)
	}
}

func (s *state) stagingEnsureAdmin(t *testing.T) {
	t.Helper()
	if s.adminToken == "" {
		s.loginAdmin(t)
	}
}

// stagingUpgradeForCredLimits bumps plan so live broker credential tests can run (free has max_brokers=0).
// Staging uses pro (1 live cred, 1 live webhook) until migration 046 is applied everywhere; local uses pro_plus.
func (s *state) stagingUpgradeForCredLimits(t *testing.T) {
	t.Helper()
	s.stagingEnsureAdmin(t)
	var me meResult
	callJSON(s, t, http.MethodGet, "/v1/me", s.ownerToken, nil, http.StatusOK, &me)
	wantPlan := "pro_plus"
	if s.stagingLiveBrokers() {
		wantPlan = "pro"
	}
	if me.Plan == wantPlan {
		return
	}
	s.upgradeOwnerPlan(t, wantPlan)
}

func (s *state) stagingUsesSingleBrokerCred() bool {
	return s.stagingLiveBrokers()
}

// ensureAuditAccessForTrades temporarily upgrades a free solo user to pro on staging
// so trade audit endpoints work without switching BROKER_MODE=live routing to real brokers.
func (s *state) ensureAuditAccessForTrades(t *testing.T, token string) func() {
	t.Helper()
	if !s.stagingLiveBrokers() {
		return func() {}
	}
	var me meResult
	callJSON(s, t, http.MethodGet, "/v1/me", token, nil, http.StatusOK, &me)
	if me.Plan != "free" {
		return func() {}
	}
	if s.adminToken == "" {
		s.loginAdmin(t)
	}
	s.upgradeOwnerPlan(t, "pro")
	return func() {
		s.upgradeOwnerPlan(t, "free")
	}
}

func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%s@example.com", prefix, uuid.NewString())
}

func callExpectStatus(s *state, t *testing.T, method, path, token string, body any, want int) {
	t.Helper()
	status, raw := s.request(t, method, path, token, body)
	if status != want {
		t.Fatalf("%s %s: want status %d, got %d, body=%s", method, path, want, status, trimBody(raw))
	}
}

func callJSON[T any](s *state, t *testing.T, method, path, token string, body any, want int, out *T) {
	t.Helper()
	status, raw := s.request(t, method, path, token, body)
	if status != want {
		t.Fatalf("%s %s: want status %d, got %d, body=%s", method, path, want, status, trimBody(raw))
	}
	if out == nil {
		return
	}
	env := envelope{}
	if len(bytes.TrimSpace(raw)) == 0 {
		t.Fatalf("%s %s: empty body, want JSON envelope", method, path)
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("%s %s: decode envelope failed: %v, body=%s", method, path, err, trimBody(raw))
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		var zero T
		*out = zero
		return
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		t.Fatalf("%s %s: decode data failed: %v, data=%s", method, path, err, string(env.Data))
	}
}

func decodeEnvelope(raw []byte, out any) error {
	env := envelope{}
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

func (s *state) request(t *testing.T, method, path, token string, body any) (int, []byte) {
	t.Helper()

	if s.stagingLiveBrokers() && method == http.MethodPost && strings.HasPrefix(path, "/v1/webhook/") {
		s.stagingIngestPause()
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("%s %s: marshal body failed: %v", method, path, err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, s.baseURL+path, reader)
	if err != nil {
		t.Fatalf("%s %s: build request failed: %v", method, path, err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: request failed: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: read body failed: %v", method, path, err)
	}
	return resp.StatusCode, raw
}

func trimBody(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if len(text) > 500 {
		return text[:500] + "..."
	}
	return text
}

func (s *state) registerOwner(t *testing.T) {
	t.Helper()
	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	for attempt := 0; attempt < 5; attempt++ {
		type response struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		}
		var out response
		status, raw := s.request(t, http.MethodPost, "/v1/auth/register", "", request{Email: s.ownerEmail, Password: s.ownerPassword})
		if status == http.StatusConflict {
			s.ownerEmail = uniqueEmail("owner")
			continue
		}
		if status != http.StatusCreated {
			t.Fatalf("POST /v1/auth/register: want status 201, got %d, body=%s", status, trimBody(raw))
		}
		if err := decodeEnvelope(raw, &out); err != nil {
			t.Fatalf("POST /v1/auth/register: decode failed: %v, body=%s", err, trimBody(raw))
		}
		s.ownerID = out.ID
		if out.Email != s.ownerEmail {
			t.Fatalf("register owner email mismatch: got %s want %s", out.Email, s.ownerEmail)
		}
		return
	}
	t.Fatal("could not register owner after retries")
}

func (s *state) registerTempUser(t *testing.T) {
	t.Helper()
	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	for attempt := 0; attempt < 5; attempt++ {
		type response struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		}
		var out response
		status, raw := s.request(t, http.MethodPost, "/v1/auth/register", "", request{Email: s.tempEmail, Password: s.tempPassword})
		if status == http.StatusConflict {
			s.tempEmail = uniqueEmail("admin-target")
			continue
		}
		if status != http.StatusCreated {
			t.Fatalf("POST /v1/auth/register: want status 201, got %d, body=%s", status, trimBody(raw))
		}
		if err := decodeEnvelope(raw, &out); err != nil {
			t.Fatalf("POST /v1/auth/register: decode failed: %v, body=%s", err, trimBody(raw))
		}
		s.tempID = out.ID
		return
	}
	t.Fatal("could not register temp user after retries")
}

func (s *state) loginOwner(t *testing.T) {
	t.Helper()
	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	var out loginResult
	callJSON(s, t, http.MethodPost, "/v1/auth/login", "", request{Email: s.ownerEmail, Password: s.ownerPassword}, http.StatusOK, &out)
	s.ownerToken = out.Token
	if out.Token == "" {
		t.Fatal("owner login returned empty token")
	}
}

func (s *state) loginAdmin(t *testing.T) {
	t.Helper()
	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	var out loginResult
	callJSON(s, t, http.MethodPost, "/v1/auth/login", "", request{Email: s.adminEmail, Password: s.adminPassword}, http.StatusOK, &out)
	s.adminToken = out.Token
	if out.Token == "" {
		t.Fatal("admin login returned empty token")
	}
}

func intPtr(n int) *int {
	return &n
}

func (s *state) assertOrderRateLimits(t *testing.T, token string, wantDisplay *int, wantEnforced int) {
	t.Helper()
	var out meResult
	callJSON(s, t, http.MethodGet, "/v1/me", token, nil, http.StatusOK, &out)
	if wantDisplay == nil {
		if out.OrdersPerSec != nil {
			t.Fatalf("orders_per_sec want unlimited (null), got %v", *out.OrdersPerSec)
		}
	} else if out.OrdersPerSec == nil || *out.OrdersPerSec != *wantDisplay {
		got := "null"
		if out.OrdersPerSec != nil {
			got = fmt.Sprintf("%d", *out.OrdersPerSec)
		}
		t.Fatalf("orders_per_sec want %d, got %s", *wantDisplay, got)
	}
	if out.OrdersPerSecEnforced != wantEnforced {
		t.Fatalf("orders_per_sec_enforced want %d, got %d", wantEnforced, out.OrdersPerSecEnforced)
	}
	if out.BrokerCredOrdersPerSec != 10 {
		t.Fatalf("broker_cred_orders_per_sec want 10, got %d", out.BrokerCredOrdersPerSec)
	}
}

func (s *state) assertMetricsEndpoint(t *testing.T) {
	t.Helper()
	status, raw := s.request(t, http.MethodGet, "/metrics", s.metricsToken, nil)
	if status == http.StatusUnauthorized && s.metricsToken == "" {
		if s.stagingLiveBrokers() {
			t.Log("GET /metrics: skipped (staging requires METRICS_TOKEN — set STAGING_METRICS_TOKEN)")
			return
		}
	}
	if status != http.StatusOK {
		t.Fatalf("GET /metrics: want 200, got %d body=%s", status, trimBody(raw))
	}
	body := string(raw)
	for _, want := range []string{
		"# HELP zettabridge_ingest_total",
		"# TYPE zettabridge_ingest_total",
		"# HELP zettabridge_trades_total",
		"# TYPE zettabridge_trades_total",
		"# HELP zettabridge_dedup_hits_total",
		"# HELP zettabridge_billing_checkout_total",
		"# HELP zettabridge_billing_webhook_total",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("GET /metrics: missing %q in body (first 300 chars: %q)", want, trimBodyForMetrics(body))
		}
	}
}

func trimBodyForMetrics(body string) string {
	body = strings.TrimSpace(body)
	if len(body) > 300 {
		return body[:300] + "..."
	}
	return body
}

func (s *state) assertMe(t *testing.T, token, email, plan, role string, _ bool) {
	t.Helper()
	var out meResult
	callJSON(s, t, http.MethodGet, "/v1/me", token, nil, http.StatusOK, &out)
	if out.Email != email {
		t.Fatalf("me email mismatch: got %s want %s", out.Email, email)
	}
	if out.Plan != plan {
		t.Fatalf("me plan mismatch: got %s want %s", out.Plan, plan)
	}
	if out.Role != role {
		t.Fatalf("me role mismatch: got %s want %s", out.Role, role)
	}
}

func (s *state) createPersonalCredential(t *testing.T, label, brokerType, accountLabel, rawCreds, exchange, product string, id *string) {
	t.Helper()
	type request struct {
		BrokerType   string `json:"broker_type"`
		AccountLabel string `json:"account_label"`
		AccountMode  string `json:"account_mode"`
		RawCreds     string `json:"raw_creds"`
		Exchange     string `json:"exchange,omitempty"`
		Product      string `json:"product,omitempty"`
	}
	req := request{
		BrokerType: brokerType, AccountLabel: accountLabel, AccountMode: "live",
		RawCreds: rawCreds, Exchange: exchange, Product: product,
	}
	var out credentialResult
	callJSON(s, t, http.MethodPost, "/v1/credentials", s.ownerToken, req, http.StatusCreated, &out)
	if out.ID == "" {
		t.Fatalf("%s: credential id missing", label)
	}
	if id != nil {
		*id = out.ID
	}
	if exchange != "" && out.Exchange != exchange {
		t.Fatalf("%s: exchange mismatch: got %q want %q", label, out.Exchange, exchange)
	}
	if product != "" && out.Product != product {
		t.Fatalf("%s: product mismatch: got %q want %q", label, out.Product, product)
	}
}

func (s *state) createPersonalWebhookCredential(t *testing.T) {
	t.Helper()
	if s.brokerEnabled("mt5_cloud") {
		type request struct {
			BrokerType   string `json:"broker_type"`
			AccountLabel string `json:"account_label"`
			AccountMode  string `json:"account_mode"`
			RawCreds     string `json:"raw_creds"`
		}
		var out credentialResult
		callJSON(s, t, http.MethodPost, "/v1/credentials", s.ownerToken, request{
			BrokerType: "mt5_cloud", AccountLabel: "owner-mock", AccountMode: "live",
			RawCreds: "mocktoken:mockaccount",
		}, http.StatusCreated, &out)
		s.personalCredWebhookID = out.ID
		s.personalWebhookSymbol = "EURUSD"
	} else if s.stagingLiveBrokers() && s.personalCredUpdateID != "" {
		// Staging ECS: paper+zerodha only — reuse the primary cred (live routing is flaky).
		s.personalCredWebhookID = s.personalCredUpdateID
		s.personalWebhookSymbol = "RELIANCE"
	} else {
		s.createPersonalCredential(t, "personal webhook cred", "zerodha", "owner-webhook",
			"api_key:wh_mock:wh_token", "NSE", "MIS", &s.personalCredWebhookID)
		s.personalWebhookSymbol = "RELIANCE"
	}
	if s.personalCredWebhookID == "" {
		t.Fatal("personal webhook credential id missing")
	}
}

func (s *state) updatePersonalCredentialIndian(t *testing.T) {
	t.Helper()
	if s.brokerEnabled("angel") {
		s.updatePersonalCredential(t)
		return
	}
	type request struct {
		AccountLabel string `json:"account_label"`
		RawCreds     string `json:"raw_creds"`
	}
	var out credentialResult
	callJSON(s, t, http.MethodPut, "/v1/credentials/"+s.personalCredUpdateID, s.ownerToken, request{
		AccountLabel: "owner-live-updated",
		RawCreds:     "api_key:mock_secret:updated_token",
	}, http.StatusOK, &out)
	if out.BrokerType != "zerodha" {
		t.Fatalf("updated credential broker_type mismatch: got %s", out.BrokerType)
	}
}

func (s *state) updatePersonalCredential(t *testing.T) {
	t.Helper()
	type request struct {
		BrokerType   string `json:"broker_type"`
		AccountLabel string `json:"account_label"`
		Exchange     string `json:"exchange"`
		RawCreds     string `json:"raw_creds"`
	}
	var out credentialResult
	callJSON(s, t, http.MethodPut, "/v1/credentials/"+s.personalCredUpdateID, s.ownerToken, request{
		BrokerType:   "angel",
		AccountLabel: "owner-live-updated",
		Exchange:     "NSE",
		RawCreds:     "api_key:client_code:updated_jwt",
	}, http.StatusOK, &out)
	if out.BrokerType != "angel" {
		t.Fatalf("updated credential broker_type mismatch: got %s", out.BrokerType)
	}
}

func (s *state) deleteCredential(t *testing.T, id, token, path string) {
	t.Helper()
	status, raw := s.request(t, http.MethodDelete, path, token, nil)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE %s: want 204, got %d, body=%s", path, status, trimBody(raw))
	}
}

func (s *state) listCredentialsHasAtLeast(t *testing.T, token string, min int) {
	t.Helper()
	var out []credentialResult
	callJSON(s, t, http.MethodGet, "/v1/credentials", token, nil, http.StatusOK, &out)
	if len(out) < min {
		t.Fatalf("expected at least %d credentials, got %d", min, len(out))
	}
}

func (s *state) listCredentialsIncludesIndianMetadata(t *testing.T, token string) {
	t.Helper()
	var out []credentialResult
	callJSON(s, t, http.MethodGet, "/v1/credentials", token, nil, http.StatusOK, &out)
	found := false
	for _, c := range out {
		if c.BrokerType == "zerodha" && c.Exchange == "NSE" && c.Product == "MIS" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected zerodha credential with exchange/product in list: %#v", out)
	}
}

func (s *state) assertLiveCredentialBlockedOnFree(t *testing.T) {
	t.Helper()
	type request struct {
		BrokerType   string `json:"broker_type"`
		AccountLabel string `json:"account_label"`
		AccountMode  string `json:"account_mode"`
		Exchange     string `json:"exchange"`
		RawCreds     string `json:"raw_creds"`
	}
	callExpectStatus(s, t, http.MethodPost, "/v1/credentials", s.ownerToken, request{
		BrokerType:   "zerodha",
		AccountLabel: "live-blocked",
		AccountMode:  "live",
		Exchange:     "NSE",
		RawCreds:     "api_key:access_token",
	}, http.StatusForbidden)
}

func (s *state) assertUpdateCredentialInvalidRawRejected(t *testing.T, credID string) {
	t.Helper()
	type request struct {
		RawCreds string `json:"raw_creds"`
	}
	callExpectStatus(s, t, http.MethodPut, "/v1/credentials/"+credID, s.ownerToken, request{
		RawCreds: "bad:creds:too:many",
	}, http.StatusBadRequest)
}

func (s *state) verifyMt5Credential(t *testing.T, credID string) {
	t.Helper()
	s.verifyCredentialValid(t, credID, "mt5_cloud", "", "")
}

func (s *state) assertIndianBrokerSignalSubmitted(t *testing.T, credID string) {
	t.Helper()
	if s.stagingLiveBrokers() {
		t.Log("staging: skipping indian broker submitted trade (live routing requires algo_id and real creds)")
		return
	}
	var wh webhookResult
	callJSON(s, t, http.MethodPost, "/v1/webhooks", s.ownerToken, createWebhookRequest{
		Label:          "indian-equity-functional",
		BrokerCredID:   credID,
		AllowedSymbols: webhookSymbols("RELIANCE"),
		LotSize:        1,
		MaxRiskPct:     0,
	}, http.StatusCreated, &wh)
	if wh.Token == "" {
		t.Fatal("indian webhook token missing")
	}

	type signalReq struct {
		Action string  `json:"action"`
		Symbol string  `json:"symbol"`
		Lot    float64 `json:"lot"`
	}
	callExpectStatus(s, t, http.MethodPost, "/v1/webhook/"+wh.Token, "", signalReq{
		Action: "BUY",
		Symbol: "RELIANCE",
		Lot:    1,
	}, http.StatusAccepted)

	tr := s.waitForTradeStatus(t, s.ownerToken, "/v1/webhooks/"+wh.ID+"/trades", "submitted")
	if tr.Symbol != "RELIANCE" {
		t.Fatalf("trade symbol mismatch: %#v", tr)
	}
	if tr.BrokerOrder == "" {
		t.Fatalf("expected broker order id on indian submitted trade: %#v", tr)
	}
	s.deleteWebhookByID(t, wh.ID, s.ownerToken)
}

func (s *state) updatePersonalWebhook(t *testing.T) {
	t.Helper()
	type request struct {
		Label   string   `json:"label"`
		LotSize float64  `json:"lot_size"`
	}
	var out webhookResult
	callJSON(s, t, http.MethodPut, "/v1/webhooks/"+s.personalWebhookID, s.ownerToken, request{
		Label:   "owner-signal-updated",
		LotSize: 0.02,
	}, http.StatusOK, &out)
	if out.Label != "owner-signal-updated" {
		t.Fatalf("webhook label not updated: %#v", out)
	}
	if out.LotSize != 0.02 {
		t.Fatalf("webhook lot_size not updated: %#v", out)
	}
}

func (s *state) rotatePersonalWebhookToken(t *testing.T) {
	t.Helper()
	s.personalWebhookOldToken = s.personalWebhookToken
	var out webhookResult
	callJSON(s, t, http.MethodPost, "/v1/webhooks/"+s.personalWebhookID+"/rotate-token", s.ownerToken, nil, http.StatusOK, &out)
	if out.Token == "" {
		t.Fatal("rotated webhook token missing")
	}
	if out.Token == s.personalWebhookOldToken {
		t.Fatal("rotated token should differ from previous")
	}
	s.personalWebhookToken = out.Token
}

func (s *state) assertUnknownWebhookTokenRejected(t *testing.T, token string) {
	t.Helper()
	type request struct {
		Action string `json:"action"`
		Symbol string `json:"symbol"`
	}
	callExpectStatus(s, t, http.MethodPost, "/v1/webhook/"+token, "", request{Action: "BUY", Symbol: s.webhookTradeSymbol()}, http.StatusUnauthorized)
}

func (s *state) assertWebhookImmutableFieldRejected(t *testing.T) {
	t.Helper()
	type request struct {
		Token string `json:"token"`
	}
	callExpectStatus(s, t, http.MethodPut, "/v1/webhooks/"+s.personalWebhookID, s.ownerToken, request{
		Token: "cannot-change",
	}, http.StatusBadRequest)
}

func (s *state) assertCredentialCreateRequiresExchange(t *testing.T) {
	t.Helper()
	type request struct {
		BrokerType   string `json:"broker_type"`
		AccountLabel string `json:"account_label"`
		RawCreds     string `json:"raw_creds"`
	}
	callExpectStatus(s, t, http.MethodPost, "/v1/credentials", s.ownerToken, request{
		BrokerType:   "zerodha",
		AccountLabel: "missing-exchange",
		RawCreds:     "api_key:access_token",
	}, http.StatusBadRequest)
}

func (s *state) assertCredentialCreateInvalidFormat(t *testing.T) {
	t.Helper()
	type request struct {
		BrokerType   string `json:"broker_type"`
		AccountLabel string `json:"account_label"`
		Exchange     string `json:"exchange"`
		RawCreds     string `json:"raw_creds"`
	}
	brokerType := "angel"
	rawCreds := "only:two_parts"
	if !s.brokerEnabled("angel") {
		brokerType = "zerodha"
		// api_key:api_secret is valid for zerodha — need too many colon parts.
		rawCreds = "api_key:secret:token:extra"
	}
	callExpectStatus(s, t, http.MethodPost, "/v1/credentials", s.ownerToken, request{
		BrokerType:   brokerType,
		AccountLabel: "bad-format",
		Exchange:     "NSE",
		RawCreds:     rawCreds,
	}, http.StatusBadRequest)
}

func (s *state) verifyCredentialValid(t *testing.T, credID, brokerType, exchange, product string) {
	t.Helper()
	var out verifyCredentialResult
	callJSON(s, t, http.MethodPost, "/v1/credentials/"+credID+"/verify", s.ownerToken, nil, http.StatusOK, &out)
	if !out.Valid {
		if s.stagingLiveBrokers() {
			t.Logf("staging: skipping live broker verify for %s (%s)", credID, out.Error)
			return
		}
		t.Fatalf("verify credential: expected valid=true, got %#v", out)
	}
	if out.BrokerType != brokerType {
		t.Fatalf("verify broker_type mismatch: got %q", out.BrokerType)
	}
	if out.Exchange != exchange {
		t.Fatalf("verify exchange mismatch: got %q want %q", out.Exchange, exchange)
	}
	if out.Product != product {
		t.Fatalf("verify product mismatch: got %q want %q", out.Product, product)
	}
}

func (s *state) assertForexSymbolRejectedOnIndianBroker(t *testing.T, credID string) {
	t.Helper()
	var wh webhookResult
	callJSON(s, t, http.MethodPost, "/v1/webhooks", s.ownerToken, createWebhookRequest{
		Label:          "forex-on-indian-cred",
		BrokerCredID:   credID,
		AllowedSymbols: webhookSymbols("EURUSD"),
		LotSize:        1,
		MaxRiskPct:     0,
		SLPoints:       20,
		TPPoints:       28,
	}, http.StatusCreated, &wh)
	if wh.Token == "" {
		t.Fatal("forex rejection test webhook token missing")
	}

	type signalReq struct {
		Action string  `json:"action"`
		Symbol string  `json:"symbol"`
		Lot    float64 `json:"lot"`
	}
	callExpectStatus(s, t, http.MethodPost, "/v1/webhook/"+wh.Token, "", signalReq{
		Action: "BUY",
		Symbol: "EURUSD",
		Lot:    1,
	}, http.StatusAccepted)

	if s.stagingLiveBrokers() {
		t.Log("staging: forex ingest accepted; skipping async rejection poll (live routing)")
		s.deleteWebhookByID(t, wh.ID, s.ownerToken)
		return
	}

	trade := s.waitForTradeStatus(t, s.ownerToken, "/v1/webhooks/"+wh.ID+"/trades", "rejected")
	if trade.ErrorCode != "invalid_symbol" {
		t.Fatalf("expected error_code invalid_symbol, got %#v", trade)
	}

	s.deleteWebhookByID(t, wh.ID, s.ownerToken)
}

func (s *state) createPersonalWebhook(t *testing.T) {
	t.Helper()
	var out webhookResult
	callJSON(s, t, http.MethodPost, "/v1/webhooks", s.ownerToken, createWebhookRequest{
		Label:          "owner-signal",
		BrokerCredID:   s.personalCredWebhookID,
		AllowedSymbols: webhookSymbols(s.webhookTradeSymbol()),
		LotSize:        0.01,
		MaxRiskPct:     1.0,
		SLPoints:       20,
		TPPoints:       28,
	}, http.StatusCreated, &out)
	s.personalWebhookID = out.ID
	s.personalWebhookToken = out.Token
	if out.ID == "" || out.Token == "" {
		t.Fatal("personal webhook create returned empty id/token")
	}
}

func (s *state) createPersonalWebhookToDelete(t *testing.T) {
	t.Helper()
	var out webhookResult
	callJSON(s, t, http.MethodPost, "/v1/webhooks", s.ownerToken, createWebhookRequest{
		Label:          "owner-delete",
		BrokerCredID:   s.personalCredWebhookID,
		AllowedSymbols: webhookSymbols(s.webhookTradeSymbol()),
		LotSize:        0.01,
		MaxRiskPct:     1.0,
		SLPoints:       20,
		TPPoints:       28,
	}, http.StatusCreated, &out)
	s.personalWebhookDeleteID = out.ID
}

func (s *state) listWebhooksHasAtLeast(t *testing.T, token string, min int) {
	t.Helper()
	var out []webhookResult
	callJSON(s, t, http.MethodGet, "/v1/webhooks", token, nil, http.StatusOK, &out)
	if len(out) < min {
		t.Fatalf("expected at least %d webhooks, got %d", min, len(out))
	}
}

func (s *state) pauseWebhook(t *testing.T, id, token string) {
	t.Helper()
	var out map[string]string
	callJSON(s, t, http.MethodPut, "/v1/webhooks/"+id+"/pause", token, nil, http.StatusOK, &out)
	if out["status"] != "paused" {
		t.Fatalf("pause webhook status mismatch: %#v", out)
	}
}

func (s *state) resumeWebhook(t *testing.T, id, token string) {
	t.Helper()
	var out map[string]string
	callJSON(s, t, http.MethodPut, "/v1/webhooks/"+id+"/resume", token, nil, http.StatusOK, &out)
	if out["status"] != "active" {
		t.Fatalf("resume webhook status mismatch: %#v", out)
	}
}

func (s *state) assertPausedWebhookRejectsSignal(t *testing.T, token string) {
	t.Helper()
	type request struct {
		Action string `json:"action"`
		Symbol string `json:"symbol"`
	}
	callExpectStatus(s, t, http.MethodPost, "/v1/webhook/"+token, "", request{Action: "BUY", Symbol: s.webhookTradeSymbol()}, http.StatusForbidden)
}

func (s *state) assertSuspendedUserRejectsWebhookSignal(t *testing.T) {
	t.Helper()
	type patchBody struct {
		Status string `json:"status"`
	}
	type userPatchResult struct {
		Status string `json:"status"`
	}
	var patched userPatchResult
	callJSON(s, t, http.MethodPatch, "/v1/admin/users/"+s.ownerID, s.adminToken, patchBody{Status: "suspended"}, http.StatusOK, &patched)
	if patched.Status != "suspended" {
		t.Fatalf("owner not suspended: %#v", patched)
	}

	type signalReq struct {
		Action string  `json:"action"`
		Symbol string  `json:"symbol"`
		Lot    float64 `json:"lot"`
	}
	callExpectStatus(s, t, http.MethodPost, "/v1/webhook/"+s.personalWebhookToken, "", signalReq{
		Action: "BUY", Symbol: s.webhookTradeSymbol(), Lot: 0.01,
	}, http.StatusForbidden)

	callJSON(s, t, http.MethodPatch, "/v1/admin/users/"+s.ownerID, s.adminToken, patchBody{Status: "active"}, http.StatusOK, &patched)
	if patched.Status != "active" {
		t.Fatalf("owner not restored: %#v", patched)
	}
}

func (s *state) assertSignalDedup(t *testing.T) {
	t.Helper()
	if s.stagingLiveBrokers() {
		// Clear per-user ingest rate window accumulated by prior webhook tests.
		time.Sleep(1100 * time.Millisecond)
	}
	type updateReq struct {
		DedupWindowSec int `json:"dedup_window_sec"`
	}
	var wh webhookResult
	callJSON(s, t, http.MethodPut, "/v1/webhooks/"+s.personalWebhookID, s.ownerToken, updateReq{DedupWindowSec: 60}, http.StatusOK, &wh)
	if wh.DedupWindowSec != 60 {
		t.Fatalf("dedup_window_sec not set: %#v", wh)
	}

	type signalReq struct {
		Action  string  `json:"action"`
		Symbol  string  `json:"symbol"`
		Lot     float64 `json:"lot"`
		Comment string  `json:"comment"`
	}
	payload := signalReq{Action: "BUY", Symbol: s.webhookTradeSymbol(), Lot: 0.01, Comment: "dedup-functional"}

	status, raw := s.request(t, http.MethodPost, "/v1/webhook/"+s.personalWebhookToken, "", payload)
	if status != http.StatusAccepted {
		t.Fatalf("first dedup ingest: want 202, got %d body=%s", status, trimBody(raw))
	}
	var first map[string]any
	if err := json.Unmarshal(raw, &first); err != nil {
		t.Fatalf("decode first dedup ingest: %v", err)
	}
	if first["queued"] != true {
		t.Fatalf("first dedup ingest not queued: %#v", first)
	}

	status, raw = s.request(t, http.MethodPost, "/v1/webhook/"+s.personalWebhookToken, "", payload)
	if status != http.StatusConflict {
		t.Fatalf("replay dedup ingest: want 409, got %d body=%s", status, trimBody(raw))
	}
	var second map[string]any
	if err := json.Unmarshal(raw, &second); err != nil {
		t.Fatalf("decode replay dedup ingest: %v", err)
	}
	if second["deduplicated"] != true {
		t.Fatalf("replay should be deduplicated: %#v", second)
	}

	payload.Comment = "dedup-functional-2"
	status, raw = s.request(t, http.MethodPost, "/v1/webhook/"+s.personalWebhookToken, "", payload)
	if status != http.StatusAccepted {
		t.Fatalf("distinct comment ingest: want 202, got %d body=%s", status, trimBody(raw))
	}
	var third map[string]any
	if err := json.Unmarshal(raw, &third); err != nil {
		t.Fatalf("decode distinct comment ingest: %v", err)
	}
	if third["queued"] != true {
		t.Fatalf("distinct comment ingest not queued: %#v", third)
	}

	if s.stagingLiveBrokers() {
		t.Log("staging: skipping dedup trade audit (personal MT5 webhook uses demo/live routing without real creds)")
		return
	}

	trades := s.waitForTrades(t, s.ownerToken, "/v1/webhooks/"+s.personalWebhookID+"/trades", 2)
	match := 0
	for _, tr := range trades {
		if tr.Comment == "dedup-functional" || tr.Comment == "dedup-functional-2" {
			match++
		}
	}
	if match != 2 {
		t.Fatalf("expected 2 dedup test trades, got %d in %#v", match, trades)
	}
}

func (s *state) ingestSignalAccepted(t *testing.T, token, action string) {
	t.Helper()
	type request struct {
		Action string  `json:"action"`
		Symbol string  `json:"symbol"`
		Lot    float64 `json:"lot"`
	}
	status, raw := s.request(t, http.MethodPost, "/v1/webhook/"+token, "", request{
		Action: action,
		Symbol: s.webhookTradeSymbol(),
		Lot:    0.01,
	})
	if status != http.StatusAccepted {
		t.Fatalf("POST /v1/webhook/%s (%s): want 202, got %d, body=%s", token, action, status, trimBody(raw))
	}
}

func (s *state) ingestSignalAndAssertTrade(t *testing.T, token, authToken, webhookID, tradesPath string) {
	t.Helper()
	if s.stagingLiveBrokers() {
		t.Log("staging: skipping webhook trade assert (live/demo brokers without real creds)")
		return
	}
	type request struct {
		Action  string  `json:"action"`
		Symbol  string  `json:"symbol"`
		Lot     float64 `json:"lot"`
		SLPts   int     `json:"sl_pts"`
		TPPts   int     `json:"tp_pts"`
		Comment string  `json:"comment"`
	}
	status, raw := s.request(t, http.MethodPost, "/v1/webhook/"+token, "", request{
		Action:  "BUY",
		Symbol:  s.webhookTradeSymbol(),
		Lot:     0.01,
		SLPts:   20,
		TPPts:   28,
		Comment: "functional-test",
	})
	if status != http.StatusAccepted {
		t.Fatalf("POST /v1/webhook/%s: want status 202, got %d, body=%s", token, status, trimBody(raw))
	}
	var ingest map[string]any
	if err := json.Unmarshal(raw, &ingest); err != nil {
		t.Fatalf("signal ingest decode failed: %v, body=%s", err, trimBody(raw))
	}
	if ingest["queued"] != true {
		t.Fatalf("signal ingest did not queue job: %#v", ingest)
	}

	tr := s.waitForTradeWithComment(t, authToken, tradesPath, "functional-test")
	if tr.Signal != "BUY" {
		t.Fatalf("unexpected trade signal: %#v", tr)
	}
	if tr.Status != "submitted" {
		t.Fatalf("expected trade status submitted, got %#v", tr)
	}
	if tr.BrokerOrder == "" {
		t.Fatalf("expected broker order id on submitted trade: %#v", tr)
	}
	if tr.ErrorCode != "" {
		t.Fatalf("submitted trade should not have error_code: %#v", tr)
	}
	if tr.SignalKey == "" {
		t.Fatalf("expected signal_key on trade audit row: %#v", tr)
	}
}

func (s *state) waitForTradeStatus(t *testing.T, token, path, wantStatus string) tradeResult {
	t.Helper()
	restore := s.ensureAuditAccessForTrades(t, token)
	defer restore()
	deadline := time.Now().Add(12 * time.Second)
	var last []tradeResult
	for time.Now().Before(deadline) {
		callJSON(s, t, http.MethodGet, path, token, nil, http.StatusOK, &last)
		for _, tr := range last {
			if s.stagingLiveBrokers() && tr.ErrorCode == "rate_limited" {
				continue
			}
			if tr.Status == wantStatus {
				return tr
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for trade status %q at %s; last=%#v", wantStatus, path, last)
	return tradeResult{}
}

func (s *state) waitForTrades(t *testing.T, token, path string, min int) []tradeResult {
	t.Helper()
	restore := s.ensureAuditAccessForTrades(t, token)
	defer restore()
	deadline := time.Now().Add(12 * time.Second)
	var last []tradeResult
	for time.Now().Before(deadline) {
		callJSON(s, t, http.MethodGet, path, token, nil, http.StatusOK, &last)
		if len(last) >= min {
			return last
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d trades at %s; last=%d", min, path, len(last))
	return nil
}

func (s *state) waitForTradeWithComment(t *testing.T, token, path, comment string) tradeResult {
	t.Helper()
	restore := s.ensureAuditAccessForTrades(t, token)
	defer restore()
	deadline := time.Now().Add(12 * time.Second)
	var last []tradeResult
	for time.Now().Before(deadline) {
		callJSON(s, t, http.MethodGet, path, token, nil, http.StatusOK, &last)
		for _, tr := range last {
			if tr.Comment == comment {
				return tr
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for trade comment %q at %s; last=%#v", comment, path, last)
	return tradeResult{}
}

func (s *state) deleteNoContent(t *testing.T, method, path, token string) {
	t.Helper()
	status, raw := s.request(t, method, path, token, nil)
	if status != http.StatusNoContent {
		t.Fatalf("%s %s: want 204, got %d, body=%s", method, path, status, trimBody(raw))
	}
}

func (s *state) deleteWebhookByID(t *testing.T, id, token string) {
	t.Helper()
	s.deleteNoContent(t, http.MethodDelete, "/v1/webhooks/"+id, token)
}

func (s *state) assertAdminMe(t *testing.T) {
	t.Helper()
	var out meResult
	callJSON(s, t, http.MethodGet, "/v1/me", s.adminToken, nil, http.StatusOK, &out)
	if out.Role != "admin" {
		t.Fatalf("admin me role mismatch: got %s", out.Role)
	}
	if out.Email != s.adminEmail {
		t.Fatalf("admin me email mismatch: got %s want %s", out.Email, s.adminEmail)
	}
}

func (s *state) assertAdminUsersListPlain(t *testing.T) {
	t.Helper()
	var out adminUsersResult
	callJSON(s, t, http.MethodGet, "/v1/admin/users?page=1&limit=50", s.adminToken, nil, http.StatusOK, &out)
	if len(out.Users) == 0 {
		t.Fatal("expected plain admin user list to contain users")
	}
}

func (s *state) assertAdminUserListing(t *testing.T) {
	t.Helper()
	var out adminUsersResult
	callJSON(s, t, http.MethodGet, "/v1/admin/users?page=1&limit=50&status=active&email="+url.QueryEscape(s.ownerEmail[:strings.Index(s.ownerEmail, "@")]), s.adminToken, nil, http.StatusOK, &out)
	if len(out.Users) == 0 {
		t.Fatal("expected admin user listing to contain at least one user")
	}
	foundOwner := false
	for _, u := range out.Users {
		if u.ID == s.ownerID {
			foundOwner = true
			break
		}
	}
	if !foundOwner {
		t.Fatalf("owner user not found in admin listing: %#v", out.Users)
	}
}

func (s *state) assertAdminUserDetailAndPatch(t *testing.T) {
	t.Helper()
	var detail userSummary
	callJSON(s, t, http.MethodGet, "/v1/admin/users/"+s.tempID, s.adminToken, nil, http.StatusOK, &detail)
	if detail.ID != s.tempID {
		t.Fatalf("admin detail user mismatch: got %s want %s", detail.ID, s.tempID)
	}

	type patchBody struct {
		Status string `json:"status,omitempty"`
	}
	var patched userSummary
	callJSON(s, t, http.MethodPatch, "/v1/admin/users/"+s.tempID, s.adminToken, patchBody{Status: "suspended"}, http.StatusOK, &patched)
	if patched.Status != "suspended" {
		t.Fatalf("temp user status not suspended: %#v", patched)
	}
	callJSON(s, t, http.MethodPatch, "/v1/admin/users/"+s.tempID, s.adminToken, patchBody{Status: "active"}, http.StatusOK, &patched)
	if patched.Status != "active" {
		t.Fatalf("temp user status not restored: %#v", patched)
	}

	type planBody struct {
		Plan string `json:"plan"`
	}
	var ownerPlan userSummary
	callJSON(s, t, http.MethodPut, "/v1/admin/users/"+s.ownerID+"/plan", s.adminToken, planBody{Plan: "pro"}, http.StatusOK, &ownerPlan)
	if ownerPlan.Plan != "pro" {
		t.Fatalf("owner plan not updated: %#v", ownerPlan)
	}

	var ownerDetail userSummary
	callJSON(s, t, http.MethodGet, "/v1/admin/users/"+s.ownerID, s.adminToken, nil, http.StatusOK, &ownerDetail)
	if ownerDetail.ID != s.ownerID {
		t.Fatalf("admin owner detail mismatch: got %s want %s", ownerDetail.ID, s.ownerID)
	}
	if ownerDetail.Plan != "pro" {
		t.Fatalf("admin owner detail plan mismatch: %#v", ownerDetail)
	}
	s.upgradeOwnerPlan(t, "free")
}

func (s *state) upgradeOwnerPlan(t *testing.T, planName string) {
	t.Helper()
	type planBody struct {
		Plan string `json:"plan"`
	}
	var out userSummary
	callJSON(s, t, http.MethodPut, "/v1/admin/users/"+s.ownerID+"/plan", s.adminToken, planBody{Plan: planName}, http.StatusOK, &out)
	if out.Plan != planName {
		t.Fatalf("owner plan not updated: got %s want %s", out.Plan, planName)
	}
	if out.BillingSource != "admin" {
		t.Fatalf("billing_source want admin got %q", out.BillingSource)
	}
}

func (s *state) wsURL(path string) string {
	base := s.baseURL
	if strings.HasPrefix(base, "https://") {
		return "wss://" + strings.TrimPrefix(base, "https://") + path
	}
	if strings.HasPrefix(base, "http://") {
		return "ws://" + strings.TrimPrefix(base, "http://") + path
	}
	return base + path
}

func (s *state) assertWebhookPnL(t *testing.T, token, webhookID string, minCount int) {
	t.Helper()
	restore := s.ensureAuditAccessForTrades(t, token)
	defer restore()

	var got pnlResult
	callJSON(s, t, http.MethodGet, "/v1/webhooks/"+webhookID+"/pnl", token, nil, http.StatusOK, &got)
	if got.WebhookID != webhookID {
		t.Fatalf("webhook pnl id mismatch: got %q want %q", got.WebhookID, webhookID)
	}
	if got.TradeCount < minCount {
		t.Fatalf("webhook pnl trade_count want >= %d got %d: %#v", minCount, got.TradeCount, got)
	}
}

func (s *state) assertSessionLogout(t *testing.T) {
	t.Helper()
	callExpectStatus(s, t, http.MethodPost, "/v1/auth/logout", s.adminToken, nil, http.StatusNoContent)
	callExpectStatus(s, t, http.MethodGet, "/v1/me", s.adminToken, nil, http.StatusUnauthorized)
	s.adminToken = ""
}

func (s *state) assertWebSocketTradePush(t *testing.T, authToken, webhookToken, webhookID string) {
	t.Helper()
	restore := s.ensureAuditAccessForTrades(t, authToken)
	defer restore()

	msgCh := make(chan wsTradeMessage, 1)
	errCh := make(chan error, 1)

	wsPath := "/v1/ws/trades?token=" + url.QueryEscape(authToken)
	conn, resp, err := websocket.DefaultDialer.Dial(s.wsURL(wsPath), http.Header{})
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("ws dial: %v status=%d body=%s", err, resp.StatusCode, trimBody(body))
		}
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()
	if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("ws upgrade want 101 got %d", resp.StatusCode)
	}

	go func() {
		_, data, err := conn.ReadMessage()
		if err != nil {
			errCh <- err
			return
		}
		var msg wsTradeMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			errCh <- err
			return
		}
		msgCh <- msg
	}()

	comment := "ws-fn-" + uuid.NewString()[:8]
	type ingestReq struct {
		Action  string  `json:"action"`
		Symbol  string  `json:"symbol"`
		Lot     float64 `json:"lot"`
		Comment string  `json:"comment"`
	}
	status, raw := s.request(t, http.MethodPost, "/v1/webhook/"+webhookToken, "", ingestReq{
		Action: "BUY", Symbol: "EURUSD", Lot: 0.01, Comment: comment,
	})
	if status != http.StatusAccepted {
		t.Fatalf("ingest for ws test: want 202 got %d body=%s", status, trimBody(raw))
	}

	select {
	case msg := <-msgCh:
		if msg.Type != "trade" {
			t.Fatalf("ws message type want trade got %q", msg.Type)
		}
		if msg.Trade.WebhookID != webhookID {
			t.Fatalf("ws trade webhook_id want %s got %s", webhookID, msg.Trade.WebhookID)
		}
	case err := <-errCh:
		t.Fatalf("ws read: %v", err)
	case <-time.After(12 * time.Second):
		t.Fatal("timed out waiting for ws trade push")
	}
}

// ── Platform invite flow (5C.1) ───────────────────────────────────────────────

func (s *state) assertPlatformInviteFlow(t *testing.T) {
	t.Helper()

	// Create invite (no email restriction, open for any address)
	type createBody struct {
		Note string `json:"note"`
	}
	type createResult struct {
		ID        string `json:"id"`
		Token     string `json:"token"`
		Note      string `json:"note"`
		ExpiresAt string `json:"expires_at"`
	}
	var inv createResult
	callJSON(s, t, http.MethodPost, "/v1/admin/invites", s.adminToken, createBody{Note: "functional-test invite"}, http.StatusCreated, &inv)
	if !strings.HasPrefix(inv.Token, "inv_") {
		t.Fatalf("platform invite token should start with inv_: %q", inv.Token)
	}
	if inv.ID == "" {
		t.Fatal("platform invite id is empty")
	}
	s.platformInviteToken = inv.Token
	s.platformInviteID = inv.ID

	// List invites — should include the one we just created
	type listResult struct {
		ID    string `json:"id"`
		Note  string `json:"note"`
	}
	var invList []listResult
	callJSON(s, t, http.MethodGet, "/v1/admin/invites", s.adminToken, nil, http.StatusOK, &invList)
	found := false
	for _, row := range invList {
		if row.ID == s.platformInviteID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created invite %s not found in list (%d items)", s.platformInviteID, len(invList))
	}

	// Register using the invite token
	type regBody struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code"`
	}
	type regResult struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	var regOut regResult
	callJSON(s, t, http.MethodPost, "/v1/auth/register", "", regBody{
		Email:      s.betaUserEmail,
		Password:   s.betaUserPassword,
		InviteCode: s.platformInviteToken,
	}, http.StatusCreated, &regOut)
	if regOut.ID == "" {
		t.Fatal("beta user registration returned empty id")
	}
	if regOut.Email != s.betaUserEmail {
		t.Fatalf("beta user email mismatch: got %q want %q", regOut.Email, s.betaUserEmail)
	}

	// Login as beta user
	type loginBody struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	type loginResult2 struct {
		Token string `json:"token"`
	}
	var loginOut loginResult2
	callJSON(s, t, http.MethodPost, "/v1/auth/login", "", loginBody{
		Email:    s.betaUserEmail,
		Password: s.betaUserPassword,
	}, http.StatusOK, &loginOut)
	if loginOut.Token == "" {
		t.Fatal("beta user login returned empty token")
	}
	s.betaUserToken = loginOut.Token

	// Revoke a fresh invite (admin CRUD; always tested regardless of CLOSED_REGISTRATION)
	var revokeInv createResult
	callJSON(s, t, http.MethodPost, "/v1/admin/invites", s.adminToken, createBody{Note: "to-revoke"}, http.StatusCreated, &revokeInv)
	callExpectStatus(s, t, http.MethodDelete, "/v1/admin/invites/"+revokeInv.ID, s.adminToken, nil, http.StatusNoContent)

	if !s.serverEnforcesClosedRegistration(t) {
		// When the server is not configured for closed registration, invite enforcement
		// assertions are not meaningful.
		t.Log("server open registration: skipping invite enforcement assertions")
		return
	}

	// Re-using the same invite should fail (already used)
	callExpectStatus(s, t, http.MethodPost, "/v1/auth/register", "", regBody{
		Email:      uniqueEmail("beta2"),
		Password:   "betapass456",
		InviteCode: s.platformInviteToken,
	}, http.StatusBadRequest)

	// Revoked invite cannot be used
	callExpectStatus(s, t, http.MethodPost, "/v1/auth/register", "", regBody{
		Email:      uniqueEmail("beta3"),
		Password:   "betapass789",
		InviteCode: revokeInv.Token,
	}, http.StatusBadRequest)
}

// ── Bracket order (SL/TP) functional tests (3.3) ─────────────────────────────

func (s *state) assertBracketOrderTrade(t *testing.T) {
	t.Helper()
	if s.stagingLiveBrokers() {
		t.Log("staging: skipping bracket order trade assert (live brokers without real creds)")
		return
	}
	if !s.brokerEnabled("mt5_cloud") {
		t.Log("skipping bracket order trade assert (mt5 adapter not enabled)")
		return
	}

	// Clear the per-user rate limit window (10 req/s) accumulated by auth_and_personal_resources.
	time.Sleep(1100 * time.Millisecond)

	// Create an MT5 credential (mock mode handles it without real creds)
	type credBody struct {
		Label       string `json:"label"`
		BrokerType  string `json:"broker_type"`
		AccountMode string `json:"account_mode"`
		RawCreds    string `json:"raw_creds"`
	}
	type credResult2 struct {
		ID string `json:"id"`
	}
	var cred credResult2
	callJSON(s, t, http.MethodPost, "/v1/credentials", s.ownerToken, credBody{
		Label:       "bracket-test-mt5",
		BrokerType:  "mt5_cloud",
		AccountMode: "live",
		RawCreds:    "bracket-tok:acct-bracket",
	}, http.StatusCreated, &cred)
	if cred.ID == "" {
		t.Fatal("bracket test credential create returned empty id")
	}

	// Create webhook with sl_pts / tp_pts defaults
	type webhookResult2 struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	var wh webhookResult2
	callJSON(s, t, http.MethodPost, "/v1/webhooks", s.ownerToken, createWebhookRequest{
		Label:          "bracket-test-wh",
		BrokerCredID:   cred.ID,
		AllowedSymbols: webhookSymbols("EURUSD"),
		LotSize:        0.01,
		SLPoints:       20,
		TPPoints:       30,
	}, http.StatusCreated, &wh)
	if wh.Token == "" {
		t.Fatal("bracket test webhook create returned empty token")
	}
	s.bracketWebhookID = wh.ID
	s.bracketWebhookToken = wh.Token

	// Ingest a BUY signal with sl_pts overriding the webhook default
	type ingestBody struct {
		Action  string  `json:"action"`
		Symbol  string  `json:"symbol"`
		Lot     float64 `json:"lot"`
		SLPts   int     `json:"sl_pts"`
		TPPts   int     `json:"tp_pts"`
		Comment string  `json:"comment"`
	}
	callExpectStatus(s, t, http.MethodPost, "/v1/webhook/"+s.bracketWebhookToken, "", ingestBody{
		Action:  "BUY",
		Symbol:  "EURUSD",
		Lot:     0.01,
		SLPts:   15,
		TPPts:   25,
		Comment: "bracket-functional",
	}, http.StatusAccepted)

	// Wait for trade — mock broker submits immediately
	tr := s.waitForTradeWithComment(t, s.ownerToken, "/v1/webhooks/"+s.bracketWebhookID+"/trades", "bracket-functional")
	if tr.Status != "submitted" {
		t.Fatalf("bracket order trade status want submitted got %q error=%q code=%q", tr.Status, tr.Error, tr.ErrorCode)
	}
	if tr.OrderType != "BRACKET" {
		t.Fatalf("bracket order trade order_type want BRACKET got %q", tr.OrderType)
	}

	// Create a second webhook without SL/TP defaults to test pure market order routing.
	var mktWh webhookResult2
	callJSON(s, t, http.MethodPost, "/v1/webhooks", s.ownerToken, createWebhookRequest{
		Label:          "market-test-wh",
		BrokerCredID:   cred.ID,
		AllowedSymbols: webhookSymbols("EURUSD"),
		LotSize:        0.01,
	}, http.StatusCreated, &mktWh)
	if mktWh.Token == "" {
		t.Fatal("market test webhook create returned empty token")
	}

	// Ingest a plain market-routed signal (no SL/TP) — entry type follows webhook default (LIMIT).
	callExpectStatus(s, t, http.MethodPost, "/v1/webhook/"+mktWh.Token, "", ingestBody{
		Action:  "BUY",
		Symbol:  "EURUSD",
		Lot:     0.01,
		Comment: "market-functional",
	}, http.StatusAccepted)

	mkt := s.waitForTradeWithComment(t, s.ownerToken, "/v1/webhooks/"+mktWh.ID+"/trades", "market-functional")
	if mkt.Status != "submitted" {
		t.Fatalf("market order trade status want submitted got %q error=%q code=%q", mkt.Status, mkt.Error, mkt.ErrorCode)
	}
	if mkt.OrderType != "LIMIT" {
		t.Fatalf("market-routed order trade order_type want LIMIT got %q", mkt.OrderType)
	}

	s.deleteWebhookByID(t, mktWh.ID, s.ownerToken)
	s.deleteWebhookByID(t, s.bracketWebhookID, s.ownerToken)
	s.deleteCredential(t, cred.ID, s.ownerToken, "/v1/credentials/"+cred.ID)
}

func (s *state) assertCNCBracketRejected(t *testing.T) {
	t.Helper()
	if s.stagingLiveBrokers() {
		t.Log("staging: skipping CNC bracket rejection test")
		return
	}

	// Create a Zerodha CNC credential
	type credBody struct {
		Label        string `json:"label"`
		BrokerType   string `json:"broker_type"`
		AccountMode  string `json:"account_mode"`
		RawCreds     string `json:"raw_creds"`
		Exchange     string `json:"exchange"`
		Product      string `json:"product"`
	}
	type credResult2 struct {
		ID string `json:"id"`
	}
	var cred credResult2
	callJSON(s, t, http.MethodPost, "/v1/credentials", s.ownerToken, credBody{
		Label:       "cnc-bracket-test",
		BrokerType:  "zerodha",
		AccountMode: "live",
		RawCreds:    "api_key:mock_secret:mock_access_token",
		Exchange:    "NSE",
		Product:     "CNC",
	}, http.StatusCreated, &cred)
	if cred.ID == "" {
		t.Fatal("CNC credential create returned empty id")
	}
	s.cncCredID = cred.ID

	// Create webhook
	type webhookResult2 struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	var wh webhookResult2
	callJSON(s, t, http.MethodPost, "/v1/webhooks", s.ownerToken, createWebhookRequest{
		Label:          "cnc-bracket-wh",
		BrokerCredID:   cred.ID,
		AllowedSymbols: webhookSymbols("RELIANCE"),
		LotSize:        1,
	}, http.StatusCreated, &wh)
	if wh.Token == "" {
		t.Fatal("CNC bracket test webhook create returned empty token")
	}
	s.cncWebhookID = wh.ID
	s.cncWebhookToken = wh.Token

	// Ingest BUY with sl_pts — should be rejected (CNC + bracket)
	type ingestBody struct {
		Action  string  `json:"action"`
		Symbol  string  `json:"symbol"`
		Lot     float64 `json:"lot"`
		SLPts   int     `json:"sl_pts"`
		Comment string  `json:"comment"`
	}
	callExpectStatus(s, t, http.MethodPost, "/v1/webhook/"+s.cncWebhookToken, "", ingestBody{
		Action:  "BUY",
		Symbol:  "RELIANCE",
		Lot:     1,
		SLPts:   20,
		Comment: "cnc-bracket-test",
	}, http.StatusAccepted)

	tr := s.waitForTradeStatus(t, s.ownerToken, "/v1/webhooks/"+s.cncWebhookID+"/trades", "rejected")
	if tr.ErrorCode != "bracket_not_supported" {
		t.Fatalf("CNC bracket rejection: want error_code=bracket_not_supported got %q (error=%q)", tr.ErrorCode, tr.Error)
	}

	s.deleteWebhookByID(t, s.cncWebhookID, s.ownerToken)
	s.deleteCredential(t, s.cncCredID, s.ownerToken, "/v1/credentials/"+s.cncCredID)
}

func (s *state) assertCancelOrderFlow(t *testing.T) {
	t.Helper()

	// Rate-limit cooldown from prior test.
	time.Sleep(1100 * time.Millisecond)

	type credBody struct {
		Label       string `json:"label"`
		BrokerType  string `json:"broker_type"`
		AccountMode string `json:"account_mode"`
		RawCreds    string `json:"raw_creds"`
		Exchange    string `json:"exchange"`
		Product     string `json:"product"`
	}
	type credResult struct {
		ID string `json:"id"`
	}
	var cred credResult
	callJSON(s, t, http.MethodPost, "/v1/credentials", s.ownerToken, credBody{
		Label:       "cancel-test-zerodha",
		BrokerType:  "zerodha",
		AccountMode: "live",
		RawCreds:    "api_key:mock_secret:mock_access_token",
		Exchange:    "NSE",
		Product:     "MIS",
	}, http.StatusCreated, &cred)
	if cred.ID == "" {
		t.Fatal("cancel test credential create returned empty id")
	}

	type whResult struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	var wh whResult
	callJSON(s, t, http.MethodPost, "/v1/webhooks", s.ownerToken, createWebhookRequest{
		Label:          "cancel-test-wh",
		BrokerCredID:   cred.ID,
		AllowedSymbols: webhookSymbols("RELIANCE"),
		LotSize:        1,
	}, http.StatusCreated, &wh)
	if wh.Token == "" {
		t.Fatal("cancel test webhook create returned empty token")
	}

	type signalReq struct {
		Action string  `json:"action"`
		Symbol string  `json:"symbol"`
		Lot    float64 `json:"lot"`
	}
	callExpectStatus(s, t, http.MethodPost, "/v1/webhook/"+wh.Token, "", signalReq{
		Action: "BUY",
		Symbol: "RELIANCE",
		Lot:    1,
	}, http.StatusAccepted)

	tradesPath := "/v1/webhooks/" + wh.ID + "/trades"
	tr := s.waitForTradeStatus(t, s.ownerToken, tradesPath, "submitted")
	if tr.ID == "" {
		t.Fatal("cancel test: expected submitted trade")
	}
	if !strings.Contains(tr.BrokerOrder, "MOCK") {
		t.Fatalf("cancel test: expected mock broker order, got %q", tr.BrokerOrder)
	}

	// Cancel the trade.
	type cancelResult struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		BrokerOrder string `json:"broker_order"`
	}
	var cancelled cancelResult
	callJSON(s, t, http.MethodDelete, "/v1/webhooks/"+wh.ID+"/trades/"+tr.ID, s.ownerToken, nil, http.StatusOK, &cancelled)
	if cancelled.Status != "cancelled" {
		t.Fatalf("cancel trade: expected status=cancelled, got %q", cancelled.Status)
	}
	if cancelled.ID != tr.ID {
		t.Fatalf("cancel trade: returned id %q != requested %q", cancelled.ID, tr.ID)
	}

	// Verify the trade list reflects the cancelled status.
	var trades []tradeResult
	callJSON(s, t, http.MethodGet, tradesPath, s.ownerToken, nil, http.StatusOK, &trades)
	found := false
	for _, t2 := range trades {
		if t2.ID == tr.ID {
			found = true
			if t2.Status != "cancelled" {
				t.Fatalf("cancel trade: trade list shows status=%q after cancel", t2.Status)
			}
			if t2.CancelledAt == nil {
				t.Fatal("cancel trade: cancelled_at should be set after cancel")
			}
		}
	}
	if !found {
		t.Fatal("cancel trade: cancelled trade not found in list")
	}

	// Double-cancel should return 422.
	callExpectStatus(s, t, http.MethodDelete, "/v1/webhooks/"+wh.ID+"/trades/"+tr.ID, s.ownerToken, nil, http.StatusUnprocessableEntity)

	s.deleteWebhookByID(t, wh.ID, s.ownerToken)
	s.deleteCredential(t, cred.ID, s.ownerToken, "/v1/credentials/"+cred.ID)
}

// assertFreePlanLimits verifies that a freshly registered free-plan user's /v1/me
// reflects the correct plan limits: 1 paper webhook, 0 live broker credentials, 1 order/sec enforced.
func (s *state) assertFreePlanLimits(t *testing.T) {
	t.Helper()
	var out meResult
	callJSON(s, t, http.MethodGet, "/v1/me", s.ownerToken, nil, http.StatusOK, &out)
	if out.Plan != "free" {
		t.Fatalf("assertFreePlanLimits: expected plan=free, got %q", out.Plan)
	}
	if out.MaxWebhooks != 1 {
		t.Fatalf("free max_webhooks want 1, got %d", out.MaxWebhooks)
	}
	if out.MaxBrokerCreds != 0 {
		t.Fatalf("free max_broker_creds want 0, got %d", out.MaxBrokerCreds)
	}
	if out.OrdersPerSecEnforced != 1 {
		t.Fatalf("free orders_per_sec_enforced want 1, got %d", out.OrdersPerSecEnforced)
	}
	if out.BrokerCredOrdersPerSec != 10 {
		t.Fatalf("free broker_cred_orders_per_sec want 10, got %d", out.BrokerCredOrdersPerSec)
	}
}

