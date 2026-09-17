package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

type PGStore struct {
	db *sql.DB
}

func NewPostgres(url string) *PGStore {
	db, err := sql.Open("postgres", url)
	if err != nil {
		panic(fmt.Sprintf("postgres: open failed: %v", err))
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		panic(fmt.Sprintf("postgres: ping failed: %v", err))
	}
	return &PGStore{db: db}
}

func (p *PGStore) Close() { p.db.Close() }

func (p *PGStore) DB() *sql.DB { return p.db }

// ── Webhooks ──────────────────────────────────────────────────────────────────

func (p *PGStore) CreateWebhook(ctx context.Context, wh *Webhook) error {
	thArg, err := tradingHoursArg(wh.TradingHours)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO webhooks
		   (id, user_id, org_id, created_by, token_hash, label, status, broker_cred_id, paper_account_id,
		    symbol, lot_size, max_risk_pct, sl_points, tp_points, default_order_type, created_at,
		    allowed_actions, allowed_symbols, max_lot_size,
		    rate_limit_per_sec, rate_limit_per_min, dedup_window_sec, timezone, trading_hours, required_comment)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
		         $16,$17,$18,$19,$20,$21,$22,$23,$24,$25)`,
		wh.ID, wh.UserID, wh.OrgID, wh.CreatedBy, wh.TokenHash, wh.Label, wh.Status, wh.BrokerCredID, wh.PaperAccountID,
		wh.Symbol, wh.LotSize, wh.MaxRiskPct, wh.SLPoints, wh.TPPoints, wh.DefaultOrderType, wh.CreatedAt,
		pqStringArray(wh.AllowedActions), pqStringArray(wh.AllowedSymbols), wh.MaxLotSize,
		wh.RateLimitPerSec, wh.RateLimitPerMin, wh.DedupWindowSec, wh.Timezone, thArg, wh.RequiredComment)
	if err != nil {
		log.Printf("store: CreateWebhook SQL error: %v", err)
	}
	return err
}

func (p *PGStore) GetWebhookByID(ctx context.Context, id string) (*Webhook, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT `+webhookSelectCols+` FROM webhooks WHERE id=$1`, id)
	wh, err := scanWebhook(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return wh, err
}

func (p *PGStore) GetWebhookByToken(ctx context.Context, token string) (*Webhook, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT `+webhookSelectCols+` FROM webhooks WHERE token_hash=$1`, token)
	wh, err := scanWebhook(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return wh, err
}

func (p *PGStore) ListWebhooksByUser(ctx context.Context, userID string) ([]*Webhook, error) {
	return p.ListWebhooksForScope(ctx, ResourceScope{UserID: userID, InOrg: false})
}

func (p *PGStore) UpdateWebhookStatus(ctx context.Context, id, status string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE webhooks SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (p *PGStore) ClearWebhookAutoPaused(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE webhooks SET auto_paused=false WHERE id=$1`, id)
	return err
}

// PauseAllWebhooksByUser pauses solo live-broker webhooks only. Paper-trading
// webhooks are excluded — Paper Trading is a separate, flat-rate tier and must
// not be affected by a live-trading plan downgrade.
func (p *PGStore) PauseAllWebhooksByUser(ctx context.Context, userID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE webhooks SET status='paused', auto_paused=true WHERE user_id=$1 AND org_id IS NULL AND broker_cred_id IS NOT NULL`,
		userID)
	return err
}

func (p *PGStore) ClearAutoPausedWebhooksByUser(ctx context.Context, userID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE webhooks SET auto_paused=false WHERE user_id=$1 AND org_id IS NULL`, userID)
	return err
}

