-- ============================================================
-- 034_paper_trading.sql
-- ============================================================
-- Phase 1 of the ZettaBridge Paper Trading Engine: first-class
-- broker-free paper accounts, orders, positions, and trades.
--
-- Purely additive — five new tables, no ALTER of any existing
-- table (broker_credentials, webhooks, trades, users, etc. are
-- untouched). Live broker trading is unaffected by this migration.

-- 1. paper_accounts — one simulated trading account per user.
CREATE TABLE IF NOT EXISTS paper_accounts (
    id                TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id           TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label             TEXT        NOT NULL DEFAULT '',
    -- market_profile: indian_equity (now) | indian_fno | forex | crypto_spot (later phases)
    market_profile    TEXT        NOT NULL DEFAULT 'indian_equity'
        CHECK (market_profile IN ('indian_equity', 'indian_fno', 'forex', 'crypto_spot')),
    base_currency     TEXT        NOT NULL DEFAULT 'INR',
    starting_balance  NUMERIC(18,8) NOT NULL DEFAULT 0 CHECK (starting_balance >= 0),
    cash_balance      NUMERIC(18,8) NOT NULL DEFAULT 0,
    status            TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'closed')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_paper_accounts_user ON paper_accounts(user_id);

-- 2. paper_orders — one row per simulated order attempt from a webhook signal.
CREATE TABLE IF NOT EXISTS paper_orders (
    id                TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id           TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    paper_account_id  TEXT        NOT NULL REFERENCES paper_accounts(id) ON DELETE CASCADE,
    webhook_id        TEXT        NULL REFERENCES webhooks(id) ON DELETE SET NULL,
    signal_id         TEXT        NOT NULL DEFAULT '',
    symbol            TEXT        NOT NULL,
    exchange          TEXT        NOT NULL DEFAULT 'NSE',
    side              TEXT        NOT NULL CHECK (side IN ('BUY', 'SELL', 'CLOSE')),
    order_type        TEXT        NOT NULL DEFAULT 'MARKET' CHECK (order_type IN ('MARKET', 'LIMIT')),
    product           TEXT        NOT NULL DEFAULT 'MIS',
    quantity          NUMERIC(18,8) NOT NULL,
    requested_price   NUMERIC(18,8) NULL,
    fill_price        NUMERIC(18,8) NULL,
    status            TEXT        NOT NULL DEFAULT 'RECEIVED'
        CHECK (status IN ('RECEIVED', 'VALIDATED', 'FILLED', 'REJECTED', 'CANCELLED')),
    reason            TEXT        NOT NULL DEFAULT '',
    raw_signal        JSONB       NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_paper_orders_account ON paper_orders(paper_account_id);
CREATE INDEX IF NOT EXISTS idx_paper_orders_webhook ON paper_orders(webhook_id) WHERE webhook_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_paper_orders_status  ON paper_orders(status);

-- 3. paper_positions — current open/closed position per account+symbol+product.
CREATE TABLE IF NOT EXISTS paper_positions (
    id                TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id           TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    paper_account_id  TEXT        NOT NULL REFERENCES paper_accounts(id) ON DELETE CASCADE,
    symbol            TEXT        NOT NULL,
    exchange          TEXT        NOT NULL DEFAULT 'NSE',
    product           TEXT        NOT NULL DEFAULT 'MIS',
    -- quantity is signed: positive = long, negative = short, 0 = closed
    quantity          NUMERIC(18,8) NOT NULL DEFAULT 0,
    avg_entry_price   NUMERIC(18,8) NOT NULL DEFAULT 0,
    last_price        NUMERIC(18,8) NOT NULL DEFAULT 0,
    unrealized_pnl    NUMERIC(18,8) NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (paper_account_id, exchange, symbol, product)
);

CREATE INDEX IF NOT EXISTS idx_paper_positions_account ON paper_positions(paper_account_id);

-- 4. paper_trades — immutable executed-trade history (fills only).
CREATE TABLE IF NOT EXISTS paper_trades (
    id                TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id           TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    paper_account_id  TEXT        NOT NULL REFERENCES paper_accounts(id) ON DELETE CASCADE,
    order_id          TEXT        NOT NULL REFERENCES paper_orders(id) ON DELETE CASCADE,
    symbol            TEXT        NOT NULL,
    exchange          TEXT        NOT NULL DEFAULT 'NSE',
    side              TEXT        NOT NULL CHECK (side IN ('BUY', 'SELL')),
    quantity          NUMERIC(18,8) NOT NULL,
    price             NUMERIC(18,8) NOT NULL,
    realized_pnl      NUMERIC(18,8) NOT NULL DEFAULT 0,
    fees              NUMERIC(18,8) NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_paper_trades_account ON paper_trades(paper_account_id);
CREATE INDEX IF NOT EXISTS idx_paper_trades_order   ON paper_trades(order_id);

-- 5. paper_account_snapshots — periodic equity/P&L snapshots (Phase 6 mark-to-market writes these).
CREATE TABLE IF NOT EXISTS paper_account_snapshots (
    id                TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id           TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    paper_account_id  TEXT        NOT NULL REFERENCES paper_accounts(id) ON DELETE CASCADE,
    equity            NUMERIC(18,8) NOT NULL,
    cash_balance      NUMERIC(18,8) NOT NULL,
    unrealized_pnl    NUMERIC(18,8) NOT NULL DEFAULT 0,
    realized_pnl      NUMERIC(18,8) NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_paper_account_snapshots_account ON paper_account_snapshots(paper_account_id, created_at DESC);
