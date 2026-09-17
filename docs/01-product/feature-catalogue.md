# Feature Catalogue
**Last updated: 2026-07-07**

Complete record of every feature in ZettaBridge. Use this as the authoritative reference for what exists, what plan tier it belongs to, what API backs it, and where it appears in the dashboard.

Canonical plan limits: `internal/plan/plan.go` and [STATUS.md](../STATUS.md).

**Status values:** Done · In Progress · Planned · Deferred · Blocked · Removed

**Plans:** `free` · `paper` · `pro` · `pro_plus` (legacy names `individual` / `household` / `enterprise` removed in migration 044; four-tier model in migration 046).

---

## 1. Authentication and Account Management

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| AUTH.1 | User registration | Email + password registration; sends email verification | Done | All plans | `POST /v1/auth/register` | `/register` |
| AUTH.2 | Email verification | Token-based email verification; gate on login when required | Done | All plans | `GET /v1/auth/verify-email` | `/verify-email` |
| AUTH.3 | Resend verification email | Resend verification link on demand | Done | All plans | `POST /v1/auth/resend-verification` | `/resend-verification` |
| AUTH.4 | Login | JWT access + refresh token pair issued on credentials | Done | All plans | `POST /v1/auth/login` | `/login` |
| AUTH.5 | Token refresh | Silent refresh of expired access tokens | Done | All plans | `POST /v1/auth/refresh` | Handled in apiFetch |
| AUTH.6 | Logout | Client-side token clear; optional server-side revocation | Done | All plans | `POST /v1/auth/logout` | `/login` redirect |
| AUTH.7 | Platform invite accept | Accept closed-beta platform invite via token link; auto-verifies email | Done | Invite-only registration | Platform invite routes | `/invites/:token` |
| AUTH.8 | Profile view | View own email, plan, billing source, usage limits, `live_trading_allowed` | Done | All plans | `GET /v1/me` | `/me` |

---

## 2. Webhook Management

Webhooks route to either a **paper account** (`paper_account_id`) or a **live broker credential** (`broker_cred_id`). Plan limits apply separately to paper vs live webhook counts.

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| WH.1 | Create webhook | Create a named webhook endpoint with a unique ingest token | Done | Paper webhooks: free (1), paper (5), pro (8), pro_plus (15). Live webhooks: pro (1), pro_plus (5) | `POST /v1/webhooks` | `/webhooks/new` |
| WH.2 | List webhooks | List all webhooks owned by user | Done | All plans | `GET /v1/webhooks` | `/webhooks` |
| WH.3 | View webhook | View webhook detail including token and guard settings | Done | All plans | `GET /v1/webhooks/:id` | `/webhooks/:id` |
| WH.4 | Edit webhook | Update name, guard settings, dedup window, paper/live target | Done | All plans | `PUT /v1/webhooks/:id` | `/webhooks/:id/edit` |
| WH.5 | Pause / resume | Pause a webhook to stop processing signals without deleting it | Done | All plans | `PUT /v1/webhooks/:id` (status field) | `/webhooks` list toggle |
| WH.6 | Delete webhook | Delete a webhook and all its trade history | Done | All plans | `DELETE /v1/webhooks/:id` | `/webhooks` list |
| WH.7 | Rotate token | Rotate the ingest token; old token immediately rejected | Done | All plans | `POST /v1/webhooks/:id/rotate-token` | `/webhooks/:id` settings tab |
| WH.8 | Signal deduplication | Prevent duplicate orders from TradingView retries within a configurable window | Done | paper, pro, pro_plus (advanced guards; paper webhooks exempt from live guard plan gate) | `dedup_window_sec` field on webhook | `/webhooks/:id` settings |
| WH.9 | Symbol filter guard | Reject signals for symbols not in the allowlist | Done | All plans | `allowed_symbols` field on webhook | `/webhooks/:id` guards |
| WH.10 | Action filter guard | Reject signals for actions (BUY/SELL/CLOSE) not in the allowlist | Done | All plans | `allowed_actions` field on webhook | `/webhooks/:id` guards |
| WH.11 | Per-webhook rate limit | Reject ingest above N signals/sec at the webhook level | Done | All plans | `rate_limit_per_sec` field on webhook | `/webhooks/:id` guards |
| WH.12 | Webhook URL copy | Copy TradingView-ready webhook URL from dashboard | Done | All plans | — (UI only) | `/webhooks/:id` settings |
| WH.13 | Paper webhook routing | Webhook with `paper_account_id` routes to paper engine (no live broker credential) | Done | All plans (within paper webhook limits) | `paper_account_id` on webhook | `/webhooks/new` |
| WH.14 | Live webhook routing | Webhook with `broker_cred_id` routes to live broker adapter | Done | pro, pro_plus only | `broker_cred_id` on webhook | `/webhooks/new` |

