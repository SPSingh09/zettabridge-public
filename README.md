# ZettaBridge — Algorithmic Trading Webhook Gateway

A production-grade Go Fiber server that accepts webhook signals from TradingView
or any HTTP client and routes them to MT5 Cloud, Zerodha, Angel One, or Dhan —
concurrently, with rate limiting, credential encryption, and a full audit log.

---

## Quick Start (local dev)

```bash
# 1. Clone & install Go 1.22+
git clone https://github.com/SPSingh09/zettabridge
cd zettabridge

# 2. Spin up Postgres + Redis + server
docker-compose up

# 3. The server is live at http://localhost:8080
```

---

## API Reference

### Auth

```
POST /v1/auth/register
{ "email": "trader@example.com", "password": "secret" }
→ { "data": { "id": "...", "email": "...", "email_verification_required": true } }  (when enabled)

GET  /v1/auth/verify-email?token=ev_...
→ { "data": { "verified": true, "email": "..." } }

POST /v1/auth/resend-verification
{ "email": "trader@example.com" }

POST /v1/auth/login
{ "email": "trader@example.com", "password": "secret" }
→ { "data": { "token": "<access JWT>", "expires_in": 86400 } }
  Returns **403** `email not verified` when `EMAIL_VERIFICATION_REQUIRED=true` and inbox not verified.

POST /v1/auth/refresh
{ "refresh_token": "rt_..." }
→ new access + rotated refresh token

POST /v1/auth/logout          (Bearer access token required)
{}                            → 204, revokes all refresh tokens for user
{ "refresh_token": "rt_..." } → 204, revokes single session (optional)

PUT  /v1/auth/password        (Bearer access token required)
{ "current_password": "...", "new_password": "..." }
→ revokes all refresh tokens, returns new access + refresh tokens
```

All protected endpoints require:
```
Authorization: Bearer <access JWT>
```

Login JWT includes `role` (`user` or `admin`). Suspended accounts receive `403 account suspended`.

### Platform Admin (Phase 0 + 1)

Bootstrap first admin:
```bash
# 1. Run migration: psql $DATABASE_URL -f migrations/002_admin_users.sql
# 2. Register admin@example.com via POST /v1/auth/register
# 3. Set env and restart server:
BOOTSTRAP_ADMIN_EMAIL=admin@example.com
```

```
GET   /v1/admin/users              → list users (?page=1&limit=50&plan=&status=&email=)
GET   /v1/admin/users/:id          → user detail + webhook/broker counts
PATCH /v1/admin/users/:id          → { "status", "orgs_enabled", "early_bird_live", "email_verified" }
PUT   /v1/admin/users/:id/plan     → { "plan": "free" | "pro" | "enterprise" }
```

Requires `Authorization: Bearer <admin JWT>`. Regular users receive `403`.

See **[docs/plans-and-billing.md](docs/plans-and-billing.md)** for plan pricing, enterprise member tiers, and early-bird live access.

### Enterprise Organizations (E0 + E1)

Run migration:
```bash
docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/003_organizations.sql
```

Bootstrap flow (admin-enabled org creation):
1. Register owner account
2. Platform admin: `PUT /v1/admin/users/:id/plan` → `enterprise`
3. Platform admin: `PATCH /v1/admin/users/:id` → `{ "orgs_enabled": true }`
4. Owner login → `POST /v1/orgs` → `{ "name": "Acme Trading" }`

```
POST   /v1/orgs           → create org (enterprise + orgs_enabled, one org per user)
GET    /v1/orgs           → list orgs for current user
GET    /v1/orgs/:id       → org detail (member only)
PATCH  /v1/orgs/:id       → rename (owner/admin)
DELETE /v1/orgs/:id       → delete org (owner only)
```

`GET /v1/me` and login include `org_id` / `org_role` when user belongs to an org. Also returns `orders_per_sec` (plan marketing; `null` = unlimited for enterprise) and `orders_per_sec_enforced` (actual worker cap per user).

Solo `free`/`pro` users are unchanged — existing webhooks/credentials remain `user_id` scoped.

Run migration:
```bash
docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/004_org_invites.sql
```

### Enterprise Members & Invites (E2)

