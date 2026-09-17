package store

import (
	"context"
	"database/sql"
	"fmt"
)

const symbolRequestSelectCols = `id, user_id, market_profile_code, exchange, symbol, reason, status, admin_note, reviewed_by, reviewed_at, resolved_instrument_id, resolved_at, created_at, updated_at`

func scanSymbolRequest(scanner interface{ Scan(dest ...any) error }) (*SymbolRequest, error) {
	var r SymbolRequest
	if err := scanner.Scan(
		&r.ID, &r.UserID, &r.MarketProfileCode, &r.Exchange, &r.Symbol, &r.Reason, &r.Status, &r.AdminNote,
		&r.ReviewedBy, &r.ReviewedAt, &r.ResolvedInstrumentID, &r.ResolvedAt, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateSymbolRequest inserts a new pending symbol request.
func (p *PGStore) CreateSymbolRequest(ctx context.Context, r *SymbolRequest) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO symbol_requests (id, user_id, market_profile_code, exchange, symbol, reason, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		r.ID, r.UserID, r.MarketProfileCode, r.Exchange, r.Symbol, r.Reason, r.Status, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("store: create symbol request: %w", err)
	}
	return nil
}

// ListSymbolRequestsByUser returns a user's own requests, newest first.
func (p *PGStore) ListSymbolRequestsByUser(ctx context.Context, userID string) ([]*SymbolRequest, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT `+symbolRequestSelectCols+`
		FROM symbol_requests WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list symbol requests by user: %w", err)
	}
	defer rows.Close()

	var out []*SymbolRequest
	for rows.Next() {
		r, err := scanSymbolRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan symbol request: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListSymbolRequestsByStatus returns requests in the given status, newest
// first. Pass "" to list all statuses.
func (p *PGStore) ListSymbolRequestsByStatus(ctx context.Context, status string) ([]*SymbolRequest, error) {
	query := `SELECT ` + symbolRequestSelectCols + ` FROM symbol_requests`
	args := []any{}
	if status != "" {
		query += ` WHERE status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list symbol requests by status: %w", err)
	}
	defer rows.Close()

	var out []*SymbolRequest
	for rows.Next() {
		r, err := scanSymbolRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan symbol request: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetSymbolRequestByID returns nil, nil when not found.
func (p *PGStore) GetSymbolRequestByID(ctx context.Context, id string) (*SymbolRequest, error) {
	row := p.db.QueryRowContext(ctx, `SELECT `+symbolRequestSelectCols+` FROM symbol_requests WHERE id = $1`, id)
	r, err := scanSymbolRequest(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get symbol request by id: %w", err)
	}
	return r, nil
}

// AcceptSymbolRequest moves a pending request to accepted. Returns
// sql.ErrNoRows if the request doesn't exist or isn't currently pending.
func (p *PGStore) AcceptSymbolRequest(ctx context.Context, id, reviewerID string) (*SymbolRequest, error) {
	row := p.db.QueryRowContext(ctx, `
		UPDATE symbol_requests
		SET status = 'accepted', reviewed_by = $2, reviewed_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING `+symbolRequestSelectCols,
		id, reviewerID,
	)
	r, err := scanSymbolRequest(row)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("store: accept symbol request: %w", err)
	}
	return r, nil
}

// RejectSymbolRequest moves a pending or accepted request to rejected.
// Returns sql.ErrNoRows if the request doesn't exist or is already
// rejected/resolved.
func (p *PGStore) RejectSymbolRequest(ctx context.Context, id, reviewerID, adminNote string) (*SymbolRequest, error) {
	row := p.db.QueryRowContext(ctx, `
		UPDATE symbol_requests
		SET status = 'rejected', admin_note = $3, reviewed_by = $2, reviewed_at = now(), updated_at = now()
		WHERE id = $1 AND status IN ('pending', 'accepted')
		RETURNING `+symbolRequestSelectCols,
		id, reviewerID, adminNote,
	)
	r, err := scanSymbolRequest(row)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("store: reject symbol request: %w", err)
	}
	return r, nil
}

// ResolveSymbolRequestsByInstrument marks every pending/accepted request for
// marketProfileCode+symbol (case-insensitive) as resolved, linking them to
// the instrument that satisfied them. Called after any instrument is
// created — regardless of whether the admin arrived via the Accept banner
// or added the symbol independently — so this is the single source of
// truth for "resolved", not a frontend-driven transition.
func (p *PGStore) ResolveSymbolRequestsByInstrument(ctx context.Context, marketProfileCode, symbol, instrumentID string) ([]*SymbolRequest, error) {
	rows, err := p.db.QueryContext(ctx, `
		UPDATE symbol_requests
		SET status = 'resolved', resolved_instrument_id = $3, resolved_at = now(), updated_at = now()
		WHERE market_profile_code = $1 AND lower(symbol) = lower($2) AND status IN ('pending', 'accepted')
		RETURNING `+symbolRequestSelectCols,
		marketProfileCode, symbol, instrumentID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: resolve symbol requests by instrument: %w", err)
	}
	defer rows.Close()

	var out []*SymbolRequest
	for rows.Next() {
		r, err := scanSymbolRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan symbol request: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