---

## 3. Broker Credential Management

Live broker credentials only — `account_mode` is **`live`** (demo removed in migration 044). Paper trading uses paper accounts, not broker credentials.

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| CRED.1 | Add credential | Add live broker credentials (encrypted at rest with AES-256-GCM) | Done | pro (1), pro_plus (3) | `POST /v1/credentials` | `/credentials/new` |
| CRED.2 | List credentials | List all credentials (raw_creds never returned) | Done | pro, pro_plus | `GET /v1/credentials` | `/credentials` |
| CRED.3 | Update credential | Replace raw credentials (e.g. after Zerodha daily token refresh) | Done | pro, pro_plus | `PUT /v1/credentials/:id` | `/credentials/:id` |
| CRED.4 | Delete credential | Delete a credential | Done | pro, pro_plus | `DELETE /v1/credentials/:id` | `/credentials/:id` |
| CRED.5 | Verify credential | Probe broker API to confirm credentials are valid and active | Done | pro, pro_plus | `POST /v1/credentials/:id/verify` | `/credentials/:id` verify button |
| CRED.6 | Live-only account mode | Credential `account_mode` must be `live`; demo mode removed | Done | pro, pro_plus | `account_mode` field (live only) | `/credentials/new` |
| CRED.7 | SEBI Algo-ID per credential | Set exchange-assigned Algo-ID on a live Indian credential | Done | pro, pro_plus (live Indian brokers only) | `algo_id` field on credential | `/credentials/new` |
| CRED.8 | Masked display | `raw_creds` and `algo_id` displayed masked in UI and never returned by GET | Done | pro, pro_plus | All `GET /v1/credentials` responses | All credential pages |
| CRED.9 | Multi-product credentials | Single credential with MIS + CNC + NRML product routing | Done | pro_plus only | Multi-product fields on credential | `/credentials/new` |
| CRED.10 | Enabled brokers list | Deployment-gated list of brokers available for credential creation | Done | pro, pro_plus | `GET /v1/brokers/enabled` | `/credentials/new` |

---

## 4. Paper Trading

Paper trading is a first-class product path via **paper accounts** and webhooks with `paper_account_id`. Execution uses the in-process or HTTP **paper adapter** (`EXECUTION_ADAPTER_MODE`).

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| PPR.1 | Create paper account | Create a simulated trading account with starting balance | Done | free (1), paper (3), pro (5), pro_plus (10) | `POST /v1/paper-accounts` | `/paper-accounts/new` |
| PPR.2 | List / view paper accounts | List and view paper account details | Done | All plans | `GET /v1/paper-accounts`, `GET /v1/paper-accounts/:id` | `/paper-accounts` |
| PPR.3 | Pause / resume / reset | Pause, resume, or reset paper account state | Done | All plans | `PUT /v1/paper-accounts/:id/pause`, `resume`, `reset` | `/paper-accounts/:id` |
| PPR.4 | Paper positions & orders | View open positions and order history | Done | All plans | `GET /v1/paper-accounts/:id/positions`, `/orders` | `/paper-accounts/:id` |
| PPR.5 | Paper P&L | Per-account P&L summary | Done | All plans | `GET /v1/paper-accounts/:id/pnl` | `/paper-accounts/:id` |
| PPR.6 | Monthly paper trade quota | Free plan capped at 10 paper trades/month | Done | free (10/mo); paper+ unlimited | Enforced at execution | — |
| PPR.7 | Paper webhook execution | Signals to paper webhooks execute in paper engine | Done | All plans (within limits) | `POST /v1/webhook/:token` → paper adapter | — |
| PPR.8 | Market data snapshots | FYERS/market-profile snapshots on paper accounts | Done | All plans | `GET /v1/paper-accounts/:id/snapshots` | `/paper-accounts/:id` |