func (p *PGStore) RotateWebhookToken(ctx context.Context, id, userID, newToken string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE webhooks SET token_hash=$1 WHERE id=$2 AND user_id=$3`,
		newToken, id, userID)
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

func (p *PGStore) UpdateWebhook(ctx context.Context, wh *Webhook) error {
	thArg, err := tradingHoursArg(wh.TradingHours)
	if err != nil {
		return err
	}
	res, err := p.db.ExecContext(ctx,
		`UPDATE webhooks
		    SET label=$1, broker_cred_id=$2, paper_account_id=$3, symbol=$4,
		        lot_size=$5, max_risk_pct=$6, sl_points=$7, tp_points=$8,
		        default_order_type=$9,
		        allowed_actions=$10, allowed_symbols=$11, max_lot_size=$12,
		        rate_limit_per_sec=$13, rate_limit_per_min=$14, dedup_window_sec=$15, timezone=$16, trading_hours=$17,
		        required_comment=$18
		  WHERE id=$19 AND user_id=$20`,
		wh.Label, wh.BrokerCredID, wh.PaperAccountID, wh.Symbol,
		wh.LotSize, wh.MaxRiskPct, wh.SLPoints, wh.TPPoints,
		wh.DefaultOrderType,
		pqStringArray(wh.AllowedActions), pqStringArray(wh.AllowedSymbols), wh.MaxLotSize,
		wh.RateLimitPerSec, wh.RateLimitPerMin, wh.DedupWindowSec, wh.Timezone, thArg, wh.RequiredComment,
		wh.ID, wh.UserID)
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

func (p *PGStore) DeleteWebhook(ctx context.Context, id string) error {
	// trades.webhook_id FK is ON DELETE SET NULL — trade rows survive for audit.
	res, err := p.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id=$1`, id)
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

// ── Broker Credentials ────────────────────────────────────────────────────────

func (p *PGStore) CreateBrokerCred(ctx context.Context, bc *BrokerCredential) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO broker_credentials
		   (id, user_id, org_id, created_by, broker_type, encrypted_creds, account_label, account_mode, exchange, product, algo_id, order_type, market_protection, execution_mode)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		bc.ID, bc.UserID, bc.OrgID, bc.CreatedBy, bc.BrokerType, bc.EncryptedCreds, bc.AccountLabel,
		bc.AccountMode, bc.Exchange, bc.Product, bc.AlgoID, bc.OrderType, bc.MarketProtection, bc.ExecutionMode)
	return err
}

func (p *PGStore) GetBrokerCred(ctx context.Context, id string) (*BrokerCredential, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT `+brokerCredSelectCols+` FROM broker_credentials WHERE id=$1`, id)
	bc, err := scanBrokerCredFull(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return bc, err
}

func (p *PGStore) ListBrokerCredsByUser(ctx context.Context, userID string) ([]*BrokerCredential, error) {
	return p.ListBrokerCredsForScope(ctx, ResourceScope{UserID: userID, InOrg: false})
}

func (p *PGStore) UpdateBrokerCred(ctx context.Context, bc *BrokerCredential) error {
	var res sql.Result
	var err error
	if bc.OrgID != nil {
		res, err = p.db.ExecContext(ctx,
			`UPDATE broker_credentials
			    SET broker_type=$1, encrypted_creds=$2, account_label=$3,
			        account_mode=$4, exchange=$5, product=$6, algo_id=$7,
			        order_type=$8, market_protection=$9, execution_mode=$10
			  WHERE id=$11 AND org_id=$12`,
			bc.BrokerType, bc.EncryptedCreds, bc.AccountLabel,
			bc.AccountMode, bc.Exchange, bc.Product, bc.AlgoID,
			bc.OrderType, bc.MarketProtection, bc.ExecutionMode, bc.ID, *bc.OrgID)
	} else {
		res, err = p.db.ExecContext(ctx,
			`UPDATE broker_credentials
			    SET broker_type=$1, encrypted_creds=$2, account_label=$3,
			        account_mode=$4, exchange=$5, product=$6, algo_id=$7,
			        order_type=$8, market_protection=$9, execution_mode=$10
			  WHERE id=$11 AND user_id=$12 AND org_id IS NULL`,
			bc.BrokerType, bc.EncryptedCreds, bc.AccountLabel,
			bc.AccountMode, bc.Exchange, bc.Product, bc.AlgoID,
			bc.OrderType, bc.MarketProtection, bc.ExecutionMode, bc.ID, bc.UserID)
	}
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

func (p *PGStore) UpdateBrokerCredStatus(ctx context.Context, id, status string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE broker_credentials SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (p *PGStore) ClearCredentialAutoPaused(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE broker_credentials SET auto_paused=false WHERE id=$1`, id)
	return err
}

func (p *PGStore) PauseAllCredentialsByUser(ctx context.Context, userID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE broker_credentials SET status='paused', auto_paused=true WHERE user_id=$1 AND org_id IS NULL`,
		userID)
	return err
}

func (p *PGStore) ClearAutoPausedCredentialsByUser(ctx context.Context, userID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE broker_credentials SET auto_paused=false WHERE user_id=$1 AND org_id IS NULL`, userID)
	return err
}

