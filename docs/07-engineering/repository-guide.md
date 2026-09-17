# Repository Guide
**Last updated: 2026-07-07**

Structure, conventions, and navigation for the ZettaBridge monorepo.

---

## Top-Level Layout

```
zettabridge/
├── cmd/
│   ├── api/                # Core API binary — main.go wires dependencies
│   ├── paper-adapter/      # Paper execution sidecar (HTTP adapter mode)
│   └── zerodha-adapter/    # Zerodha live execution sidecar
├── internal/
│   ├── adapters/           # Broker runtime adapters (paper engine, zerodha broker)
│   ├── execution/          # Execution router, orchestrator, adapter clients (local/http/server)
│   ├── integrations/brokers/  # Live broker HTTP clients (zerodha, angel, dhan, mt5, simulated)
│   ├── modules/            # HTTP handlers by domain (auth, webhooks, credentials, paperaccounts, …)
│   ├── platform/           # Queue, metrics, mailer, security/idempotency
│   ├── plan/               # Four-plan limits (free, paper, pro, pro_plus)
│   ├── store/              # PostgreSQL + Redis data access
│   ├── transport/http/     # Fiber router + middleware
│   └── config/             # Env loading + validation
├── migrations/             # Ordered SQL migration files (001–046)
├── functional-tester/      # End-to-end Go test suite (run against live server)
├── dashboard/              # Next.js 14 dashboard (App Router, TypeScript)
├── deploy/
│   ├── deploy-aws-ecs/     # Terraform: ECS Core + dashboard + paper/zerodha adapters
│   ├── deploy-lightsail/   # Optional app host; Prometheus/Grafana monitoring VM
│   ├── loadtest/           # k6 load test scripts
│   └── scripts/            # staging-smoke, staging-release, preflight
├── docker-compose.yml      # Local dev (BROKER_MODE=mock, EXECUTION_ADAPTER_MODE=local)
├── docker-compose.adapters.yml  # Overlay: HTTP sidecars on localhost
├── docker-compose.live.yml # Overlay: BROKER_MODE=live
├── docker-compose.billing.yml   # Overlay: Stripe billing test keys
├── Makefile
└── vendor/
```

---

## `internal/` Package Reference

### Execution adapters (`internal/execution/`)

| Package | Role |
|---------|------|
| `execution/router.go` | Routes jobs to paper or live path; checks `ENABLED_ADAPTERS` |
| `execution/adapter/local/` | In-process paper + live adapters (`EXECUTION_ADAPTER_MODE=local`) |
| `execution/adapter/http/` | HTTP client to sidecar services |
| `execution/adapter/server/` | Sidecar HTTP server (used by `cmd/paper-adapter`, `cmd/zerodha-adapter`) |
| `execution/orchestrator.go` | Builds and dispatches execution commands |

### Broker integrations (`internal/adapters/`, `internal/integrations/brokers/`)

- `adapters/paper/` — paper trading engine
- `adapters/zerodha/` — Zerodha Kite Connect + OAuth callback helpers
- `integrations/brokers/zerodha|angel|dhan|mt5/` — live broker HTTP clients
- `integrations/brokers/simulated/` — mock broker when `BROKER_MODE=mock`
- `integrations/brokers/brokerfactory/` — maps `broker_type` → adapter instance

### HTTP modules (`internal/modules/`)

Domain handlers mounted by `internal/transport/http/router.go`:

| Module | Routes |
|--------|--------|
| `auth` | register, login, logout, `/v1/me` |
| `webhooks` | webhook CRUD, pause, rotate-token, trades |
| `credentials` | live broker creds, Zerodha OAuth |
| `paperaccounts` | paper accounts, positions, orders, P&L |
| `signals` | ingest, WebSocket trade feed |
| `billing` | plans, checkout, Stripe webhook |
| `admin` | users, settings, market data, instruments |
| `internalapi` | adapter → Core execution events |

**Removed from router:** org modules (`/v1/orgs/*`). Legacy org store code may remain for DB compatibility.