```
GET    /v1/orgs/:id/members              → list members (any member)
GET    /v1/orgs/:id/members/:userId      → member detail
PATCH  /v1/orgs/:id/members/:userId      → { "role" } and/or { "status" } (owner/admin)
DELETE /v1/orgs/:id/members/:userId      → remove member (owner/admin, not owner)
POST   /v1/orgs/:id/invites              → { "email", "role": "admin"|"member"|"viewer" }
GET    /v1/orgs/:id/invites              → pending invites (owner/admin)
DELETE /v1/orgs/:id/invites/:inviteId    → revoke invite
GET    /v1/invites/:token                → public preview (no JWT)
POST   /v1/invites/accept                → { "token", "password" } → JWT + org membership
```

Invitees may be `free`/`pro` (no enterprise plan required). Seats = active members + pending invites.
Invite tokens are stored hashed; raw token returned once on create.

Run migration:
```bash
docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/005_e3_resources.sql
```

### Enterprise Shared Resources (E3)

Existing endpoints are org-aware when the user has active org membership:

| Endpoint | Solo user | Org member | Org viewer |
|----------|-----------|------------|------------|
| `GET /v1/webhooks` | own solo webhooks | org + legacy solo | read org + solo |
| `POST /v1/webhooks` | create solo | create org webhook | **403** |
| `PUT pause/resume`, `DELETE` | own solo | own or admin/owner for all | **404** on mutate |
| `GET /v1/credentials` | own | org cred labels (no secrets) | read only |
| `POST/PUT/DELETE /v1/credentials` | own | **owner/admin only** | **403** |

Org-scoped views:
```
GET /v1/orgs/:id/webhooks   → all org webhooks (any member)
GET /v1/orgs/:id/trades     → cross-webhook trade feed (any member)
GET /v1/orgs/:id/pnl        → org P&L rollup (any member)
```

New org webhooks/credentials get `org_id` + `created_by`. Solo resources keep `org_id` NULL.

Run migrations:
```bash
docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/006_org_audit.sql
docker compose exec -T postgres psql -U postgres -d zettabridge < migrations/007_trades_webhook_cascade.sql
```

`007` adds `ON DELETE CASCADE` on `trades.webhook_id` so webhook deletes succeed after signal ingest.

### Enterprise Governance (E4)

Org member endpoints:
```
POST /v1/orgs/:id/transfer-ownership  → { "user_id" } (owner only)
POST /v1/orgs/:id/leave               → non-owner leaves org
GET  /v1/orgs/:id/usage               → seats, webhooks, trades (owner/admin)
GET  /v1/orgs/:id/activity            → audit log (owner/admin)
```

Platform admin org endpoints:
```
GET   /v1/admin/orgs                  → list orgs (?page, limit, status, name)
GET   /v1/admin/orgs/:id              → org detail + counts
PATCH /v1/admin/orgs/:id              → { "status", "seat_limit" }
GET   /v1/admin/orgs/:id/members      → support view of members
```

Suspended orgs block org-scoped resource access. `seat_limit` cannot be set below current seats used.

---

### Broker Credentials

Broker secrets use colon-delimited `raw_creds` (plaintext over HTTPS; encrypted at rest with AES-256-GCM).

| `broker_type` | `raw_creds` format |
|---------------|-------------------|
| `zerodha` | `api_key:access_token` |
| `mt5_cloud` | `auth_token:account_id` |
| `dhan` | `client_id:access_token` |
| `angel` | `api_key:client_code:jwt` |

Indian brokers (`zerodha`, `angel`, `dhan`) require `exchange` (`NSE` or `BSE`). `product` defaults to `MIS` (`CNC`, `NRML` optional).

**Token lifecycle (v1):** manual refresh only — users re-`PUT` `raw_creds` when broker session tokens expire. No automatic refresh jobs.

| Broker | Token TTL (typical) | Refresh action |
|--------|---------------------|----------------|
| Zerodha | ~1 day | New Kite access token → PUT `api_key:access_token` |
| Angel | Session (hours) | New SmartAPI JWT → PUT `api_key:client_code:jwt` |
| Dhan | ~24 h | New token from web.dhan.co → PUT `client_id:access_token` |
| MT5 Cloud | Long-lived | Confirm MetaApi `auth_token:account_id` |

Full details: **[docs/broker-guide.md](docs/broker-guide.md)** (symbols, IP whitelist, Bruno flows).

