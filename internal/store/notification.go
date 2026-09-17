package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// InsertNotification is fire-and-forget — errors are silently dropped.
func (p *PGStore) InsertNotification(ctx context.Context, userID, typ, title, body, tradeID string) {
	var tradeVal any
	if tradeID != "" {
		tradeVal = tradeID
	}
	_, _ = p.db.ExecContext(ctx,
		`INSERT INTO notifications (id, user_id, type, title, body, trade_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		uuid.New().String(), userID, typ, title, body, tradeVal, time.Now().UTC())
}

func (p *PGStore) ListNotifications(ctx context.Context, userID string, limit int) ([]*Notification, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, user_id, type, title, body, COALESCE(trade_id, ''), read_at, created_at
		 FROM notifications WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.TradeID, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &n)
	}
	return out, rows.Err()
}

func (p *PGStore) CountUnreadNotifications(ctx context.Context, userID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL`, userID).Scan(&count)
	return count, err
}

func (p *PGStore) MarkNotificationRead(ctx context.Context, id, userID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE notifications SET read_at=NOW() WHERE id=$1 AND user_id=$2 AND read_at IS NULL`, id, userID)
	return err
}

func (p *PGStore) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE notifications SET read_at=NOW() WHERE user_id=$1 AND read_at IS NULL`, userID)
	return err
}
