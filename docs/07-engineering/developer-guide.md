# Developer Guide
**Last updated: 2026-07-07**

Everything needed to get ZettaBridge running locally, run tests, and make changes.

---

## Prerequisites

| Tool | Required version | Install |
|------|-----------------|---------|
| Go | 1.22+ | https://go.dev/dl |
| Node.js | 20+ | https://nodejs.org |
| Docker + Docker Compose | v2+ | https://docs.docker.com/get-docker |
| Make | any | Pre-installed on Linux/macOS |

Optional for load and security tests:
- k6 (load testing): https://k6.io/docs/getting-started/installation
- The functional test suite uses bash + curl, not k6.

---

## 1. Clone and Dependency Setup

```bash
git clone https://github.com/SPSingh09/zettabridge.git
cd zettabridge
```

Go dependencies are vendored in `vendor/`. No `go mod download` needed:

```bash
# Verify vendor is intact
go build ./...
```

Dashboard dependencies:

```bash
cd dashboard && npm ci && cd ..
```

---

## 2. Local Development (docker-compose)

The default `docker-compose.yml` starts PostgreSQL, Redis, the Go Core API, and the Next.js dashboard — with `BROKER_MODE=mock` and `EXECUTION_ADAPTER_MODE=local`.

```bash
docker compose up --build
```

Services:
- Backend API: `http://localhost:8080`
- Dashboard: `http://localhost:3000`
- PostgreSQL: `localhost:5432` (user: postgres, password: postgres, db: zettabridge)
- Redis: `localhost:6379`

### HTTP adapter sidecars (optional)

To exercise the same path as ECS staging (Core → sidecar HTTP):

```bash
make compose-http
# or:
docker compose -f docker-compose.yml -f docker-compose.adapters.yml --profile adapters up --build
```

This sets `EXECUTION_ADAPTER_MODE=http` and starts `paper-adapter` (8091) and `zerodha-adapter` (8092) with `ENABLED_ADAPTERS=paper,zerodha`.

Migrations run automatically on backend startup via `embed.FS`.

To run only the backend API without Docker:

```bash
make dev-api     # go run ./cmd/api
# or
make dev         # runs backend + dashboard concurrently (ctrl-C stops both)
```

Dashboard hot-reload only:

```bash
make dev-dashboard   # cd dashboard && npm run dev
```

---

## 3. Environment Variables

All configuration comes from environment variables. The defaults in `docker-compose.yml` are safe for local development. Never use development defaults in production.

### Core

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DATABASE_URL` | `postgres://...@localhost:5432/zettabridge` | PostgreSQL connection string |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection string |
| `JWT_SECRET` | `change-me-in-production-...` | HS256 signing key (min 32 chars; must not be default in production) |
| `AES_KEY` | `000...000` (32 hex bytes) | AES-256 credential encryption key (startup fails in production if all-zero) |
| `CORS_ORIGINS` | `http://localhost:3000` | Comma-separated allowed CORS origins |
| `BOOTSTRAP_ADMIN_EMAIL` | (empty) | Existing user to promote to platform admin on startup |
| `APP_ENV` | (empty) | Set to `production` to enable zero-key AES guard |
| `APP_PUBLIC_URL` | `http://localhost:3000` | Dashboard base URL (used in email links) |

### Queue and Workers

| Variable | Default | Description |
|----------|---------|-------------|
| `WORKER_COUNT` | `20` | Number of queue goroutines |
| `QUEUE_BUFFER` | `500` | In-process channel buffer size |

### Broker

| Variable | Default | Description |
|----------|---------|-------------|
| `BROKER_MODE` | `mock` | `mock` or `live`. Server env — not a user-facing plan toggle. Setting `live` auto-sets `SEBI_ALGO_ID_REQUIRED=true` |
| `EXECUTION_ADAPTER_MODE` | `local` | `local` (in-process) or `http` (sidecar services) |
| `ENABLED_ADAPTERS` | `paper,zerodha,...` | Deploy-time broker gate; docker-compose default `paper,zerodha` |
| `BROKER_HTTP_TIMEOUT_SEC` | `8` | Broker API request timeout |
| `BROKER_HTTP_GET_RETRIES` | `2` | GET request retries (not POST — order idempotency) |
| `BROKER_ANGEL_CLIENT_LOCAL_IP` | (empty) | SmartAPI `X-ClientLocalIP` header (NAT Elastic IP in production) |
| `BROKER_ANGEL_CLIENT_PUBLIC_IP` | (empty) | SmartAPI `X-ClientPublicIP` header |
| `BROKER_ANGEL_MAC_ADDRESS` | (empty) | SmartAPI `X-MACAddress` header |

