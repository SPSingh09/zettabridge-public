# ZettaBridge — Project Status
**Last updated: 2026-07-07**

This document reflects the **current shipped product** as implemented in code (`internal/plan`, `internal/transport/http/router.go`, migrations through **046**). For detailed feature IDs see [feature-catalogue.md](01-product/feature-catalogue.md).

---

## What is ZettaBridge?

ZettaBridge is a **multi-tenant algorithmic trading bridge** that connects TradingView (or any webhook-capable platform) to **paper trading** and **live Indian/international brokers**. It:

- Receives trade signals via per-webhook HTTPS tokens
- Validates, deduplicates, and rate-limits at ingest
- Routes to the **paper engine** or **live broker adapters** (Zerodha day-0 on ECS; Angel/Dhan/MT5 in code, gated by `ENABLED_ADAPTERS`)
- Persists outcomes in PostgreSQL and pushes live updates over WebSocket
- Provides a Next.js dashboard for webhooks, credentials, paper accounts, billing, and admin

**Not in product today:** multi-user orgs (`/v1/orgs/*` removed), `account_mode=demo`, user-facing mock broker selection, Household/Individual plan names.

---

## Plans (current)

Four tiers — source of truth: `internal/plan/plan.go`, migration `046_expand_plan_tiers.sql`.

| Plan | Paper accounts | Paper webhooks | Live webhooks | Live broker creds | Paper trades/mo | Live trading | Orders/sec (display / enforced) | Audit logs | Advanced webhook guards | Notifications | Multi-product creds |
|------|----------------|----------------|---------------|-------------------|-----------------|--------------|----------------------------------|------------|---------------------------|---------------|------------------------|
| **free** | 1 | 1 | 0 | 0 | 10 | No | 1 / 1 | No | No | No | No |
| **paper** | 3 | 5 | 0 | 0 | Unlimited | No | 5 / 5 | Yes | Yes | Yes | No |
| **pro** | 5 | 8 | 1 | 1 | Unlimited | Yes | 10 / 10 | Yes | Yes | Yes | No |
| **pro_plus** | 10 | 15 | 5 | 3 | Unlimited | Yes | 10 / 10 | Yes | Yes | Yes | Yes |

**Additional limits**

- Free plan: **100 webhook ingests/day** per user (Redis counter at ingest).
- Per live broker credential: **10 orders/sec** cap (`BrokerCredOrdersPerSecCap`).
- Webhook create defaults: `rate_limit_per_min=60`, `dedup_window_sec=2` (override via API; `0` disables when sent explicitly).
- Dedup: `dedup_window_sec` 0–60 on webhook; requires paid plan for **live** webhooks (paper webhooks exempt from advanced-guard plan gate).

**Billing:** Stripe checkout for `paper`, `pro`, `pro_plus` when `BILLING_ENABLED=true`. Admin override: `PUT /v1/admin/users/:id/plan`. Env: `STRIPE_PRICE_PAPER`, `STRIPE_PRICE_PRO`, `STRIPE_PRICE_PRO_PLUS`.

---

## Execution model

| Concept | Behavior |
|---------|----------|
| **Paper webhook** | `paper_account_id` set → in-process or HTTP **paper adapter** |
| **Live webhook** | `broker_cred_id` set → **live adapter** (Zerodha sidecar on ECS staging) |
| **`EXECUTION_ADAPTER_MODE`** | `local` (in-process) or `http` (Core → sidecar ECS services) |
| **`ENABLED_ADAPTERS`** | Deploy-time gate; staging default `paper,zerodha` |
| **`BROKER_MODE`** | Server env: `mock` (simulated broker for staging/CI) or `live` (real broker HTTP). **Not** a user plan feature. |
| **Credential `account_mode`** | **`live` only** (demo removed in migration 044) |

Sidecar binaries shipped: `cmd/paper-adapter`, `cmd/zerodha-adapter`. Angel/Dhan/MT5 adapters exist in `livebrokers/` but are not provisioned on ECS until enabled in Terraform.

---

## Supported brokers

| Broker | Credential type | Execution (when enabled) | Staging ECS |
|--------|-----------------|--------------------------|-------------|
| Zerodha | `zerodha` | Kite Connect / Publisher / direct API | Yes (sidecar) |
| Angel One | `angel` | SmartAPI | Code only |
| Dhan | `dhan` | REST API | Code only |
| MT5 | `mt5_cloud` | MetaApi | Code only |
| Paper | — | Internal paper engine | Yes (sidecar or cohosted) |
| FYERS | — | **Market data OAuth/quotes only** (Core admin routes) | N/A |

