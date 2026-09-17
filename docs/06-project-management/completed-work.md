# Completed Work Register
**Last updated: 2026-07-07**

Verified record of all shipped work, organized by phase in delivery order. Evidence column points to where the work can be confirmed independently.

---

## Phase 0 — Product Simplification (2026-07)

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| P0.1 | Four-plan model: Free, Paper, Pro, Pro Plus (`internal/plan/plan.go`, migration `046`) | 2026-07 | migration 046, plan tests |
| P0.2 | Removed `/v1/orgs/*` from router; org pages redirect in dashboard | 2026-07 | `internal/transport/http/router.go` |
| P0.3 | Removed `account_mode=demo`; credentials live-only (migration `044`) | 2026-07 | migration 044 |
| P0.4 | Renamed Individual/Household → Pro/Pro Plus (migration `044`) | 2026-07 | migration 044 |
| P0.5 | Paper accounts API + paper webhook destination type | 2026-07 | paper accounts handlers, dashboard |
| P0.6 | Advanced webhook guards gated on paid plans for **live** webhooks; paper exempt | 2026-07 | guard middleware, plan gates |
| P0.7 | Stripe env vars: `STRIPE_PRICE_PAPER`, `STRIPE_PRICE_PRO`, `STRIPE_PRICE_PRO_PLUS` | 2026-07 | internal/config |

---

## Phase 1 — Core Pipeline and Foundation

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| P1.1 | Webhook ingest endpoint (`POST /v1/webhook/:token`) with TradingView JSON parsing | 2026-06 | Bruno collection, functional tests |
| P1.2 | In-process Redis-backed queue with configurable worker pool (`WORKER_COUNT`, `QUEUE_BUFFER`) | 2026-06 | docker-compose, functional tests |
| P1.3 | Queue worker: signal validation → credential lookup → broker dispatch → trade insert | 2026-06 | functional tests |
| P1.4 | Mock broker factory for all broker types (zero-risk CI and local testing) | 2026-06 | functional tests |
| P1.5 | JWT authentication (`POST /v1/auth/login`) with access + refresh token pair | 2026-06 | Bruno auth collection |
| P1.6 | Refresh token rotation (`POST /v1/auth/refresh`) | 2026-06 | Bruno auth collection |
| P1.7 | AES-256-GCM encryption of broker credentials at rest (`internal/credenc`) | 2026-06 | credenc unit tests |
| P1.8 | Webhook token guards: `allowed_symbols`, `allowed_actions`, `rate_limit_per_sec` | 2026-06 | Bruno failure modes |
| P1.9 | Signal deduplication: Redis `SETNX` on signal hash within `dedup_window_sec` (pro+) | 2026-06 | operations-security.md, Bruno |
| P1.10 | Plan limits enforcement: webhook count, credential count, orders/sec per plan tier | 2026-06 | functional tests, internal/plan |
| P1.11 | Prometheus `/metrics` endpoint (optional `METRICS_TOKEN` bearer auth) | 2026-06 | /metrics scrape |

---

## Phase 2 — Multi-tenancy and Security

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| P2.1 | Enterprise org model *(removed 2026 Phase 0 — legacy DB remains)* | 2026-06 | Bruno org collection (historical) |
| P2.2 | Org invite flow: `POST /v1/orgs/:id/invites`, accept link, member role assignment | 2026-06 | Bruno invites |
| P2.3 | Admin panel endpoints: user list, plan override, suspend/unsuspend, email verify override | 2026-06 | Bruno admin collection |
| P2.4 | Compliance gates: suspended user/org blocks ingest (403) and worker execution (trade rejected) | 2026-06 | functional tests |
| P2.5 | Token revocation check (`CheckRevoked` middleware on all protected routes) | 2026-06 | functional tests |
| P2.6 | `billing_source` field (`free` / `stripe` / `admin`): admin overrides not overwritten by Stripe events | 2026-06 | internal/billing/apply_plan.go |

---

