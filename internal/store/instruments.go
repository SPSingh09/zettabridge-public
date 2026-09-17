package store

import (
	"context"
	"database/sql"
	"fmt"
)

const instrumentSelectCols = `id, market_profile_code, exchange, symbol, name, instrument_type, tick_size, lot_size, active, created_at, updated_at`

func scanInstrument(scanner interface{ Scan(dest ...any) error }) (*Instrument, error) {
	var i Instrument
	if err := scanner.Scan(&i.ID, &i.MarketProfileCode, &i.Exchange, &i.Symbol, &i.Name, &i.InstrumentType, &i.TickSize, &i.LotSize, &i.Active, &i.CreatedAt, &i.UpdatedAt); err != nil {
		return nil, err
	}
	return &i, nil
}

// ListInstruments returns instruments, optionally filtered by market profile
// code (pass "" for all profiles).
func (p *PGStore) ListInstruments(ctx context.Context, marketProfileCode string) ([]*Instrument, error) {
	query := `SELECT ` + instrumentSelectCols + ` FROM instruments`
	args := []any{}
	if marketProfileCode != "" {
		query += ` WHERE market_profile_code = $1`
		args = append(args, marketProfileCode)
	}
	query += ` ORDER BY active DESC, exchange, symbol`

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list instruments: %w", err)
	}
	defer rows.Close()

	var out []*Instrument
	for rows.Next() {
		i, err := scanInstrument(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan instrument: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// GetInstrumentByID returns nil, nil when not found.
func (p *PGStore) GetInstrumentByID(ctx context.Context, id string) (*Instrument, error) {
	row := p.db.QueryRowContext(ctx, `SELECT `+instrumentSelectCols+` FROM instruments WHERE id = $1`, id)
	i, err := scanInstrument(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get instrument by id: %w", err)
	}
	return i, nil
}

// GetInstrument returns nil, nil when not found. Symbol match is
// case-insensitive (signals may arrive in any case).
func (p *PGStore) GetInstrument(ctx context.Context, marketProfileCode, exchange, symbol string) (*Instrument, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT `+instrumentSelectCols+`
		FROM instruments
		WHERE market_profile_code = $1 AND exchange = $2 AND UPPER(symbol) = UPPER($3)`,
		marketProfileCode, exchange, symbol,
	)
	i, err := scanInstrument(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get instrument: %w", err)
	}
	return i, nil
}

// CreateInstrument inserts a new instrument.
func (p *PGStore) CreateInstrument(ctx context.Context, i *Instrument) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO instruments (id, market_profile_code, exchange, symbol, name, instrument_type, tick_size, lot_size, active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		i.ID, i.MarketProfileCode, i.Exchange, i.Symbol, i.Name, i.InstrumentType, i.TickSize, i.LotSize, i.Active, i.CreatedAt, i.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("store: create instrument: %w", err)
	}
	return nil
}

// UpdateInstrument updates the mutable fields of an instrument by ID.
// Returns sql.ErrNoRows if no such instrument exists.
func (p *PGStore) UpdateInstrument(ctx context.Context, i *Instrument) error {
	res, err := p.db.ExecContext(ctx, `
		UPDATE instruments
		SET name = $2, instrument_type = $3, tick_size = $4, lot_size = $5, active = $6, updated_at = now()
		WHERE id = $1`,
		i.ID, i.Name, i.InstrumentType, i.TickSize, i.LotSize, i.Active,
	)
	if err != nil {
		return fmt.Errorf("store: update instrument: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update instrument rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteInstrument permanently removes an instrument by ID.
func (p *PGStore) DeleteInstrument(ctx context.Context, id string) error {
	if _, err := p.db.ExecContext(ctx, `DELETE FROM instruments WHERE id = $1`, id); err != nil {
		return fmt.Errorf("store: delete instrument: %w", err)
	}
	return nil
}