---

## 5. Signal Ingest

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| ING.1 | Webhook ingest | Receive trade signal via `POST /v1/webhook/:token` (TradingView JSON) | Done | All plans | `POST /v1/webhook/:token` | — (external call) |
| ING.2 | Plan rate limiting | Enforce plan-level orders/sec cap per user in queue worker | Done | free: 1/s, paper: 5/s, pro/pro_plus: 10/s | Enforced in queue worker | — |
| ING.3 | Dedup check | Idempotent handling of TradingView retries within `dedup_window_sec`; returns **409** with `deduplicated: true` | Done | paper, pro, pro_plus | Redis SETNX in ingest handler | — |
| ING.4 | Compliance gate | Block ingest 403 if user is suspended | Done | All plans | Ingest handler middleware | — |
| ING.5 | Guard validation | Validate signal against webhook guard rules before queuing | Done | All plans | Ingest handler | — |
| ING.6 | Async queuing | Signal accepted with **202** and processed asynchronously; caller is not blocked on broker response | Done | All plans | Queue worker | — |
| ING.7 | Free daily ingest cap | Free plan: 100 webhook ingests/day (Redis counter) | Done | free | Ingest handler | — |
| ING.8 | Queue full response | Full in-process queue returns 503 with `queue_full` metric | Done | All plans | Ingest handler | — |
| ING.9 | Per-webhook rate limit | Signals above webhook `rate_limit_per_sec` return 429 with `rate_limited` metric | Done | All plans | Ingest handler | — |

---

## 6. Order Execution

Execution routing: paper webhooks → **paper adapter**; live webhooks → **live adapter** (Zerodha on ECS staging; Angel/Dhan/MT5 in code, gated by `ENABLED_ADAPTERS`). `BROKER_MODE=mock` is a **server env** for staging/CI simulated broker HTTP — not a user-facing demo mode.

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| ORD.1 | Live broker routing | Route order to live broker (Zerodha/Angel/Dhan/MT5) via live adapter | Done | pro, pro_plus | Queue worker → live adapters | — |
| ORD.2 | Paper adapter routing | Route paper webhook signals to paper engine (local or HTTP sidecar) | Done | All plans | Queue worker → paper adapter | — |
| ORD.3 | Execution adapter mode | `EXECUTION_ADAPTER_MODE=local` (in-process) or `http` (Core → sidecar ECS) | Done | Platform ops | Server env | — |
| ORD.4 | Enabled adapters gate | Deploy-time `ENABLED_ADAPTERS` (staging default: `paper,zerodha`) | Done | Platform ops | Server env | — |
| ORD.5 | Market orders | Place market order on all supported brokers and paper engine | Done | All plans (paper or live per webhook) | Broker/paper adapters | — |
| ORD.6 | Limit orders | Place limit order on all supported brokers | Done | pro, pro_plus (live); all plans (paper) | Broker adapters | — |
| ORD.7 | Bracket / SL-TP orders | CO on Zerodha, ROBO on Angel One, SL/TP on MT5 | Done | pro, pro_plus (live) | Broker adapters | — |
| ORD.8 | Cancel order | Cancel an open order via API | Done | pro, pro_plus (live); all plans (paper) | `DELETE /v1/webhooks/:id/trades/:tradeId` | `/webhooks/:id` trades tab |
| ORD.9 | Position close | CLOSE action squares off position on all brokers and paper engine | Done | All plans | `action: CLOSE` in signal | — |
| ORD.10 | SEBI Algo-ID tagging | Tag live Indian orders with exchange-assigned algo ID | Done | pro, pro_plus (live Indian only) | Queue worker + adapters | — |
| ORD.11 | Broker HTTP header redaction | Secrets and auth headers redacted from all broker HTTP logs | Done | All plans | internal/livebrokers/httpclient | — |
| ORD.12 | Broker order ID tracking | Store real broker order ID on trade row for reconciliation | Done | All plans | `broker_order` field on trade | `/webhooks/:id` trades tab |
| ORD.13 | Staging mock broker | `BROKER_MODE=mock` simulates broker HTTP for staging/CI only | Done | Platform ops (not user-facing) | Server env | — |

