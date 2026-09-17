package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/SPSingh09/zettabridge/internal/domain"
)

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (p *PGStore) CountPendingOrgInvites(ctx context.Context, orgID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM org_invites
		 WHERE org_id=$1 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > NOW()`,
		orgID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountSeatsUsed(ctx context.Context, orgID string) (int, error) {
	members, err := p.CountActiveOrgMembers(ctx, orgID)
	if err != nil {
		return 0, err
	}
	pending, err := p.CountPendingOrgInvites(ctx, orgID)
	if err != nil {
		return 0, err
	}
	return members + pending, nil
}

func (p *PGStore) CreateOrgInvite(ctx context.Context, inv *OrgInvite) error {
	inv.Email = normalizeEmail(inv.Email)
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO org_invites (id, org_id, email, role, token_hash, expires_at, invited_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		inv.ID, inv.OrgID, inv.Email, inv.Role, inv.TokenHash, inv.ExpiresAt, inv.InvitedBy, inv.CreatedAt)
	return err
}

func (p *PGStore) GetOrgInviteByID(ctx context.Context, orgID, inviteID string) (*OrgInvite, error) {
	var inv OrgInvite
	var acceptedAt, revokedAt sql.NullTime
	err := p.db.QueryRowContext(ctx,
		`SELECT id, org_id, email, role, token_hash, expires_at, accepted_at, revoked_at, invited_by, created_at
		 FROM org_invites WHERE id=$1 AND org_id=$2`, inviteID, orgID,
	).Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.ExpiresAt,
		&acceptedAt, &revokedAt, &inv.InvitedBy, &inv.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if acceptedAt.Valid {
		inv.AcceptedAt = &acceptedAt.Time
	}
	if revokedAt.Valid {
		inv.RevokedAt = &revokedAt.Time
	}
	return &inv, err
}

func (p *PGStore) GetPendingOrgInviteByTokenHash(ctx context.Context, tokenHash string) (*OrgInvite, error) {
	var inv OrgInvite
	var acceptedAt, revokedAt sql.NullTime
	err := p.db.QueryRowContext(ctx,
		`SELECT id, org_id, email, role, token_hash, expires_at, accepted_at, revoked_at, invited_by, created_at
		 FROM org_invites WHERE token_hash=$1`, tokenHash,
	).Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.ExpiresAt,
		&acceptedAt, &revokedAt, &inv.InvitedBy, &inv.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if acceptedAt.Valid {
		inv.AcceptedAt = &acceptedAt.Time
	}
	if revokedAt.Valid {
		inv.RevokedAt = &revokedAt.Time
	}
	return &inv, err
}

func (p *PGStore) ListPendingOrgInvites(ctx context.Context, orgID string) ([]*OrgInvite, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, org_id, email, role, token_hash, expires_at, accepted_at, revoked_at, invited_by, created_at
		 FROM org_invites
		 WHERE org_id=$1 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > NOW()
		 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*OrgInvite
	for rows.Next() {
		var inv OrgInvite
		var acceptedAt, revokedAt sql.NullTime
		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.ExpiresAt,
			&acceptedAt, &revokedAt, &inv.InvitedBy, &inv.CreatedAt); err != nil {
			return nil, err
		}
		if acceptedAt.Valid {
			inv.AcceptedAt = &acceptedAt.Time
		}
		if revokedAt.Valid {
			inv.RevokedAt = &revokedAt.Time
		}
		out = append(out, &inv)
	}
	return out, rows.Err()
}

func (p *PGStore) RevokeOrgInvite(ctx context.Context, orgID, inviteID string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE org_invites SET revoked_at=NOW()
		 WHERE id=$1 AND org_id=$2 AND accepted_at IS NULL AND revoked_at IS NULL`,
		inviteID, orgID)
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

// AcceptOrgInvite atomically creates membership (and user if needed), marks invite accepted,
// and pauses the member's personal webhooks.
func (p *PGStore) AcceptOrgInvite(ctx context.Context, inviteID string, user *User, member *OrgMember) ([]string, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var inv OrgInvite
	var acceptedAt, revokedAt sql.NullTime
	err = tx.QueryRowContext(ctx,
		`SELECT id, org_id, email, role, token_hash, expires_at, accepted_at, revoked_at, invited_by, created_at
		 FROM org_invites WHERE id=$1 FOR UPDATE`, inviteID,
	).Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.ExpiresAt,
		&acceptedAt, &revokedAt, &inv.InvitedBy, &inv.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	if acceptedAt.Valid || revokedAt.Valid || time.Now().After(inv.ExpiresAt) {
		return nil, fmt.Errorf("invite no longer valid")
	}

	var orgStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM organizations WHERE id=$1`, inv.OrgID,
	).Scan(&orgStatus)
	if err != nil {
		return nil, err
	}
	if orgStatus != domain.StatusActive {
		return nil, fmt.Errorf("organization is not active")
	}

	var seatLimit int
	var memberCount int
	err = tx.QueryRowContext(ctx,
		`SELECT seat_limit FROM organizations WHERE id=$1`, inv.OrgID,
	).Scan(&seatLimit)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM org_members WHERE org_id=$1 AND status='active'`, inv.OrgID,
	).Scan(&memberCount)
	if err != nil {
		return nil, err
	}
	if memberCount >= seatLimit {
		return nil, fmt.Errorf("seat limit reached")
	}

	var existingOrg string
	err = tx.QueryRowContext(ctx,
		`SELECT org_id FROM org_members WHERE user_id=$1`, user.ID,
	).Scan(&existingOrg)
	if err == nil {
		return nil, fmt.Errorf("user already belongs to an organization")
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	var existingMember string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM org_members WHERE org_id=$1 AND user_id=$2`, inv.OrgID, user.ID,
	).Scan(&existingMember)
	if err == nil {
		return nil, fmt.Errorf("already a member")
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	if user.Email != inv.Email {
		return nil, fmt.Errorf("email mismatch")
	}

	// Create user if new (ID pre-generated by caller).
	var exists string
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1`, user.ID).Scan(&exists)
	if err == sql.ErrNoRows {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO users (id, email, password_hash, plan, role, status, orgs_enabled, early_bird_live, live_orders_used, created_at, email_verified_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())`,
			user.ID, user.Email, user.PasswordHash, user.Plan, user.Role, user.Status, user.OrgsEnabled, user.EarlyBirdLive, user.LiveOrdersUsed, user.CreatedAt)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		if _, err = tx.ExecContext(ctx,
			`UPDATE users SET email_verified_at=COALESCE(email_verified_at, NOW()) WHERE id=$1`, user.ID); err != nil {
			return nil, err
		}
	}

	member.OrgID = inv.OrgID
	member.Role = inv.Role
	_, err = tx.ExecContext(ctx,
		`INSERT INTO org_members (id, org_id, user_id, role, status, joined_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		member.ID, member.OrgID, member.UserID, member.Role, member.Status, member.JoinedAt)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE org_invites SET accepted_at=NOW() WHERE id=$1`, inviteID)
	if err != nil {
		return nil, err
	}

	tokens, err := pausePersonalWebhooksTx(ctx, tx, user.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tokens, nil
}
