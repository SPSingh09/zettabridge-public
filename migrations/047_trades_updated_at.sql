-- Track when a trade row last changed (broker response, fill poll, cancel).
ALTER TABLE trades ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE trades SET updated_at = created_at WHERE updated_at IS NULL;
