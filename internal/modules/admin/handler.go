package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"

	billingpkg "github.com/SPSingh09/zettabridge/internal/billing"
	"github.com/SPSingh09/zettabridge/internal/marketdata"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/platform/mailer"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/integrations/telegram"
)

// Handler handles admin, notification, telegram settings, and metrics endpoints.
type Handler struct {
	*shared.Handler
	TG      *telegram.Sender
	Billing *billingpkg.Service
	Mail    mailer.Sender
}

// ── Package-level helpers ─────────────────────────────────────────────────────

// AdminUserID extracts and validates the :id route param as a user UUID.
func AdminUserID(c *fiber.Ctx) (string, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return "", shared.Fail(c, fiber.StatusBadRequest, "invalid user id")
	}
	return id, nil
}

// ── Admin users ───────────────────────────────────────────────────────────────

func (h *Handler) AdminListUsers(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	result, err := h.PG.ListUsersAdmin(c.Context(), store.AdminUserFilter{
		Plan:   c.Query("plan"),
		Status: c.Query("status"),
		Email:  c.Query("email"),
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list users")
	}
	return shared.Ok(c, result)
}

func (h *Handler) AdminGetUser(c *fiber.Ctx) error {
	id, err := AdminUserID(c)
	if err != nil {
		return err
	}
	user, err := h.PG.GetUserWithCounts(c.Context(), id)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if user == nil {
		return shared.Fail(c, fiber.StatusNotFound, "user not found")
	}
	return shared.Ok(c, user)
}

func (h *Handler) AdminPatchUser(c *fiber.Ctx) error {
	id, err := AdminUserID(c)
	if err != nil {
		return err
	}

	target, err := h.PG.GetUserByID(c.Context(), id)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if target == nil {
		return shared.Fail(c, fiber.StatusNotFound, "user not found")
	}

	var body struct {
		Status        *string `json:"status"`
		OrgsEnabled   *bool   `json:"orgs_enabled"`
		EmailVerified *bool   `json:"email_verified"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	if body.Status == nil && body.OrgsEnabled == nil && body.EmailVerified == nil {
		return shared.Fail(c, fiber.StatusBadRequest, "status, orgs_enabled, or email_verified is required")
	}

	actorID := middleware.UserID(c)

	if body.Status != nil {
		if !domain.IsValidStatus(*body.Status) {
			return shared.Fail(c, fiber.StatusBadRequest, domain.ErrInvalidStatus.Error())
		}
		if target.Role == domain.RoleAdmin && *body.Status == domain.StatusSuspended && target.ID == actorID {
			return shared.Fail(c, fiber.StatusForbidden, "cannot suspend your own admin account")
		}
		if target.Role == domain.RoleAdmin && *body.Status == domain.StatusSuspended {
			return shared.Fail(c, fiber.StatusForbidden, "cannot suspend platform admin accounts")
		}
		if err := h.PG.UpdateUserStatus(c.Context(), id, *body.Status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return shared.Fail(c, fiber.StatusNotFound, "user not found")
			}
			return shared.Fail(c, fiber.StatusInternalServerError, "could not update user")
		}
	}

	if body.OrgsEnabled != nil {
		if err := h.PG.UpdateUserOrgsEnabled(c.Context(), id, *body.OrgsEnabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return shared.Fail(c, fiber.StatusNotFound, "user not found")
			}
			return shared.Fail(c, fiber.StatusInternalServerError, "could not update orgs_enabled")
		}
	}

	if body.EmailVerified != nil {
		if err := h.PG.SetUserEmailVerified(c.Context(), id, *body.EmailVerified); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return shared.Fail(c, fiber.StatusNotFound, "user not found")
			}
			return shared.Fail(c, fiber.StatusInternalServerError, "could not update email_verified")
		}
	}

	user, err := h.PG.GetUserWithCounts(c.Context(), id)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	return shared.Ok(c, user)
}

func (h *Handler) AdminDeleteUser(c *fiber.Ctx) error {
	id, err := AdminUserID(c)
	if err != nil {
		return err
	}

	target, err := h.PG.GetUserByID(c.Context(), id)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if target == nil {
		return shared.Fail(c, fiber.StatusNotFound, "user not found")
	}

	actorID := middleware.UserID(c)
	if target.ID == actorID {
		return shared.Fail(c, fiber.StatusForbidden, "cannot delete your own account")
	}
	if target.Role == domain.RoleAdmin {
		return shared.Fail(c, fiber.StatusForbidden, "cannot delete platform admin accounts")
	}

	if err := h.PG.DeleteUser(c.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return shared.Fail(c, fiber.StatusNotFound, "user not found")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not delete user")
	}

	// Clear the email resend rate-limit so the address can be re-registered immediately.
	_ = h.Redis.ClearVerifyResendLimit(c.Context(), target.Email)

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) AdminSetUserPlan(c *fiber.Ctx) error {
	id, err := AdminUserID(c)
	if err != nil {
		return err
	}

	var body struct {
		Plan string `json:"plan"`
	}
	if err := c.BodyParser(&body); err != nil || body.Plan == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "plan is required")
	}

	applier := billingpkg.NewApplier(h.PG)
	if err := applier.Apply(c.Context(), billingpkg.Change{
		UserID: id,
		Plan:   body.Plan,
	}); err != nil {
		switch {
		case errors.Is(err, billingpkg.ErrInvalidPlan):
			return shared.Fail(c, fiber.StatusBadRequest, domain.ErrInvalidPlan.Error())
		case billingpkg.SeatLimitConflict(err):
			return shared.Fail(c, fiber.StatusConflict, domain.ErrSeatLimitTooLow.Error())
		case errors.Is(err, billingpkg.ErrUserNotFound):
			return shared.Fail(c, fiber.StatusNotFound, "user not found")
		default:
			return shared.Fail(c, fiber.StatusInternalServerError, "could not update plan")
		}
	}
	if err := h.PG.UpdateUserBillingSource(c.Context(), id, store.BillingSourceAdmin); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update billing source")
	}

	user, err := h.PG.GetUserWithCounts(c.Context(), id)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if user == nil {
		return shared.Fail(c, fiber.StatusNotFound, "user not found")
	}
	return shared.Ok(c, user)
}

func (h *Handler) AdminListUserAudit(c *fiber.Ctx) error {
	id, err := AdminUserID(c)
	if err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid user id")
	}
	limit := 50
	if v := c.QueryInt("limit", 0); v > 0 {
		limit = v
	}
	entries, err := h.PG.ListUserAudit(c.Context(), id, limit)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not fetch audit log")
	}
	return shared.Ok(c, entries)
}

// ── Admin platform invites ────────────────────────────────────────────────────

func (h *Handler) AdminCreatePlatformInvite(c *fiber.Ctx) error {
	callerID := middleware.UserID(c)

	var body struct {
		Email string `json:"email"`
		Note  string `json:"note"`
	}
	_ = c.BodyParser(&body)

	ttl := time.Duration(h.Cfg.PlatformInviteTTLDays) * 24 * time.Hour
	rawToken, inv, err := h.PG.CreatePlatformInvite(c.Context(), body.Email, body.Note, callerID, ttl)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not create invite")
	}

	if inv.Email != "" {
		inviteURL := h.Cfg.FrontendURL + "/register?invite=" + rawToken
		if err := h.Mail.SendPlatformInvite(c.Context(), inv.Email, inviteURL); err != nil {
			log.Printf("platform_invite: email failed invite=%s to=%s: %v", inv.ID, inv.Email, err)
		}
	}

	return shared.Created(c, fiber.Map{
		"id":         inv.ID,
		"token":      rawToken,
		"email":      inv.Email,
		"note":       inv.Note,
		"expires_at": inv.ExpiresAt,
	})
}

func (h *Handler) AdminListPlatformInvites(c *fiber.Ctx) error {
	rows, err := h.PG.ListPlatformInvites(c.Context())
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list invites")
	}
	if rows == nil {
		rows = []store.PlatformInviteRow{}
	}
	return shared.Ok(c, rows)
}

func (h *Handler) AdminRevokePlatformInvite(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid invite id")
	}
	if err := h.PG.RevokePlatformInvite(c.Context(), id); err != nil {
		if err == sql.ErrNoRows {
			return shared.Fail(c, fiber.StatusNotFound, "invite not found or already revoked")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not revoke invite")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ── Notifications ─────────────────────────────────────────────────────────────

func (h *Handler) ListNotifications(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	userPlan, err := h.UserPlan(c, uid)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user plan")
	}
	if err := plan.CanReceiveNotifications(userPlan); err != nil {
		return shared.PlanError(c, err)
	}

	limit := c.QueryInt("limit", 50)
	notifs, err := h.PG.ListNotifications(c.Context(), uid, limit)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not fetch notifications")
	}
	if notifs == nil {
		notifs = []*store.Notification{}
	}
	return shared.Ok(c, notifs)
}

func (h *Handler) CountUnreadNotifications(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	userPlan, err := h.UserPlan(c, uid)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user plan")
	}
	if err := plan.CanReceiveNotifications(userPlan); err != nil {
		return shared.PlanError(c, err)
	}

	count, err := h.PG.CountUnreadNotifications(c.Context(), uid)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not count notifications")
	}
	return shared.Ok(c, fiber.Map{"count": count})
}

func (h *Handler) MarkNotificationRead(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	id := c.Params("id")
	if err := h.PG.MarkNotificationRead(c.Context(), id, uid); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not mark notification read")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) MarkAllNotificationsRead(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	if err := h.PG.MarkAllNotificationsRead(c.Context(), uid); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not mark notifications read")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ── Telegram settings ─────────────────────────────────────────────────────────

func (h *Handler) GetTelegramSettings(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	userPlan, err := h.UserPlan(c, uid)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user plan")
	}
	if err := plan.CanReceiveNotifications(userPlan); err != nil {
		return shared.PlanError(c, err)
	}

	user, err := h.PG.GetUserByID(c.Context(), uid)
	if err != nil || user == nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user")
	}
	return shared.Ok(c, fiber.Map{
		"chat_id":   user.TelegramChatID,
		"connected": user.TelegramChatID != "",
	})
}

func (h *Handler) UpdateTelegramSettings(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	userPlan, err := h.UserPlan(c, uid)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user plan")
	}
	if err := plan.CanReceiveNotifications(userPlan); err != nil {
		return shared.PlanError(c, err)
	}

	var body struct {
		ChatID string `json:"chat_id"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	chatID := strings.TrimSpace(body.ChatID)

	if err := h.PG.UpdateUserTelegramChatID(c.Context(), uid, chatID); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update telegram settings")
	}
	return shared.Ok(c, fiber.Map{
		"chat_id":   chatID,
		"connected": chatID != "",
	})
}

// ── Platform settings: paper trading market data provider (Phase 6) ──────────

// AdminGetMarketDataProvider returns the currently active provider (DB
// override if set, else the env-configured default) and the known options.
func (h *Handler) AdminGetMarketDataProvider(c *fiber.Ctx) error {
	current, ok, err := h.PG.GetPlatformSetting(c.Context(), marketdata.SettingKeyProvider)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load setting")
	}
	if !ok || current == "" {
		current = h.Cfg.MarketDataProvider
	}
	return shared.Ok(c, fiber.Map{
		"provider":         current,
		"available":        marketdata.KnownProviders,
		"fyers_configured": h.fyersConnected(c),
	})
}

