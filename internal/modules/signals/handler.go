package signals

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/SPSingh09/zettabridge/internal/platform/security"
	"github.com/SPSingh09/zettabridge/internal/compliance"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/platform/security/idempotency"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/platform/queue"
	"github.com/SPSingh09/zettabridge/internal/store"
	"github.com/SPSingh09/zettabridge/internal/transport/websocket"
	"github.com/SPSingh09/zettabridge/internal/platform/security/webhooktoken"
)

// Handler handles signal ingestion and live trade streaming.
type Handler struct {
	*shared.Handler
	Queue  *queue.Queue
	Trades *tradepush.Hub
}

// IngestSignal is called by TradingView / external scripts.
// It validates the token, resolves the webhook config (Redis → Postgres),
// and enqueues the job immediately so HTTP can return 202 fast.
func (h *Handler) IngestSignal(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		metrics.RecordIngest(metrics.IngestRejected)
		return shared.Fail(c, fiber.StatusBadRequest, "missing token")
	}

	ctx := context.Background()
	tokenHash := webhooktoken.Hash(token)

	// 1. Look up webhook config — Redis first, fallback to Postgres
	wh, err := h.Redis.GetWebhookByToken(ctx, tokenHash)
	if err != nil {
		log.Printf("ingest: redis error: %v", err)
	}
	if wh == nil {
		wh, err = h.PG.GetWebhookByToken(ctx, tokenHash)
		if err != nil {
			metrics.RecordIngest(metrics.IngestRejected)
			return shared.Fail(c, fiber.StatusInternalServerError, "lookup failed")
		}
		if wh != nil {
			// warm the cache
			_ = h.Redis.CacheWebhook(ctx, wh)
		}
	}

	if wh == nil {
		metrics.RecordIngest(metrics.IngestRejected)
		return shared.Fail(c, fiber.StatusUnauthorized, "unknown token")
	}

	// requestID is assigned once here and reused as tradeID so every response
	// and every ingest_log row share the same identifier.
	requestID := uuid.New().String()
	ip := c.IP()

	if wh.OrgID == nil {
		mem, err := h.PG.GetActiveOrgMembershipByUser(ctx, wh.UserID)
		if err != nil {
			log.Printf("ingest: org membership lookup error: %v", err)
			return h.rejectIngestWebhook(ctx, c, wh, requestID, ip, fiber.StatusInternalServerError, "gateway_rejected", "lookup failed")
		}
		if mem != nil {
			msg := domain.ErrSoloResourcesDisabled.Error()
			return h.rejectIngestWebhook(ctx, c, wh, requestID, ip, fiber.StatusForbidden, errCodeSoloResourcesDisabled, msg)
		}
	}
	if wh.Status != "active" {
		return h.rejectIngestWebhook(ctx, c, wh, requestID, ip, fiber.StatusForbidden, errCodeWebhookPaused, "webhook is paused")
	}

	if err := h.AssertWebhookMayTrade(ctx, wh); err != nil {
		switch {
		case errors.Is(err, compliance.ErrAccountSuspended):
			return h.rejectIngestWebhook(ctx, c, wh, requestID, ip, fiber.StatusForbidden, errCodeAccountSuspended, err.Error())
		case errors.Is(err, compliance.ErrOrgSuspended):
			return h.rejectIngestWebhook(ctx, c, wh, requestID, ip, fiber.StatusForbidden, errCodeOrgSuspended, err.Error())
		default:
			log.Printf("ingest: compliance lookup error: %v", err)
			return h.rejectIngestWebhook(ctx, c, wh, requestID, ip, fiber.StatusInternalServerError, "gateway_rejected", "lookup failed")
		}
	}

	// 2. Parse signal payload.
	// Paper-destination webhooks use a distinct typed payload (ORDER_SIGNAL /
	// PRICE_UPDATE) — see paper_ingest.go. Live-broker webhooks keep the flat
	// legacy shape below, unchanged.
	if wh.PaperAccountID != nil {
		return h.ingestPaperSignal(c, wh, requestID, ip)
	}

	const maxSignalBytes = 32 * 1024
	if len(c.Body()) > maxSignalBytes {
		return h.rejectIngestUnparsed(ctx, c, wh, requestID, ip, fiber.StatusBadRequest,
			errCodeInvalidPayload, "payload too large (max 32 KB)")
	}
	var sig store.SignalPayload
	if err := c.BodyParser(&sig); err != nil {
		return h.rejectIngestUnparsed(ctx, c, wh, requestID, ip, fiber.StatusBadRequest,
			errCodeInvalidPayload, "invalid payload")
	}
	return h.processOrderSignal(c, wh, &sig, requestID, ip)
}

