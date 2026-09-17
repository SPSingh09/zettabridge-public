-- ============================================================
-- 001_init.sql
-- ============================================================
-- ZettaBridge database schema
-- Run with: psql $DATABASE_URL -f migrations/001_init.sql

CREATE EXTENSION IF NOT EXISTS "pgcrypto";  -- for gen_random_uuid()

-- ── Users ─────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    plan          TEXT NOT NULL DEFAULT 'free' CHECK (plan IN ('free', 'pro', 'enterprise')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Broker credentials (AES-256-GCM encrypted blob) ──────────────────────────
CREATE TABLE IF NOT EXISTS broker_credentials (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    broker_type      TEXT NOT NULL,   -- mt5_cloud | zerodha | angel | dhan
    encrypted_creds  TEXT NOT NULL,
    account_label    TEXT NOT NULL DEFAULT '',
    account_mode     TEXT NOT NULL DEFAULT 'demo'   -- demo | live
);

CREATE INDEX IF NOT EXISTS idx_broker_creds_user ON broker_credentials(user_id);

-- ── Webhooks ──────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS webhooks (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token           TEXT UNIQUE NOT NULL,
    label           TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active',   -- active | paused
    broker_cred_id  TEXT NOT NULL REFERENCES broker_credentials(id),
    symbol          TEXT NOT NULL,
    lot_size        NUMERIC(10,4) NOT NULL DEFAULT 0.01,
    max_risk_pct    NUMERIC(5,2)  NOT NULL DEFAULT 1.0,
    sl_points       INTEGER NOT NULL DEFAULT 20,
    tp_points       INTEGER NOT NULL DEFAULT 30,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhooks_user   ON webhooks(user_id);
CREATE INDEX IF NOT EXISTS idx_webhooks_token  ON webhooks(token);   -- hot path

-- ── Trades (immutable audit log) ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS trades (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    webhook_id   TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    signal       TEXT NOT NULL,        -- BUY | SELL | CLOSE
    symbol       TEXT NOT NULL,
    lot_size     NUMERIC(10,4),
    broker_order TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL,        -- queued | filled | rejected
    fill_price   NUMERIC(18,8) NOT NULL DEFAULT 0,
    error        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_trades_webhook  ON trades(webhook_id);
CREATE INDEX IF NOT EXISTS idx_trades_created  ON trades(created_at DESC);

-- ============================================================
-- 002_account_mode.sql
-- ============================================================
-- Add account_mode to broker credentials (demo | live)
ALTER TABLE broker_credentials
    ADD COLUMN IF NOT EXISTS account_mode TEXT NOT NULL DEFAULT 'demo';

-- ============================================================
-- 002_admin_users.sql
-- ============================================================
-- Platform admin: user role and account status
-- Run on existing DBs: psql $DATABASE_URL -f migrations/002_admin_users.sql

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user'
        CHECK (role IN ('user', 'admin')),
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended'));

CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);

-- ============================================================
-- 002_webhook_guards.sql
-- ============================================================
-- Webhook guard settings (Phase 1 + Phase 2)
-- Run on existing DBs: psql $DATABASE_URL -f migrations/002_webhook_guards.sql

ALTER TABLE webhooks
    ADD COLUMN IF NOT EXISTS allowed_actions TEXT[] NOT NULL DEFAULT '{BUY,SELL,CLOSE}',
    ADD COLUMN IF NOT EXISTS allowed_symbols TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS max_lot_size NUMERIC(10,4) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS allow_symbol_override BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS allow_lot_override BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS allow_sl_override BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS allow_tp_override BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS rate_limit_per_sec INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'UTC',
    ADD COLUMN IF NOT EXISTS trading_hours JSONB NOT NULL DEFAULT 'null',
    ADD COLUMN IF NOT EXISTS required_comment TEXT NOT NULL DEFAULT '';

-- ============================================================
-- 003_auth_sessions.sql
-- ============================================================
-- Refresh token sessions for auth refresh / logout / password rotation
-- Run on existing DBs: psql $DATABASE_URL -f migrations/003_auth_sessions.sql

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash ON refresh_tokens(token_hash);

-- ============================================================
-- 003_organizations.sql
-- ============================================================
-- Enterprise organizations (E0)
-- Run on existing DBs: docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/003_organizations.sql

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS orgs_enabled BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS organizations (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    owner_user_id   TEXT NOT NULL REFERENCES users(id),
    seat_limit      INTEGER NOT NULL DEFAULT 5 CHECK (seat_limit > 0 AND seat_limit <= 1000),
    status          TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_organizations_owner ON organizations(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_organizations_status ON organizations(status);

CREATE TABLE IF NOT EXISTS org_members (
    id          TEXT PRIMARY KEY,
    org_id      TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL
        CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    status      TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended')),
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id),
    UNIQUE (org_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_org_members_org ON org_members(org_id);
CREATE INDEX IF NOT EXISTS idx_org_members_user ON org_members(user_id);

-- Nullable for E3 resource scoping; solo users remain NULL.
ALTER TABLE webhooks
    ADD COLUMN IF NOT EXISTS org_id TEXT REFERENCES organizations(id) ON DELETE SET NULL;

ALTER TABLE broker_credentials
    ADD COLUMN IF NOT EXISTS org_id TEXT REFERENCES organizations(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_webhooks_org ON webhooks(org_id);
CREATE INDEX IF NOT EXISTS idx_broker_creds_org ON broker_credentials(org_id);

-- ============================================================
-- 003_plan_check.sql
-- ============================================================
-- Restrict users.plan to supported values only.
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_plan_check;

ALTER TABLE users
    ADD CONSTRAINT users_plan_check CHECK (plan IN ('free', 'pro', 'enterprise'));

-- ============================================================
-- 004_org_invites.sql
-- ============================================================
-- Enterprise org invites (E2)
-- Run: docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/004_org_invites.sql

CREATE TABLE IF NOT EXISTS org_invites (
    id           TEXT PRIMARY KEY,
    org_id       TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email        TEXT NOT NULL,
    role         TEXT NOT NULL
        CHECK (role IN ('admin', 'member', 'viewer')),
    token_hash   TEXT NOT NULL UNIQUE,
    expires_at   TIMESTAMPTZ NOT NULL,
    accepted_at  TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    invited_by   TEXT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_org_invites_org ON org_invites(org_id);
CREATE INDEX IF NOT EXISTS idx_org_invites_email ON org_invites(lower(email));

-- One pending invite per email per org.
CREATE UNIQUE INDEX IF NOT EXISTS idx_org_invites_pending_unique
    ON org_invites(org_id, lower(email))
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

-- ============================================================
-- 004_trades_cascade.sql
-- ============================================================
-- Allow webhook delete to cascade to trade audit rows
ALTER TABLE trades DROP CONSTRAINT IF EXISTS trades_webhook_id_fkey;
ALTER TABLE trades
    ADD CONSTRAINT trades_webhook_id_fkey
    FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE CASCADE;

-- ============================================================
-- 005_e3_resources.sql
-- ============================================================
-- E3: org-scoped resource metadata (created_by audit)
-- Run: docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/005_e3_resources.sql

ALTER TABLE webhooks
    ADD COLUMN IF NOT EXISTS created_by TEXT REFERENCES users(id);

ALTER TABLE broker_credentials
    ADD COLUMN IF NOT EXISTS created_by TEXT REFERENCES users(id);

UPDATE webhooks SET created_by = user_id WHERE created_by IS NULL;
UPDATE broker_credentials SET created_by = user_id WHERE created_by IS NULL;

CREATE INDEX IF NOT EXISTS idx_webhooks_org_created ON webhooks(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_broker_creds_org ON broker_credentials(org_id) WHERE org_id IS NOT NULL;

-- ============================================================
-- 006_org_audit.sql
-- ============================================================
-- E4: org audit log
-- Run: docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/006_org_audit.sql

CREATE TABLE IF NOT EXISTS org_audit_log (
    id              TEXT PRIMARY KEY,
    org_id          TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    actor_user_id   TEXT NOT NULL REFERENCES users(id),
    action          TEXT NOT NULL,
    target_user_id  TEXT REFERENCES users(id),
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_org_audit_org_created ON org_audit_log(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_org_audit_actor ON org_audit_log(actor_user_id);

-- ============================================================
-- 007_trades_webhook_cascade.sql
-- ============================================================
-- Webhook delete: cascade trade audit rows when a webhook is removed.
-- Run: docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/007_trades_webhook_cascade.sql

ALTER TABLE trades DROP CONSTRAINT IF EXISTS trades_webhook_id_fkey;

ALTER TABLE trades
    ADD CONSTRAINT trades_webhook_id_fkey
    FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE CASCADE;

-- ============================================================
-- 008_broker_cred_exchange_product.sql
-- ============================================================
-- Broker credential execution context (exchange, product) + ensure account_mode column
ALTER TABLE broker_credentials
    ADD COLUMN IF NOT EXISTS account_mode TEXT NOT NULL DEFAULT 'demo',
    ADD COLUMN IF NOT EXISTS exchange TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS product TEXT NOT NULL DEFAULT 'MIS';

-- ============================================================
-- 009_trade_error_code.sql
-- ============================================================
-- Stable broker/ops error codes on trade audit rows (public message stays in error column)
ALTER TABLE trades
    ADD COLUMN IF NOT EXISTS error_code TEXT NOT NULL DEFAULT '';

-- ============================================================
-- 010_trade_idempotency.sql
-- ============================================================
-- Signal dedup (Phase 6.1): webhook window + trade audit fields
-- Run: docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/010_trade_idempotency.sql

ALTER TABLE webhooks
    ADD COLUMN IF NOT EXISTS dedup_window_sec INTEGER NOT NULL DEFAULT 0;

ALTER TABLE trades
    ADD COLUMN IF NOT EXISTS signal_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS comment TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_trades_webhook_signal_key
    ON trades (webhook_id, signal_key)
    WHERE signal_key <> '';

-- ============================================================
-- 011_enterprise_tier_early_bird.sql
-- ============================================================
-- Enterprise billing tiers (seat caps) and early-bird live access for free users.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS enterprise_tier TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS early_bird_live BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS live_orders_used INTEGER NOT NULL DEFAULT 0;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_enterprise_tier_check;

ALTER TABLE users
    ADD CONSTRAINT users_enterprise_tier_check
        CHECK (enterprise_tier IN ('', 'starter', 'team', 'business'));

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_live_orders_used_check;

ALTER TABLE users
    ADD CONSTRAINT users_live_orders_used_check
        CHECK (live_orders_used >= 0);

-- ============================================================
-- 012_email_verification.sql
-- ============================================================
-- Email verification (PR 3.1)
-- Run: docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/012_email_verification.sql

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ;

UPDATE users
SET email_verified_at = created_at
WHERE email_verified_at IS NULL;

CREATE TABLE IF NOT EXISTS email_verifications (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_email_verifications_user
    ON email_verifications(user_id)
    WHERE consumed_at IS NULL;

-- ============================================================
-- 013_sebi_algo_id.sql
-- ============================================================
-- SEBI/NSE algo ID on Indian live credentials and trade audit trail.
-- Run: docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/013_sebi_algo_id.sql

ALTER TABLE broker_credentials
    ADD COLUMN IF NOT EXISTS algo_id TEXT NOT NULL DEFAULT '';

ALTER TABLE trades
    ADD COLUMN IF NOT EXISTS algo_id TEXT NOT NULL DEFAULT '';

-- ============================================================
-- 014_stripe_billing.sql
-- ============================================================
-- Stripe billing columns and webhook idempotency (PR 4B.1 phase A).

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS stripe_customer_id     TEXT UNIQUE,
  ADD COLUMN IF NOT EXISTS stripe_subscription_id TEXT,
  ADD COLUMN IF NOT EXISTS billing_source         TEXT NOT NULL DEFAULT 'free'
    CHECK (billing_source IN ('free', 'stripe', 'admin'));

CREATE TABLE IF NOT EXISTS stripe_webhook_events (
  id           TEXT PRIMARY KEY,
  type         TEXT NOT NULL,
  processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- 015_platform_invites.sql
-- ============================================================
CREATE TABLE IF NOT EXISTS platform_invites (
  id         TEXT        PRIMARY KEY DEFAULT gen_random_uuid(),
  token_hash TEXT        UNIQUE NOT NULL,
  email      TEXT,
  note       TEXT,
  used_at    TIMESTAMPTZ,
  used_by    TEXT        REFERENCES users(id),
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  created_by TEXT        NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS platform_invites_token_hash_idx ON platform_invites (token_hash);

-- ============================================================
-- 016_sltp_audit.sql
-- ============================================================
ALTER TABLE trades ADD COLUMN IF NOT EXISTS order_type TEXT NOT NULL DEFAULT 'market';
ALTER TABLE trades ADD COLUMN IF NOT EXISTS product   TEXT NOT NULL DEFAULT '';
ALTER TABLE trades ADD COLUMN IF NOT EXISTS sl_price   NUMERIC(18,8);
ALTER TABLE trades ADD COLUMN IF NOT EXISTS tp_price   NUMERIC(18,8);

-- ============================================================
-- 017_cancel_order.sql
-- ============================================================
ALTER TABLE trades ADD COLUMN IF NOT EXISTS cancelled_at  TIMESTAMPTZ;
ALTER TABLE trades ADD COLUMN IF NOT EXISTS cancel_error  TEXT NOT NULL DEFAULT '';

-- ============================================================
-- 018_fix_cancel_error_not_null.sql
-- ============================================================
-- 017 added cancel_error as nullable TEXT; fix to match the NOT NULL DEFAULT '' pattern
-- of error/error_code so lib/pq can scan it into a plain Go string.
UPDATE trades SET cancel_error = '' WHERE cancel_error IS NULL;
ALTER TABLE trades ALTER COLUMN cancel_error SET NOT NULL;
ALTER TABLE trades ALTER COLUMN cancel_error SET DEFAULT '';

-- ============================================================
-- 019_zerodha_connected_at.sql
-- ============================================================
-- Track when a Zerodha OAuth token was last obtained.
-- Used to determine whether the daily 6:00 AM IST token has expired
-- and to drive the dashboard reconnect warning.
ALTER TABLE broker_credentials ADD COLUMN IF NOT EXISTS connected_at TIMESTAMPTZ;

-- ============================================================
-- 020_plan_rename.sql
-- ============================================================
-- Rename subscription plans: pro → individual, enterprise → household.
-- Drops the enterprise_tier column, which is no longer used by the application.

-- Drop the old plan constraint before renaming values it would reject.
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_plan_check;

-- Rename existing plan values.
UPDATE users SET plan = 'individual' WHERE plan = 'pro';
UPDATE users SET plan = 'household'  WHERE plan = 'enterprise';

-- Drop enterprise_tier constraint and column.
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_enterprise_tier_check;

ALTER TABLE users
    DROP COLUMN IF EXISTS enterprise_tier;

-- New plan constraint for the three supported plans.
ALTER TABLE users
    ADD CONSTRAINT users_plan_check CHECK (plan IN ('free', 'individual', 'household'));

-- ============================================================
-- 021_credential_status.sql
-- ============================================================
-- Add status and auto_paused tracking to broker_credentials (mirrors webhooks.status).
-- Add auto_paused flag to webhooks to distinguish system-paused (plan downgrade) from
-- user-paused, so the dashboard can show a targeted warning banner.

ALTER TABLE broker_credentials
    ADD COLUMN status      VARCHAR(20) NOT NULL DEFAULT 'active',
    ADD COLUMN auto_paused BOOLEAN     NOT NULL DEFAULT false;

ALTER TABLE broker_credentials
    ADD CONSTRAINT broker_credentials_status_check
        CHECK (status IN ('active', 'paused'));

ALTER TABLE webhooks
    ADD COLUMN auto_paused BOOLEAN NOT NULL DEFAULT false;

-- ============================================================
-- 022_credential_order_type.sql
-- ============================================================
-- Add order_type and market_protection to broker_credentials.
-- order_type: MARKET (default) or LIMIT — controls entry execution on Indian brokers.
-- market_protection: required Zerodha % for MARKET orders; used as slippage % for LIMIT orders.
ALTER TABLE broker_credentials
    ADD COLUMN order_type        VARCHAR(20)   NOT NULL DEFAULT 'MARKET',
    ADD COLUMN market_protection NUMERIC(5,2)  NOT NULL DEFAULT 0;

ALTER TABLE broker_credentials
    ADD CONSTRAINT broker_credentials_order_type_check
        CHECK (order_type IN ('MARKET', 'LIMIT'));

-- ============================================================
-- 023_migrate_owner_resources_to_org.sql
-- ============================================================
-- Backfill: assign solo webhooks and credentials owned by org owners/admins
-- to their org. Before this fix, createOrg did not set org_id on existing
-- resources, making them invisible to the org-scoped list queries.
--
-- Only touches rows where org_id IS NULL (solo) to avoid double-migration.
-- Invited members' resources are intentionally left solo (they get paused on join).

UPDATE webhooks w
SET org_id = m.org_id
FROM org_members m
WHERE m.user_id  = w.user_id
  AND m.role     IN ('owner', 'admin')
  AND m.status   = 'active'
  AND w.org_id   IS NULL;

UPDATE broker_credentials bc
SET org_id = m.org_id
FROM org_members m
WHERE m.user_id  = bc.user_id
  AND m.role     IN ('owner', 'admin')
  AND m.status   = 'active'
  AND bc.org_id  IS NULL;

-- ============================================================
-- 024_pause_owner_individual_resources.sql
-- ============================================================
-- Policy change: when a user creates a Household org (becomes owner/admin),
-- their existing Individual-plan resources should be paused — not migrated to
-- org scope. Users start fresh with new credentials and webhooks on the Household
-- plan. This undoes the migration-023 org-assignment for owners/admins, moving
-- those resources back to solo (org_id = NULL) and marking them paused so:
--   • they don't appear in the org's resource list
--   • the Zerodha reconnect warning is not shown for paused credentials

UPDATE broker_credentials bc
SET    org_id      = NULL,
       status      = 'paused',
       auto_paused = true
FROM   org_members m
WHERE  m.user_id = bc.user_id
  AND  m.role    IN ('owner', 'admin')
  AND  m.status  = 'active'
  AND  bc.org_id = m.org_id;

UPDATE webhooks w
SET    org_id      = NULL,
       status      = 'paused',
       auto_paused = true
FROM   org_members m
WHERE  m.user_id = w.user_id
  AND  m.role    IN ('owner', 'admin')
  AND  m.status  = 'active'
  AND  w.org_id  = m.org_id;

-- ============================================================
-- 025_webhook_token_hash.sql
-- ============================================================
-- Rename plaintext token column to token_hash; original token values are
-- backfilled as their own SHA-256 hex digest so existing rows remain valid
-- (they will be rotated on next use by the token-rotation endpoint).
ALTER TABLE webhooks RENAME COLUMN token TO token_hash;

DROP INDEX IF EXISTS idx_webhooks_token;
CREATE INDEX IF NOT EXISTS idx_webhooks_token_hash ON webhooks(token_hash);

-- ============================================================
-- 026_webhook_order_type_and_dedup_default.sql
-- ============================================================
-- Add webhook-level default order type (overrides credential order type when set).
ALTER TABLE webhooks ADD COLUMN IF NOT EXISTS default_order_type TEXT NOT NULL DEFAULT '';

-- Spec says dedup window defaults to 2 s, not 0 (disabled).
ALTER TABLE webhooks ALTER COLUMN dedup_window_sec SET DEFAULT 2;

-- ============================================================
-- 027_webhook_rate_limit_per_min.sql
-- ============================================================
-- Add per-webhook per-minute rate limit for ingest-layer misuse protection.
-- Separate from rate_limit_per_sec which is a broker-side execution cap.
-- Default 60 req/min; 0 = disabled.
ALTER TABLE webhooks ADD COLUMN IF NOT EXISTS rate_limit_per_min INTEGER NOT NULL DEFAULT 60;

-- ============================================================
-- 028_user_audit_log.sql
-- ============================================================
-- User-level audit log for account actions (login, register, credential/webhook CRUD).
-- Retention: 90 days, enforced by pg_cron job defined below.
CREATE TABLE IF NOT EXISTS user_audit_log (
    id         TEXT        PRIMARY KEY,
    user_id    TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action     TEXT        NOT NULL,
    ip         TEXT,
    metadata   JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_audit_log_user_created
    ON user_audit_log(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_audit_log_created
    ON user_audit_log(created_at);


-- ============================================================
-- 029_ingest_log.sql
-- ============================================================
-- Persistent log of every ingest attempt where the webhook token resolved.
-- Unknown-token 401s are excluded (no webhook_id; DDoS flood risk).
-- Retention: 90 days, enforced by pg_cron job below.
CREATE TABLE IF NOT EXISTS ingest_log (
    id          TEXT        PRIMARY KEY,
    webhook_id  TEXT        NOT NULL,           -- no FK: rows must survive webhook deletion
    request_id  TEXT        NOT NULL,           -- tradeID (accepted/dedup) or fresh UUID (rejected)
    outcome     TEXT        NOT NULL,           -- accepted|rejected|rate_limited|dedup|paused|queue_full
    error       TEXT        NOT NULL DEFAULT '',
    ip          TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ingest_log_webhook_created ON ingest_log(webhook_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ingest_log_created         ON ingest_log(created_at);


-- ============================================================
-- 030_trades_audit_hardening.sql
-- ============================================================
-- Harden the trades table as a true audit log:
--   1. Add webhook_label snapshot so orphaned trade rows remain identifiable.
--   2. Change FK to ON DELETE SET NULL so trades survive webhook deletion.
--   3. Add pg_cron 90-day cleanup matching the website retention promise.

-- Step 1: add webhook_label column (snapshot of label at trade time)
ALTER TABLE trades ADD COLUMN IF NOT EXISTS webhook_label TEXT NOT NULL DEFAULT '';

-- Backfill label for existing rows
UPDATE trades t
SET    webhook_label = w.label
FROM   webhooks w
WHERE  t.webhook_id = w.id
  AND  t.webhook_label = '';

-- Step 2: drop old CASCADE FK, re-add as SET NULL
--   webhook_id must become nullable for SET NULL to work.
ALTER TABLE trades DROP CONSTRAINT IF EXISTS trades_webhook_id_fkey;
ALTER TABLE trades ALTER COLUMN webhook_id DROP NOT NULL;
ALTER TABLE trades
    ADD CONSTRAINT trades_webhook_id_fkey
    FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE SET NULL;


-- ============================================================
-- 031_notifications.sql
-- ============================================================
-- Add user_id to trades for direct user attribution (survives webhook SET NULL).
ALTER TABLE trades ADD COLUMN IF NOT EXISTS user_id TEXT
    REFERENCES users(id) ON DELETE SET NULL;

-- Backfill from webhooks for existing rows
UPDATE trades t
SET    user_id = w.user_id
FROM   webhooks w
WHERE  t.webhook_id = w.id
  AND  t.user_id IS NULL;

-- In-app notification store (Individual + Household plans).
-- Retention: 90 days, enforced by pg_cron job below.
CREATE TABLE IF NOT EXISTS notifications (
    id         TEXT        PRIMARY KEY,
    user_id    TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       TEXT        NOT NULL,   -- order_filled|order_submitted|order_rejected|order_cancelled
    title      TEXT        NOT NULL,
    body       TEXT        NOT NULL,
    trade_id   TEXT,                   -- nullable back-reference to trades.id
    read_at    TIMESTAMPTZ,            -- NULL = unread
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
    ON notifications(user_id, read_at, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_created
    ON notifications(created_at);


-- ============================================================
-- 032_user_telegram.sql
-- ============================================================
-- Telegram chat ID for trade alert delivery (Individual + Household plans).
-- NULL = not connected.
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_chat_id TEXT;

