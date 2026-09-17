package publisher

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	zerodhapub "github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/publisher"
	zerodhasettings "github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/settings"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

const publisherHandoffExpiredHTTPMsg = "This Kite basket handoff has expired and can no longer be submitted. Wait for a new signal or create a new handoff."

// Handler handles Kite Publisher HTTP endpoints.
type Handler struct {
	*shared.Handler
}

// GetPublisherOrder returns publisher order status and handoff form fields.
// Ownership is enforced — the JWT user must own the order.
func (h *Handler) GetPublisherOrder(c *fiber.Ctx) error {
	if !h.Cfg.ZerodhaPublisherEnabled || !zerodhasettings.PublisherEnabled(c.Context(), h.PG) {
		return shared.Fail(c, fiber.StatusServiceUnavailable, "publisher feature not enabled")
	}

	orderID := c.Params("id")
	if _, err := uuid.Parse(orderID); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid order id")
	}

	order, err := h.PG.GetPublisherOrder(c.Context(), orderID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if order == nil || order.UserID != middleware.UserID(c) {
		return shared.Fail(c, fiber.StatusNotFound, "publisher order not found")
	}

	expired, err := h.PG.ExpirePublisherOrderIfStale(c.Context(), orderID)
	if err != nil {
		log.Printf("publisher order %s: expiry check failed: %v", orderID, err)
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if expired {
		return shared.Fail(c, fiber.StatusGone, publisherHandoffExpiredHTTPMsg)
	}

	var ff zerodhapub.KiteFormFields
	if err := json.Unmarshal(order.BasketPayloadJSON, &ff); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not parse basket payload")
	}

	// Patch older orders that were stored before market_protection was added.
	// Kite API rejects MARKET orders without this field.
	ff.Data = zerodhapub.EnsureMarketProtection(ff.Data)

	return shared.Ok(c, fiber.Map{
		"id":         order.ID,
		"status":     order.Status,
		"expires_at": order.ExpiresAt,
		"handoff": fiber.Map{
			"provider": "zerodha",
			"method":   "POST_FORM",
			"action":   "https://kite.zerodha.com/connect/basket",
			"fields": fiber.Map{
				"api_key": ff.APIKey,
				"data":    ff.Data,
				"state":   order.SignedState,
			},
		},
	})
}

// Callback handles the public Kite redirect after the user acts on the basket.
// Query params: status=success|cancelled, request_token (success only).
// Note: Kite basket does NOT echo the state field back in the callback URL, so
// we fall back to the most recent non-terminal publisher order when state is absent.
func (h *Handler) Callback(c *fiber.Ctx) error {
	kiteStatus := strings.TrimSpace(c.Query("status"))
	requestToken := strings.TrimSpace(c.Query("request_token"))
	stateParam := strings.TrimSpace(c.Query("state"))

	var order *store.PublisherOrder
	if stateParam != "" {
		publisherOrderID, err := zerodhapub.ParseState(stateParam, h.Cfg.JWTSecret)
		if err != nil {
			log.Printf("publisher callback: invalid state: %v", err)
			return shared.Fail(c, fiber.StatusBadRequest, "invalid or expired state")
		}
		order, err = h.PG.GetPublisherOrder(c.Context(), publisherOrderID)
		if err != nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
		}
		if order == nil {
			return shared.Fail(c, fiber.StatusBadRequest, "publisher order not found")
		}
	} else {
		// Kite basket does not echo state — fall back to most recent pending order.
		if h.PG == nil {
			return shared.Fail(c, fiber.StatusInternalServerError, "store not available")
		}
		var err error
		order, err = h.PG.GetMostRecentPendingPublisherOrder(c.Context())
		if err != nil {
			log.Printf("publisher callback: fallback lookup failed: %v", err)
			return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
		}
		if order == nil {
			log.Printf("publisher callback: no pending order found (state absent)")
			return shared.Fail(c, fiber.StatusBadRequest, "no pending publisher order found")
		}
		log.Printf("publisher callback: state absent — resolved via fallback to order %s", order.ID)
	}

	// Idempotency: already terminal — just redirect.
	if isTerminal(order.Status) {
		return h.redirectToTrade(c, order)
	}

	var (
		internalStatus string
		rtPtr          *string
		now            = time.Now().UTC()
	)
	switch kiteStatus {
	case "success":
		internalStatus = "user_returned"
		if requestToken != "" {
			rtPtr = &requestToken
		}
	default:
		internalStatus = "user_cancelled"
		if kiteStatus == "" {
			kiteStatus = "unknown"
		}
	}

	if err := h.PG.UpdatePublisherOrderCallback(c.Context(), order.ID, internalStatus, kiteStatus, rtPtr, now); err != nil {
		log.Printf("publisher callback: update order %s: %v", order.ID, err)
	}

	eventType := "USER_RETURNED_FROM_KITE"
	if internalStatus == "user_cancelled" {
		eventType = "USER_CANCELLED"
	}
	_ = h.PG.InsertPublisherOrderEvent(c.Context(), &store.PublisherOrderEvent{
		ID:               uuid.New().String(),
		PublisherOrderID: order.ID,
		EventType:        eventType,
		CreatedAt:        now,
	})

	if order.TradeID != nil && *order.TradeID != "" {
		tradeStatus := "submitted"
		if internalStatus == "user_cancelled" {
			tradeStatus = "cancelled"
		}
		if err := h.PG.UpdatePublisherTradeStatus(c.Context(), *order.TradeID, tradeStatus); err != nil {
			log.Printf("publisher callback: update trade %s status: %v", *order.TradeID, err)
		} else {
			metrics.RecordTrade(tradeStatus, "", "zerodha")
		}
	}

	return h.redirectToTrade(c, order)
}