// processOrderSignal runs the shared hot-path checks (action validation,
// guard rules, rate limits, dedup) and enqueues the job. Both the live path
// (IngestSignal, legacy flat payload) and the paper ORDER_SIGNAL path
// (ingestPaperSignal, typed payload translated into a SignalPayload) call
// this so the two never drift apart on enforcement.
func (h *Handler) processOrderSignal(c *fiber.Ctx, wh *store.Webhook, sig *store.SignalPayload, requestID, ip string) error {
	ctx := context.Background()

	sig.Action = strings.ToUpper(sig.Action)
	if sig.Action != "BUY" && sig.Action != "SELL" && sig.Action != "CLOSE" {
		return h.rejectIngestOrder(ctx, c, wh, sig, requestID, ip, fiber.StatusBadRequest,
			errCodeInvalidAction, "action must be BUY, SELL, or CLOSE")
	}

	if err := guard.ValidateIngest(wh, sig, time.Now().UTC()); err != nil {
		status := fiber.StatusForbidden
		if errors.Is(err, guard.ErrCommentRequired) {
			status = fiber.StatusUnauthorized
		}
		return h.rejectIngestOrder(ctx, c, wh, sig, requestID, ip, status, gatewayErrCode(err), err.Error())
	}

	// Per-minute ingest rate limit (misuse protection)
	if wh.RateLimitPerMin > 0 {
		count, err := h.Redis.IncrWebhookRatePerMin(ctx, wh.ID)
		if err != nil {
			log.Printf("ingest: webhook per-min rate-limit redis error: %v", err)
		} else if count > int64(wh.RateLimitPerMin) {
			msg := fmt.Sprintf("webhook rate limit exceeded (%d req/min)", wh.RateLimitPerMin)
			return h.rejectIngestRateLimited(ctx, c, wh, sig, requestID, ip, "60", errCodeRateLimited, msg)
		}
	}

	// Free plan: 100 requests/day hard cap
	user, err := h.PG.GetUserByID(ctx, wh.UserID)
	if err != nil {
		log.Printf("ingest: user lookup error: %v", err)
		return h.rejectIngestInternal(ctx, c, wh, sig, requestID, ip)
	}
	if user != nil && user.Plan == "free" {
		dayCount, err := h.Redis.IncrDailyWebhookRequests(ctx, wh.UserID)
		if err != nil {
			log.Printf("ingest: daily limit redis error: %v", err)
		} else if dayCount > 100 {
			now := time.Now().UTC()
			midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
			return h.rejectIngestRateLimited(ctx, c, wh, sig, requestID, ip,
				fmt.Sprintf("%d", int(time.Until(midnight).Seconds())),
				errCodeRateLimited, "daily webhook request limit reached (free plan: 100 req/day)")
		}
	}

	// Per-second broker-side cap (enforced at ingest to protect queue)
	if wh.RateLimitPerSec > 0 {
		count, err := h.Redis.IncrWebhookRateLimit(ctx, wh.ID)
		if err != nil {
			log.Printf("ingest: webhook per-sec rate-limit redis error: %v", err)
		} else if count > int64(wh.RateLimitPerSec) {
			msg := fmt.Sprintf("webhook rate limit exceeded (%d req/s)", wh.RateLimitPerSec)
			return h.rejectIngestRateLimited(ctx, c, wh, sig, requestID, ip, "1", errCodeRateLimited, msg)
		}
	}

	signalKey := idempotency.SignalKey(wh.ID, wh, sig)
	dedupWindow := idempotency.Window(wh)
	tradeID := requestID
	if dedupWindow > 0 {
		claimed, err := h.Redis.TryClaimSignalDedup(ctx, wh.ID, signalKey, dedupWindow)
		if err != nil {
			log.Printf("ingest: dedup redis error: %v", err)
		} else if !claimed {
			return h.rejectIngestDuplicate(ctx, c, wh, sig, requestID, ip)
		}
	}

	// 3. Enqueue — non-blocking
	enqueued := h.Queue.Enqueue(queue.Job{Webhook: wh, Signal: sig, SignalKey: signalKey, TradeID: tradeID})
	if !enqueued {
		if dedupWindow > 0 {
			_ = h.Redis.ReleaseSignalDedup(ctx, wh.ID, signalKey)
		}
		return h.rejectIngestQueueFull(ctx, c, wh, sig, requestID, ip)
	}

	metrics.RecordIngest(metrics.IngestQueued)
	h.LogIngest(ctx, ip, wh.ID, requestID, "accepted", "")
	// 4. Return 202 immediately
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"status":       "accepted",
		"request_id":   tradeID,
		"queued":       true,
		"deduplicated": false,
		"message":      "Webhook accepted for processing.",
	})
}