## Phase 3.1 — Email Verification

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| P3.1.1 | `email_verified_at` column on `users`; `email_verifications` table (migration `008`) | 2026-06 | migrations/008_*.sql |
| P3.1.2 | `POST /v1/auth/register` → sends verification email (SMTP or log provider) | 2026-06 | Bruno register flow |
| P3.1.3 | `GET /v1/auth/verify-email?token=` verify endpoint | 2026-06 | Bruno verify |
| P3.1.4 | `POST /v1/auth/resend-verification` resend endpoint | 2026-06 | Bruno resend |
| P3.1.5 | Login gate: when `EMAIL_VERIFICATION_REQUIRED=true`, unverified users cannot log in | 2026-06 | functional tests |
| P3.1.6 | Invite accept auto-verifies email (no separate verify step for invited users) | 2026-06 | functional tests |
| P3.1.7 | Bootstrap admin auto-verify on first startup | 2026-06 | functional tests |
| P3.1.8 | Admin `PATCH` to manually mark user email as verified | 2026-06 | Bruno admin |

---

## Phase 3.2 — SEBI Algo-ID Tagging

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| P3.2.1 | `algo_id` column on `broker_credentials`; `algo_id` audit column on `trades` (migration `013`) | 2026-06 | migrations/013_*.sql |
| P3.2.2 | Per-credential `algo_id` with env fallbacks: `BROKER_ZERODHA_ALGO_ID`, `BROKER_ANGEL_ALGO_ID`, `BROKER_DHAN_ALGO_ID` | 2026-06 | internal/config |
| P3.2.3 | `SEBI_ALGO_ID_REQUIRED` config: auto-`true` when `BROKER_MODE=live`; fail-close in queue worker | 2026-06 | internal/config/config.go |
| P3.2.4 | Zerodha adapter: sends `algo_id` as `tag` field on live orders | 2026-06 | internal/livebrokers/zerodha.go |
| P3.2.5 | Angel One adapter: sends `algo_id` as `ordertag` field on live orders | 2026-06 | internal/livebrokers/angel.go |
| P3.2.6 | Dhan adapter: sends `algo_id` as `correlationId` field on live orders (confirmed with Dhan support) | 2026-06 | internal/livebrokers/dhan.go |
| P3.2.7 | `trades.algo_id` snapshot populated at order execution time for audit | 2026-06 | internal/queue |
| P3.2.8 | Credential API gate: rejects live Indian credential POST/PUT without resolvable `algo_id` when enforcement is on | 2026-06 | internal/handler |

> **Pending:** Sandbox validation on a live broker is still required before beta opens. See pending-work.md item P3.2.S.

---

