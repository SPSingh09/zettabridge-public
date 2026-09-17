# System Architecture
**Last updated: 2026-07-07**

---

## 1. System Overview

ZettaBridge is a multi-tenant SaaS webhook-to-broker execution bridge. External signal sources (primarily TradingView) send HTTP webhooks; ZettaBridge validates, deduplicates, queues, and routes those signals to the **paper engine** or **live broker adapters**, then records the outcome in a PostgreSQL trade log.

The system is deployed on AWS ECS Fargate in `ap-south-1` (Mumbai):

- **Core** — Go HTTP API + in-process queue worker + WebSocket hub
- **Dashboard** — Next.js standalone server (SSR + RSC)
- **Paper adapter** — sidecar ECS service (when `EXECUTION_ADAPTER_MODE=http`)
- **Zerodha adapter** — sidecar ECS service (when `EXECUTION_ADAPTER_MODE=http`)

Both Core and Dashboard sit behind an AWS ALB with ACM TLS. All outbound broker API calls exit through a static NAT Gateway Elastic IP.

```
┌──────────────────────────────────────────────────────────────────────┐
│                           Internet                                    │
└────────────┬──────────────────────────────────────┬──────────────────┘
             │ TradingView webhooks                  │ User (browser)
             ▼                                       ▼
┌────────────────────────────────────────────────────────────────────┐
│  AWS ALB (HTTPS, ACM TLS)                                          │
│  api.staging.zettabridge.net -> core:8080                          │
│  app.staging.zettabridge.net -> dashboard:3000                     │
└───────────────────────┬───────────────────────────────────────────┘
                        │ Private subnet
         ┌──────────────┴──────────────┐
         ▼                             ▼
┌─────────────────┐         ┌─────────────────────┐
│ Core            │         │ Dashboard           │
│ Go + Fiber      │◄───────►│ Next.js             │
│ ECS Fargate     │  REST   │ ECS Fargate         │
│ port 8080       │         │ port 3000           │
└────────┬────────┘         └─────────────────────┘
         │
         ├──────────────► AWS RDS PostgreSQL 15
         ├──────────────► AWS ElastiCache Redis 7
         │
         ├─ EXECUTION_ADAPTER_MODE=http ──► Paper adapter (ECS sidecar)
         │                                    Zerodha adapter (ECS sidecar)
         │
         └──────────────► NAT Gateway → Elastic IP
                          (live broker API calls egress here)
                               │
              ┌────────────────┼───────────────────┐
              ▼                ▼                   ▼
         Zerodha           Angel One             Dhan / MT5
         (ECS staging)     (code; gated)         (code; gated)
         Paper engine      SmartAPI              REST APIs
         (sidecar/local)
```

---

## 2. Backend Architecture

### 2.1 Process Structure

The Core binary (`cmd/api`) runs as a single OS process containing:

1. **Fiber HTTP server** — handles ingest and API routes (`internal/transport/http/router.go`)
2. **Queue worker pool** — N goroutines consuming from an in-process channel
3. **WebSocket hub** — `tradepush.Hub` fan-out to connected clients
4. **Zerodha refresh service** — background goroutine refreshing Zerodha access tokens (when OAuth on Core)

When `EXECUTION_ADAPTER_MODE=http`, order placement delegates to sidecar services (`cmd/paper-adapter`, `cmd/zerodha-adapter`) via internal HTTP. `ENABLED_ADAPTERS` (staging default: `paper,zerodha`) gates which adapters are provisioned.

```
cmd/api/main.go
├── config.Load()                  // env vars + Secrets Manager
├── store.NewRedis()               // ElastiCache
├── store.NewPostgres()            // RDS
├── migrate.RunAll()               // embed.FS SQL migrations
├── livebrokers.NewInfra()         // broker adapter factory
├── queue.New(N workers)           // in-process channel queue
├── tradepush.NewHub()             // WebSocket fan-out
├── q.SetTradeNotifier(hub)        // connect queue -> WebSocket
├── fiber.New()                    // HTTP server
├── handler.Register(app, ...)     // all routes
└── zerodha.NewRefreshService()    // background token refresh
```

