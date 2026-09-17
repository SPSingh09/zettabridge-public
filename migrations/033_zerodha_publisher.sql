-- ============================================================
-- 033_zerodha_publisher.sql
-- ============================================================
-- Adds execution_mode to broker_credentials and creates the
-- publisher_orders + publisher_order_events tables for the
-- Zerodha Kite Publisher (browser-redirect confirmation) flow.

-- 1. execution_mode column on broker_credentials
--    Default 'direct_api' is safe for all non-Zerodha brokers.
ALTER TABLE broker_credentials
    ADD COLUMN IF NOT EXISTS execution_mode TEXT NOT NULL DEFAULT 'direct_api';

ALTER TABLE broker_credentials
    ADD CONSTRAINT broker_credentials_execution_mode_check
    CHECK (execution_mode IN ('direct_api', 'user_api_oauth', 'publisher'));

-- 2. Backfill: existing Zerodha rows → user_api_oauth.
--    Preserves current Kite Connect API OAuth behaviour exactly.
UPDATE broker_credentials
    SET execution_mode = 'user_api_oauth'
    WHERE broker_type = 'zerodha';

-- 3. publisher_orders — one row per handoff attempt.
--    trade_id is NULL for dashboard-initiated orders (no webhook signal).
CREATE TABLE IF NOT EXISTS publisher_orders (
    id                   TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id              TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id        TEXT        NOT NULL REFERENCES broker_credentials(id) ON DELETE CASCADE,
    webhook_id           TEXT        NULL REFERENCES webhooks(id) ON DELETE SET NULL,
    trade_id             TEXT        NULL,   -- back-reference to trades.id; set after trade row is created
    broker               TEXT        NOT NULL DEFAULT 'zerodha',
    execution_mode       TEXT        NOT NULL DEFAULT 'publisher',
    basket_payload_json  JSONB       NOT NULL,
    basket_payload_hash  TEXT        NOT NULL,
    -- status: created | awaiting_confirmation | user_returned | user_cancelled | expired
    status               TEXT        NOT NULL DEFAULT 'created',
    callback_status      TEXT        NULL,   -- raw status param echoed from Kite callback
    kite_request_token   TEXT        NULL,   -- populated on successful Kite callback
    signed_state         TEXT        NOT NULL,  -- HMAC-signed state for callback verification
    redirected_at        TIMESTAMPTZ NULL,
    callback_at          TIMESTAMPTZ NULL,
    expires_at           TIMESTAMPTZ NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_publisher_orders_user
    ON publisher_orders(user_id);

CREATE INDEX IF NOT EXISTS idx_publisher_orders_credential
    ON publisher_orders(credential_id);

CREATE INDEX IF NOT EXISTS idx_publisher_orders_trade
    ON publisher_orders(trade_id)
    WHERE trade_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_publisher_orders_status
    ON publisher_orders(status, expires_at);

-- 4. publisher_order_events — full audit trail per publisher order.
CREATE TABLE IF NOT EXISTS publisher_order_events (
    id                  TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    publisher_order_id  TEXT        NOT NULL REFERENCES publisher_orders(id) ON DELETE CASCADE,
    -- event_type values:
    --   SIGNAL_RECEIVED | BASKET_CREATED | AWAITING_USER_CONFIRMATION |
    --   USER_RETURNED_FROM_KITE | USER_CANCELLED | EXPIRED | FAILED_VALIDATION
    event_type          TEXT        NOT NULL,
    event_payload       JSONB       NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pub_events_order
    ON publisher_order_events(publisher_order_id, created_at);
