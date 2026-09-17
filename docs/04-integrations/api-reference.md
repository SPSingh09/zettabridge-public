# API Reference
**Last updated: 2026-07-07**

ZettaBridge exposes a REST + WebSocket API over HTTPS. The full OpenAPI 2.0 spec is auto-generated from source annotations.

---

## 1. Spec Files

| File | Format | Use |
|------|--------|-----|
| `swagger/swagger.json` | OpenAPI 2.0 JSON | Machine-readable; import into Postman, Insomnia, or Bruno |
| `swagger/swagger.yaml` | OpenAPI 2.0 YAML | Human-readable; diff-friendly |
| `swagger/docs.go` | Go (build-ignored) | Embedded spec for future Swagger UI serving |

The spec is generated from annotations in:
- `cmd/api/docs.go` — API metadata, security schemes, tag definitions
- `internal/handler/swag_types.go` — typed request/response structs
- `internal/handler/swag_docs.go` — per-endpoint `@Router`, `@Summary`, `@Param`, `@Success`, `@Failure` stubs

Regenerate: `make swagger` (requires `go install github.com/swaggo/swag/cmd/swag@latest`).

---

## 2. Base URLs

| Environment | API Base URL |
|-------------|-------------|
| Staging | `https://api.staging.zettabridge.net` |
| Local dev | `http://localhost:8080` |

---

## 3. Authentication

Most endpoints require a JWT Bearer token. Obtain one from `POST /v1/auth/login`.

```http
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

| Token | TTL | Storage |
|-------|-----|---------|
| Access token (JWT) | 5 hours | `localStorage` key `zb_token` |
| Refresh token | Long-lived | `localStorage` key `zb_refresh` |

**Refresh:** `POST /v1/auth/refresh` with `{"refresh_token": "..."}` returns a new access token. Refresh tokens are single-use (rotated on each call).

**Logout:** `POST /v1/auth/logout` adds the access token to a Redis revocation set. All subsequent requests with that token return 401.

**WebSocket:** JWT is passed as a query parameter: `GET /v1/ws/trades?token=<jwt>`. Standard headers cannot be set during the WebSocket upgrade.

---

## 4. Endpoint Groups

### Monitoring

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/healthz` | None | Server health + timestamp; used by ECS target group |
| GET | `/metrics` | Optional bearer | Prometheus metrics; guarded by `METRICS_TOKEN` if set |

### Auth

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/v1/auth/register` | None | Register with email + password + optional invite code |
| POST | `/v1/auth/login` | None | Returns access + refresh tokens |
| POST | `/v1/auth/logout` | Bearer | Revokes access token |
| POST | `/v1/auth/refresh` | None | Exchange refresh token for new access token |
| GET | `/v1/auth/verify-email` | None | Verify email with token from verification email |
| POST | `/v1/auth/resend-verification` | None | Resend verification email |

### Account

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/me` | Bearer | Current user profile, plan limits, `live_trading_allowed`, auto-paused flags |

### Signals (TradingView entry point)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/v1/webhook/:token` | URL token | Ingest trade signal; **202** when queued, **409** when deduplicated |

The `:token` is a UUID v4 from the webhook object's `token` field. This is the URL pasted into TradingView alerts.

**Signal payload:**

```json
{
  "action": "BUY",
  "symbol": "NIFTY",
  "quantity": 1,
  "comment": "strategy_v2_long"
}
```

`action` must be in `BUY | SELL | CLOSE`. `comment` is used as part of the dedup hash.

### Streaming

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/ws/trades?token=<jwt>` | JWT query param | WebSocket; streams real-time trade updates to the authenticated user |

### Webhooks

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/webhooks` | Bearer | List all webhooks for the current user |
| POST | `/v1/webhooks` | Bearer | Create a new webhook |
| GET | `/v1/webhooks/:id` | Bearer | Get a single webhook |
| PUT | `/v1/webhooks/:id` | Bearer | Update webhook fields (label, guards, dedup, lot size) |
| DELETE | `/v1/webhooks/:id` | Bearer | Delete a webhook |
| PUT | `/v1/webhooks/:id/pause` | Bearer | Pause a webhook (stops ingest) |
| PUT | `/v1/webhooks/:id/resume` | Bearer | Resume a paused webhook |
| POST | `/v1/webhooks/:id/rotate-token` | Bearer | Generate a new ingest URL token; old token rejected immediately |
| GET | `/v1/webhooks/:id/trades` | Bearer | Trade audit log for this webhook |
| GET | `/v1/webhooks/:id/pnl` | Bearer | Aggregated P&L for this webhook |

### Trades

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/trades/:id` | Bearer | Single trade detail |
| DELETE | `/v1/webhooks/:id/trades/:tradeId` | Bearer | Cancel broker-side order |

| GET | `/v1/brokers/enabled` | Bearer | Deployment-enabled broker types (`ENABLED_ADAPTERS`) |

### Credentials (live broker)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/credentials` | Bearer | List broker credentials (never returns `raw_creds`) |
| POST | `/v1/credentials` | Bearer | Add live broker credential (`account_mode` must be `live`) |
| PUT | `/v1/credentials/:id` | Bearer | Update credential (e.g. refresh `raw_creds`, change `algo_id`) |
| DELETE | `/v1/credentials/:id` | Bearer | Delete credential |
| POST | `/v1/credentials/:id/verify` | Bearer | Probe broker API to verify credential validity |
| PUT | `/v1/credentials/:id/pause` | Bearer | Pause credential (stops trading on associated webhooks) |
| PUT | `/v1/credentials/:id/resume` | Bearer | Resume paused credential |
| GET | `/v1/credentials/zerodha/connect` | Bearer | Initiate Zerodha Kite Connect login flow |
| GET | `/v1/credentials/zerodha/status` | Bearer | Zerodha OAuth session status |
| GET | `/v1/credentials/zerodha/callback` | None | Zerodha OAuth redirect callback (browser redirect) |