### Zerodha OAuth

| Variable | Description |
|----------|-------------|
| `ZERODHA_API_KEY` | From developers.kite.trade (one per ZettaBridge instance) |
| `ZERODHA_API_SECRET` | Never stored in DB; used only for login_url and request_token exchange |
| `ZERODHA_CALLBACK_URL` | Must match redirect URL in Kite app settings |
| `ZERODHA_FRONTEND_URL` | Where to redirect browser after OAuth success |

### Compliance

| Variable | Default | Description |
|----------|---------|-------------|
| `SEBI_ALGO_ID_REQUIRED` | `false` (auto `true` in live mode) | Fail-close on live Indian orders without `algo_id` |
| `BROKER_ZERODHA_ALGO_ID` | (empty) | Platform-level Algo-ID fallback for Zerodha |
| `BROKER_ANGEL_ALGO_ID` | (empty) | Platform-level Algo-ID fallback for Angel |
| `BROKER_DHAN_ALGO_ID` | (empty) | Platform-level Algo-ID fallback for Dhan |

### Email

| Variable | Default | Description |
|----------|---------|-------------|
| `EMAIL_PROVIDER` | `log` | `log` (print to stdout) or `smtp` |
| `EMAIL_VERIFICATION_REQUIRED` | `false` | Gate login on verified email |
| `EMAIL_FROM` | (empty) | From address for outbound email |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS` | (empty) | SMTP credentials (smtp provider only) |

### Billing

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_ENABLED` | `false` | Enable Stripe billing endpoints |
| `STRIPE_SECRET_KEY` | (empty) | Stripe restricted API key |
| `STRIPE_WEBHOOK_SECRET` | (empty) | Stripe webhook signature secret |
| `STRIPE_PRICE_PAPER` | (empty) | Stripe Price ID for paper plan |
| `STRIPE_PRICE_PRO` | (empty) | Stripe Price ID for pro plan (alias: `STRIPE_PRICE_INDIVIDUAL`) |
| `STRIPE_PRICE_PRO_PLUS` | (empty) | Stripe Price ID for pro_plus plan |

### Metrics

| Variable | Default | Description |
|----------|---------|-------------|
| `METRICS_ENABLED` | `true` | Expose `GET /metrics` |
| `METRICS_TOKEN` | (empty) | Optional bearer token to protect `/metrics` |

### Registration Control

| Variable | Default | Description |
|----------|---------|-------------|
| `CLOSED_REGISTRATION` | `false` | Require `invite_code` on `POST /v1/auth/register` |
| `EARLY_BIRD_LIVE_ENABLED` | `false` | Allow live trading before plan gates (beta use) |
| `EARLY_BIRD_LIVE_ORDER_LIMIT` | `10` | Orders/day cap under early-bird mode |

---

## 4. Live Broker Mode (local)

To test with live broker mode locally:

```bash
docker compose -f docker-compose.yml -f docker-compose.live.yml up --build
```

This overlay sets `BROKER_MODE=live` and allows live broker credentials. You still need real credentials added via `POST /v1/credentials` — never put tokens in docker-compose files.

Angel SmartAPI also requires the correct `BROKER_ANGEL_CLIENT_LOCAL_IP` and `BROKER_ANGEL_CLIENT_PUBLIC_IP` to match your IP whitelist. For local dev this is typically your machine's public IP.

---

## 5. Database Migrations

Migrations are embedded in the binary via `embed.FS` and applied automatically at startup.

To run them manually against a local database:

```bash
# Individual file
psql $DATABASE_URL -f migrations/001_init.sql

# All files in order
for f in migrations/*.sql; do psql $DATABASE_URL -f "$f"; done
```

Migration files are in `migrations/` and numbered `NNN_description.sql`. They are additive — no rollback scripts. New migrations must be backwards-compatible.

---

## 6. Running Tests

```bash
# Unit + integration tests (no Docker required)
make test              # go test ./...
# or directly:
go test ./...

# Full functional E2E (requires docker-compose up first)
make functional-test

# Stripe billing flow (requires docker compose up with billing overlay)
make functional-test-billing

# Against staging URL
make functional-test-staging

# Live broker routing test (BROKER_MODE=live, mock credentials)
make functional-test-live-routing
```

The functional tests live in `functional-tester/` and use Go + HTTP against a running server. **Org lifecycle tests were removed** — the suite covers auth, paper accounts, credentials, webhooks, trades, P&L, WebSocket, and billing (with overlay).

