package credentials

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	zerodhaoauth "github.com/SPSingh09/zettabridge/internal/adapters/zerodha/oauth"
	"github.com/SPSingh09/zettabridge/internal/algo"
	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/credentialproduct"
	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/auth"
	zerodhasettings "github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/settings"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/platform/crypto"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

// Handler handles broker credential CRUD and Zerodha OAuth.
type Handler struct {
	*shared.Handler
	Infra *livebrokers.Infra
}

var ist = time.FixedZone("IST", 5*3600+30*60)

// ZerodhaTokenStatus computes whether a Zerodha credential needs reconnecting.
// Kite tokens expire at 6:00 AM IST daily. Statuses:
//
//	valid        — connected_at is after the last 6:00 AM IST expiry
//	expires_soon — valid but within 20 minutes of the next 6:00 AM IST expiry
//	expired      — connected_at is before the last 6:00 AM IST expiry, or never set
func ZerodhaTokenStatus(connectedAt *time.Time) string {
	now := time.Now().In(ist)
	todayCutoff := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, ist)

	// Use the last 6 AM expiry as cutoff:
	// - After 6 AM IST: today's 6 AM (tokens from before 6 AM today are expired)
	// - Before 6 AM IST: yesterday's 6 AM (tokens connected since yesterday 6 AM are still valid)
	lastExpiry := todayCutoff
	if now.Before(todayCutoff) {
		lastExpiry = todayCutoff.AddDate(0, 0, -1)
	}

	if connectedAt == nil || connectedAt.Before(lastExpiry) {
		return "expired"
	}
	// token is valid — warn if we're within 20 minutes of the next 6:00 AM expiry
	if now.Hour() == 5 && now.Minute() >= 40 {
		return "expires_soon"
	}
	return "valid"
}

// ── Credential package-level functions ────────────────────────────────────────

// IsValidBrokerType reports whether t is a supported broker type.
func IsValidBrokerType(t string) bool {
	switch t {
	case "mt5_cloud", "zerodha", "angel", "dhan":
		return true
	default:
		return false
	}
}

// NormalizeExecutionMode validates and returns the canonical execution_mode for a broker type.
func NormalizeExecutionMode(brokerType, requestedMode string) (string, error) {
	switch brokerType {
	case "zerodha":
		switch requestedMode {
		case "publisher":
			return "publisher", nil
		case "user_api_oauth", "":
			return "user_api_oauth", nil
		default:
			return "", fmt.Errorf("execution_mode %q is not supported for zerodha (use user_api_oauth or publisher)", requestedMode)
		}
	default:
		if requestedMode != "" && requestedMode != "direct_api" {
			return "", fmt.Errorf("execution_mode %q is not supported for %s; only direct_api is allowed", requestedMode, brokerType)
		}
		return "direct_api", nil
	}
}

// CredentialResponse returns a sanitized map for a broker credential response.
func CredentialResponse(bc *store.BrokerCredential) fiber.Map {
	products := credentialproduct.Parse(bc.Product)
	out := fiber.Map{
		"id":                bc.ID,
		"broker_type":       bc.BrokerType,
		"label":             bc.AccountLabel,
		"account_mode":      bc.AccountMode,
		"exchange":          bc.Exchange,
		"product":           credentialproduct.Default(bc.Product),
		"products":          products,
		"market_protection": bc.MarketProtection,
		"execution_mode":    bc.ExecutionMode,
	}
	if bc.AlgoID != "" {
		out["algo_id"] = bc.AlgoID
	}
	return out
}

// NormalizeCredentialOptions validates and normalizes exchange, product, and order_type for a domain.
func NormalizeCredentialOptions(brokerType, exchange, product, orderType string) (string, string, string, error) {
	switch brokerType {
	case "mt5_cloud":
		return "", "", "MARKET", nil
	case "zerodha", "angel", "dhan":
		ex := strings.TrimSpace(exchange)
		if ex == "" {
			return "", "", "", fmt.Errorf("exchange is required for Indian brokers (NSE or BSE)")
		}
		ex = strings.ToUpper(ex)
		if ex != "NSE" && ex != "BSE" {
			return "", "", "", fmt.Errorf("exchange must be NSE or BSE")
		}
		if err := credentialproduct.ValidateStored(ex, product); err != nil {
			return "", "", "", err
		}
		prod := credentialproduct.Default(product)
		ot := strings.ToUpper(strings.TrimSpace(orderType))
		if ot == "" {
			ot = "MARKET"
		}
		if ot != "MARKET" && ot != "LIMIT" {
			return "", "", "", fmt.Errorf("order_type must be MARKET or LIMIT")
		}
		return ex, prod, ot, nil
	default:
		return "", "", "", fmt.Errorf("unsupported broker_type")
	}
}

