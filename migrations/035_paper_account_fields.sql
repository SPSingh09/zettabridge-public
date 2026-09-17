-- ============================================================
-- 035_paper_account_fields.sql
-- ============================================================
-- Adds exchange and default_product to paper_accounts so the "Add Paper
-- Trading Account" form (DEMO_EXECUTION_PLAN.md Phase 0) can persist these
-- fields. Purely additive — no existing table is touched.

ALTER TABLE paper_accounts
    ADD COLUMN IF NOT EXISTS exchange TEXT NOT NULL DEFAULT 'NSE';

ALTER TABLE paper_accounts
    ADD COLUMN IF NOT EXISTS default_product TEXT NOT NULL DEFAULT 'MIS';

ALTER TABLE paper_accounts
    ADD CONSTRAINT paper_accounts_default_product_check
    CHECK (default_product IN ('MIS', 'CNC', 'NRML'));
