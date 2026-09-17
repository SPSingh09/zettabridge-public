package store

import (
	"context"
	"database/sql"
)

// pausePersonalWebhooksTx pauses all solo webhooks for a user joining an org.
func pausePersonalWebhooksTx(ctx context.Context, tx *sql.Tx, userID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx,
		`UPDATE webhooks SET status='paused'
		 WHERE user_id=$1 AND org_id IS NULL AND status='active'
		 RETURNING token_hash`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

// PausePersonalWebhooksForUser pauses solo webhooks outside a transaction (e.g. admin repair).
func (p *PGStore) PausePersonalWebhooksForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := p.db.QueryContext(ctx,
		`UPDATE webhooks SET status='paused'
		 WHERE user_id=$1 AND org_id IS NULL AND status='active'
		 RETURNING token_hash`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}
