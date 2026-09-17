package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// InsertPublisherOrder inserts a new publisher_orders row.
func (p *PGStore) InsertPublisherOrder(ctx context.Context, o *PublisherOrder) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO publisher_orders (
			id, user_id, credential_id, webhook_id, trade_id,
			broker, execution_mode,
			basket_payload_json, basket_payload_hash,
			status, signed_state, expires_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7,
			$8, $9,
			$10, $11, $12, $13, $14
		)`,
		o.ID, o.UserID, o.CredentialID, o.WebhookID, o.TradeID,
		o.Broker, o.ExecutionMode,
		o.BasketPayloadJSON, o.BasketPayloadHash,
		o.Status, o.SignedState, o.ExpiresAt, o.CreatedAt, o.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("store: insert publisher order: %w", err)
	}
	return nil
}

// GetPublisherOrder returns a publisher order by ID.
// Returns nil, nil when not found.
func (p *PGStore) GetPublisherOrder(ctx context.Context, id string) (*PublisherOrder, error) {
	o := &PublisherOrder{}
	err := p.db.QueryRowContext(ctx, `
		SELECT id, user_id, credential_id, webhook_id, trade_id,
		       broker, execution_mode,
		       basket_payload_json, basket_payload_hash,
		       status, callback_status, kite_request_token,
		       signed_state, redirected_at, callback_at, expires_at, created_at, updated_at
		FROM publisher_orders WHERE id = $1`, id,
	).Scan(
		&o.ID, &o.UserID, &o.CredentialID, &o.WebhookID, &o.TradeID,
		&o.Broker, &o.ExecutionMode,
		&o.BasketPayloadJSON, &o.BasketPayloadHash,
		&o.Status, &o.CallbackStatus, &o.KiteRequestToken,
		&o.SignedState, &o.RedirectedAt, &o.CallbackAt, &o.ExpiresAt, &o.CreatedAt, &o.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get publisher order: %w", err)
	}
	return o, nil
}

// UpdatePublisherOrderCallback records the Kite callback result.
// internalStatus is ZettaBridge's internal status (user_returned | user_cancelled).
// kiteStatus is the raw status string received from Kite (success | cancelled).
func (p *PGStore) UpdatePublisherOrderCallback(ctx context.Context, id, internalStatus, kiteStatus string, requestToken *string, callbackAt time.Time) error {
	_, err := p.db.ExecContext(ctx, `
		UPDATE publisher_orders
		SET status = $2, callback_status = $3, kite_request_token = $4,
		    callback_at = $5, updated_at = NOW()
		WHERE id = $1`,
		id, internalStatus, kiteStatus, requestToken, callbackAt,
	)
	if err != nil {
		return fmt.Errorf("store: update publisher order callback: %w", err)
	}
	return nil
}

// GetMostRecentPendingPublisherOrder returns the most recently created publisher order
// that has not yet reached a terminal status and has not expired.
// Used as a fallback when Kite basket does not echo the state param in the callback.
func (p *PGStore) GetMostRecentPendingPublisherOrder(ctx context.Context) (*PublisherOrder, error) {
	o := &PublisherOrder{}
	err := p.db.QueryRowContext(ctx, `
		SELECT id, user_id, credential_id, webhook_id, trade_id,
		       broker, execution_mode,
		       basket_payload_json, basket_payload_hash,
		       status, callback_status, kite_request_token,
		       signed_state, redirected_at, callback_at, expires_at, created_at, updated_at
		FROM publisher_orders
		WHERE status NOT IN ('user_returned', 'user_cancelled', 'expired')
		  AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1`,
	).Scan(
		&o.ID, &o.UserID, &o.CredentialID, &o.WebhookID, &o.TradeID,
		&o.Broker, &o.ExecutionMode,
		&o.BasketPayloadJSON, &o.BasketPayloadHash,
		&o.Status, &o.CallbackStatus, &o.KiteRequestToken,
		&o.SignedState, &o.RedirectedAt, &o.CallbackAt, &o.ExpiresAt, &o.CreatedAt, &o.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get most recent pending publisher order: %w", err)
	}
	return o, nil
}

// UpdatePublisherTradeStatus transitions a publisher-mode trade from pending_confirmation
// to the given status. Only updates rows still in pending_confirmation to prevent double-processing.
func (p *PGStore) UpdatePublisherTradeStatus(ctx context.Context, tradeID, status string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE trades SET status = $2, updated_at = NOW() WHERE id = $1 AND status = 'pending_confirmation'`,
		tradeID, status,
	)
	if err != nil {
		return fmt.Errorf("store: update publisher trade status: %w", err)
	}
	return nil
}

// LinkPublisherOrderTrade sets trade_id on a publisher order after the trade row is created.
func (p *PGStore) LinkPublisherOrderTrade(ctx context.Context, publisherOrderID, tradeID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE publisher_orders SET trade_id = $2, updated_at = NOW() WHERE id = $1`,
		publisherOrderID, tradeID,
	)
	if err != nil {
		return fmt.Errorf("store: link publisher order trade: %w", err)
	}
	return nil
}

// InsertPublisherOrderEvent appends an audit event to a publisher order.
func (p *PGStore) InsertPublisherOrderEvent(ctx context.Context, e *PublisherOrderEvent) error {
	// pq mishandles nil []byte as empty bytea rather than SQL NULL for JSONB columns.
	// Use a nil interface so the driver sends a proper NULL.
	var payload interface{}
	if len(e.EventPayload) > 0 {
		payload = e.EventPayload
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO publisher_order_events (id, publisher_order_id, event_type, event_payload, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		e.ID, e.PublisherOrderID, e.EventType, payload, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("store: insert publisher order event: %w", err)
	}
	return nil
}