### 2.2 Request Lifecycle — Webhook Ingest

```
POST /v1/webhook/:token
        │
        ▼
1. Token lookup         Redis / DB: resolve token → webhook record
        │
        ▼
2. Guard checks         Compliance gate (suspended?), rate limit, dedup, symbol/action guard
        │
        ▼
3. Queue push           Signal enqueued to in-process Go channel (non-blocking)
        │               Accepted → 202; duplicate within dedup window → 409
        ▼
4. Worker picks up      One of N goroutines dequeues the signal
        │
        ▼
5. Route by webhook     paper_account_id → paper adapter; broker_cred_id → live adapter
        │
        ▼
6. Algo-ID injection    algo.Resolve() injects SEBI Algo-ID (live Indian only)
        │
        ▼
7. Adapter HTTP call    Paper or live adapter (local in-process or HTTP sidecar)
        │               BROKER_MODE=mock simulates broker HTTP in staging/CI only
        ▼
8. Trade row insert     Status + broker_order_id + error_code written to PostgreSQL
        │
        ▼
9. WebSocket notify     tradepush.Hub broadcasts trade event to all connected clients
```

### 2.3 Queue Design

- **Type**: In-process Go buffered channel (`chan Signal`)
- **Workers**: Configurable via `WORKER_COUNT` env var
- **Buffer**: Configurable via `QUEUE_BUFFER` env var
- **Dedup**: Redis `SETNX` on signal hash with TTL = `dedup_window_sec`
- **Backpressure**: Full channel rejects incoming signal with 503 (rare; buffer should exceed burst)
- **Durability**: No persistence — signals in-flight on crash are dropped. TradingView retries re-send the signal; dedup window prevents duplicate orders.
- **Future scale**: At multi-ECS-instance scale, the in-process queue becomes a bottleneck. Redis Streams or SQS would replace it (tracked as post-GA decision).

### 2.4 Execution Adapter Pattern

Execution is split between **paper** and **live** adapters, selected by webhook target:

| Webhook field | Adapter | Mode |
|---------------|---------|------|
| `paper_account_id` | Paper engine | `local` (in-process) or `http` (sidecar) |
| `broker_cred_id` | Live broker (Zerodha, Angel, Dhan, MT5) | `local` or `http` sidecar |

**Configuration**

| Env var | Values | Purpose |
|---------|--------|---------|
| `EXECUTION_ADAPTER_MODE` | `local` \| `http` | In-process vs sidecar HTTP |
| `ENABLED_ADAPTERS` | e.g. `paper,zerodha` | Deploy-time gate (staging ECS default) |
| `BROKER_MODE` | `mock` \| `live` | Server staging/CI simulated broker HTTP — **not** user demo mode |

Sidecar binaries: `cmd/paper-adapter`, `cmd/zerodha-adapter`. Angel/Dhan/MT5 live adapters exist in `livebrokers/` but are not on ECS until enabled in Terraform.

All broker interactions use the `broker.Broker` interface:

```go
type Broker interface {
    PlaceOrder(ctx context.Context, order Order, cred Credential) (BrokerOrderID, error)
    CancelOrder(ctx context.Context, brokerOrderID string, cred Credential) error
    VerifyCredential(ctx context.Context, cred Credential) error
}
```

The queue worker calls the appropriate adapter. Live adapter selection is determined by `broker_type` on the credential. Paper webhooks never touch live broker credentials.

### 2.5 WebSocket Hub

`tradepush.Hub` is an in-process fan-out hub:

- Clients connect via `GET /v1/ws/trades?token=<jwt>` (JWT auth via query param — header auth is not available over browser WebSocket).
- On trade insert, the queue worker calls `hub.Notify(userID, trade)`.
- Hub delivers the event to all WebSocket connections belonging to `userID`.
- No persistence — clients that are not connected miss events (trade history is available via REST).

---

## 3. Data Model (Summary)

Full schema in `migrations/` through **`046_expand_plan_tiers.sql`**. See [database-design.md](database-design.md).

