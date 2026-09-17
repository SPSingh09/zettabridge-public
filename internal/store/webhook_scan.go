package store

import (
	"database/sql"
	"encoding/json"

	"github.com/lib/pq"
)

const webhookSelectCols = `id, user_id, org_id, created_by, token_hash, label, status, broker_cred_id, paper_account_id,
	symbol, lot_size, max_risk_pct, sl_points, tp_points, default_order_type, created_at,
	allowed_actions, allowed_symbols, max_lot_size,
	rate_limit_per_sec, rate_limit_per_min, dedup_window_sec, timezone, trading_hours, required_comment,
	auto_paused, admin_disabled`

func scanWebhook(scanner interface {
	Scan(dest ...any) error
}) (*Webhook, error) {
	var wh Webhook
	var orgID sql.NullString
	var createdBy sql.NullString
	var brokerCredID sql.NullString
	var paperAccountID sql.NullString
	var allowedActions pq.StringArray
	var allowedSymbols pq.StringArray
	var tradingHoursJSON []byte
	err := scanner.Scan(
		&wh.ID, &wh.UserID, &orgID, &createdBy, &wh.TokenHash, &wh.Label, &wh.Status,
		&brokerCredID, &paperAccountID, &wh.Symbol, &wh.LotSize, &wh.MaxRiskPct,
		&wh.SLPoints, &wh.TPPoints, &wh.DefaultOrderType, &wh.CreatedAt,
		&allowedActions, &allowedSymbols, &wh.MaxLotSize,
		&wh.RateLimitPerSec, &wh.RateLimitPerMin, &wh.DedupWindowSec, &wh.Timezone, &tradingHoursJSON, &wh.RequiredComment,
		&wh.AutoPaused, &wh.AdminDisabled,
	)
	if err != nil {
		return nil, err
	}
	if brokerCredID.Valid {
		wh.BrokerCredID = &brokerCredID.String
	}
	if paperAccountID.Valid {
		wh.PaperAccountID = &paperAccountID.String
	}
	if orgID.Valid {
		wh.OrgID = &orgID.String
	}
	if createdBy.Valid {
		wh.CreatedBy = createdBy.String
	} else {
		wh.CreatedBy = wh.UserID
	}
	if len(allowedActions) > 0 {
		wh.AllowedActions = []string(allowedActions)
	} else {
		wh.AllowedActions = []string{"BUY", "SELL", "CLOSE"}
	}
	wh.AllowedSymbols = []string(allowedSymbols)
	if len(tradingHoursJSON) == 0 || string(tradingHoursJSON) == "null" {
		wh.TradingHours = nil
	} else {
		var sched TradingSchedule
		if err := json.Unmarshal(tradingHoursJSON, &sched); err != nil {
			return nil, err
		}
		if len(sched) == 0 {
			wh.TradingHours = nil
		} else {
			wh.TradingHours = &sched
		}
	}
	return &wh, nil
}

const brokerCredSelectCols = `id, user_id, org_id, created_by, broker_type, encrypted_creds, account_label, account_mode, exchange, product, algo_id, order_type, market_protection, connected_at, status, auto_paused, execution_mode, admin_disabled`

func scanBrokerCredMeta(scanner interface {
	Scan(dest ...any) error
}) (*BrokerCredential, error) {
	var bc BrokerCredential
	var orgID sql.NullString
	var createdBy sql.NullString
	err := scanner.Scan(&bc.ID, &bc.UserID, &orgID, &createdBy, &bc.BrokerType, &bc.AccountLabel, &bc.AccountMode, &bc.Exchange, &bc.Product, &bc.AlgoID, &bc.OrderType, &bc.MarketProtection, &bc.ConnectedAt, &bc.Status, &bc.AutoPaused, &bc.ExecutionMode, &bc.AdminDisabled)
	if err != nil {
		return nil, err
	}
	if orgID.Valid {
		bc.OrgID = &orgID.String
	}
	if createdBy.Valid {
		bc.CreatedBy = createdBy.String
	} else {
		bc.CreatedBy = bc.UserID
	}
	return &bc, nil
}

func scanBrokerCredFull(scanner interface {
	Scan(dest ...any) error
}) (*BrokerCredential, error) {
	var bc BrokerCredential
	var orgID sql.NullString
	var createdBy sql.NullString
	err := scanner.Scan(&bc.ID, &bc.UserID, &orgID, &createdBy, &bc.BrokerType, &bc.EncryptedCreds, &bc.AccountLabel, &bc.AccountMode, &bc.Exchange, &bc.Product, &bc.AlgoID, &bc.OrderType, &bc.MarketProtection, &bc.ConnectedAt, &bc.Status, &bc.AutoPaused, &bc.ExecutionMode, &bc.AdminDisabled)
	if err != nil {
		return nil, err
	}
	if orgID.Valid {
		bc.OrgID = &orgID.String
	}
	if createdBy.Valid {
		bc.CreatedBy = createdBy.String
	} else {
		bc.CreatedBy = bc.UserID
	}
	return &bc, nil
}

// pqStringArray stores a TEXT[] column; nil slice must be {} not SQL NULL.
func pqStringArray(s []string) interface{} {
	if s == nil {
		return pq.Array([]string{})
	}
	return pq.Array(s)
}

func marshalTradingHours(sched *TradingSchedule) ([]byte, error) {
	if sched == nil || len(*sched) == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(sched)
}

func tradingHoursArg(sched *TradingSchedule) ([]byte, error) {
	return marshalTradingHours(sched)
}
