-- Tracks pause caused by an admin disabling a Zerodha execution mode (OAuth
-- or Publisher) platform-wide. Kept separate from auto_paused (plan
-- downgrade) so re-enabling one cause doesn't accidentally resume resources
-- paused for the other reason, and so the user can be shown a specific
-- "disabled by admin" message instead of the generic paused state.
ALTER TABLE broker_credentials ADD COLUMN IF NOT EXISTS admin_disabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE webhooks           ADD COLUMN IF NOT EXISTS admin_disabled BOOLEAN NOT NULL DEFAULT false;
