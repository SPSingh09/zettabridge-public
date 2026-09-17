# Configuration Reference
**Last updated: 2026-07-07**

All configuration is loaded from environment variables by `internal/config/config.go` at startup. Production values are injected via AWS ECS task definitions from Secrets Manager; local dev uses `docker-compose.yml` or a `.env` file.

`config.Validate()` runs at startup and will abort the process on invalid configuration: unknown `BROKER_MODE`, weak `AES_KEY` in production, or missing Stripe keys when billing is enabled.

---

## Core

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | TCP port the API server listens on |
| `APP_ENV` | _(none)_ | Set to `production` to enable production-safety guards (weak `AES_KEY` rejected at startup) |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/zettabridge?sslmode=disable` | PostgreSQL DSN |
| `REDIS_URL` | `redis://localhost:6379` | Redis URL for JWT revocation, signal dedup, and rate limiting |
| `JWT_SECRET` | `change-me-in-production-min-32-chars!!` | HS256 signing key. Min 32 chars. Rotate in production. |
| `AES_KEY` | `0000…0000` (32 zero bytes) | 32-byte hex key for AES-256-GCM credential encryption. All-zero default is rejected when `APP_ENV=production`. |
| `CORS_ORIGINS` | `http://localhost:3000` | Comma-separated allowed origins for CORS. Set to the dashboard domain in production. |
| `APP_PUBLIC_URL` | `http://localhost:8080` | Base URL of the API server. Used in email verification links and billing redirect URLs. Must be `https://` when `BILLING_ENABLED=true`. |
| `FRONTEND_URL` | value of `APP_PUBLIC_URL` | Base URL of the dashboard. Used in post-OAuth redirects. Defaults to `APP_PUBLIC_URL` if unset. |

---

## Execution Adapters

Controls how Core routes paper and live orders. See [STATUS.md](../STATUS.md).

| Variable | Default | Description |
|----------|---------|-------------|
| `EXECUTION_ADAPTER_MODE` | `local` | `local`: in-process adapters in Core. `http`: Core calls sidecar ECS services (staging default). |
| `ENABLED_ADAPTERS` | `paper,zerodha,angel,dhan,mt5` | Comma-separated deploy-time gate. Staging default: `paper,zerodha`. Disabled brokers return `broker_adapter_unavailable`. |
| `ZB_SERVICE_TOKEN` | _(empty)_ | Shared secret for adapter → Core internal API (`/v1/internal/*`). Required when `EXECUTION_ADAPTER_MODE=http`. |
| `CORE_INTERNAL_URL` | `http://localhost:8080` | Base URL adapters use to POST execution events back to Core. |
| `PAPER_ADAPTER_URL` | _(empty)_ | Required when `paper` enabled and `EXECUTION_ADAPTER_MODE=http` |
| `ZERODHA_ADAPTER_URL` | `http://localhost:8092` | Required when `zerodha` enabled and `EXECUTION_ADAPTER_MODE=http` |
| `ANGEL_ADAPTER_URL` | _(empty)_ | Required when `angel` enabled and `EXECUTION_ADAPTER_MODE=http` |
| `DHAN_ADAPTER_URL` | _(empty)_ | Required when `dhan` enabled and `EXECUTION_ADAPTER_MODE=http` |
| `MT5_ADAPTER_URL` | _(empty)_ | Required when `mt5` enabled and `EXECUTION_ADAPTER_MODE=http` |

Sidecar binaries: `cmd/paper-adapter`, `cmd/zerodha-adapter`. Local dev with sidecars: `docker compose -f docker-compose.yml -f docker-compose.adapters.yml --profile adapters up`.

---

## Queue and Workers

| Variable | Default | Description |
|----------|---------|-------------|
| `WORKER_COUNT` | `20` | Number of goroutines consuming from the signal queue |
| `QUEUE_BUFFER` | `500` | Buffered channel capacity. Ingest returns HTTP 503 `queue_full` when the channel is full. |

---

## Access Control and Registration

| Variable | Default | Description |
|----------|---------|-------------|
| `BOOTSTRAP_ADMIN_EMAIL` | _(empty)_ | On startup, promotes an existing user with this email to `role=admin`. No-op if already admin or user does not exist. |
| `CLOSED_REGISTRATION` | `false` | When `true`, `POST /v1/auth/register` requires a valid `invite_code`. Use for closed beta. |
| `PLATFORM_INVITE_TTL_DAYS` | `30` | Days until a platform invite code expires |

---

## Broker

