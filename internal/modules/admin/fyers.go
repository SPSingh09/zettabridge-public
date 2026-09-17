package admin

import (
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	fyersauth "github.com/SPSingh09/zettabridge/internal/integrations/brokers/fyers/auth"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	credenc "github.com/SPSingh09/zettabridge/internal/platform/crypto"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

// fyersConnectionAAD must match internal/integrations/brokers/fyers/refresh's constant.
const fyersConnectionAAD = "fyers_connection"

// fyersConnectionLive reports whether conn currently has a usable access
// token — present *and* not past its expiry. A stored-but-expired token
// (the silent refresh job notifies the admin on failure but never clears the
// row — see fyersrefresh.Job.tick) must not read as "connected", or the
// admin has no way to tell a stale connection from a live one.
func fyersConnectionLive(conn *store.FyersConnection) bool {
	if conn == nil || conn.EncryptedAccessToken == "" {
		return false
	}
	return conn.AccessTokenExpiresAt == nil || conn.AccessTokenExpiresAt.After(time.Now())
}

// fyersConnected reports whether FYERS has a live (non-expired) access token
// on file — used by AdminGetMarketDataProvider/AdminSetMarketDataProvider's
// "fyers_configured" field so the dashboard knows whether selecting "fyers"
// will actually do anything yet.
func (h *Handler) fyersConnected(c *fiber.Ctx) bool {
	conn, err := h.PG.GetFyersConnection(c.Context())
	if err != nil {
		return false
	}
	return fyersConnectionLive(conn)
}

// AdminGetFyersStatus reports the platform's single FYERS connection state —
// never returns the tokens/PIN themselves, only whether they're set.
func (h *Handler) AdminGetFyersStatus(c *fiber.Ctx) error {
	conn, err := h.PG.GetFyersConnection(c.Context())
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load fyers connection")
	}
	resp := fiber.Map{
		"app_configured": h.Cfg.FyersAppID != "" && h.Cfg.FyersSecretID != "",
		"connected":      false,
		"token_expired":  false,
		"pin_set":        false,
	}
	if conn != nil {
		resp["connected"] = fyersConnectionLive(conn)
		resp["token_expired"] = conn.EncryptedAccessToken != "" && !fyersConnectionLive(conn)
		resp["pin_set"] = conn.EncryptedPIN != ""
		resp["access_token_expires_at"] = conn.AccessTokenExpiresAt
		resp["refresh_token_expires_at"] = conn.RefreshTokenExpiresAt
		resp["connected_at"] = conn.ConnectedAt
	}
	return shared.Ok(c, resp)
}

// AdminFyersConnect returns the FYERS OAuth login URL for the admin to
// navigate to (mirrors ZerodhaConnect's redirect_url response shape).
func (h *Handler) AdminFyersConnect(c *fiber.Ctx) error {
	if h.Cfg.FyersAppID == "" || h.Cfg.FyersSecretID == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "FYERS_APP_ID/FYERS_SECRET_ID are not configured")
	}
	if h.Cfg.FyersCallbackURL == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "FYERS_CALLBACK_URL is not configured")
	}
	adminID := middleware.UserID(c)
	state := fyersauth.GenerateState(adminID, h.Cfg.JWTSecret)
	loginURL := fyersauth.LoginURL(h.Cfg.FyersAppID, h.Cfg.FyersCallbackURL, state)
	log.Printf("fyers_oauth: connect admin=%s", adminID)
	return c.JSON(fiber.Map{"redirect_url": loginURL})
}

// AdminFyersCallback handles the browser redirect from FYERS after admin
// consent. Public endpoint — FYERS sends the browser here directly, no JWT;
// identity is recovered from the signed state parameter.
//
// FYERS's redirect query param for the auth code is not confirmed from an
// official source (see fyersauth package docs) — this accepts either
// "auth_code" or "code" defensively.
func (h *Handler) AdminFyersCallback(c *fiber.Ctx) error {
	frontendURL := strings.TrimRight(h.Cfg.FrontendURL, "/")
	failRedirect := func(reason string) error {
		log.Printf("fyers_oauth: callback failed reason=%s", reason)
		return c.Redirect(frontendURL + "/admin/settings?fyers=error&reason=" + url.QueryEscape(reason))
	}

	authCode := c.Query("auth_code")
	if authCode == "" {
		authCode = c.Query("code")
	}
	state := c.Query("state")
	if authCode == "" || state == "" {
		return failRedirect("missing auth_code/state")
	}

	adminID, err := fyersauth.ParseState(state, h.Cfg.JWTSecret)
	if err != nil {
		log.Printf("fyers_oauth: invalid state: %v", err)
		return failRedirect("invalid state")
	}

	result, err := fyersauth.ExchangeAuthCode(h.Cfg.FyersAppID, h.Cfg.FyersSecretID, authCode)
	if err != nil {
		log.Printf("fyers_oauth: token exchange failed for admin %s: %v", adminID, err)
		return failRedirect("token exchange failed")
	}

	credKey, err := h.Cfg.AESKeyBytes()
	if err != nil {
		return failRedirect("server error")
	}
	encryptedAccess, err := credenc.Encrypt(result.AccessToken, credKey, fyersConnectionAAD)
	if err != nil {
		return failRedirect("server error")
	}
	refreshToken := result.RefreshToken
	var encryptedRefresh string
	if refreshToken != "" {
		encryptedRefresh, err = credenc.Encrypt(refreshToken, credKey, fyersConnectionAAD)
		if err != nil {
			return failRedirect("server error")
		}
	}

	accessExpiresAt := time.Now().Add(fyersauth.AccessTokenValidity)
	refreshExpiresAt := time.Now().Add(fyersauth.RefreshTokenValidity)
	if err := h.PG.UpsertFyersTokens(c.Context(), encryptedAccess, encryptedRefresh, accessExpiresAt, refreshExpiresAt, adminID); err != nil {
		log.Printf("fyers_oauth: store tokens failed for admin %s: %v", adminID, err)
		return failRedirect("server error")
	}
	log.Printf("fyers_oauth: connected by admin=%s", adminID)
	return c.Redirect(frontendURL + "/admin/settings?fyers=connected")
}

// AdminSetFyersPIN stores the admin-entered FYERS trading PIN (encrypted),
// used only by the silent refresh-token flow.
func (h *Handler) AdminSetFyersPIN(c *fiber.Ctx) error {
	var body struct {
		PIN string `json:"pin"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	pin := strings.TrimSpace(body.PIN)
	if pin == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "pin is required")
	}
	credKey, err := h.Cfg.AESKeyBytes()
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "server error")
	}
	encrypted, err := credenc.Encrypt(pin, credKey, fyersConnectionAAD)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not save pin")
	}
	if err := h.PG.SetFyersPIN(c.Context(), encrypted); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not save pin")
	}
	adminID := middleware.UserID(c)
	h.LogUserAudit(c, adminID, "fyers_pin_set", nil)
	return shared.Ok(c, fiber.Map{"pin_set": true})
}
