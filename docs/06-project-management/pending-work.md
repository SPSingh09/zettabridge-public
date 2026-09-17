# Pending Work Register
**Last updated: 2026-07-07**

All remaining items grouped by priority tier. See [STATUS.md](../STATUS.md) for current product snapshot and [milestones.md](milestones.md) for the full table with IDs, owners, and dependencies.

---

## Legend

| Status | Meaning |
|--------|---------|
| **Blocked** | Cannot start until a specific external action is completed |
| **Planned** | Ready to start; no external blocker |
| **In Progress** | Actively being worked on |

---

## Tier 1 — Beta Blockers (must complete before opening to real traders)

### Application wiring on AWS staging (Beta Phase 3)

| # | Item | Status | Blocker / Notes |
|---|------|--------|-----------------|
| B3.1 | Add `https://api.staging.zettabridge.net/v1/billing/stripe-webhook` as endpoint in Stripe Dashboard; copy new `STRIPE_WEBHOOK_SECRET` to Secrets Manager | Planned | Low priority while billing is disabled; complete before enabling `BILLING_ENABLED=true` |
| B3.2 | Verify JWT login / refresh flow works end-to-end on AWS staging domain | Planned | Manual browser test on `app.staging.zettabridge.net` |
| B3.3 | Run functional test suite against AWS staging: `FUNCTIONAL_TEST_BASE_URL=https://api.staging.zettabridge.net go test -tags functional ./...` | Planned | Requires B3.2 to pass first |
| B3.4 | Manual smoke checklist: register → verify → login → paper account → webhook → credential → trade → billing | Planned | See beta-phase-3-app-wiring.md for full checklist |

### Live broker flip (Beta Phase 4)

| # | Item | Status | Blocker / Notes |
|---|------|--------|-----------------|
| B4.1 | Whitelist NAT EIP in Angel One developer portal (App settings → IP Whitelist) | **Blocked** | Must be done manually in Angel One portal; no code change needed |
| B4.2 | Whitelist NAT EIP in Dhan developer portal | **Blocked** | Must be done manually in Dhan portal; no code change needed |
| B4.3 | Update ECS task definition: `BROKER_MODE=live`, `BROKER_ANGEL_CLIENT_LOCAL_IP`, `BROKER_ANGEL_CLIENT_PUBLIC_IP`, `BROKER_ANGEL_MAC_ADDRESS` | Blocked | Depends on B4.1 and B4.2; Zerodha staging uses zerodha-adapter sidecar |
| B4.4 | Create a real **Pro**-plan credential with live Angel, Dhan, or Zerodha API keys; verify returns `{"valid": true}` | Blocked | Depends on B4.3 |
| B4.5 | Live smoke test: place a minimal real order, confirm `status=submitted` + real broker order ID in CloudWatch and broker portal | Blocked | Depends on B4.4; blocked on broker IP whitelist / real creds for Kite |
| B4.6 | Verify SL/TP bracket order end-to-end on live broker | Blocked | Depends on B4.5 |
| B4.7 | Confirm SEBI Algo-ID appears in broker order details (sandbox validation) | Blocked | Depends on B4.3; required for P3.2.S below |
| B4.8 | Ops drills: rotate webhook token (old token 403, new token accepted), suspend/unsuspend test user, ECS force-redeploy | Planned | Can run in parallel after B4.3 |
| B4.9 | Sign off Beta Go/No-Go checklist in `docs/plans/beta-phase-4-live-broker-launch.md` | Blocked | Depends on B4.5, B4.7, B4.8 |

### SEBI Algo-ID

| # | Item | Status | Blocker / Notes |
|---|------|--------|-----------------|
| P3.2.S | Sandbox validation of Algo-ID on at least one live Indian broker before opening to traders | Blocked | Depends on B4.3 (BROKER_MODE=live + IP whitelist) |

### Execution adapter split (partial — see STATUS.md)

| # | Item | Status | Blocker / Notes |
|---|------|--------|-----------------|
| AD.1 | Enable Angel/Dhan/MT5 in `enabled_adapters` + provision ECS sidecars in Terraform | Planned | Code exists in `livebrokers/`; not in default staging `paper,zerodha` |
| AD.2 | Regenerate OpenAPI — remove stale `/v1/orgs/*` paths from swagger | Planned | Router is authoritative; swagger out of sync |
| AD.3 | Update or archive repo-root `ADAPTER_SPLIT.md` (describes old 2-plan model) | Planned | Superseded by STATUS.md + migration 046 |

### Documentation for beta

| # | Item | Status | Blocker / Notes |
|---|------|--------|-----------------|
| DOC.1 | Broker setup guide: how users register API keys with Angel One, Dhan, Zerodha, MT5 and add to ZettaBridge | Planned | Update `docs/04-integrations/` files |
| DOC.2 | Known limitations doc: Zerodha per-user API key constraint, SEBI Algo-ID requirement, manual token refresh, no payment gateway for beta | Planned | |
| DOC.3 | Update `docs/04-integrations/zerodha.md` with per-user API key requirement and daily token refresh flow | Planned | |
| DOC.4 | Dashboard credential form: add helper text for Zerodha explaining per-user API key requirement | Planned | Small UI copy change |
| DOC.5 | Incident rollback path documented: revert to `BROKER_MODE=mock` in under 5 minutes | Planned | |
| DOC.6 | Support channel set up and ready | Planned | |