| Variable | Default | Description |
|----------|---------|-------------|
| `BROKER_MODE` | `mock` | **Server env only** — not a user plan feature. `mock`: simulated broker fills (local/CI/staging). `live`: real broker HTTP calls. Any other value aborts startup. |
| `BROKER_HTTP_TIMEOUT_SEC` | `8` | Outbound broker API HTTP client timeout in seconds |
| `BROKER_HTTP_GET_RETRIES` | `2` | Retry count for idempotent broker GET requests (e.g. LTP fetch for bracket orders). Order POST calls are never retried. |

### Broker Base URL Overrides

Override the default broker API base URLs for testing against sandboxes. Empty = use built-in production URLs.

| Variable | Broker | Mode |
|----------|--------|------|
| `BROKER_MT5_BASE_URL` | MetaApi MT5 | live |
| `BROKER_MT5_DEMO_BASE_URL` | MetaApi MT5 | demo |
| `BROKER_ZERODHA_BASE_URL` | Kite Connect | live |
| `BROKER_ZERODHA_DEMO_BASE_URL` | Kite Connect | demo |
| `BROKER_ANGEL_BASE_URL` | SmartAPI | live |
| `BROKER_ANGEL_DEMO_BASE_URL` | SmartAPI | demo |
| `BROKER_DHAN_BASE_URL` | Dhan | live |
| `BROKER_DHAN_DEMO_BASE_URL` | Dhan | demo |

### Angel One Egress Identity

Angel One (SmartAPI) requires the client's public IP in order request headers. Set these to the NAT Gateway Elastic IP in production. If unset, orders from unwhitelisted IPs are rejected by Angel One.

| Variable | Default | Description |
|----------|---------|-------------|
| `BROKER_ANGEL_CLIENT_LOCAL_IP` | `127.0.0.1` | Sent as `X-ClientLocalIP` header on SmartAPI requests |
| `BROKER_ANGEL_CLIENT_PUBLIC_IP` | `127.0.0.1` | Sent as `X-ClientPublicIP` header on SmartAPI requests |
| `BROKER_ANGEL_MAC_ADDRESS` | `00:00:00:00:00:00` | Sent as `X-MACAddress` header on SmartAPI requests |

---

## SEBI Algo-ID

| Variable | Default | Description |
|----------|---------|-------------|
| `SEBI_ALGO_ID_REQUIRED` | `true` when `BROKER_MODE=live`, else `false` | When `true`, live Indian broker orders without a resolvable Algo-ID are rejected with `error_code=algo_id_required`. Override explicitly to decouple from `BROKER_MODE`. |
| `BROKER_ZERODHA_ALGO_ID` | _(empty)_ | Platform-level fallback Algo-ID for Zerodha orders (sent as Kite `tag`). Used when the credential has no `algo_id`. |
| `BROKER_ANGEL_ALGO_ID` | _(empty)_ | Platform-level fallback Algo-ID for Angel One orders (sent as SmartAPI `ordertag`) |
| `BROKER_DHAN_ALGO_ID` | _(empty)_ | Platform-level fallback Algo-ID for Dhan orders (sent as `correlationId`) |

Resolution order: credential-level `algo_id` → env fallback → reject if required.

See [sebi-compliance.md](../09-compliance-and-legal/sebi-compliance.md) for the full compliance guide.

---

## Early Bird Live Access

Grants Free plan users a limited number of real live orders during a defined window. Disabled by default.

| Variable | Default | Description |
|----------|---------|-------------|
| `EARLY_BIRD_LIVE_ENABLED` | `false` | Enables early-bird live routing for Free plan users |
| `EARLY_BIRD_LIVE_ORDER_LIMIT` | `10` | Max cumulative live orders a Free plan user can place |
| `EARLY_BIRD_LIVE_STARTS_AT` | _(empty)_ | RFC3339 timestamp; no start gate if empty |
| `EARLY_BIRD_LIVE_ENDS_AT` | _(empty)_ | RFC3339 timestamp; no expiry if empty |

---

## Metrics

| Variable | Default | Description |
|----------|---------|-------------|
| `METRICS_ENABLED` | `true` | Exposes Prometheus metrics at `GET /metrics` |
| `METRICS_TOKEN` | _(empty)_ | Optional bearer token required to scrape `/metrics`. Empty = unauthenticated scrape allowed. |

---

## Email