// PauseCredentialsByExecutionMode pauses every active credential for the given
// broker type + execution mode (across all users/orgs) when an admin disables
// that mode platform-wide, and marks them admin_disabled so users cannot
// manually resume them until the mode is re-enabled.
func (p *PGStore) PauseCredentialsByExecutionMode(ctx context.Context, brokerType, executionMode string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE broker_credentials SET status='paused', admin_disabled=true
		  WHERE broker_type=$1 AND execution_mode=$2 AND status='active'`,
		brokerType, executionMode)
	return err
}

// ClearAdminDisabledCredentials clears the admin_disabled flag for the given
// broker type + execution mode when an admin re-enables it. Credentials stay
// paused — matching the plan-downgrade pattern, the user must resume them
// manually once the block is lifted.
func (p *PGStore) ClearAdminDisabledCredentials(ctx context.Context, brokerType, executionMode string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE broker_credentials SET admin_disabled=false WHERE broker_type=$1 AND execution_mode=$2`,
		brokerType, executionMode)
	return err
}

// PauseWebhooksByExecutionMode pauses every active webhook whose linked
// broker credential uses the given broker type + execution mode.
func (p *PGStore) PauseWebhooksByExecutionMode(ctx context.Context, brokerType, executionMode string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE webhooks SET status='paused', admin_disabled=true
		  WHERE status='active' AND broker_cred_id IN (
		    SELECT id FROM broker_credentials WHERE broker_type=$1 AND execution_mode=$2
		  )`,
		brokerType, executionMode)
	return err
}

// ClearAdminDisabledWebhooks clears the admin_disabled flag on webhooks whose
// linked credential uses the given broker type + execution mode.
func (p *PGStore) ClearAdminDisabledWebhooks(ctx context.Context, brokerType, executionMode string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE webhooks SET admin_disabled=false
		  WHERE broker_cred_id IN (
		    SELECT id FROM broker_credentials WHERE broker_type=$1 AND execution_mode=$2
		  )`,
		brokerType, executionMode)
	return err
}

// UnpauseUserSoloResources re-activates solo webhooks and credentials that were
// auto-paused when the user joined an org via invite. Only touches rows with
// auto_paused=true so manually-paused resources are left alone.
func (p *PGStore) UnpauseUserSoloResources(ctx context.Context, userID string) error {
	if _, err := p.db.ExecContext(ctx,
		`UPDATE webhooks
		 SET status='active', auto_paused=false
		 WHERE user_id=$1 AND org_id IS NULL AND auto_paused=true`,
		userID); err != nil {
		return err
	}
	_, err := p.db.ExecContext(ctx,
		`UPDATE broker_credentials
		 SET status='active', auto_paused=false
		 WHERE user_id=$1 AND org_id IS NULL AND auto_paused=true`,
		userID)
	return err
}

// TouchConnectedAt sets connected_at = NOW() for a credential.
// Called after a successful Zerodha OAuth token exchange.
func (p *PGStore) TouchConnectedAt(ctx context.Context, credID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE broker_credentials SET connected_at = NOW() WHERE id = $1`, credID)
	return err
}

// ReconnectZerodhaToken atomically refreshes the encrypted token and marks
// connected_at = NOW() for an existing Zerodha credential.
// Ownership is verified upstream in zerodhaConnect (authenticated endpoint),
// so this only checks by credential id and broker_type.
// Returns sql.ErrNoRows when no matching row is found.
func (p *PGStore) ReconnectZerodhaToken(ctx context.Context, credID, encryptedCreds string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE broker_credentials
		    SET encrypted_creds=$1, connected_at=NOW()
		  WHERE id=$2 AND broker_type='zerodha'`,
		encryptedCreds, credID)
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

// ZerodhaContact holds a user's notification details for the daily reconnect reminder.
type ZerodhaContact struct {
	Email          string
	TelegramChatID string // empty if not connected
}

