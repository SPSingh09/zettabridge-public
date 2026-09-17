-- Single-row, platform-wide FYERS market-data OAuth connection (Phase 6 follow-up).
-- Not per-user — this is an admin-managed shared connection used only for
-- LTP quotes in the Paper Trading mark-to-market job, never order placement.
CREATE TABLE fyers_connection (
    id                       TEXT PRIMARY KEY DEFAULT 'default',
    encrypted_access_token   TEXT,
    encrypted_refresh_token  TEXT,
    encrypted_pin            TEXT,
    access_token_expires_at  TIMESTAMPTZ,
    refresh_token_expires_at TIMESTAMPTZ,
    connected_by             TEXT REFERENCES users(id),
    connected_at             TIMESTAMPTZ,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
