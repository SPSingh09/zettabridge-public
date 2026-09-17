package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const fyersConnectionID = "default"

// GetFyersConnection returns the single platform-wide FYERS connection row.
// Returns nil, nil if FYERS has never been connected.
func (p *PGStore) GetFyersConnection(ctx context.Context) (*FyersConnection, error) {
	c := &FyersConnection{}
	err := p.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(encrypted_access_token, ''), COALESCE(encrypted_refresh_token, ''), COALESCE(encrypted_pin, ''),
		       access_token_expires_at, refresh_token_expires_at, connected_by, connected_at, updated_at
		FROM fyers_connection WHERE id = $1`, fyersConnectionID,
	).Scan(
		&c.ID, &c.EncryptedAccessToken, &c.EncryptedRefreshToken, &c.EncryptedPIN,
		&c.AccessTokenExpiresAt, &c.RefreshTokenExpiresAt, &c.ConnectedBy, &c.ConnectedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get fyers connection: %w", err)
	}
	return c, nil
}

// UpsertFyersTokens stores a fresh access/refresh token pair after a
// successful OAuth login or silent refresh. connectedBy/connectedAt are only
// set on the initial login (pass "" / nil on a refresh to leave them
// unchanged) — see SQL COALESCE below.
func (p *PGStore) UpsertFyersTokens(ctx context.Context, accessToken, refreshToken string, accessExpiresAt, refreshExpiresAt time.Time, connectedBy string) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO fyers_connection (id, encrypted_access_token, encrypted_refresh_token,
			access_token_expires_at, refresh_token_expires_at, connected_by, connected_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), now(), now())
		ON CONFLICT (id) DO UPDATE SET
			encrypted_access_token = $2,
			encrypted_refresh_token = $3,
			access_token_expires_at = $4,
			refresh_token_expires_at = $5,
			connected_by = COALESCE(NULLIF($6, ''), fyers_connection.connected_by),
			connected_at = COALESCE(fyers_connection.connected_at, now()),
			updated_at = now()`,
		fyersConnectionID, accessToken, refreshToken, accessExpiresAt, refreshExpiresAt, connectedBy,
	)
	if err != nil {
		return fmt.Errorf("store: upsert fyers tokens: %w", err)
	}
	return nil
}

// SetFyersPIN stores the admin-entered FYERS trading PIN (encrypted),
// used only for the silent refresh-token flow.
func (p *PGStore) SetFyersPIN(ctx context.Context, encryptedPIN string) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO fyers_connection (id, encrypted_pin, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (id) DO UPDATE SET encrypted_pin = $2, updated_at = now()`,
		fyersConnectionID, encryptedPIN,
	)
	if err != nil {
		return fmt.Errorf("store: set fyers pin: %w", err)
	}
	return nil
}
