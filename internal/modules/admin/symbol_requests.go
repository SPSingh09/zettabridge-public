package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

// ── Symbol requests ────────────────────────────────────────────────────────

// AdminListSymbolRequests lists symbol requests, defaulting to ?status=pending
// (the dashboard's pending-requests banner). Pass ?status=all for every request.
func (h *Handler) AdminListSymbolRequests(c *fiber.Ctx) error {
	status := c.Query("status", "pending")
	if status == "all" {
		status = ""
	}
	requests, err := h.PG.ListSymbolRequestsByStatus(c.Context(), status)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not list symbol requests")
	}
	if requests == nil {
		requests = []*store.SymbolRequest{}
	}
	return shared.Ok(c, requests)
}

// AdminAcceptSymbolRequest moves a pending request to accepted and notifies
// the requesting user. The frontend follows this with a navigation to the
// pre-filled "add instrument" form — resolution itself happens later,
// automatically, in AdminCreateInstrument.
func (h *Handler) AdminAcceptSymbolRequest(c *fiber.Ctx) error {
	id := c.Params("id")
	adminID := middleware.UserID(c)

	r, err := h.PG.AcceptSymbolRequest(c.Context(), id, adminID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return shared.Fail(c, fiber.StatusConflict, "request not found or no longer pending")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not accept symbol request")
	}
	h.LogUserAudit(c, adminID, "symbol_request_accepted", map[string]any{"id": id, "symbol": r.Symbol})
	notifyUserSymbolRequestStatus(c.Context(), h, r.UserID, r.Symbol, "accepted", "")
	return shared.Ok(c, r)
}

// AdminRejectSymbolRequest moves a pending or accepted request to rejected
// (an admin can change their mind after accepting, before an instrument is
// actually created) and notifies the requesting user, optionally with a note.
func (h *Handler) AdminRejectSymbolRequest(c *fiber.Ctx) error {
	id := c.Params("id")
	adminID := middleware.UserID(c)

	var body struct {
		AdminNote string `json:"admin_note"`
	}
	_ = c.BodyParser(&body) // admin_note is optional; ignore empty/malformed body

	r, err := h.PG.RejectSymbolRequest(c.Context(), id, adminID, strings.TrimSpace(body.AdminNote))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return shared.Fail(c, fiber.StatusConflict, "request not found or already resolved/rejected")
		}
		return shared.Fail(c, fiber.StatusInternalServerError, "could not reject symbol request")
	}
	h.LogUserAudit(c, adminID, "symbol_request_rejected", map[string]any{"id": id, "symbol": r.Symbol})
	notifyUserSymbolRequestStatus(c.Context(), h, r.UserID, r.Symbol, "rejected", r.AdminNote)
	return shared.Ok(c, r)
}

// notifyUserSymbolRequestStatus notifies the requesting user of a symbol
// request status change via email, Telegram (if connected), and an in-app
// notification row. Shared by accept/reject above and the auto-resolve path
// in AdminCreateInstrument (market_profiles.go). Best-effort throughout —
// failures log and never fail the calling request.
func notifyUserSymbolRequestStatus(ctx context.Context, h *Handler, userID, symbol, status, adminNote string) {
	user, err := h.PG.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		log.Printf("symbol_request: notify user lookup failed user=%s: %v", userID, err)
		return
	}
	requestsURL := h.Cfg.FrontendURL + "/symbol-requests"
	if err := h.Mail.SendSymbolRequestStatusUpdate(ctx, user.Email, symbol, status, adminNote, requestsURL); err != nil {
		log.Printf("symbol_request: user email failed user=%s status=%s: %v", userID, status, err)
	}
	if user.TelegramChatID != "" {
		msg := fmt.Sprintf("📣 <b>Symbol request %s</b>\n\nYour request for <b>%s</b> is now <b>%s</b>.", status, symbol, status)
		if adminNote != "" {
			msg += fmt.Sprintf("\n\nNote: %s", adminNote)
		}
		if err := h.TG.Send(ctx, user.TelegramChatID, msg); err != nil {
			log.Printf("symbol_request: user telegram failed user=%s status=%s: %v", userID, status, err)
		}
	}
	h.PG.InsertNotification(ctx, userID, "symbol_request", "Symbol request "+status,
		fmt.Sprintf("Your request for %s is now %s.", symbol, status), "")
}
