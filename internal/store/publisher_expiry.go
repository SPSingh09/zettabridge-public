package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	// PublisherHandoffExpiredCode is persisted on trades when a Kite basket
	// handoff expires before the user submits it on Kite.
	PublisherHandoffExpiredCode = "publisher_handoff_expired"
	// PublisherHandoffExpiredMsg is the user-facing rejection reason.
	PublisherHandoffExpiredMsg = "Kite basket handoff expired before submission (30 minute limit)."
)

// PublisherOrderIsTerminal reports whether the publisher order reached a final state.
func PublisherOrderIsTerminal(status string) bool {
	switch status {
	case "user_returned", "user_cancelled", "expired":
		return true
	default:
		return false
	}
}

// PublisherOrderIsExpired reports whether a handoff can no longer be submitted on Kite.
func PublisherOrderIsExpired(o *PublisherOrder, now time.Time) bool {
	if o == nil {
		return true
	}
	if o.Status == "expired" {
		return true
	}
	return !o.ExpiresAt.After(now)
}

// ExpirePublisherOrderIfStale marks a single publisher order expired when its
// deadline has passed. Returns true when the order is (now) expired.
func (p *PGStore) ExpirePublisherOrderIfStale(ctx context.Context, orderID string) (bool, error) {
	order, err := p.GetPublisherOrder(ctx, orderID)
	if err != nil {
		return false, err
	}
	if order == nil {
		return false, nil
	}
	if PublisherOrderIsTerminal(order.Status) {
		if order.Status == "expired" && order.TradeID != nil && *order.TradeID != "" {
			if err := p.rejectExpiredPublisherTradeIfPending(ctx, *order.TradeID); err != nil {
				return false, err
			}
		}
		return order.Status == "expired", nil
	}
	if order.ExpiresAt.After(time.Now().UTC()) {
		return false, nil
	}
	if _, err := p.expirePublisherOrder(ctx, order); err != nil {
		return false, err
	}
	return true, nil
}

// ExpireStalePublisherOrders marks every overdue non-terminal publisher order as
// expired and rejects linked trades still awaiting confirmation.
func (p *PGStore) ExpireStalePublisherOrders(ctx context.Context) ([]string, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, user_id, credential_id, webhook_id, trade_id,
		       broker, execution_mode,
		       basket_payload_json, basket_payload_hash,
		       status, callback_status, kite_request_token,
		       signed_state, redirected_at, callback_at, expires_at, created_at, updated_at
		FROM publisher_orders
		WHERE expires_at <= NOW()
		  AND status NOT IN ('user_returned', 'user_cancelled', 'expired')`)
	if err != nil {
		return nil, fmt.Errorf("store: list stale publisher orders: %w", err)
	}
	defer rows.Close()

	var tradeIDs []string
	for rows.Next() {
		order := &PublisherOrder{}
		if err := rows.Scan(
			&order.ID, &order.UserID, &order.CredentialID, &order.WebhookID, &order.TradeID,
			&order.Broker, &order.ExecutionMode,
			&order.BasketPayloadJSON, &order.BasketPayloadHash,
			&order.Status, &order.CallbackStatus, &order.KiteRequestToken,
			&order.SignedState, &order.RedirectedAt, &order.CallbackAt, &order.ExpiresAt, &order.CreatedAt, &order.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan stale publisher order: %w", err)
		}
		tradeID, err := p.expirePublisherOrder(ctx, order)
		if err != nil {
			return tradeIDs, err
		}
		if tradeID != "" {
			tradeIDs = append(tradeIDs, tradeID)
		}
	}
	return tradeIDs, rows.Err()
}

func (p *PGStore) expirePublisherOrder(ctx context.Context, order *PublisherOrder) (tradeID string, err error) {
	if order == nil || PublisherOrderIsTerminal(order.Status) {
		return "", nil
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("store: begin expire publisher order tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE publisher_orders
		SET status = 'expired', updated_at = NOW()
		WHERE id = $1
		  AND status NOT IN ('user_returned', 'user_cancelled', 'expired')`,
		order.ID,
	)
	if err != nil {
		return "", fmt.Errorf("store: expire publisher order %s: %w", order.ID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("store: expire publisher order rows affected: %w", err)
	}
	if affected == 0 {
		return "", tx.Commit()
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO publisher_order_events (id, publisher_order_id, event_type, event_payload, created_at)
		VALUES ($1, $2, $3, NULL, $4)`,
		uuid.New().String(), order.ID, "HANDOFF_EXPIRED", now,
	); err != nil {
		return "", fmt.Errorf("store: insert publisher handoff expired event: %w", err)
	}

	if order.TradeID != nil && *order.TradeID != "" {
		if err := p.rejectExpiredPublisherTradeIfPendingTx(ctx, tx, *order.TradeID); err != nil {
			return "", fmt.Errorf("store: reject expired publisher trade %s: %w", *order.TradeID, err)
		}
		tradeID = *order.TradeID
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("store: commit expire publisher order: %w", err)
	}
	return tradeID, nil
}

func (p *PGStore) rejectExpiredPublisherTradeIfPending(ctx context.Context, tradeID string) error {
	_, err := p.db.ExecContext(ctx, `
		UPDATE trades
		SET status = 'rejected',
		    error = $2,
		    error_code = $3,
		    updated_at = NOW(),
		    broker_responded_at = COALESCE(broker_responded_at, NOW())
		WHERE id = $1
		  AND status = 'pending_confirmation'`,
		tradeID, PublisherHandoffExpiredMsg, PublisherHandoffExpiredCode,
	)
	return err
}

func (p *PGStore) rejectExpiredPublisherTradeIfPendingTx(ctx context.Context, tx *sql.Tx, tradeID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE trades
		SET status = 'rejected',
		    error = $2,
		    error_code = $3,
		    updated_at = NOW(),
		    broker_responded_at = COALESCE(broker_responded_at, NOW())
		WHERE id = $1
		  AND status = 'pending_confirmation'`,
		tradeID, PublisherHandoffExpiredMsg, PublisherHandoffExpiredCode,
	)
	return err
}

// GetTradeByIDWithPublisherMeta returns a trade plus publisher handoff expiry when linked.
func (p *PGStore) GetTradeByIDWithPublisherMeta(ctx context.Context, id string) (*Trade, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT t.id, COALESCE(t.user_id,''), t.webhook_id, t.webhook_label, t.signal, t.symbol, t.lot_size,
		       t.broker_order, t.signal_key, t.comment, t.algo_id, t.status, t.fill_price, t.order_type, t.product,
		       t.sl_price, t.tp_price, t.error, t.error_code, t.cancelled_at, t.cancel_error, t.created_at,
		       po.expires_at
		FROM trades t
		LEFT JOIN publisher_orders po ON po.trade_id = t.id
		WHERE t.id = $1`, id)
	var t Trade
	var expiresAt sql.NullTime
	if err := row.Scan(&t.ID, &t.UserID, &t.WebhookID, &t.WebhookLabel, &t.Signal, &t.Symbol, &t.LotSize,
		&t.BrokerOrder, &t.SignalKey, &t.Comment, &t.AlgoID, &t.Status, &t.FillPrice,
		&t.OrderType, &t.Product, &t.SLPrice, &t.TPPrice,
		&t.Error, &t.ErrorCode, &t.CancelledAt, &t.CancelError, &t.CreatedAt,
		&expiresAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if expiresAt.Valid {
		t.PublisherOrderExpiresAt = &expiresAt.Time
	}
	return &t, nil
}
