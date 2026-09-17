package internalapi

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// Handler serves adapter→core internal callbacks (service-token auth).
type Handler struct {
	*shared.Handler
}

func (h *Handler) PostExecutionEvent(c *fiber.Ctx) error {
	var body execution.ExecutionEvent
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	if strings.TrimSpace(body.RequestID) == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "request_id required")
	}

	trade, err := h.PG.GetTradeByID(c.Context(), body.RequestID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "trade lookup failed")
	}
	if trade == nil {
		return shared.Fail(c, fiber.StatusNotFound, "trade not found")
	}

	oldStatus := trade.Status
	brokerType := h.tradeBrokerType(c.Context(), trade)

	switch strings.ToLower(body.Event) {
	case "filled":
		trade.Status = "filled"
		if body.FillPrice > 0 {
			trade.FillPrice = body.FillPrice
		}
	case "submitted":
		trade.Status = "submitted"
	case "rejected", "cancelled":
		trade.Status = body.Event
	default:
		return shared.Fail(c, fiber.StatusBadRequest, "unsupported event")
	}
	if body.BrokerOrderID != "" {
		trade.BrokerOrder = body.BrokerOrderID
	}
	if err := h.PG.UpdateTradeFromExecutionEvent(c.Context(), trade); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update trade")
	}
	if trade.Status != oldStatus {
		metrics.RecordTrade(trade.Status, trade.ErrorCode, brokerType)
	}
	return shared.Ok(c, fiber.Map{"updated": true})
}

func (h *Handler) tradeBrokerType(ctx context.Context, trade *store.Trade) string {
	if trade == nil || trade.WebhookID == "" {
		return ""
	}
	wh, err := h.PG.GetWebhookByID(ctx, trade.WebhookID)
	if err != nil || wh == nil || wh.BrokerCredID == nil {
		return ""
	}
	cred, err := h.PG.GetBrokerCred(ctx, *wh.BrokerCredID)
	if err != nil || cred == nil {
		return ""
	}
	return cred.BrokerType
}

func (h *Handler) PutCredentialSession(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid credential id")
	}
	var body execution.SessionUpdate
	if err := c.BodyParser(&body); err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
	}
	if strings.TrimSpace(body.EncryptedCreds) == "" {
		return shared.Fail(c, fiber.StatusBadRequest, "encrypted_creds required")
	}

	cred, err := h.PG.GetBrokerCred(c.Context(), id)
	if err != nil || cred == nil {
		return shared.Fail(c, fiber.StatusNotFound, "credential not found")
	}
	if err := h.PG.ReconnectZerodhaToken(c.Context(), id, body.EncryptedCreds); err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not update session")
	}
	return shared.Ok(c, fiber.Map{"credential_id": id, "updated_at": time.Now().UTC()})
}
