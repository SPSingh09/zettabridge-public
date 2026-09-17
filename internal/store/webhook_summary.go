package store

import (
	"context"
	"database/sql"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
)

// rejectionReasonLabels maps a trade.ErrorCode to the human label shown in
// the webhook Summary's Risk & Validation breakdown. Codes not listed here
// fall back to the raw code string, so nothing is silently dropped.
var rejectionReasonLabels = map[string]string{
	"invalid_secret":                         "Secret Failed",
	"action_not_allowed":                     "Action Not Allowed",
	"symbol_not_allowed":                     "Invalid Symbol",
	"symbol_required":                        "Invalid Symbol",
	"quantity_required":                      "Invalid Quantity",
	"lot_exceeds_max":                        "Lot Size Exceeded",
	"outside_trading_hours":                  "Outside Trading Hours",
	"order_type_not_allowed":                 "Order Type Not Allowed",
	"gateway_rejected":                       "Validation Failed",
	"invalid_payload":                        "Invalid Payload",
	"invalid_action":                         "Invalid Action",
	"rate_limited":                           "Rate Limited",
	"queue_full":                             "Queue Full",
	"webhook_paused":                         "Webhook Paused",
	"account_suspended":                      "Account Suspended",
	"org_suspended":                          "Organization Suspended",
	"solo_resources_disabled":                "Solo Resources Disabled",
	"duplicate_suppressed":                   "Duplicate Suppressed",
	string(brokererr.CodeMarketClosed):       "Market Closed",
	string(brokererr.CodeInvalidSymbol):      "Invalid Symbol",
	string(brokererr.CodeInsufficientMargin): "Insufficient Margin",
	string(brokererr.CodeAuthFailed):         "Broker Auth Failed",
	string(brokererr.CodeInvalidCredentials): "Invalid Broker Credentials",
	string(brokererr.CodeBrokerUnreachable):  "Broker Unreachable",
	string(brokererr.CodeAlgoIDRequired):     "Algo ID Required",
	string(brokererr.CodePermissionDenied):   "Broker Permission Denied",
	string(brokererr.CodeInternal):           "Internal Validation",
	// Paper trading (internal/platform/queue/paper.go) — see paperErrCode.
	"paper_not_configured":     "Paper Trading Not Configured",
	"paper_account_not_found":  "Paper Account Not Found",
	"paper_account_inactive":   "Paper Account Inactive",
	"paper_quota_exceeded":     "Quota Exceeded",
	"instrument_not_available": "Instrument Not Available",
	"market_profile_not_found": "Market Profile Not Found",
	"quote_unavailable":        "Quote Unavailable",
	"price_required":           "Price Required",
	"product_required":         "Product Required",
	"invalid_order":            "Invalid Order",
	"invalid_price":            "Invalid Price",
}

// rejectedBeforeTradeLabel buckets rejections caught before a trade row could
// even be constructed (org/account suspended, oversized/unparseable
// payload, user lookup failure — see enrichWebhookSummary's doc comment).
// Shares a label with "gateway_rejected" above (guard.ValidateIngest
// failures) — both mean the same thing to a viewer: ZettaBridge rejected
// the signal during validation, before it ever reached the broker.
const rejectedBeforeTradeLabel = "Validation Failed"