// ListZerodhaCredentialContacts returns the email and Telegram chat ID of every
// user who has at least one live Zerodha credential. Used by the daily token
// refresh notifier to send email and/or Telegram alerts.
func (p *PGStore) ListZerodhaCredentialContacts(ctx context.Context) ([]ZerodhaContact, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT DISTINCT u.email, COALESCE(u.telegram_chat_id, '')
		   FROM users u
		   JOIN broker_credentials bc ON bc.user_id = u.id
		  WHERE bc.broker_type = 'zerodha'
		    AND bc.account_mode = 'live'
		    AND bc.status = 'active'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var contacts []ZerodhaContact
	for rows.Next() {
		var c ZerodhaContact
		if err := rows.Scan(&c.Email, &c.TelegramChatID); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}

func (p *PGStore) DeleteBrokerCred(ctx context.Context, id string, bc *BrokerCredential) error {
	var res sql.Result
	var err error
	if bc.OrgID != nil {
		res, err = p.db.ExecContext(ctx,
			`DELETE FROM broker_credentials WHERE id=$1 AND org_id=$2`, id, *bc.OrgID)
	} else {
		res, err = p.db.ExecContext(ctx,
			`DELETE FROM broker_credentials WHERE id=$1 AND user_id=$2 AND org_id IS NULL`, id, bc.UserID)
	}
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

func (p *PGStore) CountWebhooksByBrokerCred(ctx context.Context, credID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE broker_cred_id=$1`, credID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountActiveWebhooksForCred(ctx context.Context, credID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE broker_cred_id=$1 AND status='active'`, credID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountWebhooksByUser(ctx context.Context, userID string) (int, error) {
	return p.CountSoloWebhooksByUser(ctx, userID)
}

func (p *PGStore) CountSoloWebhooksByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE user_id=$1 AND org_id IS NULL`, userID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountPaperWebhooksByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE user_id=$1 AND org_id IS NULL AND paper_account_id IS NOT NULL`, userID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountLiveWebhooksByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE user_id=$1 AND org_id IS NULL AND broker_cred_id IS NOT NULL`, userID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountActivePaperWebhooksByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE user_id=$1 AND org_id IS NULL AND status='active' AND paper_account_id IS NOT NULL`, userID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountActiveLiveWebhooksByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE user_id=$1 AND org_id IS NULL AND status='active' AND broker_cred_id IS NOT NULL`, userID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountWebhooksByOrg(ctx context.Context, orgID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE org_id=$1`, orgID,
	).Scan(&count)
	return count, err
}

// CountWebhooksByPaperAccount counts webhooks routed to a specific paper
// account — used to block deletion while a webhook still references it
// (mirrors CountWebhooksByBrokerCred's role in DeleteCredential). Without this
// check, deleting the account would trip webhooks_one_destination_check via
// the paper_account_id FK's ON DELETE SET NULL, surfacing as an opaque 500.
func (p *PGStore) CountWebhooksByPaperAccount(ctx context.Context, paperAccountID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE paper_account_id=$1`, paperAccountID,
	).Scan(&count)
	return count, err
}

// ListWebhooksByPaperAccount returns every webhook routed to a paper account.
// paper_account_id has no UNIQUE constraint (unlike a live credential's
// typical 1:1 usage in practice), so this can return more than one row —
// used by the paper account Diagnostics section to aggregate signal stats
// across all of them rather than silently picking just the first.
func (p *PGStore) ListWebhooksByPaperAccount(ctx context.Context, paperAccountID string) ([]*Webhook, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT `+webhookSelectCols+`
		 FROM webhooks WHERE paper_account_id=$1
		 ORDER BY created_at ASC`, paperAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Webhook
	for rows.Next() {
		wh, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wh)
	}
	return out, rows.Err()
}

func (p *PGStore) CountBrokerCredsByUser(ctx context.Context, userID string) (int, error) {
	return p.CountSoloBrokerCredsByUser(ctx, userID)
}

func (p *PGStore) CountSoloBrokerCredsByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM broker_credentials WHERE user_id=$1 AND org_id IS NULL`, userID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountBrokerCredsByOrg(ctx context.Context, orgID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM broker_credentials WHERE org_id=$1`, orgID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountActiveWebhooksByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE user_id=$1 AND org_id IS NULL AND status='active'`, userID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountActiveWebhooksByOrg(ctx context.Context, orgID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhooks WHERE org_id=$1 AND status='active'`, orgID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountActiveCredentialsByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM broker_credentials WHERE user_id=$1 AND org_id IS NULL AND status='active'`, userID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) CountActiveCredentialsByOrg(ctx context.Context, orgID string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM broker_credentials WHERE org_id=$1 AND status='active'`, orgID,
	).Scan(&count)
	return count, err
}

