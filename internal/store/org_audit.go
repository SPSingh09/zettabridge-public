package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

func (p *PGStore) InsertOrgAudit(ctx context.Context, orgID, actorUserID, action, targetUserID string, metadata map[string]any) error {
	metaJSON := "{}"
	if metadata != nil {
		b, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		metaJSON = string(b)
	}
	var target any
	if targetUserID != "" {
		target = targetUserID
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO org_audit_log (id, org_id, actor_user_id, action, target_user_id, metadata, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)`,
		uuid.New().String(), orgID, actorUserID, action, target, metaJSON, time.Now().UTC())
	return err
}

func (p *PGStore) ListOrgAudit(ctx context.Context, orgID string, limit int) ([]*OrgAuditEntry, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, org_id, actor_user_id, action, COALESCE(target_user_id, ''), metadata::text, created_at
		 FROM org_audit_log WHERE org_id=$1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*OrgAuditEntry
	for rows.Next() {
		var e OrgAuditEntry
		if err := rows.Scan(&e.ID, &e.OrgID, &e.ActorUserID, &e.Action, &e.TargetUserID, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		if e.TargetUserID == "" {
			e.TargetUserID = ""
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
