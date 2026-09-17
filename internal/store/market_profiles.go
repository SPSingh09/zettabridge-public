package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const marketProfileSelectCols = `id, code, name, base_currency, timezone, trading_days, active, slippage_bps, fee_bps, created_at, updated_at`

func scanMarketProfile(scanner interface{ Scan(dest ...any) error }) (*MarketProfile, error) {
	var mp MarketProfile
	var tradingDaysJSON []byte
	if err := scanner.Scan(
		&mp.ID, &mp.Code, &mp.Name, &mp.BaseCurrency, &mp.Timezone, &tradingDaysJSON, &mp.Active,
		&mp.SlippageBps, &mp.FeeBps, &mp.CreatedAt, &mp.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(tradingDaysJSON) == 0 || string(tradingDaysJSON) == "null" {
		mp.TradingDays = nil
	} else {
		var sched TradingSchedule
		if err := json.Unmarshal(tradingDaysJSON, &sched); err != nil {
			return nil, err
		}
		if len(sched) == 0 {
			mp.TradingDays = nil
		} else {
			mp.TradingDays = &sched
		}
	}
	return &mp, nil
}

// ListMarketProfiles returns every market profile, active first then by code.
func (p *PGStore) ListMarketProfiles(ctx context.Context) ([]*MarketProfile, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT `+marketProfileSelectCols+` FROM market_profiles ORDER BY active DESC, code`)
	if err != nil {
		return nil, fmt.Errorf("store: list market profiles: %w", err)
	}
	defer rows.Close()

	var out []*MarketProfile
	for rows.Next() {
		mp, err := scanMarketProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan market profile: %w", err)
		}
		out = append(out, mp)
	}
	return out, rows.Err()
}

// GetMarketProfileByCode returns nil, nil when not found.
func (p *PGStore) GetMarketProfileByCode(ctx context.Context, code string) (*MarketProfile, error) {
	row := p.db.QueryRowContext(ctx, `SELECT `+marketProfileSelectCols+` FROM market_profiles WHERE code = $1`, code)
	mp, err := scanMarketProfile(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get market profile: %w", err)
	}
	return mp, nil
}

// UpdateMarketProfile updates the mutable fields of a market profile by code.
// Market profiles themselves are seeded via migration (see 039/040), not
// created through the store/API — there is deliberately no CreateMarketProfile.
// Returns sql.ErrNoRows if no such profile exists.
func (p *PGStore) UpdateMarketProfile(ctx context.Context, mp *MarketProfile) error {
	tradingDays, err := marshalTradingHours(mp.TradingDays)
	if err != nil {
		return fmt.Errorf("store: marshal trading_days: %w", err)
	}
	res, err := p.db.ExecContext(ctx, `
		UPDATE market_profiles
		SET name = $2, base_currency = $3, timezone = $4, trading_days = $5, active = $6,
		    slippage_bps = $7, fee_bps = $8, updated_at = now()
		WHERE code = $1`,
		mp.Code, mp.Name, mp.BaseCurrency, mp.Timezone, tradingDays, mp.Active, mp.SlippageBps, mp.FeeBps,
	)
	if err != nil {
		return fmt.Errorf("store: update market profile: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update market profile rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