Bruno API collections are in `bruno/ZettaBridge/` — use the Bruno desktop app or CLI for interactive exploration.

---

## 7. Load and Security Tests

```bash
# Load test smoke (small)
make loadtest-smoke

# Full load test against staging (500 VUs)
make loadtest-staging

# Security/pen test scripts against staging
make sectest-staging
```

Load test scripts are in `deploy/loadtest/k6/`. Security test scripts are in `deploy/sectest/scripts/`.

Both require external targets (staging URL) and optional config in `deploy/loadtest/env.local` or `deploy/sectest/env.local`.

---

## 8. Staging Deploy

```bash
# Preflight check from WSL (checks Docker, env, admin connectivity)
make preflight-staging

# HTTP smoke test against staging
make staging-smoke

# Push image + restart ECS tasks
make staging-release

# Full deploy (rebuild image + push + restart)
make staging-release-full
```

Staging deploy scripts are in `deploy/scripts/`. The `staging-release.sh` script handles ECR image push and ECS task force-new-deployment.

---

## 9. Dashboard Build

```bash
# Development server (hot reload)
make dev-dashboard

# Production build
make build-dashboard

# Docker production image (multi-stage)
cd dashboard && docker build --build-arg NEXT_PUBLIC_API_URL=https://api.staging.zettabridge.net -t zettabridge-dashboard .
```

`NEXT_PUBLIC_API_URL` is baked into the Next.js build at build time (not a runtime env var). To point the dashboard at a different API, rebuild the image.

---

## 10. OpenAPI Spec (Swagger)

The OpenAPI 2.0 spec is generated from Go annotations using [swaggo/swag](https://github.com/swaggo/swag). Generated output lives in `swagger/` and is committed alongside source.

### Regenerating the spec

```bash
# Install the swag CLI once (not added to go.mod — dev tool only)
go install github.com/swaggo/swag/cmd/swag@latest

# Regenerate from annotations
make swagger
# or, without Go/swag installed:
make swagger-py
```

Output: `swagger/swagger.json` (full spec), `swagger/swagger.yaml`, `swagger/docs.go` (embedded Go spec).

### Annotation files

| File | Purpose |
|------|---------|
| `cmd/api/docs.go` | API-level metadata: title, version, host, basePath, security schemes, 11 tag definitions |
| `internal/handler/swag_types.go` | Typed request/response structs used as `@Param body` and `@Success` schema refs |
| `internal/handler/swag_docs.go` | 48 never-called stub functions, one per endpoint, each with full `@Summary`, `@Tags`, `@Router`, `@Security` annotations |

### Adding annotations for a new endpoint

Add a new stub function to `internal/handler/swag_docs.go`:

```go
// myNewEndpointDoc godoc
// @Summary     Short description
// @Tags        webhooks
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body CreateWebhookRequest true "Request body"
// @Success     201 {object} APIResponse{data=WebhookResponse}
// @Failure     400 {object} ErrorResponse
// @Router      /v1/webhooks [post]
func myNewEndpointDoc() {}
```

Then run `make swagger` to regenerate. The stub function is never called; the annotations are parsed at generation time only.

---

## 11. Adding a New Broker Adapter

1. **Implement the broker** in `internal/adapters/<broker>/` or `internal/integrations/brokers/<broker>/` implementing `broker.Broker`.

2. **Register in `brokerfactory`** — add the new `broker_type` string to the factory switch.

3. **Optional sidecar** — add `cmd/<broker>-adapter/main.go` using `internal/execution/adapter/server` if deploying as HTTP sidecar.

4. **Gate with `ENABLED_ADAPTERS`** — add to Terraform `enabled_adapters` when ready for ECS.

5. **Add `raw_creds` format** to `docs/04-integrations/<broker>.md`.

6. **Add httptest fixtures** in `internal/integrations/brokers/<broker>/testdata/` for integration test coverage.

---

## 12. Code Style

- Follow standard Go conventions (`gofmt`, no custom linter configured).
- No ORM — use raw SQL via `database/sql`.
- No DI container — wire dependencies manually in `cmd/api/main.go`.
- Keep handler functions thin: validate input, call store/service, return response. Business logic belongs in `internal/` packages, not in handlers.
- `raw_creds` and credential material must never appear in log statements. Use `redact.Header()` for broker HTTP clients.
- New migrations must be additive (ALTER TABLE ADD COLUMN IF NOT EXISTS). No DROP without a careful migration path.
