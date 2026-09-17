package store

import (
	"context"
	"database/sql"
	"errors"
)

const (
	BillingSourceFree   = "free"
	BillingSourceStripe = "stripe"
	BillingSourceAdmin  = "admin"
)

// StripeWebhookEventExists reports whether a Stripe webhook event was already processed.
func (p *PGStore) StripeWebhookEventExists(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	err := p.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM stripe_webhook_events WHERE id=$1)`, eventID,
	).Scan(&exists)
	return exists, err
}

// RecordStripeWebhookEvent inserts an event id for idempotency. Returns false if already processed.
func (p *PGStore) RecordStripeWebhookEvent(ctx context.Context, eventID, eventType string) (bool, error) {
	res, err := p.db.ExecContext(ctx,
		`INSERT INTO stripe_webhook_events (id, type) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		eventID, eventType)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// UpdateUserBillingSource sets how the user's plan is managed (free, stripe, admin).
func (p *PGStore) UpdateUserBillingSource(ctx context.Context, userID, source string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET billing_source=$1 WHERE id=$2`, source, userID)
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

// SetUserStripeIDs stores Stripe customer and subscription ids for a user.
func (p *PGStore) SetUserStripeIDs(ctx context.Context, userID, customerID, subscriptionID string) error {
	var sub any
	if subscriptionID != "" {
		sub = subscriptionID
	}
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET stripe_customer_id=$1, stripe_subscription_id=$2 WHERE id=$3`,
		customerID, sub, userID)
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

// ClearUserStripeSubscription removes subscription id and optionally downgrades billing source.
func (p *PGStore) ClearUserStripeSubscription(ctx context.Context, userID string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET stripe_subscription_id=NULL WHERE id=$1`, userID)
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

// GetUserByStripeCustomerID looks up a user by Stripe customer id.
func (p *PGStore) GetUserByStripeCustomerID(ctx context.Context, customerID string) (*User, error) {
	if customerID == "" {
		return nil, nil
	}
	row := p.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE stripe_customer_id=$1`, customerID)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

// SetUserStripeCustomerID stores the Stripe customer id for checkout/portal reuse.
func (p *PGStore) SetUserStripeCustomerID(ctx context.Context, userID, customerID string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET stripe_customer_id=$1 WHERE id=$2`, customerID, userID)
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

// GetUserActiveOrgMembership returns the user's active org membership, if any.
func (p *PGStore) GetUserActiveOrgMembership(ctx context.Context, userID string) (*OrgMember, error) {
	var m OrgMember
	err := p.db.QueryRowContext(ctx,
		`SELECT id, org_id, user_id, role, status, joined_at
		 FROM org_members
		 WHERE user_id=$1 AND status='active'
		 LIMIT 1`, userID,
	).Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.Status, &m.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &m, err
}
