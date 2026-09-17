package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
)

const userColumns = `id, email, password_hash, plan, role, status, orgs_enabled, early_bird_live, live_orders_used, email_verified_at, stripe_customer_id, stripe_subscription_id, billing_source, telegram_chat_id, created_at`

func scanUser(row interface {
	Scan(dest ...any) error
}) (*User, error) {
	var u User
	var verifiedAt sql.NullTime
	var stripeCustomer, stripeSub, telegramChatID sql.NullString
	err := row.Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Plan, &u.Role, &u.Status,
		&u.OrgsEnabled, &u.EarlyBirdLive, &u.LiveOrdersUsed,
		&verifiedAt, &stripeCustomer, &stripeSub, &u.BillingSource, &telegramChatID, &u.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if verifiedAt.Valid {
		t := verifiedAt.Time
		u.EmailVerifiedAt = &t
	}
	if stripeCustomer.Valid {
		u.StripeCustomerID = stripeCustomer.String
	}
	if stripeSub.Valid {
		u.StripeSubscriptionID = stripeSub.String
	}
	if telegramChatID.Valid {
		u.TelegramChatID = telegramChatID.String
	}
	if u.BillingSource == "" {
		u.BillingSource = BillingSourceFree
	}
	return &u, nil
}

type AdminUserFilter struct {
	Plan   string
	Status string
	Email  string
	Page   int
	Limit  int
}

func (p *PGStore) CreateUser(ctx context.Context, u *User) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, plan, role, status, orgs_enabled, early_bird_live, live_orders_used, created_at, email_verified_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		u.ID, u.Email, u.PasswordHash, u.Plan, u.Role, u.Status, u.OrgsEnabled, u.EarlyBirdLive, u.LiveOrdersUsed, u.CreatedAt, u.EmailVerifiedAt)
	return err
}

func (p *PGStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := p.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE email=$1`, email)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (p *PGStore) GetUserByID(ctx context.Context, id string) (*User, error) {
	row := p.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1`, id)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// UpdateUserTelegramChatID sets or clears the Telegram chat ID.
// Pass an empty string to disconnect (stores NULL).
// DeleteUser removes the user and all their owned data.
// Orgs owned by this user are deleted first (no CASCADE on owner_user_id).
// All other user-owned rows have ON DELETE CASCADE so the user deletion handles them.
func (p *PGStore) DeleteUser(ctx context.Context, userID string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete orgs owned by this user (no CASCADE on owner_user_id FK).
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM organizations WHERE owner_user_id=$1`, userID); err != nil {
		return err
	}

	// Delete the user — remaining references cascade automatically.
	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}

	return tx.Commit()
}

func (p *PGStore) UpdateUserTelegramChatID(ctx context.Context, userID, chatID string) error {
	var val any
	if chatID != "" {
		val = chatID
	}
	_, err := p.db.ExecContext(ctx,
		`UPDATE users SET telegram_chat_id=$2 WHERE id=$1`, userID, val)
	return err
}

func (p *PGStore) PromoteBootstrapAdmin(ctx context.Context, email string) error {
	if email == "" {
		return nil
	}
	_, err := p.db.ExecContext(ctx,
		`UPDATE users SET role='admin' WHERE email=$1 AND role <> 'admin'`, email)
	return err
}

// CreateOrPromoteBootstrapAdmin creates the admin account if it does not exist, then promotes it.
// Used on startup so a fresh DB always has a working admin after migrations.
func (p *PGStore) CreateOrPromoteBootstrapAdmin(ctx context.Context, email, passwordHash string) error {
	if email == "" || passwordHash == "" {
		return nil
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, plan, role, status, orgs_enabled, early_bird_live, live_orders_used, created_at, email_verified_at)
		VALUES (gen_random_uuid()::text, $1, $2, 'free', 'admin', 'active', false, false, 0, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE SET role = 'admin'
	`, email, passwordHash)
	return err
}

func (p *PGStore) UpdateUserStatus(ctx context.Context, id, status string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET status=$1 WHERE id=$2`, status, id)
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

func (p *PGStore) UpdateUserOrgsEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET orgs_enabled=$1 WHERE id=$2`, enabled, id)
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

