package signals

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/platform/security/idempotency"
	"github.com/SPSingh09/zettabridge/internal/store"
)

const (
	errCodeInvalidPayload        = "invalid_payload"
	errCodeInvalidAction         = "invalid_action"
	errCodeRateLimited           = "rate_limited"
	errCodeQueueFull             = "queue_full"
	errCodeWebhookPaused         = "webhook_paused"
	errCodeDuplicateSuppressed   = "duplicate_suppressed"
	errCodeAccountSuspended      = "account_suspended"
	errCodeOrgSuspended          = "org_suspended"
	errCodeSoloResourcesDisabled = "solo_resources_disabled"
)

// Every helper in this file persists a rejected trades row (status=rejected with
// error + error_code) for Paper, OAuth, and Publisher webhooks whenever the
// webhook is known. Silent ingest rejections are not allowed.

// rejectIngestOrder logs, persists a rejected trade row, and returns an HTTP error.
func (h *Handler) rejectIngestOrder(ctx context.Context, c *fiber.Ctx, wh *store.Webhook, sig *store.SignalPayload, requestID, ip string, status int, errCode, errMsg string) error {
	return h.rejectIngestWithTrade(ctx, c, wh, sig, requestID, ip, status, "rejected", metrics.IngestRejected, errCode, errMsg)
}

// rejectIngestUnparsed logs, persists a minimal rejected trade row when the body
// could not be interpreted as an order signal, and returns an HTTP error.
func (h *Handler) rejectIngestUnparsed(ctx context.Context, c *fiber.Ctx, wh *store.Webhook, requestID, ip string, status int, errCode, errMsg string) error {
	return h.rejectIngestWithTrade(ctx, c, wh, nil, requestID, ip, status, "rejected", metrics.IngestRejected, errCode, errMsg)
}

// rejectIngestWebhook persists a rejected trade row for failures that happen
// before the typed parser runs, best-effort parsing the body for symbol/qty.
func (h *Handler) rejectIngestWebhook(ctx context.Context, c *fiber.Ctx, wh *store.Webhook, requestID, ip string, status int, errCode, errMsg string) error {
	sig := h.tryParseOrderSignal(c, wh)
	return h.rejectIngestWithTrade(ctx, c, wh, sig, requestID, ip, status, "rejected", metrics.IngestRejected, errCode, errMsg)
}

func (h *Handler) rejectIngestRateLimited(ctx context.Context, c *fiber.Ctx, wh *store.Webhook, sig *store.SignalPayload, requestID, ip, retryAfter string, errCode, errMsg string) error {
	if retryAfter != "" {
		c.Set("Retry-After", retryAfter)
	}
	return h.rejectIngestWithTrade(ctx, c, wh, sig, requestID, ip, fiber.StatusTooManyRequests, "rate_limited", metrics.IngestRateLimited, errCode, errMsg)
}

func (h *Handler) rejectIngestDuplicate(ctx context.Context, c *fiber.Ctx, wh *store.Webhook, sig *store.SignalPayload, requestID, ip string) error {
	msg := "duplicate signal suppressed within dedup window"
	metrics.RecordIngest(metrics.IngestDeduplicated)
	metrics.RecordDedupHit()
	h.LogIngest(ctx, ip, wh.ID, requestID, "dedup", msg)
	h.insertGatewayRejectedTrade(ctx, wh, sig, requestID, errCodeDuplicateSuppressed, msg)
	return c.Status(fiber.StatusConflict).JSON(fiber.Map{
		"request_id":   requestID,
		"queued":       false,
		"deduplicated": true,
		"message":      msg,
	})
}

func (h *Handler) rejectIngestQueueFull(ctx context.Context, c *fiber.Ctx, wh *store.Webhook, sig *store.SignalPayload, requestID, ip string) error {
	return h.rejectIngestWithTrade(ctx, c, wh, sig, requestID, ip, fiber.StatusServiceUnavailable, "queue_full", metrics.IngestQueueFull, errCodeQueueFull, "queue full — retry shortly")
}

