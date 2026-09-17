# Glossary
**Last updated: 2026-07-07**

Key terms used throughout the ZettaBridge codebase and documentation.

---

## A

**Access token**
A short-lived JWT (HS256, 5-hour TTL) issued by `POST /v1/auth/login`. Carried as a `Bearer` header on all authenticated requests. Revoked server-side via Redis on logout.

**AES_KEY**
32-byte hex string stored in AWS Secrets Manager. Used by `internal/credenc` to AES-256-GCM encrypt and decrypt `raw_creds` at rest. Server refuses to start in production with the all-zero dev default.

**Algo-ID** (SEBI Algo Identifier)
Exchange-assigned identifier required on all API-originated NSE/BSE orders from August 2025 onwards. Stored as `algo_id` on broker credentials and snapshotted on the `trades` row. See [sebi-compliance.md](../09-compliance-and-legal/sebi-compliance.md).

**ALB** (Application Load Balancer)
AWS entry point. Terminates HTTPS, redirects HTTP → HTTPS, routes `/api/*` to ECS backend and `/*` to ECS dashboard.

---

## B

**Broker**
An entity that provides market access. Supported: Zerodha (Kite Connect), Angel One (SmartAPI), Dhan, MetaApi MT5. Each is wrapped by an adapter that implements `broker.Broker`.

**Broker adapter**
A Go struct in `internal/livebrokers/` that implements `PlaceOrder`, `CancelOrder`, `VerifyCredential` for a specific broker API.

**Broker credential** (`broker_credentials` table)
A user-owned record linking a ZettaBridge account to a broker account. Contains `encrypted_creds` (AES-256-GCM ciphertext), `algo_id`, `exchange`, `product`, `account_mode` (`live` only), `status`.

**BROKER_MODE**
Server environment variable controlling broker HTTP behavior. `mock`: simulated fills for staging/CI (not a user-facing feature). `live`: real broker HTTP calls when a Pro/Pro Plus user routes a live webhook to a credential. **Not** a plan tier or dashboard setting.

---

## C

**Capability token**
The webhook ingest URL token. It functions as a bearer capability — anyone with the URL can submit signals. Must be treated as a secret and rotated if exposed.

**credenc**
`internal/credenc` — Go package providing `Encrypt` and `Decrypt` functions for AES-256-GCM broker credential encryption.

---

## D

**Dedup** (Signal deduplication)
Prevents TradingView retry signals from placing duplicate orders. Implemented via Redis `SETNX` on a hash of the signal payload within a configurable `dedup_window_sec` window. Available on **paid plans for live webhooks**; paper webhooks are exempt from the advanced-guard plan gate.

**Demo mode** *(Removed 2026 Phase 0)*
Former `account_mode: demo` on broker credentials. Replaced by the **Paper** plan and paper webhooks (`paper_account_id`). See [scope.md](../01-product/scope.md).

---

## E

**ECS Fargate**
AWS compute for ZettaBridge API server and dashboard containers. No public IPs on tasks; inbound only via ALB.

**encrypted_creds**
The AES-256-GCM ciphertext of `raw_creds` stored in `broker_credentials.encrypted_creds`. The `json:"-"` tag ensures it is never serialised in API responses.

**Execution adapter**
A deployable sidecar (`cmd/paper-adapter`, `cmd/zerodha-adapter`) or in-process runner that executes orders on behalf of Core. Core routes via `EXECUTION_ADAPTER_MODE` (`local` or `http`). Which adapters exist at runtime is gated by `ENABLED_ADAPTERS` in deploy config.

**EXECUTION_ADAPTER_MODE**
Environment variable: `local` runs paper/live executors in-process; `http` routes Core → sidecar ECS services over `X-ZB-Service-Token`.

---

## F

**Fail-close**
Design principle: when a required check fails (missing Algo-ID, suspended account, revoked token, zero dev key in production), the system explicitly rejects rather than silently allowing.

**Fan-out hub**
`internal/tradepush` — in-process WebSocket hub that broadcasts trade updates to all authenticated WebSocket connections for the owning user.

**Free plan**
Entry tier. 1 paper account, 1 paper webhook, 0 live webhooks, 0 live credentials, 10 paper trades/month, 100 webhook ingests/day. No live trading. 1 display/enforced OPS.

---

## G

**Guard**
`internal/guard` — package that validates a signal before enqueuing. Checks: webhook active, account not suspended, action in `allowed_actions`, symbol in `allowed_symbols`, rate limit, dedup.

---

## H

**Household plan** *(Removed 2026 Phase 0)*
Former org-scoped paid tier. Replaced by solo **Pro** / **Pro Plus** plans. `/v1/orgs/*` routes removed from router.

---

## I

**IDOR** (Insecure Direct Object Reference)
Attack where a user accesses another user's resource by guessing an ID. Mitigated in ZettaBridge by scoping all queries to `WHERE user_id = $userID`.

**Idempotency**
Stripe webhook events are idempotent on `stripe_webhook_events.id`. Duplicate events (Stripe replays) are silently ignored.

**Individual plan** *(Removed 2026 Phase 0)*
Former single-user paid tier. Replaced by **Pro** and **Pro Plus** (migrations 044/046).

**Ingest**
`POST /v1/webhook/:token` — the public endpoint that receives trade signals from TradingView. Non-blocking: returns HTTP 202 immediately after enqueueing.

---

## J

**JWT** (JSON Web Token)
HS256-signed token issued on login. Contains `sub` (user ID), `role` (user/admin), `exp`, `iat`. 5-hour access token TTL.

**JWT_SECRET**
32+ character string stored in AWS Secrets Manager. Used to sign and verify JWTs. Compromising this allows forging tokens for any user.