func productsFromRequest(product string, products []string) []string {
	if len(products) > 0 {
		return products
	}
	trimmed := strings.TrimSpace(product)
	if trimmed == "" {
		return nil
	}
	if strings.Contains(trimmed, ",") {
		return credentialproduct.Parse(trimmed)
	}
	return []string{trimmed}
}

func normalizeCredentialProducts(brokerType, exchange string, products []string, allowMultiple bool) (string, string, error) {
	switch brokerType {
	case "mt5_cloud":
		return "", "", nil
	case "zerodha", "angel", "dhan":
		ex, stored, err := credentialproduct.NormalizeSettings(exchange, products, allowMultiple)
		return ex, stored, err
	default:
		return "", "", fmt.Errorf("unsupported broker_type")
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// CredentialVerifyFailure returns a failure response map for credential verification.
func CredentialVerifyFailure(brokerType, errMsg string, err error) map[string]interface{} {
	out := map[string]interface{}{
		"valid":       false,
		"broker_type": brokerType,
		"error":       errMsg,
	}
	if err != nil {
		if code := brokererr.CodeOf(err); code != "" {
			out["error_code"] = string(code)
		}
		if hint := CredentialVerifyHint(brokerType, err); hint != "" {
			out["hint"] = hint
		}
	} else if errMsg != "" && strings.Contains(errMsg, "invalid format") {
		out["hint"] = fmt.Sprintf("Use raw_creds format %s", brokercreds.FormatHint(brokerType))
	}
	return out
}

// CredentialVerifyHint returns a user-facing hint for a credential verification error.
func CredentialVerifyHint(brokerType string, err error) string {
	if err == nil {
		return ""
	}
	switch brokererr.CodeOf(err) {
	case brokererr.CodeAuthFailed:
		return credentialAuthHint(brokerType)
	case brokererr.CodeBrokerUnreachable:
		return "Broker API unreachable — check network, BROKER_*_BASE_URL, and static IP whitelist with your broker"
	case brokererr.CodeRateLimited:
		return "Broker rate limit hit — wait a few seconds and retry verify"
	case brokererr.CodeInvalidSymbol:
		return "Symbol or security ID not recognized — check exchange (NSE/BSE) and trading symbol on the credential webhook"
	case brokererr.CodeInsufficientMargin:
		return "Account has insufficient margin for a probe call — credentials may still be valid; check fund limits in broker portal"
	default:
		return credentialAuthHint(brokerType)
	}
}

func credentialAuthHint(brokerType string) string {
	switch brokerType {
	case "zerodha":
		return "Kite access token may have expired — generate a new daily token and PUT raw_creds (api_key:access_token)"
	case "angel":
		return "SmartAPI JWT may have expired — re-login and PUT raw_creds (api_key:client_code:jwt)"
	case "dhan":
		return "Dhan access token expires after ~24h — regenerate from web.dhan.co and PUT raw_creds (client_id:access_token)"
	case "mt5_cloud":
		return "MetaApi auth token or account_id may be wrong — verify auth_token:account_id in MetaApi dashboard"
	default:
		return "Re-PUT raw_creds after refreshing broker session tokens (manual refresh only in v1)"
	}
}

// ── Handler methods ───────────────────────────────────────────────────────────

// EnforceResolvableAlgoID checks that the algo ID on a credential can be resolved.
func (h *Handler) EnforceResolvableAlgoID(bc *store.BrokerCredential) error {
	_, err := algo.ResolveForCredential(bc, algo.FromConfig(h.Cfg))
	return err
}

func (h *Handler) ListCredentials(c *fiber.Ctx) error {
	scope, err := h.ResolveResourceScope(c)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "scope lookup failed")
	}
	creds, err := h.PG.ListBrokerCredsForScope(c.Context(), scope)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list credentials")
	}
	if creds == nil {
		creds = []*store.BrokerCredential{}
	}
	return shared.Ok(c, creds)
}