func (h *Handler) rejectIngestWithTrade(ctx context.Context, c *fiber.Ctx, wh *store.Webhook, sig *store.SignalPayload, requestID, ip string, status int, ingestOutcome string, ingestMetric string, errCode, errMsg string) error {
	metrics.RecordIngest(ingestMetric)
	h.LogIngest(ctx, ip, wh.ID, requestID, ingestOutcome, errMsg)
	h.insertGatewayRejectedTrade(ctx, wh, sig, requestID, errCode, errMsg)
	return shared.Fail(c, status, errMsg)
}

func (h *Handler) tryParseOrderSignal(c *fiber.Ctx, wh *store.Webhook) *store.SignalPayload {
	if c == nil || wh == nil {
		return nil
	}
	body := c.Body()
	if len(body) == 0 {
		return nil
	}
	if wh.PaperAccountID != nil {
		route, sig, _, err := routePaperIngest(body)
		if err == nil && route == paperRouteOrder && sig != nil {
			return sig
		}
	}
	var sig store.SignalPayload
	if err := json.Unmarshal(body, &sig); err != nil {
		return nil
	}
	if strings.TrimSpace(sig.Action) == "" {
		return nil
	}
	sig.Action = strings.ToUpper(strings.TrimSpace(sig.Action))
	return &sig
}

func buildGatewayRejectedTrade(wh *store.Webhook, sig *store.SignalPayload, tradeID, errCode, errMsg string) *store.Trade {
	now := time.Now().UTC()
	if wh == nil {
		return nil
	}
	if sig == nil {
		return &store.Trade{
			ID:           tradeID,
			UserID:       wh.UserID,
			WebhookID:    wh.ID,
			WebhookLabel: wh.Label,
			Signal:       "—",
			Symbol:       "—",
			Status:       "rejected",
			Error:        errMsg,
			ErrorCode:    errCode,
			CreatedAt:    now,
		}
	}

	params := guard.ResolveParams(wh, sig)
	orderType := guard.WebhookOrderType(wh)
	if ot := strings.ToUpper(strings.TrimSpace(sig.OrderType)); ot == "MARKET" || ot == "LIMIT" {
		orderType = ot
	}
	if params.SLPts > 0 || params.TPPts > 0 {
		orderType = "BRACKET"
	}
	lot := params.Lot
	if lot == 0 && wh.LotSize > 0 {
		lot = wh.LotSize
	}
	signal := strings.ToUpper(strings.TrimSpace(sig.Action))
	if signal == "" {
		signal = "—"
	}
	symbol := params.Symbol
	if symbol == "" {
		symbol = "—"
	}

	return &store.Trade{
		ID:           tradeID,
		UserID:       wh.UserID,
		WebhookID:    wh.ID,
		WebhookLabel: wh.Label,
		Signal:       signal,
		Symbol:       symbol,
		LotSize:      lot,
		SignalKey:    idempotency.SignalKey(wh.ID, wh, sig),
		Comment:      sig.Comment,
		OrderType:    orderType,
		Product:      strings.ToUpper(strings.TrimSpace(params.Product)),
		Status:       "rejected",
		Error:        errMsg,
		ErrorCode:    errCode,
		CreatedAt:    now,
	}
}

func ingestBrokerType(ctx context.Context, pg *store.PGStore, wh *store.Webhook) string {
	if wh == nil {
		return ""
	}
	if wh.PaperAccountID != nil && *wh.PaperAccountID != "" {
		return "paper"
	}
	if wh.BrokerCredID != nil && pg != nil {
		if cred, err := pg.GetBrokerCred(ctx, *wh.BrokerCredID); err == nil && cred != nil {
			return cred.BrokerType
		}
	}
	return ""
}

func (h *Handler) rejectIngestInternal(ctx context.Context, c *fiber.Ctx, wh *store.Webhook, sig *store.SignalPayload, requestID, ip string) error {
	return h.rejectIngestWithTrade(ctx, c, wh, sig, requestID, ip, fiber.StatusInternalServerError, "rejected", metrics.IngestRejected, "gateway_rejected", "lookup failed")
}