```
users
├── id (UUID)
├── email, password_hash
├── plan (free / paper / pro / pro_plus)
├── billing_source (stripe / admin / free)
├── is_suspended, email_verified
└── stripe_customer_id

paper_accounts
├── id (UUID)
├── user_id → users.id
├── label, starting_balance, status
└── (positions, orders, snapshots — related tables)

broker_credentials
├── id (UUID)
├── user_id → users.id
├── broker_type (zerodha / angel / dhan / mt5_cloud)
├── raw_creds (AES-256-GCM encrypted)
├── account_mode (live only — demo removed migration 044)
├── algo_id (SEBI Algo-ID, nullable)
└── status (active / paused / error)

webhooks
├── id (UUID)
├── user_id → users.id
├── token (UUID v4, ingest URL key)
├── paper_account_id → paper_accounts.id (paper webhooks)
├── broker_cred_id → broker_credentials.id (live webhooks)
├── status (active / paused)
├── dedup_window_sec, rate_limit_per_sec
├── allowed_symbols, allowed_actions (JSONB)
└── created_at

trades
├── id (UUID)
├── webhook_id → webhooks.id
├── signal (original JSON payload)
├── status (queued / placed / filled / rejected / cancelled)
├── broker_order (broker-assigned order ID)
├── algo_id (snapshot at execution time)
├── error_code (structured error enum)
└── created_at

orgs / org_members / org_invites / org_audit_log
└── Legacy tables only — org product removed from API/UI (migration 044)
```

Key constraints:
- `raw_creds` has `json:"-"` struct tag — never serialised into API responses.
- `algo_id` is snapshotted on each trade row for audit trail even if credential is later updated.
- Migrations are additive; no rollback scripts.

---

## 4. API Surface

All routes are defined in `internal/transport/http/router.go`. Base URL: `https://api.staging.zettabridge.net`.

> **Note:** OpenAPI (`swagger/docs.go`) may still list removed `/v1/orgs/*` paths — treat the router as authoritative.

### 4.1 Public Endpoints (no auth required)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/webhook/:token` | Receive and queue a trade signal (202 accepted; 409 deduplicated) |
| `GET` | `/v1/ws/trades` | WebSocket trade feed (`?token=<jwt>`) |
| `POST` | `/v1/billing/stripe-webhook` | Stripe webhook event handler |
| `GET` | `/healthz` | Health check; returns `{"status":"ok"}` |
| `GET` | `/metrics` | Prometheus metrics scrape endpoint |
| `POST` | `/v1/auth/register` | Register a new user |
| `POST` | `/v1/auth/login` | Login; returns JWT pair |
| `POST` | `/v1/auth/refresh` | Refresh access token |
| `GET` | `/v1/auth/verify-email` | Verify email via token |
| `POST` | `/v1/auth/resend-verification` | Resend verification email |
| Platform invite routes | | Closed-beta registration invites |

### 4.2 Authenticated Endpoints (JWT required)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/me` | Current user profile + plan + usage + `live_trading_allowed` |
| `POST` | `/v1/auth/logout` | Logout (token revocation) |
| **Webhooks** | | |
| `GET/POST` | `/v1/webhooks` | List / create webhooks |
| `GET/PUT/DELETE` | `/v1/webhooks/:id` | View / update / delete webhook |
| `POST` | `/v1/webhooks/:id/rotate-token` | Rotate ingest token |
| `GET` | `/v1/webhooks/:id/trades` | List trades for webhook |
| `DELETE` | `/v1/webhooks/:id/trades/:tradeId` | Cancel an order |
| `GET` | `/v1/webhooks/:id/pnl` | P&L summary for webhook |
| **Paper accounts** | | |
| `GET/POST` | `/v1/paper-accounts` | List / create paper accounts |
| `GET/PATCH/DELETE` | `/v1/paper-accounts/:id` | View / update / delete |
| `GET` | `/v1/paper-accounts/:id/positions`, `/orders`, `/pnl`, `/snapshots` | Paper trading state |
| **Credentials** | | |
| `GET/POST` | `/v1/credentials` | List / create live credentials (pro/pro_plus) |
| `GET/PUT/DELETE` | `/v1/credentials/:id` | View / update / delete credential |
| `POST` | `/v1/credentials/:id/verify` | Probe live broker to verify credential |
| `GET` | `/v1/brokers/enabled` | Deployment-enabled brokers |
| Zerodha OAuth routes | | Per-user Kite Connect flow |
| **Billing** | | |
| `GET` | `/v1/billing/plans` | List available plans |
| `POST` | `/v1/billing/checkout` | Create Stripe checkout session (disabled) |
| `POST` | `/v1/billing/portal` | Create Stripe customer portal session (disabled) |