---

## Tier 2 — Security Hardening (required before public GA)

| # | Item | Status | Notes |
|---|------|--------|-------|
| S1 | Add security response headers: `Strict-Transport-Security`, `X-Content-Type-Options`, `X-Frame-Options` (Fiber middleware) | Planned | Internal pre-pen-test hardening |
| S2 | Confirm all `secret` / `key` / `password` fields are scrubbed from logs | Planned | Audit `internal/livebrokers/httpclient/` redaction rules |
| S3 | Verify rate limiting covers `/v1/auth/login`, `/v1/auth/register`, and webhook ingest endpoints | Planned | |
| S4 | Load test: 500 concurrent webhook ingest requests over 60s, p99 < 100ms (`k6 run --vus 500 --duration 60s deploy/loadtest/k6/webhook-ingest.js`) | Planned | Requires AWS staging; run after beta opens |
| S5 | Penetration test: JWT forgery, token replay, IDOR on webhooks/creds, ingest flood, SSRF, Stripe webhook bypass, TLS cipher review | Planned | Requires S1–S3 done first |
| S6 | AWS KMS envelope encryption: replace raw `AES_KEY` env var with KMS-wrapped DEK; add `key_version` column to `broker_credentials` | Planned | See `internal/credenc`; requires KMS CMK already in place |
| S7 | Credential versioning + zero-downtime rotation (dual DEK window) | Planned | Depends on S6 |

---

## Tier 3 — Production Hardening (required for public GA)

| # | Item | Status | Notes |
|---|------|--------|-------|
| G1 | CloudWatch alarms wired and tested (all 8 types: order rejection, queue depth, 5xx, p95 latency, ECS unhealthy, RDS CPU, Redis memory, broker auth failure) | Planned | |
| G2 | AWS WAF on ALB: `AWSManagedRulesCommonRuleSet`, `AWSManagedRulesKnownBadInputsRuleSet`, custom rate limits on login and ingest | Planned | |
| G3 | CI/CD pipeline: test → build (ECR push with commit SHA tag) → staging-deploy → staging-functional-test → manual approve → prod blue/green via CodeDeploy | Planned | |
| G4 | Blue/green deploy tested: 10% traffic increments, auto-rollback on health check failure | Planned | Depends on G3 |
| G5 | Disaster recovery drill: RDS PITR restore to test subnet, confirm app connects and migrations at expected version; document RTO < 1 hour | Planned | |
| G6 | Redis outage behavior documented: cache-only, rate limits and dedup reset gracefully | Planned | |
| G7 | Terraform IaC for production environment (parity with staging modules, S3 state backend + DynamoDB lock) | Planned | |
| G8 | Oncall runbooks: ORDER_REJECTED spike, QUEUE_DEPTH spike, 5XX spike, BROKER_MODE=mock rollback | Planned | |

---

## Tier 4 — Dashboard Remaining Features

| # | Item | Status | Notes |
|---|------|--------|-------|
| D2 | P&L time-series charts (recharts component exists; needs trade history timestamps added to API) | Planned | |
| D3 | Platform admin read-only ops view | Planned | |

---

## Tier 5 — Payment Gateway (blocked externally)

| # | Item | Status | Blocker |
|---|------|--------|---------|
| PAY.1 | Register ZettaBridge as a legal business entity in India | **Blocked** | Business / legal process |
| PAY.2 | Apply for Stripe India onboarding (invite-only) | **Blocked** | Depends on PAY.1 |
| PAY.3 | Enable `BILLING_ENABLED=true` + configure `STRIPE_PRICE_PAPER`, `STRIPE_PRICE_PRO`, `STRIPE_PRICE_PRO_PLUS` in Terraform tfvars | **Blocked** | Depends on PAY.1, PAY.2 |
| PAY.4 | End-to-end billing smoke test: register → checkout → Stripe webhook → plan updated | **Blocked** | Depends on PAY.3 |

**Current workaround for beta:** Admin manually assigns plan via `/admin` panel using `billing_source=admin`. This is sufficient for the closed beta.

---

## Tier 6 — Deferred Post-GA

| # | Item | Notes |
|---|------|-------|
| DEF.1 | Zerodha WebSocket tick feed (real-time LTP cache for SL/TP) | REST polling sufficient for beta |
| DEF.2 | Full mark-to-market P&L | Requires position-sync beyond trades table |
| DEF.3 | Automatic broker OAuth token refresh | Manual PUT documented; v1 is sufficient |
| DEF.4 | NATS / Kafka message queue | In-process queue meets scale targets through GA |
| DEF.5 | Mobile-responsive dashboard polish | Post-launch UX iteration |
| DEF.6 | Richer P&L analytics (Sharpe ratio, drawdown) | Post-launch product iteration |
| DEF.7 | Multi-region AWS deployment | Only when latency or compliance demands it |
| DEF.8 | Per-user trade alert webhooks / notifications | Post-GA feature |
