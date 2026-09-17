package store

import (
	"context"
	"database/sql"
)

func (p *PGStore) ListOrgMembers(ctx context.Context, orgID string) ([]*OrgMemberProfile, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT m.id, m.org_id, m.user_id, m.role, m.status, m.joined_at, u.email
		 FROM org_members m
		 INNER JOIN users u ON u.id = m.user_id
		 WHERE m.org_id=$1
		 ORDER BY m.joined_at ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*OrgMemberProfile
	for rows.Next() {
		var item OrgMemberProfile
		if err := rows.Scan(
			&item.ID, &item.OrgID, &item.UserID, &item.Role, &item.Status, &item.JoinedAt, &item.Email,
		); err != nil {
			return nil, err
		}
		out = append(out, &item)
	}
	return out, rows.Err()
}

func (p *PGStore) GetOrgMemberProfile(ctx context.Context, orgID, userID string) (*OrgMemberProfile, error) {
	var item OrgMemberProfile
	err := p.db.QueryRowContext(ctx,
		`SELECT m.id, m.org_id, m.user_id, m.role, m.status, m.joined_at, u.email
		 FROM org_members m
		 INNER JOIN users u ON u.id = m.user_id
		 WHERE m.org_id=$1 AND m.user_id=$2`, orgID, userID,
	).Scan(&item.ID, &item.OrgID, &item.UserID, &item.Role, &item.Status, &item.JoinedAt, &item.Email)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &item, err
}

func (p *PGStore) UpdateOrgMemberRole(ctx context.Context, orgID, userID, role string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE org_members SET role=$1 WHERE org_id=$2 AND user_id=$3 AND role <> 'owner'`,
		role, orgID, userID)
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

func (p *PGStore) UpdateOrgMemberStatus(ctx context.Context, orgID, userID, status string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE org_members SET status=$1 WHERE org_id=$2 AND user_id=$3 AND role <> 'owner'`,
		status, orgID, userID)
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

func (p *PGStore) RemoveOrgMember(ctx context.Context, orgID, userID string) error {
	res, err := p.db.ExecContext(ctx,
		`DELETE FROM org_members WHERE org_id=$1 AND user_id=$2 AND role <> 'owner'`,
		orgID, userID)
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

func (p *PGStore) UserInAnyOrg(ctx context.Context, userID string) (bool, error) {
	var orgID string
	err := p.db.QueryRowContext(ctx,
		`SELECT org_id FROM org_members WHERE user_id=$1`, userID,
	).Scan(&orgID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (p *PGStore) GetOrgMemberByEmail(ctx context.Context, orgID, email string) (*OrgMemberProfile, error) {
	var item OrgMemberProfile
	err := p.db.QueryRowContext(ctx,
		`SELECT m.id, m.org_id, m.user_id, m.role, m.status, m.joined_at, u.email
		 FROM org_members m
		 INNER JOIN users u ON u.id = m.user_id
		 WHERE m.org_id=$1 AND lower(u.email)=lower($2)`, orgID, email,
	).Scan(&item.ID, &item.OrgID, &item.UserID, &item.Role, &item.Status, &item.JoinedAt, &item.Email)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &item, err
}