| Variable | Default | Description |
|----------|---------|-------------|
| `EMAIL_VERIFICATION_REQUIRED` | `true` | When `true`, users must click an email verification link before accessing authenticated endpoints. Set `false` for local dev and CI. |
| `EMAIL_VERIFICATION_TTL_HOURS` | `24` | How long a verification link remains valid |
| `EMAIL_PROVIDER` | `log` | `log`: prints email body to stdout (dev). `smtp`: sends via SMTP. |
| `EMAIL_FROM` | `noreply@localhost` | Sender address on all outbound emails |
| `SMTP_HOST` | _(empty)_ | SMTP server hostname. Required when `EMAIL_PROVIDER=smtp`. |
| `SMTP_PORT` | `587` | SMTP server port |
| `SMTP_USER` | _(empty)_ | SMTP username |
| `SMTP_PASS` | _(empty)_ | SMTP password. Inject from Secrets Manager; never hardcode. |

---

## Billing (Stripe)

All Stripe variables are required when `BILLING_ENABLED=true`. Startup fails if any are missing or if `APP_PUBLIC_URL` is not `https://`.

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_ENABLED` | `false` | Enables Stripe checkout, portal, and webhook processing. Disabled for beta. |
| `STRIPE_SECRET_KEY` | _(empty)_ | Stripe secret key (`sk_live_…` or `sk_test_…`). Inject from Secrets Manager. |
| `STRIPE_WEBHOOK_SECRET` | _(empty)_ | Stripe webhook signing secret (`whsec_…`). Verifies inbound Stripe event signatures. |
| `STRIPE_PRICE_PAPER` | _(empty)_ | Stripe Price ID for the **paper** plan (`price_…`) |
| `STRIPE_PRICE_PRO` | _(empty)_ | Stripe Price ID for the **pro** plan (`price_…`). Legacy alias: `STRIPE_PRICE_INDIVIDUAL` |
| `STRIPE_PRICE_PRO_PLUS` | _(empty)_ | Stripe Price ID for the **pro_plus** plan (`price_…`) |
| `BILLING_SUCCESS_URL` | `APP_PUBLIC_URL/billing/success?session_id={CHECKOUT_SESSION_ID}` | Stripe Checkout success redirect |
| `BILLING_CANCEL_URL` | `APP_PUBLIC_URL/billing/cancel` | Stripe Checkout cancel redirect |
| `BILLING_PORTAL_RETURN_URL` | value of `BILLING_CANCEL_URL` | Return URL from Stripe Customer Portal |

---

## Zerodha OAuth

Used by `GET /v1/auth/zerodha/login` and `GET /v1/auth/zerodha/callback`. Optional — only configure if offering the Kite OAuth convenience flow.

| Variable | Default | Description |
|----------|---------|-------------|
| `ZERODHA_API_KEY` | _(empty)_ | Kite Connect app API key |
| `ZERODHA_API_SECRET` | _(empty)_ | Kite Connect app API secret. Inject from Secrets Manager; never log. |
| `ZERODHA_CALLBACK_URL` | _(empty)_ | Redirect URL registered in the Kite app (e.g. `https://api.staging.zettabridge.net/v1/auth/zerodha/callback`) |
| `ZERODHA_FRONTEND_URL` | _(empty)_ | Where to redirect the browser after a successful OAuth exchange |

---

## Startup Validation

`config.Validate()` aborts startup if:

1. `BROKER_MODE` is not `mock` or `live` (case-insensitive).
2. `AES_KEY` is not a valid 32-byte hex string.
3. `APP_ENV=production` and `AES_KEY` is all-zero (the dev default).
4. `BILLING_ENABLED=true` and any of `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_PRICE_PAPER`, `STRIPE_PRICE_PRO`, `STRIPE_PRICE_PRO_PLUS` are empty (legacy `STRIPE_PRICE_INDIVIDUAL` satisfies `STRIPE_PRICE_PRO`).
5. `BILLING_ENABLED=true` and `APP_PUBLIC_URL` does not start with `https://`.
6. `EXECUTION_ADAPTER_MODE=http` and a required `*_ADAPTER_URL` is missing for an adapter listed in `ENABLED_ADAPTERS`.

---

## Minimal Local Dev Configuration

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/zettabridge?sslmode=disable
REDIS_URL=redis://localhost:6379
JWT_SECRET=dev-secret-change-me-min-32-chars!!
AES_KEY=0000000000000000000000000000000000000000000000000000000000000000
BROKER_MODE=mock
EXECUTION_ADAPTER_MODE=local
ENABLED_ADAPTERS=paper,zerodha
EMAIL_PROVIDER=log
EMAIL_VERIFICATION_REQUIRED=false
SEBI_ALGO_ID_REQUIRED=false
BILLING_ENABLED=false
METRICS_ENABLED=true
```

The full docker-compose stack in `docker-compose.yml` sets all of these automatically.