// MarkPublisherOrderSubmitted lets the owning user manually confirm a basket
// was submitted on Kite when the automatic browser redirect back to
// /v1/publisher/callback never happens. This is a real, observed failure
// mode: Kite's own connect-session finalize step (kite.zerodha.com/.../finish)
// can error out *after* the basket has already been placed and executed —
// entirely inside Kite's own systems, after our POST to connect/basket has
// already handed off control — which strands the order in
// pending_confirmation forever even though it actually filled. This performs
// the same status transition the automatic callback would have made for
// status=success, so the trade stops reading as stuck/expired.
func (h *Handler) MarkPublisherOrderSubmitted(c *fiber.Ctx) error {
	orderID := c.Params("id")
	if _, err := uuid.Parse(orderID); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid order id")
	}

	order, err := h.PG.GetPublisherOrder(c.Context(), orderID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if order == nil || order.UserID != middleware.UserID(c) {
		return shared.Fail(c, fiber.StatusNotFound, "publisher order not found")
	}
	if isTerminal(order.Status) {
		return shared.Fail(c, fiber.StatusConflict, "this order has already been finalized")
	}

	now := time.Now().UTC()
	if err := h.PG.UpdatePublisherOrderCallback(c.Context(), order.ID, "user_returned", "success", nil, now); err != nil {
		log.Printf("publisher mark-submitted: update order %s: %v", order.ID, err)
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update order")
	}
	_ = h.PG.InsertPublisherOrderEvent(c.Context(), &store.PublisherOrderEvent{
		ID:               uuid.New().String(),
		PublisherOrderID: order.ID,
		EventType:        "USER_MARKED_SUBMITTED",
		CreatedAt:        now,
	})

	if order.TradeID != nil && *order.TradeID != "" {
		if err := h.PG.UpdatePublisherTradeStatus(c.Context(), *order.TradeID, "submitted"); err != nil {
			log.Printf("publisher mark-submitted: update trade %s status: %v", *order.TradeID, err)
		} else {
			metrics.RecordTrade("submitted", "", "zerodha")
		}
	}

	return shared.Ok(c, fiber.Map{"status": "submitted"})
}

func isTerminal(status string) bool {
	switch status {
	case "user_returned", "user_cancelled", "expired":
		return true
	}
	return false
}

func (h *Handler) redirectToTrade(c *fiber.Ctx, order *store.PublisherOrder) error {
	base := h.Cfg.EffectiveDashboardURL()
	if order.TradeID != nil && *order.TradeID != "" {
		return c.Redirect(base+"/trades/"+*order.TradeID+"?publisher=1", fiber.StatusFound)
	}
	return c.Redirect(base+"/dashboard?publisher=1", fiber.StatusFound)
}
