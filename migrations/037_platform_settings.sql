-- Generic key-value store for global, admin-configurable platform settings.
-- First consumer: market_data_provider (Phase 6 runtime provider toggle).
CREATE TABLE platform_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
