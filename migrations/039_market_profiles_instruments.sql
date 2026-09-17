-- Phase 7: admin-configurable market profiles + a symbol master (instruments)
-- table for Paper Trading. Purely additive — no existing table touched.

CREATE TABLE market_profiles (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    code          TEXT UNIQUE NOT NULL,     -- stable identifier, e.g. "indian_equity" — referenced by paper_accounts.market_profile
    name          TEXT NOT NULL,            -- display name, e.g. "Indian Equity"
    base_currency TEXT NOT NULL DEFAULT 'INR',
    timezone      TEXT NOT NULL DEFAULT 'Asia/Kolkata',
    trading_days  JSONB,                    -- store.TradingSchedule shape: {"mon":[{"start":"09:15","end":"15:30"}], ...}; NULL/empty = 24/7
    active        BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO market_profiles (code, name, base_currency, timezone, trading_days, active) VALUES (
    'indian_equity',
    'Indian Equity',
    'INR',
    'Asia/Kolkata',
    '{
        "mon": [{"start": "09:15", "end": "15:30"}],
        "tue": [{"start": "09:15", "end": "15:30"}],
        "wed": [{"start": "09:15", "end": "15:30"}],
        "thu": [{"start": "09:15", "end": "15:30"}],
        "fri": [{"start": "09:15", "end": "15:30"}]
    }'::jsonb,
    true
);

CREATE TABLE instruments (
    id                  TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    market_profile_code TEXT NOT NULL REFERENCES market_profiles(code),
    exchange            TEXT NOT NULL,
    symbol              TEXT NOT NULL,
    name                TEXT NOT NULL,
    instrument_type     TEXT NOT NULL DEFAULT 'EQUITY', -- EQUITY | INDEX | FUTURES | OPTIONS
    tick_size           DOUBLE PRECISION NOT NULL DEFAULT 0.05,
    lot_size            DOUBLE PRECISION NOT NULL DEFAULT 1,
    active              BOOLEAN NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (market_profile_code, exchange, symbol)
);

INSERT INTO instruments (market_profile_code, exchange, symbol, name, instrument_type, tick_size, lot_size) VALUES
    ('indian_equity', 'NSE', 'RELIANCE',  'Reliance Industries',      'EQUITY', 0.05, 1),
    ('indian_equity', 'NSE', 'TCS',       'Tata Consultancy Services', 'EQUITY', 0.05, 1),
    ('indian_equity', 'NSE', 'INFY',      'Infosys',                  'EQUITY', 0.05, 1),
    ('indian_equity', 'NSE', 'SBIN',      'State Bank of India',      'EQUITY', 0.05, 1),
    ('indian_equity', 'NSE', 'HDFCBANK',  'HDFC Bank',                'EQUITY', 0.05, 1),
    ('indian_equity', 'NSE', 'ICICIBANK', 'ICICI Bank',               'EQUITY', 0.05, 1),
    ('indian_equity', 'NSE', 'NIFTY',     'Nifty 50 Index',           'INDEX',  0.05, 1),
    ('indian_equity', 'NSE', 'BANKNIFTY', 'Nifty Bank Index',         'INDEX',  0.05, 1);