---

## K

**Kite Connect**
Zerodha's API platform. Access tokens expire daily (~3 AM IST). Each Kite Connect app trades only the API key owner's account — multi-user OAuth is not supported.

---

## L

**Live broker**
A real broker API (vs mock). Available to **Pro** and **Pro Plus** users on webhooks with `broker_cred_id` when `BROKER_MODE=live`.

**lot_size**
Webhook-level multiplier. Signal `quantity` × `lot_size` = shares/lots sent to the broker.

---

## M

**Migration**
Numbered SQL file in `migrations/001_init.sql` through `migrations/046_expand_plan_tiers.sql`. Applied on server startup via `internal/migrate`.

**mock broker**
`internal/mockbrokers` — simulates broker fills when `BROKER_MODE=mock` (staging/CI). Returns `MOCK-` prefixed order IDs. Distinct from the **paper engine** used for user paper trading.

**Multi-tenant**
Multiple users share the same ZettaBridge infrastructure. Data isolation is enforced via `user_id` scoping in all queries.

---

## N

**NAT Gateway**
AWS NAT Gateway with a static Elastic IP. All ECS outbound broker API calls exit through this IP. Brokers (Dhan, Angel) whitelist this IP.

---

## O

**Org** (Organisation) *(Removed from product 2026 Phase 0)*
Former multi-user entity with shared webhook/credential pools. API routes removed; legacy DB columns/tables remain for existing data.

**OPS** (Orders Per Second)
Rate limit unit. SEBI regulates Indian algo trading by OPS threshold (≤10 OPS = generic Algo ID; >10 OPS = registered Algo Provider).

---

## P

**Paper account** (`paper_accounts` table)
User-owned simulated trading account. Webhooks with `paper_account_id` route to the paper engine (in-process or HTTP paper adapter). Positions, orders, and PnL persist in PostgreSQL.

**Paper plan**
Paid simulation tier. 3 paper accounts, 5 paper webhooks, unlimited paper trades, audit logs, advanced webhook guards, notifications. No live trading. 5 display/enforced OPS.

**Plan**
Subscription tier: **Free**, **Paper**, **Pro**, **Pro Plus**. Controls paper/live webhook limits, credential limits, OPS cap, and live trading access. Source: `internal/plan/plan.go`.

**PlaceOrder**
`broker.Broker.PlaceOrder(ctx, order, cred)` — the core method every broker adapter implements. Called by queue workers when processing a signal.

**Pro plan**
Paid live-trading tier. 5 paper accounts, 8 paper webhooks, **1 live webhook**, 1 live credential, unlimited paper trades, live trading enabled. 10 OPS. Multi-product creds: No.

**Pro Plus plan**
Top paid tier. 10 paper accounts, 15 paper webhooks, **5 live webhooks**, 3 live credentials, unlimited paper trades, live trading, multi-product credentials (MIS + CNC + NRML). 10 OPS.

**P&L** (Profit and Loss)
Aggregate metric computed from filled trades: `total_pnl`, `win_rate`, `trade_count`, `total_volume`. Available per-webhook.

---

## Q

**Queue**
`internal/queue` — in-process Go buffered channel (`QUEUE_BUFFER`, default 1000 signals) + `WORKER_COUNT` goroutines (default 20). Non-blocking ingest; workers call `PlaceOrder` asynchronously.

---

## R

**raw_creds**
Plaintext broker credentials submitted by the user on POST/PUT. Format is colon-delimited (`api_key:access_token`). Encrypted before storage; never returned by GET endpoints.

**Refresh token**
Long-lived token stored as a SHA-256 hash in `refresh_tokens`. Single-use — rotated on each `POST /v1/auth/refresh`. Revoked on logout.

**Revocation**
On logout, the JWT is stored in Redis with TTL = remaining expiry. `CheckRevoked` middleware checks Redis on every request and returns 401 if found.

---

## S

**SEBI** (Securities and Exchange Board of India)
India's capital markets regulator. Mandates Algo-ID tagging on all API-originated orders from August 2025.

**signal**
A JSON payload from TradingView: `{action, symbol, quantity, comment}`. Received at `POST /v1/webhook/:token`.

**signal_key**
Redis key used for dedup: hash of webhook ID + action + symbol + lot + comment (or full payload hash if no comment). Stored with `SETNX` for `dedup_window_sec`.

**SmartAPI**
Angel One's API platform.

---

## T

**Trade**
A row in the `trades` table representing one attempted order execution. Fields include `status` (queued → placed → filled/rejected), `broker_order`, `error_code`, `algo_id`.

**TradingView**
External charting and alerting platform. Sends webhook HTTP POST to ZettaBridge's ingest endpoint when a strategy condition fires.

**Trust boundary**
A line where security assumptions change. ZettaBridge has three: TradingView → ALB (webhook URL token), Browser → API (JWT Bearer), ECS → AWS (IAM role).

---

## V

**Vendor**
`vendor/` directory in the repo root. All Go dependencies are vendored; `go mod vendor` pins versions. Go operates in `-mod=vendor` mode automatically.

---

## W

**Webhook** (ZettaBridge)
A ZettaBridge configuration object (`webhooks` table) linking a TradingView alert to either a **paper account** (`paper_account_id`) or a **live broker credential** (`broker_cred_id`). Has its own UUID v4 ingest token, guard settings, and dedup config.

**Worker**
A goroutine in `internal/queue` that reads from the buffered channel and calls `PlaceOrder` for each signal. Worker count is configurable via `WORKER_COUNT`.

**WebSocket**
`GET /v1/ws/trades?token=<jwt>` — real-time trade update stream. JWT passed as query param because standard WebSocket upgrade cannot carry custom headers.
