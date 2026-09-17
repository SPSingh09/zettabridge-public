package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/SPSingh09/zettabridge/internal/domain"
)

func (p *PGStore) CountOrgWebhooks(ctx context.Context, orgID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE org_id=$1`, orgID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountOrgCredentials(ctx context.Context, orgID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM broker_credentials WHERE org_id=$1`, orgID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountOrgTradesSince(ctx context.Context, orgID string, since time.Time) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM trades t
		 INNER JOIN webhooks w ON w.id = t.webhook_id
		 WHERE w.org_id=$1 AND t.created_at >= $2`, orgID, since,
	).Scan(&count)
	return count, err
}

func (p *PGStore) GetOrgUsage(ctx context.Context, orgID string) (*OrgUsage, error) {
	org, err := p.GetOrgByID(ctx, orgID)
	if err != nil || org == nil {
		return nil, err
	}
	members, err := p.CountActiveOrgMembers(ctx, orgID)
	if err != nil {
		return nil, err
	}
	pending, err := p.CountPendingOrgInvites(ctx, orgID)
	if err != nil {
		return nil, err
	}
	seatsUsed, err := p.CountSeatsUsed(ctx, orgID)
	if err != nil {
		return nil, err
	}
	webhooks, err := p.CountOrgWebhooks(ctx, orgID)
	if err != nil {
		return nil, err
	}
	creds, err := p.CountOrgCredentials(ctx, orgID)
	if err != nil {
		return nil, err
	}
	trades30, err := p.CountOrgTradesSince(ctx, orgID, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		return nil, err
	}
	trades24, err := p.CountOrgTradesSince(ctx, orgID, time.Now().Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	return &OrgUsage{
		SeatLimit:         org.SeatLimit,
		SeatsUsed:         seatsUsed,
		MemberCount:       members,
		PendingInvites:    pending,
		WebhookCount:      webhooks,
		CredentialCount:   creds,
		TradesLast30Days:  trades30,
		TradesLast24Hours: trades24,
	}, nil
}

func (p *PGStore) TransferOrgOwnership(ctx context.Context, orgID, fromUserID, toUserID string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var orgOwner string
	err = tx.QueryRowContext(ctx,
		`SELECT owner_user_id FROM organizations WHERE id=$1 AND status='active' FOR UPDATE`, orgID,
	).Scan(&orgOwner)
	if err == sql.ErrNoRows {
		return sql.ErrNoRows
	}
	if err != nil {
		return err
	}
	if orgOwner != fromUserID {
		return fmt.Errorf("not organization owner")
	}

	var targetRole string
	err = tx.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2 AND status='active'`,
		orgID, toUserID,
	).Scan(&targetRole)
	if err == sql.ErrNoRows {
		return fmt.Errorf("target not member")
	}
	if err != nil {
		return err
	}
	if targetRole == domain.RoleOwner {
		return fmt.Errorf("target already owner")
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE organizations SET owner_user_id=$1 WHERE id=$2`, toUserID, orgID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE org_members SET role='admin' WHERE org_id=$1 AND user_id=$2`, orgID, fromUserID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE org_members SET role='owner' WHERE org_id=$1 AND user_id=$2`, orgID, toUserID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (p *PGStore) LeaveOrg(ctx context.Context, orgID, userID string) error {
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

func (p *PGStore) UpdateOrgStatus(ctx context.Context, orgID, status string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE organizations SET status=$1 WHERE id=$2`, status, orgID)
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

func (p *PGStore) UpdateOrgSeatLimit(ctx context.Context, orgID string, seatLimit int) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE organizations SET seat_limit=$1 WHERE id=$2`, seatLimit, orgID)
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

func (p *PGStore) ListOrgsAdmin(ctx context.Context, f AdminOrgFilter) (*AdminOrgListResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 {
		f.Limit = 50
	}
	if f.Limit > 100 {
		f.Limit = 100
	}
	offset := (f.Page - 1) * f.Limit

	var conditions []string
	var args []any
	n := 1
	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("o.status = $%d", n))
		args = append(args, f.Status)
		n++
	}
	if f.Name != "" {
		conditions = append(conditions, fmt.Sprintf("o.name ILIKE $%d", n))
		args = append(args, "%"+f.Name+"%")
		n++
	}
	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := `SELECT COUNT(*) FROM organizations o ` + where
	var total int
	if err := p.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}

	listQuery := fmt.Sprintf(`
		SELECT o.id, o.name, o.slug, o.owner_user_id, o.seat_limit, o.status, o.created_at,
		       COALESCE(m.cnt, 0), COALESCE(w.cnt, 0), COALESCE(c.cnt, 0), COALESCE(i.cnt, 0)
		FROM organizations o
		LEFT JOIN (SELECT org_id, COUNT(*) AS cnt FROM org_members WHERE status='active' GROUP BY org_id) m ON m.org_id = o.id
		LEFT JOIN (SELECT org_id, COUNT(*) AS cnt FROM webhooks WHERE org_id IS NOT NULL GROUP BY org_id) w ON w.org_id = o.id
		LEFT JOIN (SELECT org_id, COUNT(*) AS cnt FROM broker_credentials WHERE org_id IS NOT NULL GROUP BY org_id) c ON c.org_id = o.id
		LEFT JOIN (
			SELECT org_id, COUNT(*) AS cnt FROM org_invites
			WHERE accepted_at IS NULL AND revoked_at IS NULL AND expires_at > NOW()
			GROUP BY org_id
		) i ON i.org_id = o.id
		%s
		ORDER BY o.created_at DESC
		LIMIT $%d OFFSET $%d`, where, n, n+1)

	listArgs := append(append([]any{}, args...), f.Limit, offset)
	rows, err := p.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []*AdminOrgSummary
	for rows.Next() {
		var item AdminOrgSummary
		var pending int
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Slug, &item.OwnerUserID, &item.SeatLimit, &item.Status, &item.CreatedAt,
			&item.MemberCount, &item.WebhookCount, &item.CredentialCount, &pending,
		); err != nil {
			return nil, err
		}
		item.SeatsUsed = item.MemberCount + pending
		orgs = append(orgs, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(f.Limit)))
	}
	return &AdminOrgListResult{
		Orgs:       orgs,
		Total:      total,
		Page:       f.Page,
		Limit:      f.Limit,
		TotalPages: totalPages,
	}, nil
}

func (p *PGStore) GetAdminOrgSummary(ctx context.Context, orgID string) (*AdminOrgSummary, error) {
	org, err := p.GetOrgByID(ctx, orgID)
	if err != nil || org == nil {
		return nil, err
	}
	summary := &AdminOrgSummary{Organization: *org}
	summary.MemberCount, err = p.CountActiveOrgMembers(ctx, orgID)
	if err != nil {
		return nil, err
	}
	summary.SeatsUsed, err = p.CountSeatsUsed(ctx, orgID)
	if err != nil {
		return nil, err
	}
	summary.WebhookCount, err = p.CountOrgWebhooks(ctx, orgID)
	if err != nil {
		return nil, err
	}
	summary.CredentialCount, err = p.CountOrgCredentials(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return summary, nil
}
