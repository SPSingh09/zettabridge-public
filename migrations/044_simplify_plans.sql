-- Phase 0: two plans (free/pro), solo-only, live-only broker credentials.

-- Drop plan constraint before renaming values (old check allows individual/household, not pro).
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_plan_check;

UPDATE users SET plan = 'pro' WHERE plan = 'individual';
UPDATE users SET plan = 'free' WHERE plan IN ('household', 'enterprise');
UPDATE users SET plan = 'free' WHERE plan NOT IN ('free', 'pro');

ALTER TABLE users ADD CONSTRAINT users_plan_check CHECK (plan IN ('free', 'pro'));

UPDATE webhooks SET org_id = NULL WHERE org_id IS NOT NULL;
UPDATE broker_credentials SET org_id = NULL WHERE org_id IS NOT NULL;

UPDATE broker_credentials SET account_mode = 'live' WHERE account_mode IS NULL OR account_mode = 'demo';

ALTER TABLE broker_credentials DROP CONSTRAINT IF EXISTS broker_credentials_account_mode_check;
ALTER TABLE broker_credentials ADD CONSTRAINT broker_credentials_account_mode_check CHECK (account_mode = 'live');