## Phase 3.3 — SL/TP and Bracket Orders

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| P3.3.1 | Zerodha: cover order (CO) type with LTP fetch for SL price computation | 2026-06 | internal/livebrokers/zerodha.go |
| P3.3.2 | Angel One: ROBO bracket order type | 2026-06 | internal/livebrokers/angel.go |
| P3.3.3 | MT5/MetaApi: optional `stopLossInPips` / `takeProfitInPips` on place order | 2026-06 | internal/livebrokers/mt5.go |
| P3.3.4 | Guard: reject bracket order if product type is unsupported for the broker | 2026-06 | internal/handler |
| P3.3.5 | Integration tests with httptest fixtures for all bracket paths | 2026-06 | internal/livebrokers/*_test.go |

---

## Phase 3.4 — CancelOrder

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| P3.4.1 | `broker.Broker` interface extended with `CancelOrder(ctx, orderID)` | 2026-06 | internal/broker/broker.go |
| P3.4.2 | Per-broker `CancelOrder` implementation (Zerodha, Angel, Dhan, MT5, mock) | 2026-06 | internal/livebrokers/ |
| P3.4.3 | `DELETE /v1/webhooks/:id/trades/:tradeId` API endpoint | 2026-06 | Bruno cancel |
| P3.4.4 | Trade status transitions to `cancelled` on successful cancel | 2026-06 | functional tests |

---

## Phase 4A — Backend Product APIs

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| P4A.1.1 | `GET /v1/webhooks/:id/pnl` — per-webhook P&L rollup (trade count, notional, win-rate) | 2026-06 | Bruno P&L |
| P4A.1.2 | `GET /v1/orgs/:id/pnl` — org-level P&L rollup *(endpoint removed; per-webhook PnL remains)* | 2026-06 | Bruno P&L (historical) |
| P4A.2.1 | `GET /v1/ws/trades` WebSocket endpoint — JWT via `?token=` query param | 2026-06 | dashboard WebSocket feed |
| P4A.2.2 | In-process fan-out hub: trade status updates pushed to connected clients on insert | 2026-06 | internal/handler/ws_trades.go |
| P4A.3.1 | Grafana dashboard JSON (`deploy/grafana/`) — ingest, trades by error_code, broker latency, dedup hits | 2026-06 | deploy/grafana/ |
| P4A.4.1 | Prometheus alert rules → Alertmanager → Telegram (auth spikes, ingest rejected, 5xx, down) | 2026-06 | observability.md |

---

## Phase 4B — Billing and Dashboard

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| P4B.1.1 | Stripe billing code: `GET /v1/billing/plans`, `POST /v1/billing/checkout`, `POST /v1/billing/portal` | 2026-06 | Bruno billing, internal/billing/ |
| P4B.1.2 | Stripe webhook handler (`POST /v1/billing/stripe-webhook`): idempotent, handles checkout/subscription events | 2026-06 | internal/handler/stripe_webhook.go |
| P4B.1.3 | `stripe_webhook_events` idempotency table (migration `012`) | 2026-06 | migrations/012_stripe_billing.sql |
| P4B.1.4 | `billing_source=admin` override: admin plan changes not overwritten by Stripe webhook events | 2026-06 | internal/billing/apply_plan.go |
| P4B.1.5 | Billing metrics: `billing_checkout_total`, `billing_webhook_total{status}` | 2026-06 | Prometheus /metrics |
| P4B.2.1 | Dashboard — auth flows: register, verify email, resend, login, logout, invite accept | 2026-06 | app.staging.zettabridge.net |
| P4B.2.2 | Dashboard — webhooks: list, create, edit, pause/resume, rotate token, delete | 2026-06 | app.staging.zettabridge.net |
| P4B.2.3 | Dashboard — credentials: list, create (masked display), verify inline, delete | 2026-06 | app.staging.zettabridge.net |
| P4B.2.4 | Dashboard — trades: live WebSocket feed per webhook, fallback polling, status/error_code display | 2026-06 | app.staging.zettabridge.net |
| P4B.2.5 | Dashboard — billing page: plan cards, Stripe Checkout redirect, Customer Portal link | 2026-06 | app.staging.zettabridge.net |
| P4B.2.6 | Dashboard — org page *(removed; redirects to /webhooks or /admin)* | 2026-06 | app.staging.zettabridge.net |
| P4B.2.7 | Dashboard — admin page: user table, plan override, suspend/unsuspend | 2026-06 | app.staging.zettabridge.net |
| P4B.2.8 | Dashboard — P&L tab: stat cards per webhook | 2026-06 | app.staging.zettabridge.net |
| P4B.2.9 | Dashboard — unverified email banner with inline resend action | 2026-06 | app.staging.zettabridge.net |
| P4B.2.10 | Dashboard — `/billing/success` and `/billing/cancel` landing pages | 2026-06 | app.staging.zettabridge.net |

---

## Phase 5A — AWS Infrastructure (Staging)

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| P5A.0 | AWS account secured: IAM admin + MFA, root key disabled, CloudTrail, billing alerts | 2026-06-19 | AWS console |
| P5A.1 | VPC: 2 public + 2 private subnets, IGW, security groups (ALB / ECS / RDS / Redis) | 2026-06-19 | deploy/deploy-aws-ecs/terraform/ |
| P5A.2 | NAT Gateway + Elastic IP (static egress); outbound IP confirmed via `curl checkip` from ECS task | 2026-06-19 | ECS task test |
| P5A.3 | RDS PostgreSQL 15: private subnet, automated 7-day backups, `sg-rds` access control | 2026-06-19 | RDS console |
| P5A.4 | ElastiCache Redis 7: private subnet, `sg-redis` access control | 2026-06-19 | ElastiCache console |
| P5A.5 | AWS Secrets Manager secrets + KMS CMK (`alias/zettabridge-staging`) | 2026-06-19 | Secrets Manager console |
| P5A.6 | ECS task IAM role (`ECSTaskRole-ZettaBridge`): least-privilege Secrets + KMS + CloudWatch | 2026-06-19 | IAM console |
| P5A.7 | ECR repositories (`zettabridge-backend`, `zettabridge-dashboard`): immutable tags, scan-on-push | 2026-06-19 | ECR console |
| P5A.8 | ECS Fargate cluster `zettabridge-staging`, Container Insights enabled | 2026-06-19 | ECS console |
| P5A.9 | Backend ECS service: Fargate, private subnets, secrets from Secrets Manager, health check `/healthz` | 2026-06-19 | ECS console |
| P5A.10 | Dashboard ECS service: Fargate, private subnets, `NEXT_PUBLIC_API_URL` wired | 2026-06-19 | ECS console |
| P5A.11 | ALB (internet-facing): HTTP→HTTPS redirect, TLS 1.2+ policy | 2026-06-19 | ALB console |
| P5A.12 | ACM certificates: `api.staging.zettabridge.net` + `app.staging.zettabridge.net` (status: Issued) | 2026-06-19 | ACM console |
| P5A.13 | Cloudflare DNS CNAME records to ALB; both domains resolve over HTTPS with `/healthz` 200 | 2026-06-19 | `curl https://api.staging.zettabridge.net/healthz` |
| P5A.14 | CloudWatch log groups: `/ecs/zettabridge-backend`, `/ecs/zettabridge-dashboard` (30-day retention) | 2026-06-19 | CloudWatch console |
| P5A.15 | Terraform staging modules applied and validated (`deploy/deploy-aws-ecs/terraform/staging/`) | 2026-06-19 | terraform.tfstate |

---

## Plan Rename — Free / Individual / Household *(superseded by Phase 0)*

> **Superseded 2026-07** by four-tier model (Free / Paper / Pro / Pro Plus). Migration 046 is current.

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| PR.M.1 | Go constants: `plan.PlanFree`, `plan.PlanIndividual`, `plan.PlanHousehold` | 2026-06-21 | internal/plan/plan.go (historical) |
| PR.M.2 | Config: `StripePriceIndividual`, `StripePriceHousehold` env vars | 2026-06-21 | internal/config/ (historical) |
| PR.M.3 | Store: `UpdateUserPlan` updated; legacy `enterprise_tier` column removed from active use | 2026-06-21 | internal/store/ |
| PR.M.4 | DB migration `020_plan_rename.sql` applied on RDS staging | 2026-06-21 | CloudWatch log: 19:13:57 |
| PR.M.5 | Dashboard TypeScript plan types, UI labels, billing page, admin page updated | 2026-06-21 | dashboard/app/ |
| PR.M.6 | Bruno collection updated with new plan names | 2026-06-21 | bruno/ |

---

## Execution Adapter Split *(partial — 2026-07)*

| ID | Item | Completed | Evidence |
|----|------|-----------|----------|
| EA.1 | `EXECUTION_ADAPTER_MODE` local/http; execution router in Core | 2026-07 | internal/execution |
| EA.2 | Paper adapter binary (`cmd/paper-adapter`); cohosted on staging | 2026-07 | deploy-aws-ecs, docker-compose.adapters.yml |
| EA.3 | Zerodha adapter binary (`cmd/zerodha-adapter`); ECS sidecar on staging | 2026-07 | exec-zerodha.staging.zettabridge.net |
| EA.4 | `ENABLED_ADAPTERS` deploy gate; `GET /v1/brokers/enabled` | 2026-07 | Terraform, router |
| EA.5 | Internal routes: `/v1/internal/execution-events`, `/v1/internal/credentials/:id/session` | 2026-07 | router.go |
| EA.6 | Angel/Dhan/MT5 adapter code in `livebrokers/` | 2026-06 | internal/livebrokers/ |

> **Pending:** Angel/Dhan/MT5 ECS provisioning and E2E validation. See pending-work.md AD.1.

---

## Live Broker Adapters

| ID | Broker | Item | Completed | Evidence |
|----|--------|------|-----------|----------|
| LB.1 | MT5 / MetaApi | Market/limit orders, balance fetch, position close, SL/TP | 2026-06 | internal/livebrokers/mt5.go |
| LB.2 | Zerodha | Market/limit/CO orders, balance, cancel, SEBI tag | 2026-06 | internal/livebrokers/zerodha.go |
| LB.3 | Angel One | Market/limit/ROBO orders, balance, cancel, ordertag | 2026-06 | internal/livebrokers/angel.go |
| LB.4 | Dhan | Market/limit orders, balance, cancel, correlationId | 2026-06 | internal/livebrokers/dhan.go |
| LB.5 | All | HTTP client with header redaction (no secrets in logs) | 2026-06 | internal/livebrokers/httpclient/ |
| LB.6 | All | `POST /v1/credentials/:id/verify` — per-broker equity/balance probe | 2026-06 | Bruno verify |
