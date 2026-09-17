package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// InsertIngestLog is fire-and-forget — errors are silently dropped.
// outcome values: accepted | rejected | rate_limited | dedup | paused | queue_full
func (p *PGStore) InsertIngestLog(ctx context.Context, webhookID, requestID, outcome, errMsg, ip string) {
	var ipVal any
	if ip != "" {
		ipVal = ip
	}
	_, _ = p.db.ExecContext(ctx,
		`INSERT INTO ingest_log (id, webhook_id, request_id, outcome, error, ip, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		uuid.New().String(), webhookID, requestID, outcome, errMsg, ipVal, time.Now().UTC())
}