### Paper accounts

Paper webhooks route via `paper_account_id` (not `broker_cred_id`). See [getting-started.md](../10-user-guides/getting-started.md).

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/paper-accounts` | Bearer | List paper trading accounts |
| POST | `/v1/paper-accounts` | Bearer | Create paper account (plan limits apply) |
| GET | `/v1/paper-accounts/:id` | Bearer | Get paper account detail |
| PATCH | `/v1/paper-accounts/:id` | Bearer | Update label / settings |
| PUT | `/v1/paper-accounts/:id/pause` | Bearer | Pause paper account |
| PUT | `/v1/paper-accounts/:id/resume` | Bearer | Resume paper account |
| PUT | `/v1/paper-accounts/:id/reset` | Bearer | Reset balances and positions |
| DELETE | `/v1/paper-accounts/:id` | Bearer | Delete paper account |
| GET | `/v1/paper-accounts/:id/positions` | Bearer | Open paper positions |
| POST | `/v1/paper-accounts/:id/positions/close` | Bearer | Close a paper position |
| GET | `/v1/paper-accounts/:id/orders` | Bearer | Paper order history |
| GET | `/v1/paper-accounts/:id/pnl` | Bearer | Paper account P&L |
| GET | `/v1/market-profiles` | Bearer | Active market profiles for paper symbol selection |

### Billing

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/billing/plans` | Bearer | List plans (`free`, `paper`, `pro`, `pro_plus`) with limits |
| POST | `/v1/billing/checkout` | Bearer | Create Stripe Checkout session (when `BILLING_ENABLED=true`) |
| POST | `/v1/billing/portal` | Bearer | Create Stripe Customer Portal session |
| POST | `/v1/webhooks/stripe` | Stripe signature | Stripe event webhook handler (internal) |

### Internal (adapter → Core, service token)

Used when `EXECUTION_ADAPTER_MODE=http`. Authenticated with `Authorization: Bearer <ZB_SERVICE_TOKEN>`.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/v1/internal/execution-events` | Service token | Adapter reports order fill/reject outcomes |
| PUT | `/v1/internal/credentials/:id/session` | Service token | Adapter updates broker session tokens after OAuth |

### Organisations (removed)

All `/v1/orgs/*` routes were **removed** from the router (migration 044). Regenerate OpenAPI from source with `make swagger` or `make swagger-py`.

### Admin (requires `role: admin` JWT claim)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/admin/users` | Admin Bearer | List all users with billing source |
| GET | `/v1/admin/users/:id` | Admin Bearer | User detail |
| PATCH | `/v1/admin/users/:id` | Admin Bearer | Patch user status or email_verified |
| PUT | `/v1/admin/users/:id/plan` | Admin Bearer | Set user plan (`free` / `paper` / `pro` / `pro_plus`; `billing_source=admin`) |
| GET | `/v1/admin/users/:id/audit` | Admin Bearer | User audit log |
| POST | `/v1/admin/invites` | Admin Bearer | Create platform invite |
| GET | `/v1/admin/invites` | Admin Bearer | List platform invites |
| GET | `/v1/admin/stats` | Admin Bearer | Platform-wide trade/user stats (if mounted) |

---

## 5. Common Error Response

All error paths return:

```json
{
  "error": "human-readable message",
  "error_code": "machine_readable_code"
}
```

Key `error_code` values:

| Code | Meaning |
|------|---------|
| `account_suspended` | User is suspended |
| `webhook_paused` | Webhook is manually or auto-paused |
| `guard_violation` | Signal blocked by `allowed_actions` or `allowed_symbols` |
| `rate_limited` | Per-webhook or per-plan rate limit exceeded |
| `queue_full` | In-process queue at capacity (transient) |
| `algo_id_required` | Live Indian broker order missing SEBI Algo-ID |
| `auth_failed` | Broker credential rejected by broker API |
| `invalid_credentials` | Credential format invalid at verify time |
| `dedup` | Signal was a duplicate within the dedup window |
| `email_not_verified` | Action requires verified email |

---

## 6. Signal Ingest Flow

```
POST /v1/webhook/:token
  → guard checks (active, not suspended, allowed_action, allowed_symbol, rate limit, dedup)
  → if duplicate within dedup_window_sec → HTTP 409 { queued: false, deduplicated: true }
  → enqueue to in-process channel (non-blocking, returns 202 immediately)
  → queue worker picks up
  → route: paper_account_id → paper engine/adapter; broker_cred_id → live adapter
  → maporder.Build → algo.Resolve (SEBI Algo-ID) → broker.PlaceOrder
  → trade row inserted (status: placed → filled/rejected)
  → WebSocket fan-out to authenticated owner
```

**HTTP status codes:**
- **202** — signal accepted into the queue (`queued: true`)
- **409** — duplicate within dedup window (`deduplicated: true`; not an error)
- **429** — rate limited (`rate_limited` metric)
- **403/422** — guard or plan violation (`rejected` metric)

The ingest endpoint never waits for broker execution. HTTP 202 means the signal was accepted into the queue, not that an order was placed.

### Webhook routing

| Webhook field set | Execution path |
|-------------------|----------------|
| `paper_account_id` | Paper engine (`EXECUTION_ADAPTER_MODE=local`) or paper sidecar (`http`) |
| `broker_cred_id` | Live broker adapter (Zerodha day-0 on ECS; others gated by `ENABLED_ADAPTERS`) |

Plan limits: four tiers — `free`, `paper`, `pro`, `pro_plus`. See [STATUS.md](../STATUS.md) and `internal/plan/plan.go`.
