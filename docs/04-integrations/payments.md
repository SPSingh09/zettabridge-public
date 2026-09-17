# Billing and Plans
**Last updated: 2026-07-07**

ZettaBridge supports **four plans** (`free`, `paper`, `pro`, `pro_plus`). Stripe integration is implemented but **disabled by default** (`BILLING_ENABLED=false`) until payment gateway is enabled in production.

Source of truth: `internal/plan/plan.go`, `internal/billing/plans.go`, `GET /v1/billing/plans`.

---

## 1. Plans

| Plan | Paper accounts | Paper webhooks | Live webhooks | Live creds | Paper trades/mo | Live | Orders/sec | Audit | Guards | Notifications | Multi-product |
|------|----------------|----------------|---------------|------------|-----------------|------|------------|-------|--------|---------------|---------------|
| **free** | 1 | 1 | 0 | 0 | 10 | No | 1 | No | No* | No | No |
| **paper** | 3 | 5 | 0 | 0 | ∞ | No | 5 | Yes | Yes | Yes | No |
| **pro** | 5 | 8 | 1 | 1 | ∞ | Yes | 10 | Yes | Yes | Yes | No |
| **pro_plus** | 10 | 15 | 5 | 3 | ∞ | Yes | 10 | Yes | Yes | Yes | Yes |

\*Free tier: basic symbol/action guards only; advanced guards (dedup, rate limits, trading hours) require paid plan on **live** webhooks. Paper webhooks on paid plans get advanced guards without live trading.

**Free daily cap:** 100 webhook ingests per user per UTC day.

**Checkout:** `paper`, `pro`, `pro_plus` only (`POST /v1/billing/checkout`). Free is the default registration tier.

---

## 2. Execution vs billing

| Question | Answer |
|----------|--------|
| Can Free users trade live? | **No** — `LiveAllowed=false`; cannot create live broker credentials or live webhooks. |
| What is `BROKER_MODE=mock`? | **Server** env for staging/CI — simulated broker fills for **paid** live webhooks. Not a Free-tier feature. |
| Is `account_mode=demo` supported? | **No** — only `live` credentials (migration 044). Use **paper webhooks** for simulation. |
| Org / Household billing? | **Removed** — solo accounts only. |

---

## 3. Admin plan override

```http
PUT /v1/admin/users/:id/plan
Authorization: Bearer <admin JWT>
Content-Type: application/json

{"plan": "pro_plus"}
```

Valid values: `free`, `paper`, `pro`, `pro_plus`. Sets `billing_source=admin`. Used for beta pilots, load-test users, and manual upgrades.

---

## 4. Stripe integration

When enabling billing:

```bash
BILLING_ENABLED=true
STRIPE_SECRET_KEY=sk_live_...
STRIPE_WEBHOOK_SECRET=whsec_...
STRIPE_PRICE_PAPER=price_...
STRIPE_PRICE_PRO=price_...
STRIPE_PRICE_PRO_PLUS=price_...
```

Legacy alias: `STRIPE_PRICE_INDIVIDUAL` maps to `STRIPE_PRICE_PRO` in config.

Webhook endpoint: `POST /v1/billing/stripe/webhook` (signature verified).

Customer portal: `POST /v1/billing/portal` (JWT).

---

## 5. Plan API

```http
GET /v1/billing/plans
Authorization: Bearer <JWT>
```

Returns `billing_enabled`, checkout URLs, and per-plan limits (`max_paper_webhooks`, `max_live_webhooks`, `max_brokers`, `live_trading`, etc.).

User profile: `GET /v1/me` includes `plan`, `billing_source`, `live_trading_allowed`, and limit fields for UI gating.

---

## 6. Operational notes

- Load-test user is upgraded to **pro_plus** by `deploy/loadtest/setup-loadtest-webhook.sh` (falls back to **pro** if migration 046 not applied).
- Downgrade does not delete excess webhooks automatically — user must delete over-limit resources before downgrade succeeds (API enforcement on create, not retroactive delete).

See [runbook.md](../05-operations/runbook.md) for billing incident handling.
