package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/SPSingh09/zettabridge/internal/platform/security/invitetoken"
	"github.com/google/uuid"
)

// CreatePlatformInvite generates a one-time invite token, stores only its hash, and
// returns the raw token to the caller (shown once, never persisted).
func (p *PGStore) CreatePlatformInvite(ctx context.Context, email, note, createdBy string, ttl time.Duration) (rawToken string, inv *PlatformInvite, err error) {
	raw, hash, err := invitetoken.Generate()
	if err != nil {
		return "", nil, err
	}
	inv = &PlatformInvite{
		ID:        uuid.New().String(),
		TokenHash: hash,
		Email:     normalizeEmail(email),
		Note:      note,
		ExpiresAt: time.Now().UTC().Add(ttl),
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
	}
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO platform_invites (id, token_hash, email, note, expires_at, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		inv.ID, inv.TokenHash, inv.Email, inv.Note, inv.ExpiresAt, inv.CreatedBy, inv.CreatedAt)
	if err != nil {
		return "", nil, err
	}
	return raw, inv, nil
}

// GetValidPlatformInviteByTokenHash looks up an invite by token hash and returns nil
// if the token does not exist, is expired, has been used, or has been revoked.
func (p *PGStore) GetValidPlatformInviteByTokenHash(ctx context.Context, tokenHash string) (*PlatformInvite, error) {
	var inv PlatformInvite
	var usedAt, revokedAt sql.NullTime
	var usedBy sql.NullString
	err := p.db.QueryRowContext(ctx,
		`SELECT id, token_hash, email, note, used_at, used_by, expires_at, revoked_at, created_by, created_at
		 FROM platform_invites WHERE token_hash=$1`, tokenHash,
	).Scan(&inv.ID, &inv.TokenHash, &inv.Email, &inv.Note,
		&usedAt, &usedBy, &inv.ExpiresAt, &revokedAt, &inv.CreatedBy, &inv.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if usedAt.Valid || revokedAt.Valid || time.Now().After(inv.ExpiresAt) {
		return nil, nil
	}
	if usedAt.Valid {
		inv.UsedAt = &usedAt.Time
	}
	if usedBy.Valid {
		inv.UsedBy = usedBy.String
	}
	if revokedAt.Valid {
		inv.RevokedAt = &revokedAt.Time
	}
	return &inv, nil
}

// UsePlatformInvite marks an invite as consumed by the given user.
func (p *PGStore) UsePlatformInvite(ctx context.Context, inviteID, userID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE platform_invites SET used_at=NOW(), used_by=$1 WHERE id=$2 AND used_at IS NULL`,
		userID, inviteID)
	return err
}

// ListPlatformInvites returns all platform invites ordered by creation time, newest first.
func (p *PGStore) ListPlatformInvites(ctx context.Context) ([]PlatformInviteRow, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT pi.id, pi.token_hash, pi.email, pi.note,
		        pi.used_at, pi.used_by, pi.expires_at, pi.revoked_at,
		        pi.created_by, pi.created_at,
		        COALESCE(u.email, '') AS used_by_email
		 FROM platform_invites pi
		 LEFT JOIN users u ON u.id = pi.used_by
		 ORDER BY pi.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlatformInviteRow
	for rows.Next() {
		var r PlatformInviteRow
		var usedAt, revokedAt sql.NullTime
		var usedBy sql.NullString
		if err := rows.Scan(
			&r.ID, &r.TokenHash, &r.Email, &r.Note,
			&usedAt, &usedBy, &r.ExpiresAt, &revokedAt,
			&r.CreatedBy, &r.CreatedAt, &r.UsedByEmail,
		); err != nil {
			return nil, err
		}
		if usedAt.Valid {
			r.UsedAt = &usedAt.Time
		}
		if usedBy.Valid {
			r.UsedBy = usedBy.String
		}
		if revokedAt.Valid {
			r.RevokedAt = &revokedAt.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RevokePlatformInvite marks an invite as revoked. Returns sql.ErrNoRows if not found
// or already revoked/used.
func (p *PGStore) RevokePlatformInvite(ctx context.Context, id string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE platform_invites SET revoked_at=NOW()
		 WHERE id=$1 AND revoked_at IS NULL AND used_at IS NULL`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