```
POST /v1/credentials
{
  "broker_type":   "zerodha",
  "account_label": "Zerodha Demo",
  "account_mode":  "demo",
  "exchange":      "NSE",
  "product":       "MIS",
  "raw_creds":     "api_key:access_token"
}

PUT  /v1/credentials/:id
POST /v1/credentials/:id/verify   → { valid, broker_type, account_mode?, exchange?, product?, error?, error_code?, hint? }
GET  /v1/credentials
```

`BROKER_MODE=mock` (default) uses simulated adapters for dev/CI. `BROKER_MODE=live` uses `internal/livebrokers` (real HTTP) for pro/enterprise and eligible early-bird free users. Free users outside the early-bird window route to **mock** (Indian brokers) or the **MT5 demo** server when configured — see [docs/plans-and-billing.md](docs/plans-and-billing.md#free-tier-execution-routing).

**Verify (`POST /v1/credentials/:id/verify`):** checks `raw_creds` format and credential options. When execution routes to live, also calls broker equity/margin API. Mock/paper routes skip the live probe. On failure returns `error`, optional `error_code` (e.g. `auth_failed`), and `hint` with refresh steps (manual token re-PUT in v1).

### Live broker adapters (`BROKER_MODE=live`)

| Broker | Equity probe | Orders (v1) | Symbol resolution | CLOSE policy |
|--------|--------------|-----------|-------------------|--------------|
| `mt5_cloud` | Account information | Market BUY/SELL | Webhook `symbol` as-is | `POSITIONS_CLOSE_SYMBOL` (symbol-only) |
| `zerodha` | Kite `user/margins` | Market only (no CO/bracket) | Kite `tradingsymbol` | Fetch net position → opposite MARKET |
| `angel` | SmartAPI `getRMS` | Market only | `searchScrip` + Redis cache (`symtoken:angel:…`) | Fetch positions → opposite MARKET |
| `dhan` | `fundlimit` (`availabelBalance`) | Market only | Instrument CSV + Redis cache (`symtoken:dhan:…`) | Fetch positions → opposite MARKET |

Indian brokers map credential `exchange` → broker segment (`NSE` → `NSE_EQ`, etc.) and `product` → broker product (`MIS` → `INTRADAY`, `CNC` → `CNC`, `NRML` → `MARGIN`).

**Live broker HTTP:** shared outbound client with configurable timeout and GET-only retries. Base URLs default to each broker’s public API; override with env vars. Credentials with `account_mode=demo` use `*_DEMO_BASE_URL` when set.

| Variable | Default | Description |
|----------|---------|-------------|
| BROKER_HTTP_TIMEOUT_SEC | 8 | Outbound broker HTTP timeout (seconds) |
| BROKER_HTTP_GET_RETRIES | 2 | Retries for idempotent GETs only (502/503/504) |
| BROKER_MT5_BASE_URL | MetaApi live API | MT5 live base URL |
| BROKER_MT5_DEMO_BASE_URL | (falls back to live default) | MT5 demo override |
| BROKER_ZERODHA_BASE_URL | https://api.kite.trade | Zerodha live base URL |
| BROKER_ZERODHA_DEMO_BASE_URL | (falls back to live default) | Zerodha demo override |
| BROKER_ANGEL_BASE_URL | https://apiconnect.angelone.in | Angel live base URL |
| BROKER_ANGEL_DEMO_BASE_URL | (falls back to live default) | Angel demo override |
| BROKER_DHAN_BASE_URL | https://api.dhan.co | Dhan live base URL |
| BROKER_DHAN_DEMO_BASE_URL | (falls back to live default) | Dhan demo override |
| BROKER_ANGEL_CLIENT_LOCAL_IP | 127.0.0.1 | SmartAPI `X-ClientLocalIP` (set to server egress in prod) |
| BROKER_ANGEL_CLIENT_PUBLIC_IP | 127.0.0.1 | SmartAPI `X-ClientPublicIP` (NAT/static IP Angel whitelists) |
| BROKER_ANGEL_MAC_ADDRESS | 00:00:00:00:00:00 | SmartAPI `X-MACAddress` (shared server value for all users) |

Broker HTTP logs include `broker`, `op`, `method`, `path`, `status`, and `latency_ms` — never auth headers or tokens.

**IP egress:** MT5, Zerodha, Angel, and Dhan may require **static IP whitelisting** for order APIs. Route outbound traffic through a NAT Gateway (or equivalent) and register those IPs with each broker. Dhan order placement explicitly requires whitelisted IPs.

| Broker | Whitelist | Notes |
|--------|-----------|-------|
| Dhan | Developer portal → IP | Required for live orders |
| Angel | SmartAPI app → static IP | Set `BROKER_ANGEL_CLIENT_*` to NAT egress |
| Zerodha | Kite app (optional) | If IP restriction enabled on API key |
| MetaApi | Account setup | `auth_token:account_id` provisioning |

Portal steps: **[docs/broker-guide.md](docs/broker-guide.md#ip-whitelist-production)**.

---

### Order mapping & CLOSE policy

- **Symbols:** send `SYMBOL` in webhook/signal; exchange is taken from the credential for Indian brokers.
- **Quantity:** MT5 = standard lots; India = integer share quantity from risk engine.
- **SL/TP (v1):** Indian live adapters use **market orders only** — webhook `sl_points` / `tp_points` are not sent as bracket/CO orders until LTP-based conversion is added. MT5 live also uses market-style orders in v1.
- **CLOSE:**
  - **MT5:** MetaApi `POSITIONS_CLOSE_SYMBOL` for the resolved symbol
  - **Zerodha / Angel / Dhan:** read open net position for symbol + exchange + product → place opposite MARKET order (fails with `no_position` if flat)

Forex symbols (e.g. `EURUSD`) are rejected when the webhook credential is an Indian broker.

---

### Trade lifecycle

| Layer | Status | Meaning |
|-------|--------|---------|
| HTTP 202 ingest | `queued: true` | Signal passed guards and entered the internal queue |
| DB `trades.status` | `queued` → `submitted` \| `filled` \| `rejected` | Broker outcome |

Under `BROKER_MODE=mock`, successful orders are recorded as **`submitted`** (not `filled`). `filled` is used when the broker confirms a fill price.

Rejected trades include `error` (public message) and `error_code` (stable ops code, e.g. `auth_failed`, `invalid_symbol`, `rate_limited`). Error codes are not returned on the ingest HTTP response.

Trade rows also store `comment` (from the signal) and `signal_key` (idempotency hash) for ops correlation. See **[docs/audit-trail.md](docs/audit-trail.md)**.

---

### Webhooks

**Symbol conventions:** pair webhook `symbol` with credential `broker_type`. Use `EURUSD` / `GBPUSD` with `mt5_cloud`; use equity tickers (`RELIANCE`, `TCS`) with Indian brokers + matching `exchange` on the credential. Forex symbols on Indian credentials are rejected (`error_code=invalid_symbol`). See **[docs/broker-guide.md](docs/broker-guide.md#symbol-conventions)**.

```
POST /v1/webhooks
{
  "label":          "EURUSD Scalper",
  "broker_cred_id": "<credential-id>",
  "symbol":         "EURUSD",
  "lot_size":       0.01,
  "max_risk_pct":   1.0,
  "sl_points":      20,
  "tp_points":      28,
  "allowed_actions": ["BUY", "SELL"],
  "max_lot_size":   0.5
}
```

**Guard settings (all plans):** `allowed_actions`, `max_lot_size`

**Guard settings (pro+):** `allowed_symbols`, `signal_overrides`, `rate_limit_per_sec`, `dedup_window_sec`, `timezone`, `trading_hours`, `required_comment`

`allowed_symbols` Option A: empty list → only `symbol` is permitted; non-empty → whitelist only.

Example pro guard config:
```json
{
  "allowed_symbols": ["EURUSD", "GBPUSD"],
  "signal_overrides": { "symbol": false, "lot": false, "sl_pts": true, "tp_pts": true },
  "rate_limit_per_sec": 2,
  "timezone": "Europe/London",
  "trading_hours": { "mon": [{"start": "08:00", "end": "17:00"}] },
  "required_comment": "my-tv-secret"
}
```

→ create response: `{ "data": { "token": "wh_live_abc123...", ... } }`

```
GET  /v1/webhooks                    → list all your webhooks
PUT  /v1/webhooks/:id                → partial update (label, symbol, lot_size, max_risk_pct, sl_points, tp_points, broker_cred_id)
POST /v1/webhooks/:id/rotate-token   → issue new URL token (old token stops working; update TradingView alerts)
PUT  /v1/webhooks/:id/pause          → pause (stops processing signals)
PUT  /v1/webhooks/:id/resume         → resume
DELETE /v1/webhooks/:id              → delete
GET  /v1/webhooks/:id/trades         → last 100 trades for this webhook (includes comment, signal_key, error_code)
GET  /v1/webhooks/:id/pnl            → P&L rollup (pro+; trade_count, by_status, notional, fill_rate, win_rate)
GET  /v1/ws/trades                   → WebSocket live trade push (JWT; `{ "type": "trade", "trade": {...} }`)
```

---

### Signal Ingestion (TradingView webhook URL)

```
POST /v1/webhook/wh_live_abc123...
Content-Type: application/json

{
  "action":  "BUY",        // BUY | SELL | CLOSE
  "symbol":  "EURUSD",     // optional override
  "lot":     0.01,         // optional override
  "sl_pts":  20,
  "tp_pts":  28,
  "comment": "tv_alert"
}

→ 202 { "queued": true, "request_id": "req_xyz" }
```

**In TradingView:** paste your webhook URL and use this alert message template:
```json
{"action":"{{strategy.order.action}}","symbol":"{{ticker}}","comment":"{{strategy.order.comment}}"}
```

---

## Environment Variables

| Variable       | Default                    | Description                          |
|----------------|----------------------------|--------------------------------------|
| PORT           | 8080                       | HTTP listen port                     |
| DATABASE_URL   | postgres://...localhost... | PostgreSQL connection string         |
| REDIS_URL      | redis://localhost:6379     | Redis connection string              |
| JWT_SECRET     | (required in prod)         | HS256 signing key, min 32 chars      |
| ACCESS_TOKEN_TTL_SEC | 3600                 | Access JWT lifetime (seconds)        |
| REFRESH_TOKEN_TTL_DAYS | 30                   | Refresh token lifetime (days)        |
| AES_KEY        | (required in prod)         | 32-byte hex for credential encryption|
| WORKER_COUNT   | 20                         | Goroutine worker pool size           |
| QUEUE_BUFFER   | 500                        | Buffered channel size                |
| CORS_ORIGINS   | http://localhost:3000      | Allowed origins                      |
| BOOTSTRAP_ADMIN_EMAIL | (optional)          | Promotes existing user to `admin` on startup |
| DEFAULT_ORG_SEAT_LIMIT | 5                  | Legacy fallback; org seats come from owner's `enterprise_tier` |
| EARLY_BIRD_LIVE_ENABLED | false             | Platform early-bird live master switch |
| EARLY_BIRD_LIVE_ORDER_LIMIT | 10            | Max live orders per eligible free user |
| EARLY_BIRD_LIVE_STARTS_AT | (empty)         | Global offer start (RFC3339) |
| EARLY_BIRD_LIVE_ENDS_AT | (empty)           | Global offer expiry (RFC3339) |
| ORG_INVITE_TTL_DAYS    | 7                  | Days until org invite expires |
| EMAIL_VERIFICATION_REQUIRED | true            | Block login until email verified |
| EMAIL_VERIFICATION_TTL_HOURS | 24             | Verification link expiry |
| EMAIL_PROVIDER | log                        | `log` (dev) or `smtp` |
| EMAIL_FROM     | noreply@localhost          | From address for verification mail |
| APP_PUBLIC_URL | http://localhost:8080      | Base URL for verify links |
| SMTP_HOST / SMTP_PORT / SMTP_USER / SMTP_PASS | — | SMTP when `EMAIL_PROVIDER=smtp` |
| BROKER_MODE    | mock                       | `mock` (dev/CI) or `live` (real broker HTTP) |
| BROKER_HTTP_TIMEOUT_SEC | 8                 | Outbound broker HTTP timeout (seconds) |
| BROKER_HTTP_GET_RETRIES | 2                 | GET-only retries on 502/503/504 |
| BROKER_MT5_BASE_URL | (MetaApi default)     | MT5 live API base URL |
| BROKER_MT5_DEMO_BASE_URL | (optional)        | MT5 demo API base URL |
| BROKER_ZERODHA_BASE_URL | https://api.kite.trade | Zerodha live API base URL |
| BROKER_ZERODHA_DEMO_BASE_URL | (optional)   | Zerodha demo API base URL |
| BROKER_ANGEL_BASE_URL | https://apiconnect.angelone.in | Angel live API base URL |
| BROKER_ANGEL_DEMO_BASE_URL | (optional)      | Angel demo API base URL |
| BROKER_DHAN_BASE_URL | https://api.dhan.co  | Dhan live API base URL |
| BROKER_DHAN_DEMO_BASE_URL | (optional)       | Dhan demo API base URL |
| BROKER_ANGEL_CLIENT_LOCAL_IP | 127.0.0.1     | Angel SmartAPI local IP header |
| BROKER_ANGEL_CLIENT_PUBLIC_IP | 127.0.0.1    | Angel SmartAPI public/egress IP header |
| BROKER_ANGEL_MAC_ADDRESS | 00:00:00:00:00:00 | Angel SmartAPI MAC header |

---

## Project Layout

```
zettabridge/
├── cmd/api/main.go          # Entry point, app wiring, graceful shutdown
├── internal/
│   ├── config/config.go        # Env var loader
│   ├── handler/handler.go      # All Fiber HTTP handlers + route registration
│   ├── middleware/auth.go      # JWT validation middleware
│   ├── queue/queue.go          # Buffered channel worker pool
│   ├── broker/broker.go        # Broker interface + order types
│   ├── mockbrokers/            # Simulated adapters (dev/CI)
│   ├── livebrokers/            # Live HTTP adapters (MT5, Zerodha, Angel, Dhan)
│   ├── brokerfactory/          # mock|live routing (live never falls back to mock)
│   ├── brokers/
│   │   ├── maporder/           # Webhook → PlaceRequest mapping
│   │   └── symboltoken/        # Redis-backed instrument token cache
│   └── store/
│       ├── models.go           # Shared domain structs
│       ├── postgres.go         # PostgreSQL queries
│       └── redis.go            # Redis cache + rate limit helpers
├── migrations/
│   ├── 001_init.sql            # PostgreSQL DDL
│   ├── 002_webhook_guards.sql  # Guard columns (run on existing DBs)
│   └── 003_auth_sessions.sql   # Refresh tokens (run on existing DBs)
├── docker-compose.yml          # Local dev stack
├── Dockerfile                  # Multi-stage production image
└── go.mod
```

---

## Testing

See **[docs/testing.md](docs/testing.md)** for the full strategy.

```bash
make test              # unit + httptest + integration (CI-safe)
make functional-test   # E2E API via docker-compose (mock brokers)
```

Golden broker fixtures: `internal/livebrokers/testdata/`. Manual live QA: Bruno **Live Smoke** folder (`BROKER_MODE=live`).

---

## Production Checklist

See **[docs/roadmap.md](docs/roadmap.md)** for pending work, phased implementation order, and sprint plan.

See **[docs/deploy-production.md](docs/deploy-production.md)** for ECS env template, rollout steps, and broker IP whitelist.

See **[docs/broker-guide.md](docs/broker-guide.md)** for credential formats, token refresh, and symbol conventions.

See **[docs/operations-security.md](docs/operations-security.md)** for incident response (duplicate orders, credential leak), suspension behavior, log hygiene, and egress verification.

- [ ] Set a strong `JWT_SECRET` (32+ random chars)
- [x] Set `AES_KEY` to a random 32-byte hex (`openssl rand -hex 32`); credentials encrypted at rest via AES-256-GCM (`internal/credenc`)
- [ ] Set `APP_ENV=production` and `BROKER_MODE=live` on ECS tasks (server fails startup on invalid `BROKER_MODE` or weak `AES_KEY`)
- [ ] Use AWS ECS + ECR; deploy `ap-south-1` (India) and/or EU/US for MT5 (see deploy runbook)
- [ ] NAT Gateway with static egress IPs for broker whitelist
- [ ] Whitelist NAT egress IP at **Dhan** (required) and **Angel** SmartAPI app
- [ ] Set `BROKER_ANGEL_CLIENT_PUBLIC_IP` / `BROKER_ANGEL_CLIENT_LOCAL_IP` to NAT egress IP
- [x] Document token refresh runbook for users — see [docs/broker-guide.md](docs/broker-guide.md#token-refresh-v1--manual-only)
- [x] Operational & security runbook — see [docs/operations-security.md](docs/operations-security.md)
- [ ] Enable RDS automated backups
- [x] Add Prometheus metrics (`GET /metrics`) — see [docs/deploy-production.md](docs/deploy-production.md#post-deploy-monitoring)
- [ ] Grafana dashboard (wire Prometheus to your charts)
- [ ] Integrate Stripe for plan billing