// enrichWebhookSummary populates the Signal Processing / Broker Execution /
// Risk & Validation / Recent Activity fields on summary for a single
// webhook. It combines two data sources:
//   - trades: every signal that reached webhook-level guard validation,
//     whether it was rejected there (see insertGatewayRejectedTrade),
//     rejected by the worker/broker, or executed. This is the primary
//     source for rejection categorization (ZettaBridge vs broker) and
//     order-side/latency stats.
//   - ingest_log: every HTTP ingest attempt, including several rejection
//     paths that happen before a trade can even be constructed (org/account
//     suspended, oversized/unparseable payload, user lookup failure) and so
//     never get a trades row — plus dedup/rate-limit/pause/queue-full
//     outcomes, which never get one by design. ingest_log.request_id is the
//     same ID used as trades.id for the one rejection path that does insert
//     a row (insertGatewayRejectedTrade), so a NOT EXISTS anti-join on that
//     ID reliably avoids double-counting that path while still catching the
//     others.
func (p *PGStore) enrichWebhookSummary(ctx context.Context, webhookID string, summary *PnLSummary) error {
	var dedup, rateLimited, paused, queueFull, rejectedNoTrade int
	rows, err := p.db.QueryContext(ctx, `
		SELECT outcome,
			COUNT(*) FILTER (WHERE outcome <> 'rejected'
				OR NOT EXISTS (SELECT 1 FROM trades t WHERE t.id = ingest_log.request_id))::int
		FROM ingest_log WHERE webhook_id=$1
		GROUP BY outcome`, webhookID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var outcome string
		var count int
		if err := rows.Scan(&outcome, &count); err != nil {
			rows.Close()
			return err
		}
		switch outcome {
		case "dedup":
			dedup = count
		case "rate_limited":
			rateLimited = count
		case "paused":
			paused = count
		case "queue_full":
			queueFull = count
		case "rejected":
			rejectedNoTrade = count
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	summary.DuplicateSuppressed = dedup
	summary.RateLimitedSignals = rateLimited
	summary.PausedSignals = paused
	summary.QueueFullSignals = queueFull

	summary.RejectionReasons = map[string]int{}
	if rejectedNoTrade > 0 {
		summary.RejectionReasons[rejectedBeforeTradeLabel] = rejectedNoTrade
		summary.ZettaBridgeRejected += rejectedNoTrade
	}

	rows, err = p.db.QueryContext(ctx,
		`SELECT status, error_code, signal, COUNT(*)::int
		 FROM trades WHERE webhook_id=$1
		 GROUP BY status, error_code, signal`, webhookID)
	if err != nil {
		return err
	}
	var totalTrades, rejected int
	for rows.Next() {
		var status, errorCode, signal string
		var count int
		if err := rows.Scan(&status, &errorCode, &signal, &count); err != nil {
			rows.Close()
			return err
		}
		totalTrades += count
		switch signal {
		case "BUY":
			summary.BuyOrders += count
		case "SELL":
			summary.SellOrders += count
		case "CLOSE":
			summary.CloseOrders += count
		}
		switch status {
		case "submitted", "filled", "pending_confirmation":
			summary.SubmittedToBroker += count
			if status == "filled" {
				summary.Filled += count
			}
			if status == "pending_confirmation" {
				summary.PendingConfirmation += count
			}
		case "cancelled":
			summary.Cancelled += count
		case "rejected":
			rejected += count
			if brokererr.Category(errorCode) == brokererr.CategoryBroker {
				summary.BrokerRejected += count
			} else {
				summary.ZettaBridgeRejected += count
			}
			label := rejectionReasonLabels[errorCode]
			if label == "" {
				if errorCode == "" {
					label = "Unspecified"
				} else {
					label = errorCode
				}
			}
			summary.RejectionReasons[label] += count
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	summary.RejectedSignals = rejected + rejectedNoTrade

	summary.AcceptedSignals = totalTrades - rejected
	// totalTrades already includes every accepted ORDER_SIGNAL (queued → terminal).
	// ingest_log "accepted" rows share trades.id via request_id — do not add them
	// again. Ingest-only outcomes (dedup, rate_limited, paused, queue_full) and
	// gateway rejects without a trade row fill the remainder.
	summary.TotalSignals = totalTrades + rejectedNoTrade + dedup + rateLimited + paused + queueFull
	if summary.TotalSignals > 0 {
		rate := float64(summary.SubmittedToBroker) / float64(summary.TotalSignals)
		summary.BrokerSuccessRate = &rate
	}

	var avgLatencyMs sql.NullFloat64
	var lastTradeSignalAt, lastBrokerResponseAt sql.NullTime
	// Avg latency = worker queued (created_at) → first broker/paper delivery
	// (broker_responded_at). Fill poll, cancel, and other later writes bump
	// updated_at but must not affect this metric. The worker→broker attempt
	// is hard-bounded by tradeTimeout (10s, see queue.go), so anything past
	// 60s cannot be genuine ZettaBridge/broker latency — it's contamination
	// (e.g. a Publisher human-confirmation delay that leaked into this
	// column via historical data) and is excluded rather than skewing the
	// average.
	err = p.db.QueryRowContext(ctx, `
		SELECT
			AVG(EXTRACT(EPOCH FROM (broker_responded_at - created_at)) * 1000)
				FILTER (WHERE broker_responded_at IS NOT NULL
					AND status IN ('submitted','filled','rejected','pending_confirmation')
					AND broker_responded_at - created_at < INTERVAL '60 seconds'),
			MAX(created_at),
			MAX(broker_responded_at)
		FROM trades WHERE webhook_id=$1`, webhookID,
	).Scan(&avgLatencyMs, &lastTradeSignalAt, &lastBrokerResponseAt)
	if err != nil {
		return err
	}
	if avgLatencyMs.Valid {
		summary.AvgLatencyMs = &avgLatencyMs.Float64
	}
	if lastBrokerResponseAt.Valid {
		summary.LastBrokerResponseAt = &lastBrokerResponseAt.Time
	}

	// Last Signal Received: ingest_log sees every HTTP attempt (including
	// dedup/rate-limited/paused ones a trades-only MAX(created_at) would
	// miss), so prefer it and fall back to the trades-derived time for
	// webhooks with data older than ingest_log's own rollout.
	var lastIngestAt sql.NullTime
	if err := p.db.QueryRowContext(ctx,
		`SELECT MAX(created_at) FROM ingest_log WHERE webhook_id=$1`, webhookID,
	).Scan(&lastIngestAt); err != nil {
		return err
	}
	switch {
	case lastIngestAt.Valid:
		summary.LastSignalAt = &lastIngestAt.Time
	case lastTradeSignalAt.Valid:
		summary.LastSignalAt = &lastTradeSignalAt.Time
	}

	var lastRejectionReason string
	var lastRejectionAt sql.NullTime
	err = p.db.QueryRowContext(ctx, `
		SELECT error, created_at FROM trades
		WHERE webhook_id=$1 AND status='rejected'
		ORDER BY created_at DESC LIMIT 1`, webhookID,
	).Scan(&lastRejectionReason, &lastRejectionAt)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && lastRejectionAt.Valid {
		summary.LastRejectionReason = lastRejectionReason
		summary.LastRejectionAt = &lastRejectionAt.Time
	}

	// A gateway-only rejection (no trades row) might be more recent than the
	// latest trades-table rejection — compare and take whichever is later.
	var lastOrphanReason string
	var lastOrphanAt sql.NullTime
	err = p.db.QueryRowContext(ctx, `
		SELECT error, created_at FROM ingest_log
		WHERE webhook_id=$1 AND outcome='rejected'
			AND NOT EXISTS (SELECT 1 FROM trades t WHERE t.id = ingest_log.request_id)
		ORDER BY created_at DESC LIMIT 1`, webhookID,
	).Scan(&lastOrphanReason, &lastOrphanAt)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && lastOrphanAt.Valid && (summary.LastRejectionAt == nil || lastOrphanAt.Time.After(*summary.LastRejectionAt)) {
		summary.LastRejectionReason = lastOrphanReason
		summary.LastRejectionAt = &lastOrphanAt.Time
	}

	return nil
}
