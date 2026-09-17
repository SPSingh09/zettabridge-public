package admin

import (
	"github.com/gofiber/fiber/v2"

	zerodhasettings "github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/settings"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

// ── Zerodha OAuth / Kite Publisher toggles ────────────────────────────────────

// AdminGetZerodhaOAuthEnabled reports whether Zerodha OAuth login and order
// execution is currently enabled. Defaults to true when unset.
func (h *Handler) AdminGetZerodhaOAuthEnabled(c *fiber.Ctx) error {
	v, ok, err := h.PG.GetPlatformSetting(c.Context(), zerodhasettings.OAuthEnabledSettingKey)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load setting")
	}
	enabled := true
	if ok {
		enabled = v == "true"
	}
	return shared.Ok(c, fiber.Map{"enabled": enabled})
}

// AdminSetZerodhaOAuthEnabled toggles Zerodha OAuth at runtime — enforced at
// login, callback, credential creation/verification, and live order
// execution/cancellation, with no restart needed.
func (h *Handler) AdminSetZerodhaOAuthEnabled(c *fiber.Ctx) error {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil || body.Enabled == nil {
		return shared.Fail(c, fiber.StatusBadRequest, "enabled (boolean) is required")
	}
	value := "false"
	if *body.Enabled {
		value = "true"
	}
	adminID := middleware.UserID(c)
	if err := h.PG.SetPlatformSetting(c.Context(), zerodhasettings.OAuthEnabledSettingKey, value, adminID); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update setting")
	}
	if *body.Enabled {
		_ = h.PG.ClearAdminDisabledCredentials(c.Context(), "zerodha", "user_api_oauth")
		_ = h.PG.ClearAdminDisabledWebhooks(c.Context(), "zerodha", "user_api_oauth")
	} else {
		_ = h.PG.PauseCredentialsByExecutionMode(c.Context(), "zerodha", "user_api_oauth")
		_ = h.PG.PauseWebhooksByExecutionMode(c.Context(), "zerodha", "user_api_oauth")
	}
	h.LogUserAudit(c, adminID, "zerodha_oauth_enabled_changed", map[string]any{"enabled": *body.Enabled})
	return shared.Ok(c, fiber.Map{"enabled": *body.Enabled})
}

// AdminGetZerodhaPublisherEnabled reports whether Zerodha Kite Publisher
// credential creation and order execution is currently enabled. Defaults to
// true when unset.
func (h *Handler) AdminGetZerodhaPublisherEnabled(c *fiber.Ctx) error {
	v, ok, err := h.PG.GetPlatformSetting(c.Context(), zerodhasettings.PublisherEnabledSettingKey)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load setting")
	}
	enabled := true
	if ok {
		enabled = v == "true"
	}
	return shared.Ok(c, fiber.Map{"enabled": enabled})
}

// AdminSetZerodhaPublisherEnabled toggles Zerodha Kite Publisher at runtime —
// enforced at credential creation and order placement, with no restart needed.
func (h *Handler) AdminSetZerodhaPublisherEnabled(c *fiber.Ctx) error {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil || body.Enabled == nil {
		return shared.Fail(c, fiber.StatusBadRequest, "enabled (boolean) is required")
	}
	value := "false"
	if *body.Enabled {
		value = "true"
	}
	adminID := middleware.UserID(c)
	if err := h.PG.SetPlatformSetting(c.Context(), zerodhasettings.PublisherEnabledSettingKey, value, adminID); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update setting")
	}
	if *body.Enabled {
		_ = h.PG.ClearAdminDisabledCredentials(c.Context(), "zerodha", "publisher")
		_ = h.PG.ClearAdminDisabledWebhooks(c.Context(), "zerodha", "publisher")
	} else {
		_ = h.PG.PauseCredentialsByExecutionMode(c.Context(), "zerodha", "publisher")
		_ = h.PG.PauseWebhooksByExecutionMode(c.Context(), "zerodha", "publisher")
	}
	h.LogUserAudit(c, adminID, "zerodha_publisher_enabled_changed", map[string]any{"enabled": *body.Enabled})
	return shared.Ok(c, fiber.Map{"enabled": *body.Enabled})
}
