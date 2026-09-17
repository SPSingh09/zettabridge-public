# Technology Stack
**Last updated: 2026-07-07**

All versions are as of 2026-07-07. See `go.mod` and `dashboard/package.json` for the authoritative version pins.

---

## Backend (Go)

| Component | Technology | Version | Rationale |
|-----------|-----------|---------|-----------|
| Language | Go | 1.22 | Strong concurrency primitives for queue worker; fast single-binary deployments; team familiarity |
| HTTP framework | Fiber v2 | 2.52.5 | Fasthttp-based; middleware chain maps cleanly to auth + compliance guard layers; comparable performance to Echo/Gin |
| WebSocket | gofiber/contrib/websocket | 1.0.0 | Thin wrapper over fasthttp WebSocket; integrates with Fiber route groups |
| WebSocket transport | fasthttp/websocket | 1.5.8 | Underlying WebSocket implementation used by the Fiber WebSocket contrib |
| JWT auth | golang-jwt/jwt v4 | 4.5.0 | Standard HS256 signing; access + refresh token pair |
| JWT middleware | gofiber/jwt v3 | 3.3.10 | Fiber-native JWT middleware integration |
| PostgreSQL driver | lib/pq | 1.10.9 | Mature `database/sql`-compatible driver; no ORM (raw SQL for full control over migration and query shape) |
| Redis client | redis/go-redis v9 | 9.5.1 | In-process queue dedup, rate limiting, webhook token cache |
| Prometheus metrics | prometheus/client_golang | 1.19.1 | Standard Go Prometheus client; custom counters + histograms for ingest, trades, broker latency |
| Stripe billing | stripe/stripe-go v81 | 81.4.0 | Official Stripe Go SDK; checkout sessions, customer portal, webhook signature verification |
| Crypto | golang.org/x/crypto | 0.22.0 | bcrypt for password hashing; AES-256-GCM via stdlib; scrypt not used |
| UUID | google/uuid | 1.6.0 | v4 random UUIDs for entity IDs and webhook tokens |
| HTTP compression | klauspost/compress | 1.17.7 | Indirect via Fiber; Brotli + gzip response compression |

### Key design decisions
- **No ORM.** All SQL is hand-written via `database/sql` + `lib/pq`. Migrations are plain `.sql` files embedded in the binary via `embed.FS`.
- **No DI framework.** Dependencies are wired manually in `cmd/api/main.go`. This keeps startup legible and avoids reflection-based magic.
- **Broker adapters** implement a `broker.Broker` interface — the queue worker is broker-agnostic.
- **Execution adapter sidecars** — when `EXECUTION_ADAPTER_MODE=http`, Core delegates to Fargate services (`cmd/paper-adapter`, `cmd/zerodha-adapter`) via private Cloud Map DNS. Gated by `ENABLED_ADAPTERS` (staging default: `paper,zerodha`).

---

## Frontend (Next.js Dashboard)

| Component | Technology | Version | Rationale |
|-----------|-----------|---------|-----------|
| Framework | Next.js (App Router) | 14.2.29 | SSR for auth pages; RSC for data fetching; standalone Docker output |
| Language | TypeScript | 5.x | Type-safe API client shape matching Go response structures |
| Styling | Tailwind CSS | 3.4.14 | Utility-first; no design-system lock-in for MVP |
| Component primitives | Radix UI (label, slot) | 2.1.0 / 1.1.0 | Accessible, unstyled primitives used by shadcn/ui components |
| Data fetching | SWR | 2.2.5 | Stale-while-revalidate; lightweight for webhook/credential list polling |
| Charts | Recharts | 3.8.1 | React charting library; used for P&L time-series (planned) |
| Icons | Lucide React | 0.400.0 | Consistent icon set |
| CSS utilities | clsx + tailwind-merge | 2.1.1 / 2.5.2 | Conditional className composition |
| Build | `next build` → standalone | — | Docker multi-stage build; `server.js` node runtime output |

### Key design decisions
- **App Router** (not Pages Router) — uses React Server Components and the `(auth)` / `(app)` route group pattern for layout separation.
- **JWT in localStorage** — simpler than httpOnly cookie proxy for MVP; mitigated by CSP headers (planned).
- **SWR for polling** — WebSocket is used for live trade feed; SWR handles everything else with stale-while-revalidate.
- **No state management library** — React context + SWR is sufficient; Redux/Zustand not needed at this scale.

---

## Databases

| Service | Technology | Version | Hosted on | Rationale |
|---------|-----------|---------|-----------|-----------|
| Primary database | PostgreSQL | 15 | AWS RDS (db.t4g.micro staging, db.t3.small GA) | Relational model fits audit requirements; PITR backup; Multi-AZ for production |
| Cache / queue | Redis | 7.x | AWS ElastiCache (cache.t4g.micro) | Signal dedup (SETNX), webhook token cache, JWT revocation, rate limiting |

### Schema management
- Migrations: plain SQL files embedded via Go `embed.FS` in `migrations/`
- Applied automatically at startup via `internal/migrate.RunAll()`
- 46 migrations shipped as of 2026-07-07 (latest: `046_expand_plan_tiers.sql`)
- No rollback scripts — migrations are additive; destructive changes are avoided