func (h *Handler) ListEnabledBrokers(c *fiber.Ctx) error {
	router := execution.NewRouter(h.Cfg.EnabledAdapters)
	return shared.Ok(c, fiber.Map{
		"enabled_adapters": router.EnabledAdapters(),
		"enabled_brokers":  router.EnabledBrokers(),
	})
}

func (h *Handler) CreateCredential(c *fiber.Ctx) error {
	scope, err := h.ResolveResourceScope(c)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "scope lookup failed")
	}

	var body struct {
		BrokerType       string   `json:"broker_type"`
		AccountLabel     string   `json:"account_label"`
		AccountMode      string   `json:"account_mode"`      // live
		Exchange         string   `json:"exchange"`          // NSE | BSE (Indian brokers)
		Product          string   `json:"product"`           // MIS | CNC | NRML (single-product plans)
		Products         []string `json:"products"`          // Pro Plus: one or more of MIS, CNC, NRML
		AlgoID           string   `json:"algo_id"`           // SEBI exchange algo ID (Indian live)
		MarketProtection float64  `json:"market_protection"` // Zerodha market protection % (MARKET) or LTP slippage % (LIMIT)
		RawCreds         string   `json:"raw_creds"`         // colon-delimited per broker_type
		ExecutionMode    string   `json:"execution_mode"`    // user_api_oauth | publisher | direct_api
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	if !IsValidBrokerType(body.BrokerType) {
		return shared.Fail(c, fiber.StatusBadRequest, "broker_type required")
	}
	router := execution.NewRouter(h.Cfg.EnabledAdapters)
	if err := router.RequireBroker(body.BrokerType); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, fmt.Sprintf("broker %s is not enabled on this deployment", body.BrokerType))
	}

	executionMode, err := NormalizeExecutionMode(body.BrokerType, body.ExecutionMode)
	if err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, err.Error())
	}
	if body.BrokerType == "zerodha" {
		if executionMode == "publisher" {
			if !zerodhasettings.PublisherEnabled(c.Context(), h.PG) {
				return shared.Fail(c, fiber.StatusForbidden, "Zerodha Publisher is currently disabled by the platform admin")
			}
		} else if !zerodhasettings.OAuthEnabled(c.Context(), h.PG) {
			return shared.Fail(c, fiber.StatusForbidden, "Zerodha OAuth is currently disabled by the platform admin")
		}
	}

	// Load user early — needed to gate Zerodha OAuth skip for free-plan users.
	user, err := h.PG.GetUserByID(c.Context(), scope.UserID)
	if err != nil || user == nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user")
	}

	// Publisher mode uses only api_key — no OAuth required, but raw_creds must be encrypted.
	_ = executionMode == "publisher"

	if strings.TrimSpace(body.RawCreds) == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "raw_creds is required")
	}
	if err := brokercreds.Validate(body.BrokerType, body.RawCreds); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest,
			fmt.Sprintf("invalid raw_creds (expected %s)", brokercreds.FormatHintForMode(body.BrokerType, executionMode)))
	}

	exchange, product, err := normalizeCredentialProducts(
		body.BrokerType, body.Exchange, productsFromRequest(body.Product, body.Products),
		plan.LimitsFor(plan.EffectivePlan(user.Plan)).MultiProductCredentials,
	)
	if err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, err.Error())
	}

	if err := h.EnforceBrokerLimit(c, scope, user); err != nil {
		return shared.PlanError(c, err)
	}

	accountMode, err := plan.NormalizeAccountMode(user.Plan, body.AccountMode)
	if err != nil {
		return shared.PlanError(c, err)
	}

	credID := uuid.New().String()
	credKey, keyErr := h.Cfg.AESKeyBytes()
	if keyErr != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load encryption key")
	}
	encrypted, encErr := credenc.Encrypt(body.RawCreds, credKey, credID)
	if encErr != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not save credential")
	}

	bc := &store.BrokerCredential{
		ID:               credID,
		UserID:           scope.UserID,
		CreatedBy:        scope.UserID,
		BrokerType:       body.BrokerType,
		EncryptedCreds:   encrypted,
		AccountLabel:     body.AccountLabel,
		AccountMode:      accountMode,
		Exchange:         exchange,
		Product:          product,
		AlgoID:           strings.TrimSpace(body.AlgoID),
		OrderType:        "MARKET",
		MarketProtection: body.MarketProtection,
		ExecutionMode:    executionMode,
	}
	if err := h.EnforceResolvableAlgoID(bc); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, brokererr.PublicFrom(err))
	}
	if err := h.PG.CreateBrokerCred(c.Context(), bc); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not save credential")
	}
	h.LogUserAudit(c, bc.UserID, "credential_created", map[string]any{"broker_type": bc.BrokerType, "label": bc.AccountLabel})
	return shared.Created(c, CredentialResponse(bc))
}