---

## 7. Trade Monitoring

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| TRD.1 | Trade list | View all trades for a webhook with status and error codes | Done | All plans | `GET /v1/webhooks/:id/trades` | `/webhooks/:id` trades tab |
| TRD.2 | Live WebSocket feed | Real-time trade updates pushed on insert | Done | All plans | `GET /v1/ws/trades?token=` | `/webhooks/:id` trades tab |
| TRD.3 | Trade error codes | Structured `error_code` on rejected trades (auth_failed, rate_limited, algo_id_required, etc.) | Done | All plans | Trade row `error_code` field | `/webhooks/:id` trades tab |
| TRD.4 | Algo-ID audit | `algo_id` value snapshotted on trade row for regulatory audit | Done | pro, pro_plus (live) | Trade row `algo_id` field | — |
| TRD.5 | Audit log access | Trade history and audit views on paid plans | Done | paper, pro, pro_plus | Trade list endpoints | `/webhooks/:id` trades tab |

---

## 8. P&L and Analytics

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| PNL.1 | Per-webhook P&L | Trade count, total notional, win-rate per webhook | Done | All plans | `GET /v1/webhooks/:id/pnl` | `/webhooks/:id` P&L tab |
| PNL.2 | Per-paper-account P&L | P&L summary for paper accounts | Done | All plans | `GET /v1/paper-accounts/:id/pnl` | `/paper-accounts/:id` |
| PNL.3 | P&L time-series charts | Historical P&L over time with recharts | Planned | All plans | Requires timestamp addition to P&L API | `/webhooks/:id` P&L tab |

---

## 9. Subscription and Billing

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| BILL.1 | Plan display | Show current plan, billing source, usage vs limits | Done | All plans | `GET /v1/me`, `GET /v1/billing/plans` | `/billing` |
| BILL.2 | Stripe checkout | Self-serve upgrade via Stripe Checkout Session | Done (disabled) | paper, pro, pro_plus | `POST /v1/billing/checkout` | `/billing` upgrade button |
| BILL.3 | Stripe customer portal | Manage subscription, payment method, cancel via Stripe Portal | Done (disabled) | paper, pro, pro_plus | `POST /v1/billing/portal` | `/billing` manage button |
| BILL.4 | Stripe webhook handler | Idempotent plan sync on checkout/subscription events | Done (disabled) | System | `POST /v1/billing/stripe-webhook` | — |
| BILL.5 | Admin plan override | Platform admin sets user plan directly (`billing_source=admin`) | Done | Platform admin | `PUT /v1/admin/users/:id/plan` | `/admin` page |
| BILL.6 | Billing success page | Landing page after Stripe checkout; polls for plan update | Done | paper, pro, pro_plus | `GET /v1/me` | `/billing/success` |
| BILL.7 | Billing cancel page | Landing page when checkout is abandoned | Done | paper, pro, pro_plus | — | `/billing/cancel` |

> **Note:** BILL.2, BILL.3, BILL.4 are code-complete but disabled (`BILLING_ENABLED=false`). Stripe price env vars: `STRIPE_PRICE_PAPER`, `STRIPE_PRICE_PRO`, `STRIPE_PRICE_PRO_PLUS`. Beta uses BILL.5 only.

---

## 10. Organisation Management

