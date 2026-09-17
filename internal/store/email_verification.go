package store

import (
	"context"
	"database/sql"
	"time"
)

func (p *PGStore) CreateEmailVerification(ctx context.Context, v *EmailVerification) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO email_verifications (id, user_id, token_hash, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		v.ID, v.UserID, v.TokenHash, v.ExpiresAt, v.CreatedAt)
	return err
}

func (p *PGStore) InvalidatePendingEmailVerifications(ctx context.Context, userID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE email_verifications SET consumed_at=NOW()
		 WHERE user_id=$1 AND consumed_at IS NULL`, userID)
	return err
}

func (p *PGStore) GetPendingEmailVerificationByHash(ctx context.Context, tokenHash string) (*EmailVerification, error) {
	var v EmailVerification
	var consumed sql.NullTime
	err := p.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, expires_at, consumed_at, created_at
		 FROM email_verifications
		 WHERE token_hash=$1`, tokenHash,
	).Scan(&v.ID, &v.UserID, &v.TokenHash, &v.ExpiresAt, &consumed, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if consumed.Valid {
		t := consumed.Time
		v.ConsumedAt = &t
	}
	return &v, nil
}

func (p *PGStore) ConsumeEmailVerification(ctx context.Context, id, userID string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE email_verifications SET consumed_at=NOW()
		 WHERE id=$1 AND user_id=$2 AND consumed_at IS NULL AND expires_at > NOW()`, id, userID)
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

	res, err = tx.ExecContext(ctx,
		`UPDATE users SET email_verified_at=NOW() WHERE id=$1 AND email_verified_at IS NULL`, userID)
	if err != nil {
		return err
	}
	n, err = res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Already verified — still mark token consumed above.
		_, err = tx.ExecContext(ctx,
			`UPDATE users SET email_verified_at=COALESCE(email_verified_at, NOW()) WHERE id=$1`, userID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (p *PGStore) VerifyUserEmail(ctx context.Context, userID string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET email_verified_at=COALESCE(email_verified_at, NOW()) WHERE id=$1`, userID)
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

func (p *PGStore) VerifyUserEmailByAddress(ctx context.Context, email string) error {
	if email == "" {
		return nil
	}
	_, err := p.db.ExecContext(ctx,
		`UPDATE users SET email_verified_at=COALESCE(email_verified_at, NOW()) WHERE email=$1`, email)
	return err
}

func (p *PGStore) SetUserEmailVerified(ctx context.Context, userID string, verified bool) error {
	var verifiedAt interface{}
	if verified {
		verifiedAt = time.Now().UTC()
	}
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET email_verified_at=$1 WHERE id=$2`, verifiedAt, userID)
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
