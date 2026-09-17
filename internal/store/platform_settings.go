package store

import (
	"context"
	"database/sql"
	"fmt"
)

// GetPlatformSetting returns a global admin-configurable setting's value.
// ok is false when no row exists for key (callers should fall back to a
// compiled-in/env default, not treat this as an error).
func (p *PGStore) GetPlatformSetting(ctx context.Context, key string) (value string, ok bool, err error) {
	err = p.db.QueryRowContext(ctx,
		`SELECT value FROM platform_settings WHERE key = $1`, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get platform setting: %w", err)
	}
	return value, true, nil
}

// SetPlatformSetting upserts a global admin-configurable setting.
func (p *PGStore) SetPlatformSetting(ctx context.Context, key, value, updatedBy string) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO platform_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (key) DO UPDATE SET value = $2, updated_by = $3, updated_at = now()`,
		key, value, updatedBy,
	)
	if err != nil {
		return fmt.Errorf("store: set platform setting: %w", err)
	}
	return nil
}
