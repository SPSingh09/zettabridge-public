package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// PaperWebhookDiagnostics aggregates webhook-level signal-processing stats
// across every webhook routed to a paper account (there can be more than
// one — see ListWebhooksByPaperAccount). Counts come from ingest_log
// (every ingest attempt) and fill rate from paper_orders (simulated
// execution outcomes — paper webhooks never write to trades).
type PaperWebhookDiagnostics struct {
	SignalsReceived     int
	SignalsAccepted     int
	SignalsRejected     int
	DuplicateSuppressed int
	FillRate            *float64
	LastSignalAt        *time.Time
}

// GetPaperAccountWebhookDiagnostics returns zero-value diagnostics (not an
// error) when webhookIDs is empty — a paper account can exist with no
// webhook routed to it yet.
func (p *PGStore) GetPaperAccountWebhookDiagnostics(ctx context.Context, webhookIDs []string) (*PaperWebhookDiagnostics, error) {
	diag := &PaperWebhookDiagnostics{}
	if len(webhookIDs) == 0 {
		return diag, nil
	}
	ids := pq.Array(webhookIDs)

	var lastSignalAt sql.NullTime
	err := p.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::int,
		       COUNT(*) FILTER (WHERE outcome = 'accepted')::int,
		       COUNT(*) FILTER (WHERE outcome IN ('rejected', 'rate_limited', 'paused', 'queue_full'))::int,
		       COUNT(*) FILTER (WHERE outcome = 'dedup')::int,
		       MAX(created_at)
		FROM ingest_log WHERE webhook_id = ANY($1)`, ids,
	).Scan(&diag.SignalsReceived, &diag.SignalsAccepted, &diag.SignalsRejected,
		&diag.DuplicateSuppressed, &lastSignalAt)
	if err != nil {
		return nil, err
	}
	if lastSignalAt.Valid {
		diag.LastSignalAt = &lastSignalAt.Time
	}

	var filled, rejected int
	err = p.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FILTER (WHERE status = 'FILLED')::int,
		       COUNT(*) FILTER (WHERE status = 'REJECTED')::int
		FROM paper_orders WHERE webhook_id = ANY($1)`, ids,
	).Scan(&filled, &rejected)
	if err != nil {
		return nil, err
	}
	if filled+rejected > 0 {
		rate := float64(filled) / float64(filled+rejected)
		diag.FillRate = &rate
	}

	return diag, nil
}
