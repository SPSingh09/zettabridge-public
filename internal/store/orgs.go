package store

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

var slugSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugSanitizer.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "org"
	}
	if len(s) > 48 {
		s = s[:48]
		s = strings.TrimRight(s, "-")
	}
	return s
}

// CreateOrgWithOwner inserts an organization and owner membership atomically.
// Personal webhooks for the owner are paused so trading is org-only.
func (p *PGStore) CreateOrgWithOwner(ctx context.Context, org *Organization, ownerMember *OrgMember) ([]string, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var existing string
	err = tx.QueryRowContext(ctx,
		`SELECT org_id FROM org_members WHERE user_id=$1`, ownerMember.UserID,
	).Scan(&existing)
	if err == nil {
		return nil, fmt.Errorf("user already belongs to an organization")
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	slug := org.Slug
	for i := 0; i < 100; i++ {
		candidate := slug
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", slug, i)
		}
		var taken string
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM organizations WHERE slug=$1`, candidate,
		).Scan(&taken)
		if err == sql.ErrNoRows {
			org.Slug = candidate
			break
		}
		if err != nil {
			return nil, err
		}
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO organizations (id, name, slug, owner_user_id, seat_limit, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		org.ID, org.Name, org.Slug, org.OwnerUserID, org.SeatLimit, org.Status, org.CreatedAt)
	if err != nil {
		return nil, err
	}

	ownerMember.OrgID = org.ID
	_, err = tx.ExecContext(ctx,
		`INSERT INTO org_members (id, org_id, user_id, role, status, joined_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		ownerMember.ID, ownerMember.OrgID, ownerMember.UserID, ownerMember.Role, ownerMember.Status, ownerMember.JoinedAt)
	if err != nil {
		return nil, err
	}

	tokens, err := pausePersonalWebhooksTx(ctx, tx, ownerMember.UserID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tokens, nil
}

// BuildOrgSlug returns a base slug from a display name.
func BuildOrgSlug(name string) string {
	return slugify(name)
}

func (p *PGStore) GetOrgByID(ctx context.Context, id string) (*Organization, error) {
	var o Organization
	err := p.db.QueryRowContext(ctx,
		`SELECT id, name, slug, owner_user_id, seat_limit, status, created_at
		 FROM organizations WHERE id=$1`, id,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.OwnerUserID, &o.SeatLimit, &o.Status, &o.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &o, err
}

func (p *PGStore) GetOrgByOwnerUserID(ctx context.Context, ownerUserID string) (*Organization, error) {
	var o Organization
	err := p.db.QueryRowContext(ctx,
		`SELECT id, name, slug, owner_user_id, seat_limit, status, created_at
		 FROM organizations WHERE owner_user_id=$1`, ownerUserID,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.OwnerUserID, &o.SeatLimit, &o.Status, &o.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &o, err
}

func (p *PGStore) ListOrgsByUserID(ctx context.Context, userID string) ([]*Organization, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT o.id, o.name, o.slug, o.owner_user_id, o.seat_limit, o.status, o.created_at
		 FROM organizations o
		 INNER JOIN org_members m ON m.org_id = o.id
		 WHERE m.user_id=$1 AND m.status='active'
		 ORDER BY o.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Organization
	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.OwnerUserID, &o.SeatLimit, &o.Status, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &o)
	}
	return out, rows.Err()
}

func (p *PGStore) GetOrgMembership(ctx context.Context, orgID, userID string) (*OrgMember, error) {
	var m OrgMember
	err := p.db.QueryRowContext(ctx,
		`SELECT id, org_id, user_id, role, status, joined_at
		 FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, userID,
	).Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.Status, &m.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &m, err
}

func (p *PGStore) GetActiveOrgMembershipByUser(ctx context.Context, userID string) (*OrgMember, error) {
	var m OrgMember
	err := p.db.QueryRowContext(ctx,
		`SELECT m.id, m.org_id, m.user_id, m.role, m.status, m.joined_at
		 FROM org_members m
		 INNER JOIN organizations o ON o.id = m.org_id
		 WHERE m.user_id=$1 AND m.status='active' AND o.status='active'`, userID,
	).Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.Status, &m.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &m, err
}

func (p *PGStore) CountActiveOrgMembers(ctx context.Context, orgID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM org_members WHERE org_id=$1 AND status='active'`, orgID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) UpdateOrgName(ctx context.Context, orgID, name string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE organizations SET name=$1 WHERE id=$2 AND status='active'`, name, orgID)
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

func (p *PGStore) DeleteOrg(ctx context.Context, orgID string) error {
	res, err := p.db.ExecContext(ctx,
		`DELETE FROM organizations WHERE id=$1`, orgID)
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
