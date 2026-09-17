package queue

import (
	"context"
	"log"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// rejectTrade records a worker-side guard-validation rejection, tagging it
// with the same stable error code the HTTP-ingest gateway path uses (via
// guard.ErrCode) so the webhook Summary view can categorize it as a
// ZettaBridge-side rejection regardless of which path caught it.
func (q *Queue) rejectTrade(ctx context.Context, wh *store.Webhook, sig *store.SignalPayload, signalKey, tradeID string, guardErr error) {
	q.rejectTradeWithCode(ctx, wh, sig, signalKey, tradeID, guard.ErrCode(guardErr), guardErr.Error(), brokerTypeForWebhook(ctx, q.pg, wh))
}

func (q *Queue) rejectTradeWithCode(ctx context.Context, wh *store.Webhook, sig *store.SignalPayload, signalKey, tradeID, code, message, brokerType string) {
	trade := q.buildRejectedTrade(wh, sig, signalKey, tradeID, code, message)
	log.Printf("worker: rejected webhook=%s code=%s reason=%s", wh.ID, code, message)
	q.persistTrade(ctx, trade, brokerType)
}

func (q *Queue) buildRejectedTrade(wh *store.Webhook, sig *store.SignalPayload, signalKey, tradeID, code, message string) *store.Trade {
	params := guard.ResolveParams(wh, sig)
	if q != nil {
		_ = q.resolveWebhookProduct(context.Background(), wh, &params)
	}
	if tradeID == "" {
		tradeID = newTradeID()
	}
	trade := &store.Trade{
		ID:           tradeID,
		UserID:       wh.UserID,
		WebhookID:    wh.ID,
		WebhookLabel: wh.Label,
		Signal:       sig.Action,
		Symbol:       params.Symbol,
		SignalKey:    signalKey,
		Comment:      sig.Comment,
		Status:       "rejected",
		Error:        message,
		ErrorCode:    code,
		CreatedAt:    nowUTC(),
	}
	applyTradeSignalMeta(trade, wh, sig, params, params.Product)
	return trade
}

func (q *Queue) rejectBrokerTrade(ctx context.Context, trade *store.Trade, brokerType string, err error) {
	code, message := brokererr.TradeFields(err)
	q.rejectExistingTrade(ctx, trade, code, message, brokerType)
}

func (q *Queue) rejectExistingTrade(ctx context.Context, trade *store.Trade, code, message, brokerType string) {
	if trade == nil {
		return
	}
	trade.Status = "rejected"
	trade.Error = message
	trade.ErrorCode = code
	log.Printf("worker: rejected webhook=%s code=%s reason=%s", trade.WebhookID, code, message)
	q.finalizeTrade(ctx, trade, brokerType)
}

func (q *Queue) persistTrade(ctx context.Context, trade *store.Trade, brokerType string) {
	if trade == nil {
		return
	}
	metrics.RecordTrade(trade.Status, trade.ErrorCode, brokerType)
	if err := q.pg.PersistTradeAudit(ctx, trade); err != nil {
		log.Printf("worker: CRITICAL trade audit persist failed id=%s webhook=%s status=%s code=%s: %v",
			trade.ID, trade.WebhookID, trade.Status, trade.ErrorCode, err)
		return
	}
	if trade.UserID != "" {
		go q.dispatchAlerts(context.Background(), trade)
	}
	if q.notifier != nil {
		q.notifier.OnTradeInserted(ctx, trade)
	}
}

func (q *Queue) insertTrade(ctx context.Context, trade *store.Trade, brokerType string) {
	q.persistTrade(ctx, trade, brokerType)
}

func (q *Queue) finalizeTrade(ctx context.Context, trade *store.Trade, brokerType string) {
	if trade == nil {
		return
	}
	metrics.RecordTrade(trade.Status, trade.ErrorCode, brokerType)
	if err := q.pg.FinalizeTrade(ctx, trade); err != nil {
		log.Printf("worker: trade finalize failed id=%s: %v — attempting persist", trade.ID, err)
		if err := q.pg.PersistTradeAudit(ctx, trade); err != nil {
			log.Printf("worker: CRITICAL trade audit persist failed id=%s webhook=%s: %v", trade.ID, trade.WebhookID, err)
			return
		}
	}
	if trade.UserID != "" {
		go q.dispatchAlerts(context.Background(), trade)
	}
	if q.notifier != nil {
		q.notifier.OnTradeInserted(ctx, trade)
	}
}