func (p *PGStore) HasAutoPausedItems(ctx context.Context, userID string) (bool, error) {
	var has bool
	// Show the downgrade banner when there are auto_paused items the user has not
	// yet acted on.  "Acted on" means the user has resumed at least one item of
	// that type (active count > 0).  Once they do, the banner for that type clears
	// even if other auto_paused items remain paused (they cannot resume those until
	// they upgrade).  Both halves must be resolved before the banner fully clears.
	err := p.db.QueryRowContext(ctx, `
		SELECT
			(EXISTS(SELECT 1 FROM webhooks           WHERE user_id=$1 AND auto_paused=true LIMIT 1)
			 AND NOT EXISTS(SELECT 1 FROM webhooks   WHERE user_id=$1 AND status='active'  LIMIT 1))
		OR
			(EXISTS(SELECT 1 FROM broker_credentials WHERE user_id=$1 AND auto_paused=true LIMIT 1)
			 AND NOT EXISTS(SELECT 1 FROM broker_credentials WHERE user_id=$1 AND status='active' LIMIT 1))
	`, userID).Scan(&has)
	return has, err
}

// ── Trades ────────────────────────────────────────────────────────────────────

func (p *PGStore) InsertTrade(ctx context.Context, t *Trade) error {
	var userIDVal any
	if t.UserID != "" {
		userIDVal = t.UserID
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO trades
		   (id, user_id, webhook_id, webhook_label, signal, symbol, lot_size, broker_order, signal_key, comment, algo_id, status, fill_price, order_type, product, sl_price, tp_price, error, error_code, created_at, updated_at, broker_responded_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,
		         CASE WHEN $12 = 'queued' THEN NULL ELSE $21::timestamptz END)`,
		t.ID, userIDVal, t.WebhookID, t.WebhookLabel, t.Signal, t.Symbol, t.LotSize,
		t.BrokerOrder, t.SignalKey, t.Comment, t.AlgoID, t.Status, t.FillPrice,
		t.OrderType, t.Product, t.SLPrice, t.TPPrice,
		t.Error, t.ErrorCode, t.CreatedAt, t.CreatedAt)
	return err
}

// PersistTradeAudit writes a trade audit row for every signal outcome. It inserts
// a new row, or updates an existing row with the same id when one is already
// present (e.g. gateway reject after worker queued the same request_id).
func (p *PGStore) PersistTradeAudit(ctx context.Context, t *Trade) error {
	if t == nil {
		return fmt.Errorf("store: nil trade")
	}
	if err := p.InsertTrade(ctx, t); err != nil {
		if isTradeDuplicateKey(err) {
			return p.FinalizeTrade(ctx, t)
		}
		return err
	}
	return nil
}

func isTradeDuplicateKey(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// FinalizeTrade updates an existing trade row after async worker execution.
func (p *PGStore) FinalizeTrade(ctx context.Context, t *Trade) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE trades SET
			status=$2, fill_price=$3, lot_size=$4, broker_order=$5, algo_id=$6,
			order_type=$7, product=$8, sl_price=$9, tp_price=$10,
			error=$11, error_code=$12, updated_at=NOW(),
			broker_responded_at=COALESCE(broker_responded_at, NOW())
		 WHERE id=$1`,
		t.ID, t.Status, t.FillPrice, t.LotSize, t.BrokerOrder, t.AlgoID,
		t.OrderType, t.Product, t.SLPrice, t.TPPrice, t.Error, t.ErrorCode)
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

// HasActiveTradeWithSignalKey reports whether a non-rejected trade with the same key exists within windowSec.
func (p *PGStore) HasActiveTradeWithSignalKey(ctx context.Context, webhookID, signalKey string, windowSec int) (bool, error) {
	if signalKey == "" || windowSec <= 0 {
		return false, nil
	}
	var exists bool
	err := p.db.QueryRowContext(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM trades
		   WHERE webhook_id=$1 AND signal_key=$2
		   AND status IN ('queued','submitted','filled','pending_confirmation')
		     AND created_at > NOW() - ($3::int * interval '1 second')
		 )`, webhookID, signalKey, windowSec).Scan(&exists)
	return exists, err
}

func (p *PGStore) GetTradeByID(ctx context.Context, id string) (*Trade, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(user_id,''), webhook_id, webhook_label, signal, symbol, lot_size, broker_order, signal_key, comment, algo_id, status, fill_price, order_type, product, sl_price, tp_price, error, error_code, cancelled_at, cancel_error, created_at
		 FROM trades WHERE id=$1`, id)
	var t Trade
	if err := row.Scan(&t.ID, &t.UserID, &t.WebhookID, &t.WebhookLabel, &t.Signal, &t.Symbol, &t.LotSize,
		&t.BrokerOrder, &t.SignalKey, &t.Comment, &t.AlgoID, &t.Status, &t.FillPrice,
		&t.OrderType, &t.Product, &t.SLPrice, &t.TPPrice,
		&t.Error, &t.ErrorCode, &t.CancelledAt, &t.CancelError, &t.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// MarkTradeCancelled records the outcome of a cancel attempt.
// An empty cancelErr marks the trade as fully cancelled; a non-empty value stores the error (status stays unchanged).
func (p *PGStore) MarkTradeCancelled(ctx context.Context, id string, cancelErr string) error {
	var err error
	if cancelErr == "" {
		_, err = p.db.ExecContext(ctx,
			`UPDATE trades SET status='cancelled', cancelled_at=NOW(), updated_at=NOW() WHERE id=$1`, id)
	} else {
		_, err = p.db.ExecContext(ctx,
			`UPDATE trades SET cancel_error=$2 WHERE id=$1`, id, cancelErr)
	}
	return err
}

func (p *PGStore) UpdateTradeFromExecutionEvent(ctx context.Context, t *Trade) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE trades SET status=$2, fill_price=$3, broker_order=COALESCE(NULLIF($4,''), broker_order),
		 updated_at=NOW(), broker_responded_at=COALESCE(broker_responded_at, NOW()) WHERE id=$1`,
		t.ID, t.Status, t.FillPrice, t.BrokerOrder)
	return err
}

func (p *PGStore) ListTradesByWebhook(ctx context.Context, webhookID string, limit int) ([]*Trade, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT t.id, COALESCE(t.user_id,''), t.webhook_id, t.webhook_label, t.signal, t.symbol, t.lot_size,
		        t.broker_order, t.signal_key, t.comment, t.algo_id, t.status, t.fill_price, t.order_type, t.product,
		        t.sl_price, t.tp_price, t.error, t.error_code, t.cancelled_at, t.cancel_error, t.created_at,
		        po.expires_at
		 FROM trades t
		 LEFT JOIN publisher_orders po ON po.trade_id = t.id
		 WHERE t.webhook_id=$1 ORDER BY t.created_at DESC LIMIT $2`, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Trade
	for rows.Next() {
		var t Trade
		if err := rows.Scan(&t.ID, &t.UserID, &t.WebhookID, &t.WebhookLabel, &t.Signal, &t.Symbol, &t.LotSize,
			&t.BrokerOrder, &t.SignalKey, &t.Comment, &t.AlgoID, &t.Status, &t.FillPrice,
			&t.OrderType, &t.Product, &t.SLPrice, &t.TPPrice,
			&t.Error, &t.ErrorCode, &t.CancelledAt, &t.CancelError, &t.CreatedAt,
			&t.PublisherOrderExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// ListTradesForExport returns up to 10 000 trades for a webhook, optionally
// filtered by a half-open time range [from, to). Nil bounds are unconstrained.
func (p *PGStore) ListTradesForExport(ctx context.Context, webhookID string, from, to *time.Time) ([]*Trade, error) {
	q := `SELECT id, COALESCE(user_id,''), webhook_id, webhook_label, signal, symbol, lot_size, broker_order, signal_key, comment, algo_id, status, fill_price, order_type, product, sl_price, tp_price, error, error_code, cancelled_at, cancel_error, created_at
	      FROM trades WHERE webhook_id=$1`
	args := []any{webhookID}
	if from != nil {
		args = append(args, *from)
		q += fmt.Sprintf(" AND created_at >= $%d", len(args))
	}
	if to != nil {
		args = append(args, *to)
		q += fmt.Sprintf(" AND created_at < $%d", len(args))
	}
	q += " ORDER BY created_at ASC LIMIT 10000"

	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Trade
	for rows.Next() {
		var t Trade
		if err := rows.Scan(&t.ID, &t.UserID, &t.WebhookID, &t.WebhookLabel, &t.Signal, &t.Symbol, &t.LotSize,
			&t.BrokerOrder, &t.SignalKey, &t.Comment, &t.AlgoID, &t.Status, &t.FillPrice,
			&t.OrderType, &t.Product, &t.SLPrice, &t.TPPrice,
			&t.Error, &t.ErrorCode, &t.CancelledAt, &t.CancelError, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}