// AssertWebhookMayTrade checks compliance for trading.
func (h *Handler) AssertWebhookMayTrade(ctx context.Context, wh *store.Webhook) error {
	user, err := h.PG.GetUserByID(ctx, wh.UserID)
	if err != nil {
		return err
	}
	var org *store.Organization
	if wh.OrgID != nil && *wh.OrgID != "" {
		org, err = h.PG.GetOrgByID(ctx, *wh.OrgID)
		if err != nil {
			return err
		}
	}
	return compliance.WebhookMayTrade(user, org)
}

// WsTradesUpgrade upgrades the HTTP connection to a WebSocket.
func (h *Handler) WsTradesUpgrade(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}
	raw := MiddlewareBearerOrQuery(c)
	if raw == "" {
		return shared.Fail(c, fiber.StatusUnauthorized, "invalid or missing token")
	}
	userID, err := security.ParseAccessToken(h.Cfg.JWTSecret, raw)
	if err != nil {
		return shared.Fail(c, fiber.StatusUnauthorized, "invalid or missing token")
	}
	revoked, err := h.Redis.IsJWTRevoked(c.Context(), raw)
	if err == nil && revoked {
		return shared.Fail(c, fiber.StatusUnauthorized, "token revoked")
	}
	c.Locals("ws_user_id", userID)
	return c.Next()
}

// WsTrades handles the WebSocket connection for live trade streaming.
func (h *Handler) WsTrades(conn *websocket.Conn) {
	userID, _ := conn.Locals("ws_user_id").(string)
	if userID == "" || h.Trades == nil {
		_ = conn.Close()
		return
	}
	cleanup := h.Trades.Register(userID, conn)
	defer cleanup()

	// Read loop — discard client messages; break on disconnect or error.
	// NOTE: do NOT write to conn here; all writes (including pong responses)
	// are handled exclusively by the writePump goroutine to avoid concurrent
	// write panics. The custom ping handler in writePump sends pong frames.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// insertGatewayRejectedTrade persists and broadcasts a trade record for signals
// that are rejected at the HTTP ingest layer (after the body is parsed) so that
// failures are visible in the webhook trades table alongside broker-level rejections.
func (h *Handler) insertGatewayRejectedTrade(ctx context.Context, wh *store.Webhook, sig *store.SignalPayload, tradeID, errCode, errMsg string) {
	trade := buildGatewayRejectedTrade(wh, sig, tradeID, errCode, errMsg)
	if trade == nil {
		return
	}
	brokerType := ingestBrokerType(ctx, h.PG, wh)
	metrics.RecordTrade(trade.Status, trade.ErrorCode, brokerType)
	if err := h.PG.PersistTradeAudit(ctx, trade); err != nil {
		log.Printf("ingest: CRITICAL gateway reject trade audit failed id=%s webhook=%s code=%s: %v", tradeID, wh.ID, errCode, err)
		return
	}
	if h.Trades != nil {
		h.Trades.OnTradeInserted(ctx, trade)
	}
}

// gatewayErrCode maps a guard validation error to a stable error code string.
func gatewayErrCode(err error) string {
	return guard.ErrCode(err)
}

// MiddlewareBearerOrQuery extracts the token from the query string or Authorization header.
func MiddlewareBearerOrQuery(c *fiber.Ctx) string {
	if tok := c.Query("token"); tok != "" {
		return tok
	}
	a := c.Get("Authorization")
	if strings.HasPrefix(a, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(a, "Bearer "))
	}
	return ""
}