func (p *PGStore) UpdateUserPlan(ctx context.Context, id, planName string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET plan=$1 WHERE id=$2`, planName, id)
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

func (p *PGStore) UpdateUserEarlyBirdLive(ctx context.Context, id string, enabled bool) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET early_bird_live=$1 WHERE id=$2`, enabled, id)
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

func (p *PGStore) IncrementUserLiveOrdersUsed(ctx context.Context, userID string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET live_orders_used = live_orders_used + 1 WHERE id=$1`, userID)
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

func (p *PGStore) SyncOrgSeatLimitForOwner(ctx context.Context, ownerUserID string, seatLimit int) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE organizations SET seat_limit=$1 WHERE owner_user_id=$2`, seatLimit, ownerUserID)
	return err
}

func (p *PGStore) SetOrgStatusForOwner(ctx context.Context, ownerUserID, status string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE organizations SET status=$1 WHERE owner_user_id=$2`, status, ownerUserID)
	return err
}

func (p *PGStore) GetUserWithCounts(ctx context.Context, id string) (*UserWithCounts, error) {
	u, err := p.GetUserByID(ctx, id)
	if err != nil || u == nil {
		return nil, err
	}
	whCount, err := p.CountWebhooksByUser(ctx, id)
	if err != nil {
		return nil, err
	}
	brCount, err := p.CountBrokerCredsByUser(ctx, id)
	if err != nil {
		return nil, err
	}
	return &UserWithCounts{
		User:         *u,
		WebhookCount: whCount,
		BrokerCount:  brCount,
	}, nil
}

func (p *PGStore) ListUsersAdmin(ctx context.Context, f AdminUserFilter) (*AdminUserListResult, error) {
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

	if f.Plan != "" {
		conditions = append(conditions, fmt.Sprintf("plan = $%d", n))
		args = append(args, f.Plan)
		n++
	}
	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", n))
		args = append(args, f.Status)
		n++
	}
	if f.Email != "" {
		conditions = append(conditions, fmt.Sprintf("email ILIKE $%d", n))
		args = append(args, "%"+f.Email+"%")
		n++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := `SELECT COUNT(*) FROM users ` + where
	var total int
	if err := p.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}

	listQuery := fmt.Sprintf(`
		SELECT u.id, u.email, u.password_hash, u.plan, u.role, u.status, u.orgs_enabled, u.early_bird_live, u.live_orders_used, u.email_verified_at, u.stripe_customer_id, u.stripe_subscription_id, u.billing_source, u.created_at,
		       COALESCE(w.cnt, 0), COALESCE(b.cnt, 0)
		FROM users u
		LEFT JOIN (SELECT user_id, COUNT(*) AS cnt FROM webhooks GROUP BY user_id) w ON w.user_id = u.id
		LEFT JOIN (SELECT user_id, COUNT(*) AS cnt FROM broker_credentials GROUP BY user_id) b ON b.user_id = u.id
		%s
		ORDER BY u.created_at DESC
		LIMIT $%d OFFSET $%d`, where, n, n+1)

	listArgs := append(append([]any{}, args...), f.Limit, offset)
	rows, err := p.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*UserWithCounts
	for rows.Next() {
		var item UserWithCounts
		var verifiedAt sql.NullTime
		var stripeCustomer, stripeSub sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Email, &item.PasswordHash, &item.Plan, &item.Role, &item.Status, &item.OrgsEnabled, &item.EarlyBirdLive, &item.LiveOrdersUsed, &verifiedAt, &stripeCustomer, &stripeSub, &item.BillingSource, &item.CreatedAt,
			&item.WebhookCount, &item.BrokerCount,
		); err != nil {
			return nil, err
		}
		if verifiedAt.Valid {
			t := verifiedAt.Time
			item.EmailVerifiedAt = &t
		}
		if stripeCustomer.Valid {
			item.StripeCustomerID = stripeCustomer.String
		}
		if stripeSub.Valid {
			item.StripeSubscriptionID = stripeSub.String
		}
		if item.BillingSource == "" {
			item.BillingSource = BillingSourceFree
		}
		users = append(users, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(f.Limit)))
	}

	return &AdminUserListResult{
		Users:      users,
		Total:      total,
		Page:       f.Page,
		Limit:      f.Limit,
		TotalPages: totalPages,
	}, nil
}