**Removed:** all `/v1/orgs/*` routes. Dashboard org pages redirect to `/webhooks` or `/admin`.

### 4.3 Internal Endpoints (service token)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/internal/execution-events` | Sidecar → Core execution callbacks |
| `GET` | `/v1/internal/credentials/:id/session` | Sidecar credential session fetch |

### 4.4 Admin Endpoints (platform admin JWT claim required)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/admin/users` | List all users |
| `GET/PATCH` | `/v1/admin/users/:id` | View / update user (suspend, verify, plan) |
| `PUT` | `/v1/admin/users/:id/plan` | Set user plan (`free`, `paper`, `pro`, `pro_plus`) |
| Admin settings, market data, instruments, symbol requests | | Platform operator routes |

---

## 5. Deployment Architecture

### 5.1 AWS Infrastructure (ap-south-1)

```
VPC: 10.0.0.0/16
├── Public subnets (2 AZs)
│   ├── ALB (internet-facing)
│   └── NAT Gateway + Elastic IP (static egress for broker whitelisting)
│
└── Private subnets (2 AZs)
    ├── ECS Fargate: zettabridge-core (1 task, port 8080)
    ├── ECS Fargate: zettabridge-dashboard (1 task, port 3000)
    ├── ECS Fargate: paper-adapter, zerodha-adapter (when EXECUTION_ADAPTER_MODE=http)
    ├── RDS PostgreSQL 15 (db.t4g.micro staging, single-AZ)
    └── ElastiCache Redis 7 (cache.t4g.micro, single node)
```

### 5.2 Container Registry

- AWS ECR: `zettabridge-backend`, `zettabridge-dashboard`
- Image tags: `latest` for staging deploys; immutable tags for production
- Scan-on-push enabled

### 5.3 Secrets and Configuration

All runtime secrets are in AWS Secrets Manager:

| Secret | Description |
|--------|-------------|
| `DATABASE_URL` | RDS PostgreSQL connection string |
| `REDIS_URL` | ElastiCache Redis URL |
| `JWT_SECRET` | HS256 signing key |
| `AES_KEY` | 32-byte AES-256 encryption key for `raw_creds` |
| `STRIPE_SECRET_KEY` | Stripe API secret (billing disabled) |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook signature verification key |
| `EMAIL_*` | SMTP configuration |

Secrets Manager is encrypted with KMS CMK (`alias/zettabridge-staging`). ECS task role has `secretsmanager:GetSecretValue` and `kms:Decrypt` permissions (least-privilege).

### 5.4 DNS and TLS

- DNS: Cloudflare (DNS-only, grey cloud — no Cloudflare proxy)
- TLS: AWS ACM certificates, attached to ALB
- Domains: `api.staging.zettabridge.net` (backend), `app.staging.zettabridge.net` (dashboard)
- HTTP to HTTPS redirect on ALB listener

### 5.5 IaC

- Terraform modules: `deploy/deploy-aws-ecs/terraform/staging/`
- Backend: S3 (`zettabridge-terraform-state` / `staging/terraform.tfstate`) + DynamoDB lock (`zettabridge-terraform-locks`)
- AWS provider `~>5.0`, Terraform `>=1.5.0`

---

## 6. External Integrations