**Removed from product (2026).** All `/v1/orgs/*` routes removed from router. Dashboard org pages redirect to `/webhooks` or `/admin`. Legacy org tables/columns remain in DB for existing data only.

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| ORG.1 | Create org | Org owner creates an organisation | **Removed** | — | — (removed) | Redirects to `/webhooks` |
| ORG.2 | Invite member | Invite user by email | **Removed** | — | — (removed) | — |
| ORG.3 | Org usage view | Org-wide usage vs limits | **Removed** | — | — (removed) | — |
| ORG.4 | Org suspension | Platform admin suspends org | **Removed** | — | — (removed) | — |
| ORG.5 | Org switcher | Switch active org | **Removed** | — | — (removed) | — |

---

## 11. Admin Panel (Platform Operator)

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| ADM.1 | User list | List all users with plan, billing source, and status | Done | Platform admin | `GET /v1/admin/users` | `/admin` |
| ADM.2 | Set user plan | Override user plan to any tier (`free`, `paper`, `pro`, `pro_plus`) | Done | Platform admin | `PUT /v1/admin/users/:id/plan` | `/admin` |
| ADM.3 | Suspend user | Block user from login and trading | Done | Platform admin | `PATCH /v1/admin/users/:id` | `/admin` |
| ADM.4 | Unsuspend user | Restore suspended user | Done | Platform admin | `PATCH /v1/admin/users/:id` | `/admin` |
| ADM.5 | Verify user email | Manually mark user email as verified | Done | Platform admin | `PATCH /v1/admin/users/:id` | `/admin` |
| ADM.6 | Admin ops view | Read-only operational view for support staff | Planned | Platform admin | — | `/admin` (extended) |

---

## 12. Observability and Monitoring

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| OBS.1 | Prometheus metrics | `zettabridge_ingest_total` (queued, deduplicated, rejected, rate_limited, queue_full), trades, dedup hits, broker latency | Done | Platform ops | `GET /metrics` | Grafana (Lightsail VM) |
| OBS.2 | Grafana dashboard | Pre-built JSON: ingest rate, trades by error_code, broker HTTP latency, dedup hits | Done | Platform ops | — | `deploy/deploy-lightsail/grafana/` |
| OBS.3 | Telegram alerting | Alertmanager → Telegram on auth spikes, ingest rejected, 5xx, service down | Done | Platform ops | — | Alertmanager config |
| OBS.4 | CloudWatch logs | ECS task logs with 30-day retention | Done | Platform ops | — | CloudWatch |
| OBS.5 | CloudWatch alarms | 8 alarm types: rejection rate, queue depth, 5xx, latency, ECS health, RDS, Redis, broker auth | Planned | Platform ops | — | CloudWatch |

> **Metric note:** `zettabridge_ingest_total{status="queued"}` means the signal was **accepted** and enqueued. `rate_limited` and `queue_full` are separate status labels.

---

## 13. Security

| ID | Feature | Description | Status | Plan Availability | Related API | Dashboard Page |
|----|---------|-------------|--------|------------------|-------------|----------------|
| SEC.1 | AES-256-GCM credential encryption | Broker credentials encrypted at rest; raw_creds never in logs or API responses | Done | All plans | — | — |
| SEC.2 | JWT with refresh rotation | Short-lived access tokens + rotating refresh tokens | Done | All plans | Auth endpoints | — |
| SEC.3 | Production AES key validation | Server refuses startup with all-zero dev key when `APP_ENV=production` | Done | Platform | config.Validate() | — |
| SEC.4 | AWS Secrets Manager | All secrets in Secrets Manager; no plaintext secrets in task defs or ECR images | Done | Platform | — | — |
| SEC.5 | KMS CMK encryption | Secrets Manager secrets encrypted with customer-managed KMS key | Done | Platform | — | — |
| SEC.6 | Static NAT egress IP | All broker API calls exit through a single whitelistable Elastic IP | Done | Platform | — | — |
| SEC.7 | Security response headers | HSTS, X-Content-Type-Options, X-Frame-Options | Planned | Platform | Fiber middleware | — |
| SEC.8 | KMS envelope encryption | Replace raw AES_KEY with KMS-wrapped DEK per credential | Planned | Platform | internal/credenc | — |
| SEC.9 | AWS WAF | Managed rule sets + rate limits on ALB | Planned | Platform | — | — |
