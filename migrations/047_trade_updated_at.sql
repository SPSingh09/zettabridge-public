-- Records when a trade's status last changed (queued -> submitted/filled/
-- rejected/cancelled), so the webhook Summary view can compute execution
-- latency and an accurate "last broker response" time. Existing rows have
-- no way to recover their real transition time, so they backfill to
-- created_at (reads as ~0 latency, which is expected for historical data).
ALTER TABLE trades ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
UPDATE trades SET updated_at = created_at;