`GET /v1/brokers/enabled` returns deployment-enabled brokers.

---

## API surface (active)

Mounted in `internal/transport/http/router.go`:

- **Public:** `/healthz`, `/metrics`, auth, `POST /v1/webhook/:token`, Stripe webhook, platform invites, Zerodha OAuth callback (Core when not HTTP adapters), FYERS admin callback
- **JWT:** `/v1/me`, webhooks, credentials, paper accounts, billing, trades/PnL, notifications, Telegram, publisher orders, symbol requests
- **Admin:** users, settings, market data, instruments, symbol requests
- **Internal (service token):** `/v1/internal/execution-events`, `/v1/internal/credentials/:id/session`

**Removed from router:** all `/v1/orgs/*` routes. Legacy org columns/tables remain in DB for existing data.

OpenAPI (`swagger/docs.go`) may still list org paths — treat router as authoritative until swagger is regenerated.

---

## Dashboard (current)

Nav: Webhooks, Broker Accounts, Paper Accounts, Requests, Billing, Help, Admin (role-gated).

- Org pages redirect to `/webhooks` or `/admin`
- Live trading UI gated by `live_trading_allowed` from `/v1/me`
- Advanced webhook guards (dedup, rate limits, trading hours) on paid plans for **live** webhooks; paper webhooks exempt
- Pro Plus: multi-product credentials (MIS + CNC + NRML)

---

## Observability

Prometheus metrics on Core `GET /metrics`:

| Metric | Labels |
|--------|--------|
| `zettabridge_ingest_total` | `queued`, `deduplicated`, `rejected`, `rate_limited`, `queue_full` |
| `zettabridge_trades_total` | `status`, `error_code`, `broker_type` |
| `zettabridge_dedup_hits_total` | — |
| `zettabridge_broker_http_duration_seconds` | `broker`, `op` |

Grafana dashboard: **ZettaBridge Overview** (`deploy/deploy-lightsail/grafana/dashboards/zettabridge.json`, v13). Monitoring stack runs on **Lightsail VM**; Prometheus scrapes ECS staging via `AWS_STAGING_SCRAPE_HOST`. See [observability.md](05-operations/observability.md).

---

## Deployment targets

| Target | Path | Notes |
|--------|------|-------|
| Local / CI | `docker-compose.yml` | `BROKER_MODE=mock`, `EXECUTION_ADAPTER_MODE=local` |
| HTTP adapters (dev) | `docker-compose.adapters.yml` | Sidecars on localhost |
| AWS ECS staging | `deploy/deploy-aws-ecs/` | Fargate Core + dashboard + paper/zerodha adapters, RDS, ElastiCache |
| Lightsail / VPS | `deploy/deploy-lightsail/` | Optional app host; **monitoring sidecar** (Prometheus/Grafana) |
| Load tests | `deploy/loadtest/` | k6 against staging API |

---

## Migrations

Latest: **`046_expand_plan_tiers.sql`** (four-plan CHECK constraint).

Notable: **044** — removed `individual`/`household` plan names, detached `org_id`, live-only credentials.

---

## What is done (high level)

- Webhook ingest → in-process queue → paper or live execution (local or HTTP adapters)
- Four plan tiers with paper + live limits
- Paper trading engine with positions, orders, FYERS/market-profile snapshots
- Zerodha live (+ optional Kite Publisher), Angel/Dhan/MT5 live broker code
- SEBI Algo-ID, SL/TP brackets, cancel order
- JWT auth, AES credential encryption, dedup, guards, rate limits
- Stripe billing code, admin plan override
- Next.js dashboard, WebSocket trade feed, PnL API
- AWS ECS staging, functional + load tests, Grafana/Alertmanager on Lightsail VM

---

## Known gaps / stale artifacts

| Item | Status |
|------|--------|
| Org product feature | Removed from API/UI; DB legacy remains |
| Swagger org endpoints | Stale vs router |
| `ADAPTER_SPLIT.md` (repo root) | Describes old 2-plan model; superseded by this doc + migration 046 |
| Angel/Dhan/MT5 on ECS | Not in default `enabled_adapters` |
| Payment gateway in India | Stripe env vars exist; enable when business registration complete |
| Live end-to-end smoke on real Kite | Blocked on broker IP whitelist / real creds |

See [pending-work.md](06-project-management/pending-work.md) for open items.
