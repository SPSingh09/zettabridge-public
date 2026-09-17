-- ============================================================
-- 036_webhook_paper_destination.sql
-- ============================================================
-- Phase 3 of the Paper Trading Engine: a webhook may route signals to a
-- paper trading account instead of a live broker credential.
--
-- broker_cred_id becomes nullable; paper_account_id is added. Exactly one
-- of the two must be set. Existing rows all have broker_cred_id set and
-- paper_account_id NULL, so the new CHECK constraint holds immediately —
-- no backfill needed, no live-trading behavior changes.

ALTER TABLE webhooks
    ALTER COLUMN broker_cred_id DROP NOT NULL;

ALTER TABLE webhooks
    ADD COLUMN IF NOT EXISTS paper_account_id TEXT REFERENCES paper_accounts(id) ON DELETE SET NULL;

ALTER TABLE webhooks
    ADD CONSTRAINT webhooks_one_destination_check
    CHECK (
        (broker_cred_id IS NOT NULL AND paper_account_id IS NULL) OR
        (broker_cred_id IS NULL AND paper_account_id IS NOT NULL)
    );

CREATE INDEX IF NOT EXISTS idx_webhooks_paper_account ON webhooks(paper_account_id) WHERE paper_account_id IS NOT NULL;
