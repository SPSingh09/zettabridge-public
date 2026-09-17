# Database Design
**Last updated: 2026-07-07**

PostgreSQL 15 (AWS RDS). Schema evolves through numbered migration files in `migrations/`. Applied automatically at startup via `embed.FS`.

**Latest migration:** `046_expand_plan_tiers.sql` — four-plan CHECK constraint (`free`, `paper`, `pro`, `pro_plus`).

Canonical plan limits: `internal/plan/plan.go`, [STATUS.md](../STATUS.md).

---

## Schema Overview

```
users ──────────────────────────────────────────────────────────────────┐
  ├── paper_accounts (user_id FK)                                       │
  │     ├── paper_orders, paper_positions, paper_trades                 │
  │     └── paper_account_snapshots                                     │
  │                                                                      │
  ├── broker_credentials (user_id FK; org_id legacy nullable)           │
  │     └── webhooks (broker_cred_id OR paper_account_id — XOR)         │
  │           └── trades (webhook_id FK)                                 │
  │                                                                      │
  ├── refresh_tokens (user_id FK)                                        │
  ├── email_verifications (user_id FK)                                   │
  └── stripe_webhook_events (idempotency)                                │

organizations / org_members / org_invites / org_audit_log  (LEGACY ONLY)
  └── Org product removed from API/UI (migration 044 detached org_id).
      Tables retained for existing data; no active `/v1/orgs/*` routes.
```

---

## Table Definitions (current state after migration 046)

### `users`

Primary account table. Created in migration 001; plan CHECK updated in 044 and **046**.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK, DEFAULT gen_random_uuid() | UUID v4 |
| `email` | TEXT | UNIQUE, NOT NULL | |
| `password_hash` | TEXT | NOT NULL | bcrypt |
| `plan` | TEXT | NOT NULL, DEFAULT 'free', CHECK IN (`free`,`paper`,`pro`,`pro_plus`) | Migration **046** |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | |
| `role` | TEXT | NOT NULL, DEFAULT 'user', CHECK ('user','admin') | Platform admin flag |
| `status` | TEXT | NOT NULL, DEFAULT 'active', CHECK ('active','suspended') | Compliance gate |
| `orgs_enabled` | BOOLEAN | NOT NULL, DEFAULT false | **Legacy** — org product removed |
| `email_verified_at` | TIMESTAMPTZ | nullable | NULL = unverified |
| `stripe_customer_id` | TEXT | UNIQUE, nullable | Set on first Stripe checkout |
| `stripe_subscription_id` | TEXT | nullable | Current active subscription |
| `billing_source` | TEXT | NOT NULL, DEFAULT 'free', CHECK ('free','stripe','admin') | 'admin' = admin override; takes precedence |

**Indexes:** `idx_users_role`, `idx_users_status`

**Key constraints:**
- `billing_source = 'admin'` is not overwritten by Stripe webhook events.
- `email_verified_at` is back-filled to `created_at` for users created before migration 012 (so existing users are not locked out).

---

### `broker_credentials`

Encrypted **live** broker credentials per user. One row per credential set. Never returns `raw_creds` via API.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK, DEFAULT gen_random_uuid() | UUID v4 |
| `user_id` | TEXT | NOT NULL, FK → users(id) ON DELETE CASCADE | |
| `org_id` | TEXT | nullable, FK → organizations(id) | **Legacy** — nulled in migration 044 |
| `broker_type` | TEXT | NOT NULL | 'zerodha', 'angel', 'dhan', 'mt5_cloud' |
| `encrypted_creds` | TEXT | NOT NULL | AES-256-GCM base64 ciphertext of `raw_creds` |
| `account_label` | TEXT | NOT NULL, DEFAULT '' | User-visible display name |
| `account_mode` | TEXT | NOT NULL, DEFAULT 'live', CHECK (`live` only) | Demo removed in migration **044** |
| `created_by` | TEXT | nullable, FK → users(id) | Added migration 005 |
| `exchange` | TEXT | NOT NULL, DEFAULT '' | e.g. 'NSE', 'BSE' |
| `product` | TEXT | NOT NULL, DEFAULT 'MIS' | 'MIS', 'CNC', 'NRML' (multi-product on pro_plus) |
| `algo_id` | TEXT | NOT NULL, DEFAULT '' | SEBI Algo-ID (migration 013) |
| `status` | TEXT | NOT NULL, DEFAULT 'active', CHECK ('active','paused') | Migration 021 |
| `auto_paused` | BOOLEAN | NOT NULL, DEFAULT false | System-paused on plan downgrade (migration 021) |
| `order_type` | TEXT | NOT NULL, DEFAULT 'MARKET', CHECK ('MARKET','LIMIT') | Migration 022 |
| `market_protection` | NUMERIC(5,2) | NOT NULL, DEFAULT 0 | Zerodha market order % protection |

**Indexes:** `idx_broker_creds_user`, `idx_broker_creds_org`

**Key constraints:**
- `encrypted_creds` column name in DB; Go model field is `RawCreds` with `json:"-"` tag.
- `algo_id` empty string means no Algo-ID set; fail-close triggers on empty when `SEBI_ALGO_ID_REQUIRED=true`.
- `auto_paused=true` means the system paused it (e.g. plan downgrade or org creation); user can see targeted warning.

---

### `webhooks`

One webhook = one ingest URL token + exactly one destination: **live credential** or **paper account** (XOR constraint, migration 036).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK, DEFAULT gen_random_uuid() | UUID v4 |
| `user_id` | TEXT | NOT NULL, FK → users(id) ON DELETE CASCADE | |
| `org_id` | TEXT | nullable | **Legacy** — nulled in migration 044 |
| `token` | TEXT | UNIQUE, NOT NULL | UUID v4 ingest URL token |
| `label` | TEXT | NOT NULL, DEFAULT '' | User-visible name |
| `status` | TEXT | NOT NULL, DEFAULT 'active' | 'active' or 'paused' |
| `broker_cred_id` | TEXT | nullable, FK → broker_credentials(id) | Live webhooks (pro/pro_plus) |
| `paper_account_id` | TEXT | nullable, FK → paper_accounts(id) | Paper webhooks (all plans) |
| `symbol` | TEXT | NOT NULL | Default symbol (can be overridden by signal) |
| `lot_size` | NUMERIC(10,4) | NOT NULL, DEFAULT 0.01 | Default lot size |
| `max_risk_pct` | NUMERIC(5,2) | NOT NULL, DEFAULT 1.0 | Risk guard |
| `sl_points` | INTEGER | NOT NULL, DEFAULT 20 | Default stop-loss offset |
| `tp_points` | INTEGER | NOT NULL, DEFAULT 30 | Default take-profit offset |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | |
| `allowed_actions` | TEXT[] | NOT NULL, DEFAULT '{BUY,SELL,CLOSE}' | Action guard |
| `allowed_symbols` | TEXT[] | NOT NULL, DEFAULT '{}' | Symbol guard (empty = allow all) |
| `max_lot_size` | NUMERIC(10,4) | NOT NULL, DEFAULT 0 | Lot size guard (0 = no limit) |
| `allow_symbol_override` | BOOLEAN | NOT NULL, DEFAULT true | Allow signal to override webhook symbol |
| `allow_lot_override` | BOOLEAN | NOT NULL, DEFAULT true | |
| `allow_sl_override` | BOOLEAN | NOT NULL, DEFAULT true | |
| `allow_tp_override` | BOOLEAN | NOT NULL, DEFAULT true | |
| `rate_limit_per_sec` | INTEGER | NOT NULL, DEFAULT 0 | Per-webhook rate limit (0 = unlimited) |
| `timezone` | TEXT | NOT NULL, DEFAULT 'UTC' | Trading hours timezone |
| `trading_hours` | JSONB | NOT NULL, DEFAULT 'null' | Optional trading hours window |
| `required_comment` | TEXT | NOT NULL, DEFAULT '' | Signal must contain this comment to be accepted |
| `dedup_window_sec` | INTEGER | NOT NULL, DEFAULT 0 | Signal dedup TTL in seconds (0 = disabled) |
| `created_by` | TEXT | nullable, FK → users(id) | |
| `auto_paused` | BOOLEAN | NOT NULL, DEFAULT false | System-paused (migration 021) |

**Indexes:** `idx_webhooks_user`, `idx_webhooks_token` (hot path), `idx_webhooks_org`, `idx_webhooks_org_created`

**Key constraint:** `(broker_cred_id IS NOT NULL AND paper_account_id IS NULL) OR (broker_cred_id IS NULL AND paper_account_id IS NOT NULL)` — exactly one execution target.

---

### `paper_accounts`

Simulated trading accounts (migration 034). Paper webhooks route here instead of live broker credentials.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUID v4 |
| `user_id` | TEXT | NOT NULL, FK → users(id) | |
| `label` | TEXT | NOT NULL | Display name |
| `market_profile` | TEXT | NOT NULL, DEFAULT 'indian_equity' | indian_equity, indian_fno, forex, crypto_spot |
| `base_currency` | TEXT | NOT NULL, DEFAULT 'INR' | |
| `starting_balance` | NUMERIC(18,8) | NOT NULL, >= 0 | |
| `cash_balance` | NUMERIC(18,8) | NOT NULL | Current cash |
| `status` | TEXT | NOT NULL, CHECK ('active','paused','closed') | |
| `created_at` / `updated_at` | TIMESTAMPTZ | NOT NULL | |

Related tables: `paper_orders`, `paper_positions`, `paper_trades`, `paper_account_snapshots` (added in later migrations).

Plan limits enforced in application layer (`internal/plan/plan.go`): free 1, paper 3, pro 5, pro_plus 10 accounts.

---

### `trades`

Immutable audit log. One row per processed signal. Append-only — never updated after insert.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK, DEFAULT gen_random_uuid() | UUID v4 |
| `webhook_id` | TEXT | NOT NULL, FK → webhooks(id) ON DELETE CASCADE | |
| `signal` | TEXT | NOT NULL | BUY \| SELL \| CLOSE |
| `symbol` | TEXT | NOT NULL | Resolved symbol at execution time |
| `lot_size` | NUMERIC(10,4) | nullable | |
| `broker_order` | TEXT | NOT NULL, DEFAULT '' | Broker-assigned order ID (empty on rejection) |
| `status` | TEXT | NOT NULL | 'queued', 'placed', 'filled', 'rejected', 'cancelled' |
| `fill_price` | NUMERIC(18,8) | NOT NULL, DEFAULT 0 | Execution price (0 if unavailable) |
| `error` | TEXT | NOT NULL, DEFAULT '' | Human-readable error message |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | |
| `error_code` | TEXT | NOT NULL, DEFAULT '' | Structured error code (migration 009) |
| `signal_key` | TEXT | NOT NULL, DEFAULT '' | Dedup hash key (migration 010) |
| `comment` | TEXT | NOT NULL, DEFAULT '' | Signal comment field |
| `algo_id` | TEXT | NOT NULL, DEFAULT '' | SEBI Algo-ID snapshot at execution (migration 013) |

**Indexes:** `idx_trades_webhook`, `idx_trades_created DESC`, `idx_trades_webhook_signal_key` (WHERE signal_key <> '')

**Key constraints:**
- `algo_id` is snapshotted from the credential at execution time for audit trail — not a FK, so it persists even if the credential is later updated or deleted.
- `error_code` is a stable enum string (defined in `internal/brokererr`). `error` is a human-readable message (may vary).
- Trades are never updated or deleted (cascade delete only happens when the parent webhook is deleted).

---

### `refresh_tokens`

Server-side session tracking for JWT refresh and logout. One row per active session.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | Session ID |
| `user_id` | TEXT | NOT NULL, FK → users(id) ON DELETE CASCADE | |
| `token_hash` | TEXT | NOT NULL, UNIQUE | SHA-256 of the raw refresh token |
| `expires_at` | TIMESTAMPTZ | NOT NULL | |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | |
| `revoked_at` | TIMESTAMPTZ | nullable | Set on logout |

**Indexes:** `idx_refresh_tokens_user`, `idx_refresh_tokens_hash`

---

### `email_verifications`

Verification token per user. One active token at a time (old tokens left in table until consumed or expired).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | |
| `user_id` | TEXT | NOT NULL, FK → users(id) ON DELETE CASCADE | |
| `token_hash` | TEXT | NOT NULL, UNIQUE | SHA-256 of the raw email token |
| `expires_at` | TIMESTAMPTZ | NOT NULL | |
| `consumed_at` | TIMESTAMPTZ | nullable | Set when user clicks verify link |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | |

**Indexes:** `idx_email_verifications_user` (WHERE consumed_at IS NULL)

---

### `organizations` (legacy)

**Org product removed from API/UI.** Tables retained for historical data only. Migration 044 set `org_id = NULL` on webhooks and credentials.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUID v4 |
| `name` | TEXT | NOT NULL | Display name |
| `slug` | TEXT | NOT NULL, UNIQUE | URL-safe identifier |
| `owner_user_id` | TEXT | NOT NULL, FK → users(id) | |
| `seat_limit` | INTEGER | NOT NULL, DEFAULT 5 | Legacy |
| `status` | TEXT | NOT NULL, DEFAULT 'active' | |
| `created_at` | TIMESTAMPTZ | NOT NULL | |

### `org_members` (legacy)

Many-to-many between users and organizations. No active API access.

### `org_invites` (legacy)

Email-based org invitations. No active API access.

### `org_audit_log` (legacy)

Append-only audit trail of org admin actions. No active API access.

---

### `stripe_webhook_events`

Idempotency table for Stripe webhook events. Prevents double-processing on Stripe retries.

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | Stripe event ID (`evt_...`) — used as idempotency key |
| `type` | TEXT | Stripe event type (e.g. `checkout.session.completed`) |
| `processed_at` | TIMESTAMPTZ | |

---

## Migration History

| Migration | Key change |
|-----------|-----------|
| 001_init | Base schema: users, broker_credentials, webhooks, trades |
| 002_account_mode | broker_credentials.account_mode |
| 002_admin_users | users.role, users.status |
| 002_webhook_guards | Webhook guard columns (allowed_actions, allowed_symbols, rate_limit_per_sec, etc.) |
| 003_auth_sessions | refresh_tokens table |
| 003_organizations | organizations, org_members tables; org_id on webhooks and credentials |
| 003_plan_check | Updated plan CHECK constraint |
| 004_org_invites | org_invites table with pending-unique index |
| 005_e3_resources | created_by on webhooks and credentials |
| 006_org_audit | org_audit_log table |
| 007_trades_webhook_cascade | trades.webhook_id cascade delete fix |
| 008_broker_cred_exchange_product | exchange, product columns on credentials |
| 009_trade_error_code | trades.error_code |
| 010_trade_idempotency | webhooks.dedup_window_sec; trades.signal_key, trades.comment |
| 011_enterprise_tier_early_bird | Enterprise tier + early-bird columns |
| 012_email_verification | users.email_verified_at; email_verifications table |
| 013_sebi_algo_id | broker_credentials.algo_id; trades.algo_id |
| 014_stripe_billing | users.stripe_customer_id/subscription_id/billing_source; stripe_webhook_events table |
| 015_platform_invites | Platform-level invite codes for closed registration |
| 016_sltp_audit | SL/TP fields on trades for audit |
| 017_cancel_order | Cancel order support on trades |
| 018_fix_cancel_error_not_null | Fix NOT NULL on cancel-related error fields |
| 019_zerodha_connected_at | Zerodha OAuth connection timestamp on credentials |
| 020_plan_rename | pro → individual, enterprise → household; drop enterprise_tier |
| 021_credential_status | broker_credentials.status, auto_paused; webhooks.auto_paused |
| 022_credential_order_type | broker_credentials.order_type, market_protection |
| 023_migrate_owner_resources_to_org | Backfill org_id on owner/admin webhooks and credentials |
| 024_pause_owner_individual_resources | Undo 023 for owners on join; set auto_paused=true instead |
| 034_paper_trading | paper_accounts, paper_orders, paper_positions, paper_trades |
| 036_webhook_paper_destination | webhooks.paper_account_id; XOR with broker_cred_id |
| 043–045 | Symbol requests, market profile, paper symbol normalization |
| **044_simplify_plans** | Removed individual/household; detached org_id; **live-only** account_mode |
| **046_expand_plan_tiers** | Four-plan CHECK: free, paper, pro, pro_plus |

> Migrations 025–033 and others exist between 024 and 034; see `migrations/` directory for full list.

---

## Design Decisions

- **TEXT PKs (UUID strings)** — UUIDs generated by `pgcrypto.gen_random_uuid()` at DB level. No auto-increment integers to avoid enumeration attacks.
- **Append-only trades** — `trades` is never updated post-insert. Cancellations create a new row with status='cancelled'. This guarantees a complete audit trail.
- **`raw_creds` never in API** — The Go model has `json:"-"` on the encrypted field. A separate `RawCreds` form field is accepted on POST/PUT but never returned on GET.
- **Additive migrations only** — All migrations use `ADD COLUMN IF NOT EXISTS`. No `DROP COLUMN` without a preceding deprecation migration. This allows rolling deploys.
- **Paper vs live webhooks** — XOR constraint on `broker_cred_id` / `paper_account_id`; paper trading does not use broker credentials.
- **Live-only credentials** — `account_mode` CHECK is `live` only (migration 044); users practice on paper accounts instead of demo credentials.
- **Legacy org tables** — Retained for data integrity; org product removed from router and dashboard (2026).
- **Algo-ID snapshot** — `trades.algo_id` is a TEXT snapshot, not a FK to credentials. Preserves the Algo-ID used at execution time even if the credential is later updated.