func (h *Handler) UpdateCredential(c *fiber.Ctx) error {
	id := c.Params("id")

	existing, err := h.RequireCredentialAccess(c, id, true)
	if err != nil {
		return err
	}

	var body struct {
		BrokerType       *string  `json:"broker_type"`
		AccountLabel     *string  `json:"account_label"`
		AccountMode      *string  `json:"account_mode"`
		Exchange         *string  `json:"exchange"`
		Product          *string  `json:"product"`
		Products         []string `json:"products"`
		AlgoID           *string  `json:"algo_id"`
		MarketProtection *float64 `json:"market_protection"`
		RawCreds         *string  `json:"raw_creds"`
		ExecutionMode    *string  `json:"execution_mode"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	if body.BrokerType == nil && body.AccountLabel == nil && body.AccountMode == nil &&
		body.Exchange == nil && body.Product == nil && len(body.Products) == 0 && body.AlgoID == nil &&
		body.MarketProtection == nil && body.RawCreds == nil &&
		body.ExecutionMode == nil {
		return shared.Fail(c, fiber.StatusBadRequest, "at least one field required")
	}

	user, err := h.PG.GetUserByID(c.Context(), existing.UserID)
	if err != nil || user == nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user")
	}

	if body.ExecutionMode != nil {
		newMode, err := NormalizeExecutionMode(existing.BrokerType, *body.ExecutionMode)
		if err != nil {
			return shared.Fail(c, fiber.StatusBadRequest, err.Error())
		}
		if newMode != existing.ExecutionMode {
			count, err := h.PG.CountActiveWebhooksForCred(c.Context(), existing.ID)
			if err != nil {
				return shared.Fail(c, fiber.StatusInternalServerError, "could not check webhook usage")
			}
			if count > 0 && c.Query("force") != "true" {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error":           fmt.Sprintf("credential has %d active webhook(s); pass ?force=true to switch execution mode", count),
					"active_webhooks": count,
				})
			}
		}
		existing.ExecutionMode = newMode
	}

	if body.BrokerType != nil {
		if *body.BrokerType == "" || !IsValidBrokerType(*body.BrokerType) {
			return shared.Fail(c, fiber.StatusBadRequest, "invalid broker_type")
		}
		existing.BrokerType = *body.BrokerType
	}
	if body.Exchange != nil || body.Product != nil || len(body.Products) > 0 {
		ex := existing.Exchange
		if body.Exchange != nil {
			ex = *body.Exchange
		}
		reqProducts := productsFromRequest(derefString(body.Product), body.Products)
		if len(reqProducts) == 0 {
			reqProducts = credentialproduct.Parse(existing.Product)
		}
		ex, prod, err := normalizeCredentialProducts(
			existing.BrokerType, ex, reqProducts,
			plan.LimitsFor(plan.EffectivePlan(user.Plan)).MultiProductCredentials,
		)
		if err != nil {
			return shared.Fail(c, fiber.StatusBadRequest, err.Error())
		}
		existing.Exchange = ex
		existing.Product = prod
	}
	if body.MarketProtection != nil {
		existing.MarketProtection = *body.MarketProtection
	}
	if body.AccountLabel != nil {
		existing.AccountLabel = *body.AccountLabel
	}
	if body.AlgoID != nil {
		existing.AlgoID = strings.TrimSpace(*body.AlgoID)
	}
	if body.AccountMode != nil {
		accountMode, err := plan.NormalizeAccountMode(user.Plan, *body.AccountMode)
		if err != nil {
			return shared.PlanError(c, err)
		}
		existing.AccountMode = accountMode
	}
	if body.RawCreds != nil {
		if strings.TrimSpace(*body.RawCreds) == "" {
			return shared.Fail(c, fiber.StatusBadRequest, "raw_creds cannot be empty")
		}
		if err := brokercreds.Validate(existing.BrokerType, *body.RawCreds); err != nil {
			return shared.Fail(c, fiber.StatusBadRequest,
				fmt.Sprintf("invalid raw_creds (expected %s)", brokercreds.FormatHintForMode(existing.BrokerType, existing.ExecutionMode)))
		}
		credKey, err := h.Cfg.AESKeyBytes()
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "could not load encryption key")
		}
		encrypted, err := credenc.Encrypt(*body.RawCreds, credKey, existing.ID)
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "could not save credential")
		}
		existing.EncryptedCreds = encrypted
	}

	if err := plan.CanUseCredential(user.Plan, existing); err != nil {
		return shared.PlanError(c, err)
	}
	if err := h.EnforceResolvableAlgoID(existing); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, brokererr.PublicFrom(err))
	}

	if err := h.PG.UpdateBrokerCred(c.Context(), existing); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update credential")
	}
	return shared.Ok(c, CredentialResponse(existing))
}

func (h *Handler) VerifyCredential(c *fiber.Ctx) error {
	id := c.Params("id")

	existing, err := h.RequireCredentialAccess(c, id, false)
	if err != nil {
		return err
	}

	// Placeholder credentials (free-plan Zerodha without OAuth) have empty EncryptedCreds.
	// There is nothing to verify against a live broker — treat the credential as valid.
	if strings.TrimSpace(existing.EncryptedCreds) == "" {
		if _, _, _, err := NormalizeCredentialOptions(existing.BrokerType, existing.Exchange, existing.Product, existing.OrderType); err != nil {
			return shared.Ok(c, CredentialVerifyFailure(existing.BrokerType, err.Error(), nil))
		}
		return shared.Ok(c, fiber.Map{
			"valid":        true,
			"broker_type":  existing.BrokerType,
			"account_mode": existing.AccountMode,
			"exchange":     existing.Exchange,
			"product":      existing.Product,
		})
	}

	credKey, err := h.Cfg.AESKeyBytes()
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load encryption key")
	}
	plaintext, err := credenc.PlaintextOrDecrypt(existing.EncryptedCreds, credKey, existing.ID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not read credential")
	}
	parsed, err := brokercreds.Parse(existing.BrokerType, plaintext)
	if err != nil {
		return shared.Ok(c, CredentialVerifyFailure(existing.BrokerType,
			fmt.Sprintf("invalid format (expected %s)", brokercreds.FormatHint(existing.BrokerType)), nil))
	}
	if _, _, _, err := NormalizeCredentialOptions(existing.BrokerType, existing.Exchange, existing.Product, existing.OrderType); err != nil {
		return shared.Ok(c, CredentialVerifyFailure(existing.BrokerType, err.Error(), nil))
	}
	// Publisher mode only needs api_key — no OAuth access_token needed.
	if existing.BrokerType == "zerodha" && existing.ExecutionMode == "publisher" {
		return shared.Ok(c, fiber.Map{
			"valid":          true,
			"broker_type":    existing.BrokerType,
			"account_mode":   existing.AccountMode,
			"execution_mode": existing.ExecutionMode,
		})
	}

	// Zerodha credential saved as api_key:api_secret but OAuth not yet completed.
	// No access_token means live API calls will fail — signal the frontend to trigger OAuth.
	if existing.BrokerType == "zerodha" && existing.AccountMode == "live" && parsed.AccessToken == "" {
		return shared.Ok(c, fiber.Map{
			"valid":        false,
			"broker_type":  existing.BrokerType,
			"account_mode": existing.AccountMode,
			"needs_oauth":  true,
			"message":      "API key saved — connect with Zerodha to complete setup",
		})
	}

	if existing.BrokerType == "zerodha" && !zerodhasettings.OAuthEnabled(c.Context(), h.PG) {
		return shared.Ok(c, CredentialVerifyFailure(existing.BrokerType,
			"Zerodha OAuth is currently disabled by the platform admin", nil))
	}

	user, err := h.PG.GetUserByID(c.Context(), existing.UserID)
	if err != nil || user == nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user")
	}
	effectivePlan := plan.EffectivePlan(user.Plan)
	route, err := brokerfactory.ResolveExecution(h.Cfg.BrokerMode, effectivePlan, existing)
	if err != nil {
		return shared.PlanError(c, err)
	}
	check := route.CredentialForExecution(existing)
	check.EncryptedCreds = plaintext
	b, err := brokerfactory.New(route.Mode, h.Infra, check, parsed)
	if err != nil {
		return shared.Ok(c, CredentialVerifyFailure(existing.BrokerType, err.Error(), err))
	}
	if _, err := b.GetAccountEquity(c.Context()); err != nil {
		return shared.Ok(c, CredentialVerifyFailure(existing.BrokerType, brokererr.PublicFrom(err), err))
	}

	return shared.Ok(c, fiber.Map{
		"valid":        true,
		"broker_type":  existing.BrokerType,
		"account_mode": existing.AccountMode,
		"exchange":     existing.Exchange,
		"product":      existing.Product,
	})
}

func (h *Handler) DeleteCredential(c *fiber.Ctx) error {
	id := c.Params("id")

	existing, err := h.RequireCredentialAccess(c, id, true)
	if err != nil {
		return err
	}

	count, err := h.PG.CountWebhooksByBrokerCred(c.Context(), id)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not check webhook usage")
	}
	if count > 0 {
		return shared.Fail(c, fiber.StatusConflict, "credential is in use by one or more webhooks")
	}

	if err := h.PG.DeleteBrokerCred(c.Context(), id, existing); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not delete credential")
	}
	h.LogUserAudit(c, existing.UserID, "credential_deleted", map[string]any{"broker_type": existing.BrokerType, "label": existing.AccountLabel})
	return c.SendStatus(fiber.StatusNoContent)
}

// ── Credential status ─────────────────────────────────────────────────────────

func (h *Handler) PauseCredential(c *fiber.Ctx) error {
	return h.SetCredentialStatus(c, "paused")
}

func (h *Handler) ResumeCredential(c *fiber.Ctx) error {
	return h.SetCredentialStatus(c, "active")
}

func (h *Handler) SetCredentialStatus(c *fiber.Ctx, status string) error {
	id := c.Params("id")
	bc, err := h.RequireCredentialAccess(c, id, true)
	if err != nil {
		return err
	}

	if status == "active" {
		if bc.AdminDisabled {
			modeLabel := "OAuth"
			if bc.ExecutionMode == "publisher" {
				modeLabel = "Kite Publisher"
			}
			return shared.Fail(c, fiber.StatusForbidden,
				fmt.Sprintf("Zerodha %s is currently disabled by the ZettaBridge admin — this credential cannot be resumed until it is re-enabled", modeLabel))
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
		// Block live credential resume on plans that don't allow live trading.
		if err := plan.CanUseCredential(user.Plan, bc); err != nil {
			return shared.PlanError(c, err)
		}
		if err := h.EnforceCredentialResumeLimit(c, scope, user); err != nil {
			return err
		}
	}

	if err := h.PG.UpdateBrokerCredStatus(c.Context(), id, status); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "update failed")
	}
	if status == "active" {
		_ = h.PG.ClearCredentialAutoPaused(c.Context(), id)
	}
	return shared.Ok(c, fiber.Map{"status": status})
}

// ── Zerodha OAuth ─────────────────────────────────────────────────────────────

// ZerodhaStatus returns the connection status of the user's Zerodha credentials.
// The dashboard polls this to decide whether to show the reconnect warning banner.
// Credentials are scoped to the user's active context: org-scoped when the user
// is an org member, solo when not. This prevents spurious warnings for resources
// that aren't visible or active in the current context.
//
//	GET /v1/credentials/zerodha/status
func (h *Handler) ZerodhaStatus(c *fiber.Ctx) error {
	userID := middleware.UserID(c)

	scope := store.ResourceScope{UserID: userID, InOrg: false}

	creds, err := h.PG.ListBrokerCredsForScope(c.Context(), scope)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load credentials")
	}

	type credStatus struct {
		CredentialID   string `json:"credential_id"`
		AccountLabel   string `json:"account_label"`
		Status         string `json:"status"` // valid | expires_soon | expired
		NeedsReconnect bool   `json:"needs_reconnect"`
	}

	var result []credStatus
	for _, bc := range creds {
		if bc.BrokerType != "zerodha" || bc.AccountMode != "live" || bc.Status != "active" || bc.ExecutionMode == "publisher" {
			continue
		}
		s := ZerodhaTokenStatus(bc.ConnectedAt)
		result = append(result, credStatus{
			CredentialID:   bc.ID,
			AccountLabel:   bc.AccountLabel,
			Status:         s,
			NeedsReconnect: s != "valid",
		})
	}
	if result == nil {
		result = []credStatus{}
	}
	return shared.Ok(c, result)
}

// ZerodhaConnectInfo returns the Kite Connect callback URL that users must configure
// in their own Kite Connect app on developers.kite.trade.
//
//	GET /v1/credentials/zerodha/connect-info
func (h *Handler) ZerodhaConnectInfo(c *fiber.Ctx) error {
	return shared.Ok(c, fiber.Map{
		"callback_url":           h.Cfg.ZerodhaOAuthCallbackURL(),
		"publisher_callback_url": h.Cfg.EffectiveZerodhaPublisherCallbackURL(),
		"execution_adapter_mode": h.Cfg.ExecutionAdapterMode,
		"oauth_enabled":          zerodhasettings.OAuthEnabled(c.Context(), h.PG),
		"publisher_enabled":      zerodhasettings.PublisherEnabled(c.Context(), h.PG),
	})
}

// ZerodhaConnect returns the Kite OAuth redirect URL using the user's own API key.
// The credential must already exist (created via POST /v1/credentials) before calling this.
//
//	GET /v1/credentials/zerodha/connect?id=CRED_ID
func (h *Handler) ZerodhaConnect(c *fiber.Ctx) error {
	if !zerodhasettings.OAuthEnabled(c.Context(), h.PG) {
		return shared.Fail(c, fiber.StatusForbidden, "Zerodha OAuth is currently disabled by the platform admin")
	}

	userID := middleware.UserID(c)
	credID := strings.TrimSpace(c.Query("id"))
	if credID == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "id is required")
	}

	scope := store.ResourceScope{UserID: userID, InOrg: false}
	if mem, err := h.PG.GetActiveOrgMembershipByUser(c.Context(), userID); err == nil && mem != nil {
		scope = store.ResourceScope{OrgID: mem.OrgID, UserID: userID, InOrg: true}
	}
	creds, err := h.PG.ListBrokerCredsForScope(c.Context(), scope)
	if err != nil {
		log.Printf("zerodha_oauth: connect scope lookup user=%s: %v", userID, err)
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	// Verify the credential belongs to this user's scope (meta scan, no secrets).
	inScope := false
	for _, c := range creds {
		if c.ID == credID && c.BrokerType == "zerodha" {
			inScope = true
			break
		}
	}
	if !inScope {
		log.Printf("zerodha_oauth: connect user=%s cred=%s not in scope", userID, credID)
		return shared.Fail(c, fiber.StatusNotFound, "credential not found")
	}

	// Fetch the full credential (includes encrypted_creds) now that scope is confirmed.
	bc, err := h.PG.GetBrokerCred(c.Context(), credID)
	if err != nil || bc == nil {
		log.Printf("zerodha_oauth: connect fetch cred=%s: %v", credID, err)
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}

	if bc.ExecutionMode == "publisher" {
		return shared.Fail(c, fiber.StatusBadRequest, "publisher credentials do not use OAuth — no reconnect needed")
	}
	if strings.TrimSpace(bc.EncryptedCreds) == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "no API credentials found — enter your api_key:api_secret in the field above, then click Reconnect")
	}
	credKey, err := h.Cfg.AESKeyBytes()
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "server error")
	}
	plaintext, err := credenc.PlaintextOrDecrypt(bc.EncryptedCreds, credKey, bc.ID)
	if err != nil {
		log.Printf("zerodha_oauth: connect decrypt cred=%s: %v", credID, err)
		return shared.Fail(c, fiber.StatusInternalServerError, "could not read credential")
	}
	parsed, err := brokercreds.Parse("zerodha", plaintext)
	if err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "credential not in expected format — re-enter api_key:api_secret")
	}

	state := zerodhaauth.GenerateState(userID, credID, nil, h.Cfg.JWTSecret)
	loginURL := zerodhaauth.LoginURL(parsed.APIKey, state)
	log.Printf("zerodha_oauth: connect user=%s cred=%q", userID, credID)
	return c.JSON(fiber.Map{"redirect_url": loginURL})
}

// ZerodhaCallback handles the browser redirect from Kite after user consent.
func (h *Handler) ZerodhaCallback(c *fiber.Ctx) error {
	return zerodhaoauth.HandleCallback(c, zerodhaoauth.Deps{Cfg: h.Cfg, Store: h.PG})
}
