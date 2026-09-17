package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type UserAuditEntry struct {
	ID        string    `db:"id"         json:"id"`
	UserID    string    `db:"user_id"    json:"user_id"`
	Action    string    `db:"action"     json:"action"`
	IP        string    `db:"ip"         json:"ip,omitempty"`
	Metadata  string    `db:"metadata"   json:"metadata,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

func (p *PGStore) InsertUserAudit(ctx context.Context, userID, action, ip string, metadata map[string]any) {
	metaJSON := "{}"
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			metaJSON = string(b)
		}
	}
	var ipVal any
	if ip != "" {
		ipVal = ip
	}
	_, _ = p.db.ExecContext(ctx,
		`INSERT INTO user_audit_log (id, user_id, action, ip, metadata, created_at)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
		uuid.New().String(), userID, action, ipVal, metaJSON, time.Now().UTC())
}

func (p *PGStore) ListUserAudit(ctx context.Context, userID string, limit int) ([]*UserAuditEntry, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, user_id, action, COALESCE(ip, ''), COALESCE(metadata::text, '{}'), created_at
		 FROM user_audit_log WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*UserAuditEntry
	for rows.Next() {
		var e UserAuditEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.IP, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
