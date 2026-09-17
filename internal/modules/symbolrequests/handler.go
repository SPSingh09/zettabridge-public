// Package symbolrequests lets a paper-trading user ask for a symbol to be
// added to a market profile's instrument master (surfaced from the
// "unknown or inactive symbol" rejection), and track the status of their
// own requests. Admin-side review (list/accept/reject) lives in
// internal/modules/admin/symbol_requests.go — resolution itself happens
// automatically in AdminCreateInstrument once a matching instrument exists.
package symbolrequests

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/SPSingh09/zettabridge/internal/integrations/telegram"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/platform/mailer"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

// Handler handles user-facing symbol request endpoints.
type Handler struct {
	*shared.Handler
	Mail mailer.Sender
	TG   *telegram.Sender
}

// CreateSymbolRequest submits a new pending symbol request for the current user.
func (h *Handler) CreateSymbolRequest(c *fiber.Ctx) error {
	userID := middleware.UserID(c)

	var body struct {
		MarketProfileCode string `json:"market_profile_code"`
		Exchange          string `json:"exchange"`
		Symbol            string `json:"symbol"`
		Reason            string `json:"reason"`
	}
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	profileCode := strings.TrimSpace(body.MarketProfileCode)
	exchange := strings.ToUpper(strings.TrimSpace(body.Exchange))
	symbol := strings.ToUpper(strings.TrimSpace(body.Symbol))
	reason := strings.TrimSpace(body.Reason)
	if profileCode == "" || symbol == "" || reason == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "market_profile_code, symbol, and reason are required")
	}

	profile, err := h.PG.GetMarketProfileByCode(c.Context(), profileCode)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if profile == nil {
		return shared.Fail(c, fiber.StatusBadRequest, "unknown market_profile_code: "+profileCode)
	}

	user, err := h.PG.GetUserByID(c.Context(), userID)
	if err != nil || user == nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}

	now := time.Now().UTC()
	r := &store.SymbolRequest{
		ID:                uuid.New().String(),
		UserID:            userID,
		MarketProfileCode: profileCode,
		Exchange:          exchange,
		Symbol:            symbol,
		Reason:            reason,
		Status:            "pending",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := h.PG.CreateSymbolRequest(c.Context(), r); err != nil {
		if isUniqueViolation(err) {
			return shared.Fail(c, fiber.StatusConflict, "you already have an open request for this symbol")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not create symbol request")
	}

	h.notifySupport(c.Context(), r, user.Email)

	return shared.Created(c, r)
}

// ListMySymbolRequests returns the current user's own requests, newest first.
func (h *Handler) ListMySymbolRequests(c *fiber.Ctx) error {
	userID := middleware.UserID(c)
	requests, err := h.PG.ListSymbolRequestsByUser(c.Context(), userID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list symbol requests")
	}
	if requests == nil {
		requests = []*store.SymbolRequest{}
	}
	return shared.Ok(c, requests)
}

// notifySupport tells the configured admin/support contact a new request
// came in. Both channels are optional (env-configured) and best-effort.
func (h *Handler) notifySupport(ctx context.Context, r *store.SymbolRequest, requesterEmail string) {
	if h.Cfg.SupportNotificationEmail != "" {
		adminURL := h.Cfg.FrontendURL + "/admin"
		if err := h.Mail.SendSymbolRequestReceived(ctx, h.Cfg.SupportNotificationEmail, r.Symbol, r.Exchange, r.MarketProfileCode, r.Reason, requesterEmail, adminURL); err != nil {
			log.Printf("symbol_request: admin email failed request=%s: %v", r.ID, err)
		}
	}
	if h.Cfg.SupportTelegramChatID != "" {
		msg := fmt.Sprintf(
			"🆕 <b>New symbol request</b>\n\n%s requested <b>%s</b> (%s / %s)\n\nReason: %s",
			requesterEmail, r.Symbol, r.Exchange, r.MarketProfileCode, r.Reason,
		)
		if err := h.TG.Send(ctx, h.Cfg.SupportTelegramChatID, msg); err != nil {
			log.Printf("symbol_request: admin telegram failed request=%s: %v", r.ID, err)
		}
	}
}

// isUniqueViolation reports whether err looks like a Postgres unique
// constraint violation (pq error code 23505). Matched by message substring
// to avoid importing the pq driver package just for this (mirrors
// admin.isUniqueViolation).
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}
