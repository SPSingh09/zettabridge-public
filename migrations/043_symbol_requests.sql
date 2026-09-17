-- Lets a paper-trading user request a symbol be added to an instrument
-- master (surfaced from the "unknown or inactive symbol" rejection),
-- and an admin review it. Status lifecycle:
--   pending -> accepted -> resolved (auto, when a matching instrument is created)
--   pending -> rejected
--   accepted -> rejected (admin changes mind before creating the instrument)

CREATE TABLE symbol_requests (
    id                     TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id                TEXT NOT NULL REFERENCES users(id),
    market_profile_code    TEXT NOT NULL REFERENCES market_profiles(code),
    exchange               TEXT NOT NULL DEFAULT '',
    symbol                 TEXT NOT NULL,
    reason                 TEXT NOT NULL,
    status                 TEXT NOT NULL DEFAULT 'pending', -- pending | accepted | rejected | resolved
    admin_note             TEXT NOT NULL DEFAULT '',
    reviewed_by            TEXT REFERENCES users(id),
    reviewed_at            TIMESTAMPTZ,
    resolved_instrument_id TEXT REFERENCES instruments(id) ON DELETE SET NULL,
    resolved_at            TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_symbol_requests_user ON symbol_requests(user_id);
CREATE INDEX idx_symbol_requests_status ON symbol_requests(status);

-- One active (pending/accepted) request per user per profile+symbol.
CREATE UNIQUE INDEX idx_symbol_requests_active_dedup
    ON symbol_requests(user_id, market_profile_code, lower(symbol))
    WHERE status IN ('pending', 'accepted');
