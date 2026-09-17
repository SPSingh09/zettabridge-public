-- Immutable worker→broker delivery timestamp for latency metrics.
-- Set once when the worker finalizes execution; never updated by fill poll,
-- cancel, or other later status transitions.
ALTER TABLE trades ADD COLUMN IF NOT EXISTS broker_responded_at TIMESTAMPTZ;

UPDATE trades
SET broker_responded_at = updated_at
WHERE broker_responded_at IS NULL
  AND status IN ('submitted', 'filled', 'rejected', 'pending_confirmation');