| Integration | Protocol | Auth | Notes |
|-------------|---------|------|-------|
| TradingView | HTTPS webhook inbound | Bearer token (webhook URL) | Caller; no ZB-initiated connection |
| Zerodha Kite Connect | HTTPS REST | Per-user API key + access token | Per-user credential; token expires daily |
| Angel One SmartAPI | HTTPS REST | Per-user API key + session token | Token expires in hours; needs frequent refresh |
| Dhan | HTTPS REST | Per-user access token | Token TTL ~24h |
| MetaApi (MT5) | HTTPS REST | Per-account MetaApi token | Long-lived; MetaApi proxy to broker |
| Stripe | HTTPS REST + webhook | Restricted API key + webhook secret | Disabled; code complete |
| SMTP (Zoho Mail) | SMTP/TLS | Per-account credentials | Email verification and invites |
| AWS Secrets Manager | AWS SDK | ECS task IAM role | Secret fetch at startup |
| AWS CloudWatch | AWS SDK | ECS task IAM role | Log group `ecs/zettabridge-*` |
| Prometheus / Grafana | HTTP pull | No auth (private subnet) | `GET /metrics` |
| Telegram | HTTPS webhook | Bot token in Alertmanager config | Ops alerting via Alertmanager |

---

## 7. Security Architecture

| Layer | Mechanism |
|-------|-----------|
| Transport | TLS 1.2+ enforced by ACM + ALB; HTTP redirected to HTTPS |
| Authentication | JWT HS256; access token 5h TTL; server-side revocation via Redis on logout |
| Credential storage | AES-256-GCM; key in Secrets Manager; `raw_creds` never returned by GET or logged |
| Secrets | AWS Secrets Manager + KMS CMK; no secrets in Dockerfile or environment variables in task definition |
| Egress | All outbound broker calls via NAT Gateway with static Elastic IP; whitelistable by brokers |
| Ingest token | UUID v4 per webhook; rotatable; treated as bearer capability |
| Admin access | Separate JWT claim; not self-promotable; admin endpoints in dedicated Fiber group |
| Network | Private subnets for all data services; no public IP on ECS tasks or RDS |

Planned security hardening (GA gates):
- HTTP security response headers (HSTS, X-Content-Type-Options, X-Frame-Options, CSP)
- AWS WAF on ALB
- KMS envelope encryption per credential (replaces single-key AES model)
- Penetration test

---

## 8. Observability Architecture

| Component | Technology | Detail |
|-----------|-----------|--------|
| Metrics | Prometheus | `zettabridge_ingest_total` (queued=accepted, deduplicated, rejected, rate_limited, queue_full), `zettabridge_trades_total`, `zettabridge_dedup_hits_total`, `zettabridge_broker_http_duration_seconds` |
| Dashboards | Grafana | ZettaBridge Overview on Lightsail VM; scrapes ECS staging via `AWS_STAGING_SCRAPE_HOST` |
| Alerting | Alertmanager → Telegram | Thresholds: rejection rate > 10%, 5xx > 5/min, queue_depth > 1000, service down |
| Logs | CloudWatch Logs | `/ecs/zettabridge-core`, `/ecs/zettabridge-dashboard`; 30-day retention |
| Health check | ALB target group | `GET /healthz` HTTP 200 required; ECS task replaced on consecutive failures |

---

## 9. Key Architectural Constraints and Trade-offs

| Decision | Constraint | Trade-off |
|----------|-----------|-----------|
| Single ECS task | ECS desired count = 1 for beta | Simplifies in-process queue and WebSocket hub; no Redis pub/sub needed; no horizontal scale at beta |
| In-process queue | Signal state lost on crash | Acceptable because TradingView retries; dedup window prevents duplicates |
| localStorage JWT | XSS risk vs. httpOnly cookie | Simpler for MVP; no Next.js proxy layer needed; mitigated by CSP (planned) |
| No ORM | Raw SQL via `database/sql` | Full query control; schema clarity; no magic; requires discipline on migrations |
| Per-user broker credentials | Cannot do platform-level OAuth | Required by broker ToS and Zerodha confirmed constraint; each user manages own tokens |
| Mumbai region only | No global latency optimisation | Correct for Indian broker target; global expansion is a post-v1 track |

For the rationale behind each decision, see [decision-log.md](../02-project-management/decision-log.md).