---

## Infrastructure and Cloud

| Component | Technology | Version / Details | Rationale |
|-----------|-----------|-------------------|-----------|
| Cloud provider | AWS | — | Mature managed services; ap-south-1 closest to Indian brokers |
| Region | ap-south-1 (Mumbai) | — | Lowest latency to NSE/BSE broker API servers |
| Container runtime | AWS ECS Fargate | — | Core + dashboard + **paper/zerodha adapter sidecars** (conditional via Terraform `enabled_adapters`) |
| Container registry | AWS ECR | — | Immutable image tags; scan-on-push; co-located with ECS |
| Load balancer | AWS ALB | — | HTTP→HTTPS redirect; host-header routing to backend and dashboard |
| TLS certificates | AWS ACM | — | Managed cert issuance and renewal; attached to ALB |
| Database | AWS RDS PostgreSQL 15 | — | Managed; PITR; 7-day automated backups |
| Cache | AWS ElastiCache Redis 7 | — | Managed; private subnet; no auth token for staging |
| Secret management | AWS Secrets Manager | — | All secrets (JWT, AES key, DB URL, Stripe keys) |
| Secret encryption | AWS KMS CMK | `alias/zettabridge-staging` | Customer-managed key for Secrets Manager encryption |
| IAM | ECS task role | `ECSTaskRole-ZettaBridge` | Least-privilege: Secrets, KMS, CloudWatch logs |
| DNS | Cloudflare | — | `zettabridge.net` registrar + DNS (DNS-only, grey cloud) |
| IaC | Terraform | ~5.0 AWS provider, ~3.6 random provider | Staging modules at `deploy/deploy-aws-ecs/terraform/staging/`; S3 backend + DynamoDB lock |

---

## Observability

| Component | Technology | Notes |
|-----------|-----------|-------|
| Metrics | Prometheus (pull) | Custom metrics in `internal/platform/metrics`; scraped from Core `GET /metrics` |
| Dashboards | Grafana | **ZettaBridge Overview** v13 — `deploy/deploy-lightsail/grafana/dashboards/zettabridge.json` |
| Monitoring host | Lightsail VM | Prometheus + Grafana + Alertmanager; scrapes ECS staging when `AWS_STAGING_SCRAPE_HOST` is set |
| Alerting | Alertmanager → Telegram | Rules at `deploy/deploy-lightsail/prometheus/alerts/` |
| Container logs | AWS CloudWatch | `/ecs/zettabridge-backend`, `/ecs/zettabridge-dashboard`; 30-day retention |
| Uptime monitoring | ALB health checks | `GET /healthz` → `{"status":"ok"}`; ECS auto-restarts failed tasks |

---

## CI / Testing

| Component | Technology | Notes |
|-----------|-----------|-------|
| Unit tests | Go `testing` | Standard library; run with `go test ./...` |
| Integration tests | Go `testing` + `net/http/httptest` | Broker adapter tests with httptest fixtures in `internal/livebrokers/testdata/` |
| Functional tests | Go `testing` (build tag `functional`) | End-to-end against a running server; `functional-tester/` directory |
| API testing | Bruno | Bruno collection at `bruno/ZettaBridge/`; covers all endpoint flows |
| Load testing | k6 | Scripts at `deploy/loadtest/k6/`; targets: 500 VUs, 60s, p99 < 100ms |
| Security testing | Shell scripts | Scripts at `deploy/sectest/scripts/`; IDOR, JWT tamper, injection, webhook flood probes |
| CI/CD | Not yet configured | Manual deploy via `deploy/deploy-aws-ecs/scripts/`; CI/CD pipeline is a GA gate (G3) |

---

## Build and Packaging

### Backend Docker image

```
Base build:  golang:1.22-bookworm
Runtime:     ubuntu:24.04
Binary:      CGO_ENABLED=0 GOOS=linux, multi-arch via TARGETARCH build arg
Output:      /zettabridge (single static binary)
Port:        8080
```

### Dashboard Docker image

```
Stage 1 (deps):    node:20-alpine — npm ci
Stage 2 (builder): node:20-alpine — next build, NEXT_PUBLIC_API_URL baked in at build time
Stage 3 (runner):  node:20-alpine — standalone Next.js server
Port:              3000
User:              nextjs (non-root)
```

---

## External Services

| Service | Purpose | Integration point |
|---------|---------|------------------|
| Zerodha Kite Connect | Indian broker execution | `internal/livebrokers/zerodha.go`; REST API |
| Angel One SmartAPI | Indian broker execution | `internal/livebrokers/angel.go`; REST API |
| Dhan | Indian broker execution | `internal/livebrokers/dhan.go`; REST API |
| MetaApi (MT5) | Forex/CFD broker execution | `internal/livebrokers/mt5.go`; MetaApi REST API |
| Stripe | Subscription billing | `internal/billing/`; disabled pending payment gateway approval |
| SMTP / Zoho Mail | Email verification | `internal/notify/`; configurable via `EMAIL_PROVIDER` env |
| Telegram | Ops alerting | Alertmanager webhook receiver |
| TradingView | Signal source (external) | No direct integration; users configure ZettaBridge webhook URL in TradingView alerts |
