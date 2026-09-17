# PR 4B.1 — Stripe billing

> **Historical implementation plan.** Current product behavior is defined in [STATUS.md](../STATUS.md) and [scope.md](../01-product/scope.md). Last reviewed: 2026-07-07.

Plan for self-serve subscription checkout and plan sync, replacing manual admin plan changes for paid users while keeping admin override for comps and enterprise deals.

**Roadmap:** [roadmap.md](../roadmap.md#pr-4b1--stripe) · **Depends on:** [phase-5a-staging-infra.md](phase-5a-staging-infra.md) (public HTTPS + `APP_PUBLIC_URL`) · **Pricing reference:** [plans-and-billing.md](../plans-and-billing.md)

---

## Why this is needed

Today every plan change goes through platform admin:

```http
PUT /v1/admin/users/:id/plan
{ "plan": "pro" }
```

That works for closed beta and functional tests but blocks:

- **Self-serve upgrades** (Free → Paper → Pro → Pro Plus)
- **Closed beta at scale** (roadmap 5C.1 allows manual billing *or* Stripe)
- **Revenue** before dashboard MVP (4B.2) — checkout can redirect back to a minimal “billing success” page or Bruno until UI exists

Billing identity should match **verified email** (PR 3.1) so Stripe Customer email aligns with the ZettaBridge account.

---

## Goals (v1)

| Goal | v1 behavior |
|------|-------------|
| Products | Map 1:1 to `internal/plan`: Free (no SKU), Pro ($29/mo), Enterprise tiers starter/team/business ($99/$199/$299) |
| Checkout | Stripe Checkout Session (subscription mode) for upgrade paths |
| Portal | Stripe Customer Portal for payment method, cancel, plan change where Stripe supports it |
| Webhooks | Idempotent handler updates `users.plan` + `enterprise_tier`; sync org `seat_limit` for enterprise owners |
| Admin override | `PUT /v1/admin/users/:id/plan` remains; sets `billing_source=admin` so webhooks do not downgrade comp accounts |
| Dev/local | `BILLING_ENABLED=false` — checkout endpoints return 503 or skip registration; no Stripe keys required |
| Org members | Only **org owners** (or solo users) initiate checkout; invitees keep `billing_plan` unchanged |

### Non-goals (v1)

- Usage-based metering (order/sec is plan-capped in-app, not billed per order)
- Annual pricing / coupons (add Price IDs later)
- Dashboard billing UI (4B.2) — API + redirect URLs only
- Stripe Tax / invoicing customization
- Per-seat billing for org **members** (enterprise is per-org tier, not per invitee)

---

## Stripe catalog

Create in Stripe Dashboard (or CLI) with **metadata** on each Price for server-side mapping:

| Stripe Product | Price (monthly) | Metadata `plan` | Metadata `enterprise_tier` |
|----------------|-----------------|-----------------|--------------------------|
| ZettaBridge Pro | $29 | `pro` | — |
| ZettaBridge Enterprise Starter | $99 | `enterprise` | `starter` |
| ZettaBridge Enterprise Team | $199 | `enterprise` | `team` |
| ZettaBridge Enterprise Business | $299 | `enterprise` | `business` |

Env vars (price IDs, not amounts — amounts live in Stripe):

| Variable | Description |
|----------|-------------|
| `STRIPE_SECRET_KEY` | Secret API key |
| `STRIPE_WEBHOOK_SECRET` | Signing secret for `/v1/webhooks/stripe` |
| `STRIPE_PRICE_PRO` | Price ID for Pro |
| `STRIPE_PRICE_ENTERPRISE_STARTER` | Price ID for Enterprise starter |
| `STRIPE_PRICE_ENTERPRISE_TEAM` | Price ID for Enterprise team |
| `STRIPE_PRICE_ENTERPRISE_BUSINESS` | Price ID for Enterprise business |
| `BILLING_ENABLED` | `true` in staging/prod with keys set |
| `APP_PUBLIC_URL` | Already used for email links; success/cancel URLs for Checkout |

Free tier: no Stripe subscription; default on register.

---

## Data model

Migration `012_stripe_billing.sql`:

```sql
ALTER TABLE users
  ADD COLUMN stripe_customer_id     TEXT UNIQUE,
  ADD COLUMN stripe_subscription_id TEXT,
  ADD COLUMN billing_source         TEXT NOT NULL DEFAULT 'free'
    CHECK (billing_source IN ('free', 'stripe', 'admin'));

CREATE TABLE stripe_webhook_events (
  id           TEXT PRIMARY KEY,  -- Stripe event id (evt_...)
  type         TEXT NOT NULL,
  processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

| Field | Purpose |
|-------|---------|
| `stripe_customer_id` | Reuse across Checkout / Portal sessions |
| `stripe_subscription_id` | Latest active subscription; lookup on subscription webhooks |
| `billing_source` | `admin` = ignore Stripe-driven downgrades; `stripe` = webhook owns plan |
| `stripe_webhook_events` | Idempotency — skip duplicate deliveries |

**Plan application** reuses existing store helpers:

- `UpdateUserPlanAndTier(ctx, userID, plan, enterpriseTier)`
- `SyncOrgSeatLimitForOwner(ctx, ownerUserID, seatLimit)` when plan is enterprise

Extract shared logic from `adminSetUserPlan` into e.g. `internal/billing/apply_plan.go` so admin and Stripe both call the same validation (seat limit vs org size, valid tiers).

---

## HTTP API (new)

All billing routes under JWT + `CheckRevoked`, except the Stripe webhook.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/v1/billing/plans` | User | Public plan matrix + Stripe price hints (from config / static map) |
| `POST` | `/v1/billing/checkout` | User | Body: `{ "plan": "pro" }` or `{ "plan": "enterprise", "enterprise_tier": "team" }` → `{ "url": "https://checkout.stripe.com/..." }` |
| `POST` | `/v1/billing/portal` | User | Returns Customer Portal URL for existing `stripe_customer_id` |
| `POST` | `/v1/webhooks/stripe` | Stripe signature | Raw body; no JWT |

### Checkout rules

1. Reject if `!user.EmailVerified()` when `EMAIL_VERIFICATION_REQUIRED=true`
2. Reject if user is **org member but not owner** (effective plan is enterprise via org; billing is owner's problem)
3. Reject enterprise checkout if user already owns an org with `seats_used > newSeatLimit` (same conflict as admin plan change)
4. Create or retrieve Stripe Customer (`email`, `metadata[user_id]`)
5. Create Checkout Session: `mode=subscription`, `line_items=[{ price }]`, `success_url`, `cancel_url`, `client_reference_id=user_id`, subscription metadata `{ plan, enterprise_tier }`

### Portal rules

- Requires `stripe_customer_id`
- Return URL → `APP_PUBLIC_URL` + `/billing` (404 OK until 4B.2; use `/` or configurable `BILLING_RETURN_URL`)

---

## Webhook handler

**Endpoint:** `POST /v1/webhooks/stripe`  
**Middleware:** `stripe.ConstructEvent(body, Stripe-Signature, STRIPE_WEBHOOK_SECRET)` — reject 400 on bad signature.

Register route **before** `/v1` JWT group (like `/v1/webhook/:token` ingest).

### Events (v1)

| Event | Action |
|-------|--------|
| `checkout.session.completed` | Resolve `client_reference_id` → user; read subscription + price metadata → `applyPlan` |
| `customer.subscription.updated` | Map price → plan/tier; update user if `billing_source=stripe` |
| `customer.subscription.deleted` | Set plan `free`, clear `stripe_subscription_id`, clear `enterprise_tier` if was enterprise-only |
| `invoice.payment_failed` | Log + optional email; **v1:** no immediate downgrade (Stripe retries); document ops follow-up |

Skip processing if event ID exists in `stripe_webhook_events`. Insert event ID after successful apply (same transaction as plan update).

### Mapping price → plan

```go
func PlanFromStripePriceID(priceID string, cfg *config.Config) (plan, tier string, ok bool)
```

Read from env Price IDs; optionally fall back to Stripe Price metadata fetch (cache at startup).

---

## Admin override interaction

When admin calls `PUT /v1/admin/users/:id/plan`:

1. Apply plan via shared `applyPlan` (unchanged validation)
2. Set `billing_source = 'admin'`
3. Do **not** cancel Stripe subscription automatically in v1 (ops manual in Stripe Dashboard if needed) — document in runbook

When user completes Stripe Checkout:

1. Set `billing_source = 'stripe'`
2. Store customer + subscription IDs

When admin sets plan back to `free` on a Stripe user:

- v1: set `billing_source=admin` and leave subscription cancellation to ops, **or** call `subscription.Cancel` if `STRIPE_SECRET_KEY` set (optional stretch).

---

## Code layout

```
internal/billing/
  stripe_client.go    // thin wrapper around stripe-go
  checkout.go         // CreateCheckoutSession, CreatePortalSession
  webhook.go          // event dispatch + idempotency
  apply_plan.go       // shared with admin handler
  price_map.go        // env price ID → plan/tier
internal/handler/
  billing.go          // GET plans, POST checkout, POST portal
  stripe_webhook.go   // POST /v1/webhooks/stripe
internal/store/
  billing.go          // stripe columns, webhook_events CRUD
migrations/
  012_stripe_billing.sql
```

Dependency: `github.com/stripe/stripe-go/v79` (or current v79+).

Wire in `cmd/api/main.go`: pass Stripe client + config into handler when `BILLING_ENABLED`.

---

## Config validation

Extend `config.Validate()`:

- If `BILLING_ENABLED=true`: require `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, all four Price IDs, `APP_PUBLIC_URL` https
- If `BILLING_ENABLED=false`: Stripe vars optional (local/docker-compose unchanged)

---

## Testing

| Layer | What |
|-------|------|
| Unit | `PlanFromStripePriceID`, `applyPlan` seat-limit conflicts, webhook idempotency |
| Handler | Checkout rejects unverified email, org non-owner, invalid tier |
| Stripe CLI | `stripe listen --forward-to localhost:8080/v1/webhooks/stripe` + trigger fixtures |
| Functional | Optional `FUNCTIONAL_TEST_STRIPE=1` with test keys; default skip (like email verify) |
| Bruno | `Billing/checkout.bru`, `Billing/portal.bru`, failure mode free user 403 on enterprise without orgs_enabled |

Functional tests today use admin plan upgrade — **keep that path**; Stripe tests are additive.

---

## Deployment checklist (staging)

1. [ ] 5A: HTTPS ALB, `APP_PUBLIC_URL=https://staging.example.com`
2. [ ] Stripe test mode products + prices + metadata
3. [ ] Webhook endpoint in Stripe Dashboard → `https://staging.example.com/v1/webhooks/stripe`
4. [ ] Secrets Manager: `STRIPE_*` keys (see [phase-5a-staging-infra.md](phase-5a-staging-infra.md))
5. [ ] Run migration `012_stripe_billing.sql`
6. [ ] Smoke: register → verify email → checkout Pro → webhook → `GET /v1/me` shows `plan: pro`, `billing_source: stripe`
7. [ ] Enterprise: owner checkout team tier → org `seat_limit` 12

---

## Implementation phases

### Phase A — Foundation (1–2 days)

- [x] Migration + store methods
- [x] `internal/billing/apply_plan` extracted from admin handler
- [x] Config + env examples (`deploy/env/runtime.env.example`)
- [x] `GET /v1/billing/plans`

### Phase B — Checkout + Portal (1–2 days)

- [x] Stripe client + checkout/portal handlers
- [x] Email-verified + org-owner guards
- [x] Bruno requests

### Phase C — Webhooks (1–2 days)

- [x] Signature verification route
- [x] `checkout.session.completed`, `subscription.updated`, `subscription.deleted`
- [x] Idempotency table
- [x] Stripe CLI manual test doc in [testing.md](../testing.md)

### Phase D — Hardening (1 day)

- [x] Admin `billing_source=admin` on manual plan set
- [x] Metrics: `billing_checkout_total`, `billing_webhook_total{status}`
- [x] Ops note in [operations-security.md](../operations-security.md) (refund, dispute, comp accounts)
- [x] Roadmap checkbox 4B.1

**Total estimate:** L (≈5–7 dev days) per roadmap.

---

## Edge cases

| Case | Behavior |
|------|----------|
| User downgrades Pro → Free in Portal | Webhook sets `plan=free`; webhooks/creds over free limits remain but **new** creates blocked (existing plan enforcement) |
| Enterprise owner downgrades with active org | Block at Checkout if seats_used > target tier; webhook downgrade same as admin conflict |
| User in org (non-owner) hits checkout | 403 — effective enterprise via membership |
| Duplicate webhook | Idempotent on `evt_*` |
| Checkout completed but webhook delayed | UI shows “processing”; poll `GET /v1/me` or 4B.2 success page |
| `BROKER_MODE=live` + free after cancel | Existing free-tier routing rules apply |

---

## Product decisions (locked)

| Question | Decision |
|----------|----------|
| Success URL | Yes — `{APP_PUBLIC_URL}/billing/success?session_id={CHECKOUT_SESSION_ID}` (override via `BILLING_SUCCESS_URL`) |
| Enterprise before org | Yes — checkout allowed before `POST /v1/orgs`; seat limit from tier applies on org create |
| Cancel subscription | **Immediate** downgrade to free (not end-of-period); see `billing.CancelImmediate` |

---

## Open questions (Phase B+)

1. **Tax** — Stripe Tax off for v1; revisit for GA.

---

## Related files (today)

| Area | File |
|------|------|
| Plan limits | `internal/plan/plan.go`, `enterprise_tier.go` |
| Admin plan set | `internal/handler/admin.go` → `adminSetUserPlan` |
| Effective plan / org | `internal/plan/effective.go`, `GET /v1/me` |
| Pricing doc | `docs/plans-and-billing.md` |