// AdminSetMarketDataProvider switches the active provider at runtime — the
// background SnapshotJob re-reads this setting on every sweep, so the
// change takes effect on the next tick with no server restart.
func (h *Handler) AdminSetMarketDataProvider(c *fiber.Ctx) error {
	var body struct {
		Provider string `json:"provider"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	provider := strings.TrimSpace(body.Provider)
	valid := false
	for _, p := range marketdata.KnownProviders {
		if p == provider {
			valid = true
			break
		}
	}
	if !valid {
		return shared.Fail(c, fiber.StatusBadRequest, "provider must be one of: "+strings.Join(marketdata.KnownProviders, ", "))
	}

	adminID := middleware.UserID(c)
	if err := h.PG.SetPlatformSetting(c.Context(), marketdata.SettingKeyProvider, provider, adminID); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update setting")
	}
	h.LogUserAudit(c, adminID, "market_data_provider_changed", map[string]any{"provider": provider})
	return shared.Ok(c, fiber.Map{
		"provider":         provider,
		"available":        marketdata.KnownProviders,
		"fyers_configured": h.fyersConnected(c),
	})
}

// AdminGetMarketDataInterval returns the currently active paper-trading
// snapshot sweep cadence (DB override if set, else the env-configured
// default) and the allowed range.
func (h *Handler) AdminGetMarketDataInterval(c *fiber.Ctx) error {
	seconds := h.Cfg.MarketDataSnapshotIntervalSec
	if v, ok, err := h.PG.GetPlatformSetting(c.Context(), marketdata.SettingKeyIntervalSec); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load setting")
	} else if ok {
		if parsed, err := strconv.Atoi(v); err == nil {
			seconds = parsed
		}
	}
	return shared.Ok(c, fiber.Map{
		"interval_seconds": seconds,
		"min_seconds":      marketdata.MinSnapshotIntervalSec,
		"max_seconds":      marketdata.MaxSnapshotIntervalSec,
	})
}

// AdminSetMarketDataInterval changes the sweep cadence at runtime — the
// background SnapshotJob re-reads this setting before every cycle, so the
// change takes effect starting with the next sweep, no server restart.
func (h *Handler) AdminSetMarketDataInterval(c *fiber.Ctx) error {
	var body struct {
		IntervalSeconds int `json:"interval_seconds"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	if body.IntervalSeconds < marketdata.MinSnapshotIntervalSec || body.IntervalSeconds > marketdata.MaxSnapshotIntervalSec {
		return shared.Fail(c, fiber.StatusBadRequest, fmt.Sprintf(
			"interval_seconds must be between %d and %d", marketdata.MinSnapshotIntervalSec, marketdata.MaxSnapshotIntervalSec))
	}

	adminID := middleware.UserID(c)
	if err := h.PG.SetPlatformSetting(c.Context(), marketdata.SettingKeyIntervalSec, strconv.Itoa(body.IntervalSeconds), adminID); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update setting")
	}
	h.LogUserAudit(c, adminID, "market_data_interval_changed", map[string]any{"interval_seconds": body.IntervalSeconds})
	return shared.Ok(c, fiber.Map{
		"interval_seconds": body.IntervalSeconds,
		"min_seconds":      marketdata.MinSnapshotIntervalSec,
		"max_seconds":      marketdata.MaxSnapshotIntervalSec,
	})
}

// ── Metrics ───────────────────────────────────────────────────────────────────

func (h *Handler) ScrapeMetrics(c *fiber.Ctx) error {
	if !h.Cfg.MetricsEnabled {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if token := h.Cfg.MetricsToken; token != "" {
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
