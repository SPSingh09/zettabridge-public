package store

import (
	"context"
	"time"
)

// PendingFillTrade is a live trade awaiting Kite fill confirmation.
type PendingFillTrade struct {
	Trade        Trade
	BrokerCredID string
}

// ListPendingZerodhaOAuthFills returns submitted Zerodha OAuth trades with a broker order id.
func (p *PGStore) ListPendingZerodhaOAuthFills(ctx context.Context, maxAge time.Duration, limit int) ([]PendingFillTrade, error) {
	if limit <= 0 {
		limit = 500
	}
	if maxAge <= 0 {
		maxAge = 48 * time.Hour
	}
	cutoff := time.Now().Add(-maxAge)

	rows, err := p.db.QueryContext(ctx,
		`SELECT t.id, COALESCE(t.user_id,''), t.webhook_id, t.webhook_label, t.signal, t.symbol, t.lot_size,
		        t.broker_order, t.signal_key, t.comment, t.algo_id, t.status, t.fill_price, t.order_type, t.product,
		        t.sl_price, t.tp_price, t.error, t.error_code, t.cancelled_at, t.cancel_error, t.created_at,
		        bc.id
		 FROM trades t
		 JOIN webhooks w ON w.id = t.webhook_id
		 JOIN broker_credentials bc ON bc.id = w.broker_cred_id
		 WHERE t.status = 'submitted'
		   AND COALESCE(t.broker_order, '') <> ''
		   AND bc.broker_type = 'zerodha'
		   AND bc.execution_mode IN ('user_api_oauth', '')
		   AND bc.status = 'active'
		   AND t.created_at >= $1
		 ORDER BY t.created_at ASC
		 LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingFillTrade
	for rows.Next() {
		var item PendingFillTrade
		if err := rows.Scan(
			&item.Trade.ID, &item.Trade.UserID, &item.Trade.WebhookID, &item.Trade.WebhookLabel,
			&item.Trade.Signal, &item.Trade.Symbol, &item.Trade.LotSize, &item.Trade.BrokerOrder,
			&item.Trade.SignalKey, &item.Trade.Comment, &item.Trade.AlgoID, &item.Trade.Status,
			&item.Trade.FillPrice, &item.Trade.OrderType, &item.Trade.Product,
			&item.Trade.SLPrice, &item.Trade.TPPrice, &item.Trade.Error, &item.Trade.ErrorCode,
			&item.Trade.CancelledAt, &item.Trade.CancelError, &item.Trade.CreatedAt,
			&item.BrokerCredID,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// UpdateTradeFillOutcome updates status/fill_price and optionally rejection error for a polled fill.
func (p *PGStore) UpdateTradeFillOutcome(ctx context.Context, id, status string, fillPrice float64, errMsg string) error {
	if errMsg != "" {
		_, err := p.db.ExecContext(ctx,
			`UPDATE trades SET status=$2, fill_price=$3, error=$4, updated_at=NOW() WHERE id=$1 AND status='submitted'`,
			id, status, fillPrice, errMsg)
		return err
	}
	_, err := p.db.ExecContext(ctx,
		`UPDATE trades SET status=$2, fill_price=$3, updated_at=NOW() WHERE id=$1 AND status='submitted'`,
		id, status, fillPrice)
	return err
}
