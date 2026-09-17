# Product Scope
**Last updated: 2026-07-07**

Defines what ZettaBridge does and does not do today. Source of truth for limits: `internal/plan/plan.go`.

---

## In scope

### Core platform
- HTTPS webhook ingest (TradingView JSON and generic POST)
- In-process worker queue with Redis-backed dedup and rate limits
- **Paper trading** (dedicated plan tier and webhook destination type)
- **Live broker execution** (Pro / Pro Plus) via in-process or HTTP execution adapters
- Trade ledger in PostgreSQL; WebSocket push to dashboard
- REST API for webhooks, credentials, paper accounts, trades, PnL, billing, admin
- Next.js dashboard (self-serve onboarding and monitoring)

### Execution adapters
- **Paper adapter** — simulated fills, positions, PnL (`paperengine`)
- **Zerodha adapter** — Kite Connect (day-0 on ECS staging); Publisher mode when enabled
- **Angel / Dhan / MT5** — implemented in `livebrokers/`; enabled only when listed in `ENABLED_ADAPTERS` and provisioned in deploy config
- **`EXECUTION_ADAPTER_MODE`:** `local` or `http` (Core ↔ sidecar)
- **`BROKER_MODE=mock`** — server-level simulated broker for staging/CI (not a user-facing demo mode)

### Broker integrations (live, when enabled)
- **Zerodha** — market/limit, CO brackets, SEBI Algo-ID via `tag`
- **Angel One** — market/limit, ROBO brackets, `ordertag`, IP whitelist
- **Dhan** — market/limit, `correlationId`, IP whitelist
- **MT5 / MetaApi** — market/limit with SL/TP

### Compliance
- SEBI Algo-ID on live Indian orders (fail-close when required)
- `trades.algo_id` audit column

### Plans and limits
| Plan | Paper | Live | Summary |
|------|-------|------|---------|
| **Free** | 1 account, 1 paper webhook, 10 paper trades/mo | No | Entry tier |
| **Paper** | 3 accounts, 5 paper webhooks, unlimited paper trades | No | Simulation-focused paid tier |
| **Pro** | 5 accounts, 8 paper + **1 live** webhook, 1 live cred | Yes | Single live webhook |
| **Pro Plus** | 10 accounts, 15 paper + **5 live** webhooks, 3 live creds | Yes | Multi-webhook load tests, multi-product creds |

Advanced webhook guards (dedup window, per-webhook rate limits, trading hours) require a paid plan for **live** webhooks; **paper webhooks are exempt**.

### Security
- AES-256-GCM credentials at rest
- JWT + refresh tokens
- Webhook token guards, ingest rate limits, plan caps
- Static NAT egress for broker IP whitelisting (AWS)

### Infrastructure
- AWS ECS Fargate (Mumbai), RDS PostgreSQL, ElastiCache Redis
- Lightsail (or similar) for Prometheus/Grafana monitoring sidecar
- Terraform under `deploy/deploy-aws-ecs/`

---

## Out of scope

### Permanently excluded
| Item | Reason |
|------|--------|
| Investment advice or signals | Execution bridge only |
| Market data / charting (except paper LTP snapshots) | Users bring signals from TradingView etc. |
| Strategy backtesting | Outside mandate |
| Native mobile apps | Not v1 |
| Multi-user Zerodha OAuth under one API key | Kite constraint — per-user API keys |
| FIX / direct market access | Broker SDK/API only |
| Regulated intermediary / brokerage | Technology bridge only |

### Removed from product (2026 Phase 0)
| Item | Replacement |
|------|-------------|
| **Org / Household / Enterprise** multi-user | Solo accounts; legacy DB columns remain |
| **`account_mode=demo`** | Paper trading plan + paper webhooks |
| **User-facing mock/demo broker** | `BROKER_MODE=mock` on server for staging only |
| **Individual / Household plan names** | **Pro / Pro Plus** (migration 044/046) |

### Deferred
| Item | Condition |
|------|-----------|
| Stripe/Razorpay live in India | Business registration |
| Angel/Dhan/MT5 on ECS staging | Enable in `enabled_adapters` + E2E validation |
| FYERS live execution | Market-data only today |

---

## Scope change process

1. Update this file and [STATUS.md](../STATUS.md)
2. Update `internal/plan/plan.go` if limits change
3. Add migration if DB constraints change
4. Update [feature-catalogue.md](feature-catalogue.md) and dashboard copy
