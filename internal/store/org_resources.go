package store

import (
	"context"
	"database/sql"
	"log"
)

// ResourceScope identifies solo vs org resource ownership for queries.
type ResourceScope struct {
	UserID  string
	OrgID   string
	OrgRole string
	InOrg   bool
}

func (p *PGStore) ListWebhooksForScope(ctx context.Context, scope ResourceScope) ([]*Webhook, error) {
	var rows *sql.Rows
	var err error

	if scope.InOrg {
		rows, err = p.db.QueryContext(ctx,
			`SELECT `+webhookSelectCols+`
			 FROM webhooks
			 WHERE org_id=$1
			 ORDER BY created_at DESC`, scope.OrgID)
	} else {
		rows, err = p.db.QueryContext(ctx,
			`SELECT `+webhookSelectCols+`
			 FROM webhooks WHERE user_id=$1 AND org_id IS NULL
			 ORDER BY created_at DESC`, scope.UserID)
	}
	if err != nil {
		log.Printf("store: ListWebhooksForScope SQL error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var out []*Webhook
	for rows.Next() {
		wh, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wh)
	}
	return out, rows.Err()
}

func (p *PGStore) ListWebhooksByOrg(ctx context.Context, orgID string) ([]*Webhook, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT `+webhookSelectCols+`
		 FROM webhooks WHERE org_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Webhook
	for rows.Next() {
		wh, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wh)
	}
	return out, rows.Err()
}

func (p *PGStore) ListBrokerCredsForScope(ctx context.Context, scope ResourceScope) ([]*BrokerCredential, error) {
	var rows *sql.Rows
	var err error

	if scope.InOrg {
		rows, err = p.db.QueryContext(ctx,
			`SELECT id, user_id, org_id, created_by, broker_type, account_label, account_mode, exchange, product, algo_id, order_type, market_protection, connected_at, status, auto_paused, execution_mode, admin_disabled
			 FROM broker_credentials
			 WHERE org_id=$1
			 ORDER BY account_label ASC`, scope.OrgID)
	} else {
		rows, err = p.db.QueryContext(ctx,
			`SELECT id, user_id, org_id, created_by, broker_type, account_label, account_mode, exchange, product, algo_id, order_type, market_protection, connected_at, status, auto_paused, execution_mode, admin_disabled
			 FROM broker_credentials WHERE user_id=$1 AND org_id IS NULL
			 ORDER BY account_label ASC`, scope.UserID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*BrokerCredential
	for rows.Next() {
		bc, err := scanBrokerCredMeta(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, bc)
	}
	return out, rows.Err()
}

func (p *PGStore) ListTradesByOrg(ctx context.Context, orgID string, limit int) ([]*Trade, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT t.id, COALESCE(t.user_id,''), t.webhook_id, t.webhook_label, t.signal, t.symbol, t.lot_size, t.broker_order, t.signal_key, t.comment, t.algo_id, t.status, t.fill_price, t.order_type, t.product, t.sl_price, t.tp_price, t.error, t.error_code, t.cancelled_at, t.cancel_error, t.created_at
		 FROM trades t
		 INNER JOIN webhooks w ON w.id = t.webhook_id
		 WHERE w.org_id=$1
		 ORDER BY t.created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Trade
	for rows.Next() {
		var t Trade
		if err := rows.Scan(&t.ID, &t.UserID, &t.WebhookID, &t.WebhookLabel, &t.Signal, &t.Symbol, &t.LotSize,
			&t.BrokerOrder, &t.SignalKey, &t.Comment, &t.AlgoID, &t.Status, &t.FillPrice,
			&t.OrderType, &t.Product, &t.SLPrice, &t.TPPrice,
			&t.Error, &t.ErrorCode, &t.CancelledAt, &t.CancelError, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}