### `plan`
Four tiers: `free`, `paper`, `pro`, `pro_plus`. Limits in `plan.go` / `limits.go` — paper vs live webhook caps, orders/sec, multi-product creds (pro_plus).

### `platform/queue`
In-process signal queue + worker pool. Workers call execution router → paper engine or live adapter.

### `store`
PostgreSQL (`PGStore`) and Redis (`RedisStore`) — users, webhooks, credentials, paper trading, trades, billing. Org tables exist for legacy data only.

---

## `dashboard/` Structure

```
dashboard/
├── app/
│   ├── (auth)/                 # Auth route group (shared layout)
│   │   ├── login/page.tsx
│   │   ├── register/page.tsx
│   │   ├── verify-email/page.tsx
│   │   └── resend-verification/page.tsx
│   ├── (app)/                  # App route group (requires auth)
│   │   ├── layout.tsx          # NavBar + auth guard
│   │   ├── webhooks/
│   │   ├── credentials/
│   │   ├── paper-accounts/
│   │   ├── billing/
│   │   ├── me/
│   │   ├── admin/
│   │   └── help/
│   ├── invites/[token]/page.tsx
│   └── page.tsx                # Root redirect → /webhooks or /login
├── components/
│   └── ui/                     # shadcn/ui components (Button, Input, etc.)
├── lib/
│   ├── api.ts                  # Typed API client with apiFetch (auth + refresh)
│   ├── auth.ts                 # localStorage JWT helpers
│   └── types.ts                # TypeScript types matching Go response shapes
├── public/
├── next.config.js
├── tailwind.config.ts
└── package.json
```

The `(auth)` and `(app)` route groups use Next.js App Router's parenthesis group convention for shared layouts without affecting URL paths.

`lib/api.ts` handles all `fetch()` calls with automatic token injection and silent refresh on 401.

---

## `migrations/` Naming Convention

```
NNN_description.sql
```

Files are applied in lexicographic order. Numbers are left-padded to three digits. Each file is a standalone SQL script with `ALTER TABLE ... IF NOT EXISTS` patterns for idempotency.

New migration checklist:
- [ ] Use `ADD COLUMN IF NOT EXISTS` (never bare `ADD COLUMN`)
- [ ] Name the migration descriptively: `025_webhook_tags.sql`
- [ ] Test locally by running `docker compose down -v && docker compose up`
- [ ] The binary picks it up automatically via `embed.FS`

---

## `deploy/` Structure

```
deploy/
├── deploy-aws-ecs/
│   ├── terraform/staging/      # Terraform modules: VPC, ECS, RDS, Redis, ALB, Secrets
│   └── scripts/                # ECR push, ECS deploy, smoke test scripts
├── loadtest/
│   ├── k6/                     # k6 script files
│   └── run-loadtest.sh
├── sectest/
│   ├── scripts/                # IDOR, JWT tamper, injection, flood probes
│   └── run-sectest.sh
└── scripts/
    ├── staging-smoke.sh
    ├── staging-release.sh
    ├── monitoring-up.sh
    └── preflight-wsl.sh
```

---

## Conventions

### Import paths
All internal packages use the module path `github.com/SPSingh09/zettabridge/internal/<package>`.

### Error handling
- Handlers return Fiber JSON errors: `c.Status(statusCode).JSON(fiber.Map{"error": "..."})`
- Store functions return `(result, error)` — callers check error; 404 is `nil, nil`
- Broker adapters return `(brokerOrderID, error)` — on error, the queue worker records `error_code`

### Test file locations
- Unit/integration tests: co-located with the package being tested (`package_test.go`)
- Functional tests: `functional-tester/functional_test.go` (bash-driven)
- Bruno: `bruno/ZettaBridge/` (manual + CI curl)

### Secret hygiene
Never put secrets in:
- Dockerfiles (`ENV` instructions)
- `docker-compose.yml` checked-in values (dev defaults are okay; prod values must use Secrets Manager)
- Log statements
- Test fixtures committed to git

Redact broker auth headers using `internal/livebrokers/httpclient` — it automatically redacts `Authorization`, `auth-token`, `access-token`, `x-api-key`, and any header containing `token` or `secret`.
