package signals

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/store"
)

const maxPaperSignalBytes = 32 * 1024

// ingestPaperSignal parses the typed paper-webhook payload and dispatches to
// the ORDER_SIGNAL or PRICE_UPDATE path. Called only when wh.PaperAccountID
// is set (see IngestSignal) — live-broker webhooks never reach here.
func (h *Handler) ingestPaperSignal(c *fiber.Ctx, wh *store.Webhook, requestID, ip string) error {
	ctx := context.Background()

	if len(c.Body()) > maxPaperSignalBytes {
		return h.rejectIngestUnparsed(ctx, c, wh, requestID, ip, fiber.StatusBadRequest,
			errCodeInvalidPayload, "payload too large (max 32 KB)")
	}

	body := c.Body()
	route, sig, raw, err := routePaperIngest(body)
	if err != nil {
		if sig != nil {
			return h.rejectIngestOrder(ctx, c, wh, sig, requestID, ip, fiber.StatusBadRequest,
				"gateway_rejected", err.Error())
		}
		return h.rejectIngestUnparsed(ctx, c, wh, requestID, ip, fiber.StatusBadRequest,
			errCodeInvalidPayload, "invalid payload")
	}

	switch route {
	case paperRouteOrder:
		return h.processOrderSignal(c, wh, sig, requestID, ip)
	case paperRoutePriceUpdate:
		return h.ingestPaperPriceUpdate(c, wh, raw, requestID, ip)
	default:
		return h.rejectIngestUnparsed(ctx, c, wh, requestID, ip, fiber.StatusBadRequest,
			errCodeInvalidPayload, "invalid payload")
	}
}

// paperOrderFromPayload maps a typed paper ORDER_SIGNAL body into the shared
// SignalPayload used by processOrderSignal. quantity is preferred; lot is
// accepted as a legacy alias (same field name live webhooks use).
func paperOrderFromPayload(raw *store.PaperSignalPayload) *store.SignalPayload {
	if raw == nil {
		return &store.SignalPayload{}
	}
	lot := raw.Quantity
	if lot == 0 {
		lot = raw.Lot
	}
	return &store.SignalPayload{
		Action:    strings.ToUpper(strings.TrimSpace(raw.Action)),
		Symbol:    raw.Symbol,
		Lot:       lot,
		Price:     raw.Price,
		Comment:   raw.Comment,
		Product:   raw.Product,
		OrderType: raw.OrderType,
	}
}

// ingestPaperPriceUpdate marks a symbol's last price on any open paper
// position (unrealized P&L only). It never creates an order, trade, or cash
// movement, so it is synchronous — no queue involved.
func (h *Handler) ingestPaperPriceUpdate(c *fiber.Ctx, wh *store.Webhook, raw *store.PaperSignalPayload, requestID, ip string) error {
	ctx := context.Background()

	if wh.RequiredComment != "" && raw.Comment != wh.RequiredComment {
		metrics.RecordIngest(metrics.IngestRejected)
		h.LogIngest(ctx, ip, wh.ID, requestID, "rejected", guard.ErrCommentRequired.Error())
		return shared.Fail(c, fiber.StatusUnauthorized, guard.ErrCommentRequired.Error())
	}

	symbol := strings.TrimSpace(raw.Symbol)
	if symbol == "" {
		if syms := guard.EffectiveAllowedSymbols(wh); len(syms) == 1 {
			symbol = syms[0]
		}
	}
	if symbol == "" {
		metrics.RecordIngest(metrics.IngestRejected)
		h.LogIngest(ctx, ip, wh.ID, requestID, "rejected", guard.ErrSymbolRequired.Error())
		return shared.Fail(c, fiber.StatusBadRequest, guard.ErrSymbolRequired.Error())
	}
	if err := guard.ValidateSymbol(wh, symbol); err != nil {
		metrics.RecordIngest(metrics.IngestRejected)
		h.LogIngest(ctx, ip, wh.ID, requestID, "rejected", err.Error())
		return shared.Fail(c, fiber.StatusForbidden, err.Error())
	}
	if raw.Price <= 0 {
		metrics.RecordIngest(metrics.IngestRejected)
		h.LogIngest(ctx, ip, wh.ID, requestID, "rejected", "price must be greater than zero")
		return shared.Fail(c, fiber.StatusBadRequest, "price must be greater than zero")
	}

	account, err := h.PG.GetPaperAccount(ctx, *wh.PaperAccountID)
	if err != nil || account == nil {
		metrics.RecordIngest(metrics.IngestRejected)
		h.LogIngest(ctx, ip, wh.ID, requestID, "rejected", "paper account not found")
		return shared.Fail(c, fiber.StatusInternalServerError, "paper account not found")
	}
	if account.Status != "active" {
		metrics.RecordIngest(metrics.IngestRejected)
		h.LogIngest(ctx, ip, wh.ID, requestID, "rejected", "paper account is "+account.Status)
		return shared.Fail(c, fiber.StatusForbidden, "paper account is "+account.Status)
	}

	position, err := h.PG.GetPaperPosition(ctx, account.ID, account.Exchange, symbol, account.DefaultProduct)
	if err != nil {
		metrics.RecordIngest(metrics.IngestRejected)
		h.LogIngest(ctx, ip, wh.ID, requestID, "rejected", "lookup failed")
		return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
	}

	marked := false
	if position != nil && position.Quantity != 0 {
		position.LastPrice = raw.Price
		position.UnrealizedPnL = domain.UnrealizedPnL(position.Quantity, position.AvgEntryPrice, raw.Price)
		position.UpdatedAt = time.Now().UTC()
		if err := h.PG.UpdatePaperPositionMark(ctx, position); err != nil {
			log.Printf("ingest: paper price update failed: %v", err)
			metrics.RecordIngest(metrics.IngestRejected)
			h.LogIngest(ctx, ip, wh.ID, requestID, "rejected", "mark update failed")
			return shared.Fail(c, fiber.StatusInternalServerError, "mark update failed")
		}
		marked = true
	}

	metrics.RecordIngest(metrics.IngestQueued)
	h.LogIngest(ctx, ip, wh.ID, requestID, "accepted", "")
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":     "ok",
		"request_id": requestID,
		"type":       string(domain.SignalTypePriceUpdate),
		"marked":     marked,
	})
}
