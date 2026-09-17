# Product Requirements Document (PRD)
**Last updated: 2026-07-07**
**Owner:** Surya
**Status:** Living document — update as features ship or requirements change

---

## 1. Purpose

This document defines the functional and non-functional requirements for ZettaBridge at the beta release gate. It serves as the authoritative record of what the system must do, who does it, and how "done" is defined for each requirement.

For the full feature list with plan-tier availability, see [feature-catalogue.md](feature-catalogue.md).
For current shipped state, see [STATUS.md](../STATUS.md).
For scope boundaries (in/out/deferred), see [scope.md](scope.md).

---

## 2. User Roles

| Role | Description |
|------|-------------|
| **Anonymous** | Unauthenticated visitor. Can only access the login and registration pages. |
| **Registered User** | Authenticated user with a verified email address and a plan (`free`, `paper`, `pro`, or `pro_plus`). |
| **Platform Admin** | Internal operator. Has access to admin API group; can set plans, suspend users, and verify emails. Separate JWT claim — not self-promotable. |

> **Removed:** Org Member and Org Owner roles. Multi-user org product removed from API/UI (2026); legacy org data remains in DB only.

---

## 3. User Journeys

### 3.1 Onboarding (all plans)

1. User lands on `/register`, submits email + password.
2. System sends verification email; user lands on `/verify-email` waiting screen.
3. User clicks link in email → `GET /v1/auth/verify-email?token=...` → verified.
4. User logs in at `/login`; dashboard redirects to `/webhooks`.
5. User creates a **paper account** at `/paper-accounts/new` (free plan: 1 account, 10 trades/month).
6. User creates first **paper webhook** at `/webhooks/new`, linking `paper_account_id`.
7. User copies webhook URL from dashboard, pastes it as a TradingView alert URL.
8. First signal arrives; user sees a trade row appear live on the `/webhooks/:id` trades tab and paper positions update.

### 3.2 Paper trading (free and paper plans)

1. User on `free` or `paper` plan creates paper accounts and paper webhooks (within plan limits).
2. User configures webhook guards (dedup, rate limits) on paid `paper` plan.
3. Signals route to the paper engine via paper adapter (local or HTTP sidecar).
4. User monitors positions, orders, and P&L on `/paper-accounts/:id`.
5. Free plan users hit monthly paper trade quota (10) or daily ingest cap (100) when exceeded.

### 3.3 Going live (pro / pro_plus)

1. User upgrades plan (or receives admin plan override for beta).
2. User adds a **live broker credential** at `/credentials/new` (`account_mode: live` only; demo removed).
3. User enters Algo-ID assigned by their broker for Indian live credentials.
4. User creates a **live webhook** with `broker_cred_id` (pro: 1 live webhook; pro_plus: up to 5).
5. System validates Algo-ID is present and non-empty. If missing, queue worker rejects orders with `algo_id_required`.
6. User runs a live market order with a small quantity to confirm end-to-end execution.
7. User checks trade row for broker order ID to confirm fill on broker side.

### 3.4 Incident response (Platform Admin)

1. Admin receives Telegram alert for auth spike or 5xx.
2. Admin logs in to dashboard `/admin` (admin JWT claim required).
3. Admin views user list; suspends abusive/compromised account.
4. If credential compromise suspected: admin suspends account, advises user to rotate credentials at broker side.
5. If duplicate orders observed: admin advises user to enable `dedup_window_sec` on webhook (returns 409 on duplicate, not re-queued).

---

## 4. Functional Requirements

### 4.1 Authentication

| ID | Requirement | Acceptance Criteria |
|----|-------------|---------------------|
| FR-AUTH-1 | Users must register with an email and password | Registration endpoint returns 201; verification email is sent |
| FR-AUTH-2 | Email must be verified before login is allowed (when `EMAIL_VERIFICATION_REQUIRED=true`) | Unverified login returns 403 with `email_not_verified` |
| FR-AUTH-3 | Login issues JWT access token (5h TTL) and refresh token | Token pair returned; access token is a signed HS256 JWT |
| FR-AUTH-4 | Access tokens must be refreshable without re-entering password | `POST /v1/auth/refresh` with valid refresh token returns new access token |
| FR-AUTH-5 | Logout must invalidate the session server-side | Post-logout, the access token is rejected by protected endpoints |
| FR-AUTH-6 | Platform invite tokens must auto-verify the invitee's email | Accepting a platform invite marks email as verified if not already |
| FR-AUTH-7 | Platform admin role is not self-promotable | Only a bootstrap admin user (set at deployment) has the admin claim |

### 4.2 Webhooks

| ID | Requirement | Acceptance Criteria |
|----|-------------|---------------------|
| FR-WH-1 | Each user can create webhooks up to plan limits (paper and live counted separately) | Creating beyond plan limit returns 429 with `plan_limit_exceeded` |
| FR-WH-2 | Each webhook has a unique, unguessable ingest token | Token is a UUID v4; rotatable at any time |
| FR-WH-3 | Paused webhooks reject ingest with 403 | Signal to paused webhook returns `webhook_paused` error |
| FR-WH-4 | Webhooks can have symbol and action allowlists (guards) | Signal failing guard check returns `guard_violation` error |
| FR-WH-5 | Dedup window rejects duplicate signals within the configured window | Second identical signal within window returns **409** with `deduplicated: true` |
| FR-WH-6 | Token rotation immediately invalidates the old token | Old token returns 404 after rotation |
| FR-WH-7 | Paper webhooks must reference a paper account; live webhooks must reference a live credential | Validation on create/update; live webhooks require `pro` or `pro_plus` |

### 4.3 Credentials

| ID | Requirement | Acceptance Criteria |
|----|-------------|---------------------|
| FR-CRED-1 | `raw_creds` must be encrypted at rest (AES-256-GCM) | Database column is always a base64 ciphertext; plaintext never stored |
| FR-CRED-2 | `raw_creds` must never be returned by any GET endpoint | GET credential endpoints omit `raw_creds` field |
| FR-CRED-3 | `raw_creds` must never appear in application logs | Log hygiene audit must confirm no log line contains credential material |
| FR-CRED-4 | Credential verify endpoint must probe the live broker API | `POST /v1/credentials/:id/verify` returns broker-confirmed status |
| FR-CRED-5 | Live mode credentials for Indian brokers must have `algo_id` set | Order placement with live Indian credential and no `algo_id` → `algo_id_required` |
| FR-CRED-6 | Credential limit enforced per plan | pro: 1 live credential; pro_plus: 3; free/paper: 0 |
| FR-CRED-7 | `account_mode` must be `live` only | Demo mode rejected; migration 044 removed demo |

### 4.4 Paper Trading

| ID | Requirement | Acceptance Criteria |
|----|-------------|---------------------|
| FR-PPR-1 | Users can create paper accounts within plan limits | free: 1, paper: 3, pro: 5, pro_plus: 10 |
| FR-PPR-2 | Paper webhooks route to paper adapter, not live broker | `paper_account_id` set → paper engine execution |
| FR-PPR-3 | Free plan monthly paper trade quota enforced | 11th paper trade in calendar month returns `paper_trade_quota_exceeded` |
| FR-PPR-4 | Paper positions and orders persisted and queryable | REST endpoints return current state |

### 4.5 Signal Ingest

| ID | Requirement | Acceptance Criteria |
|----|-------------|---------------------|
| FR-ING-1 | Ingest endpoint returns 202 immediately after queuing (409 on dedup) | Response time < 50ms p99 regardless of broker latency |
| FR-ING-2 | Ingest must accept TradingView's standard webhook JSON format | `{"symbol":"NIFTY","action":"BUY","quantity":1}` successfully accepted and queued |
| FR-ING-3 | Suspended users must be blocked at ingest | Ingest returns 403 `account_suspended` |
| FR-ING-4 | Per-webhook rate limit must be enforced | Signals above `rate_limit_per_sec` return 429; metric `rate_limited` |
| FR-ING-5 | Plan-level orders/sec cap must be enforced in the queue worker | pro/pro_plus: 10/s enforced; paper: 5/s; free: 1/s |
| FR-ING-6 | Free plan daily ingest cap | 101st ingest in calendar day returns 429 |

### 4.6 Order Execution

| ID | Requirement | Acceptance Criteria |
|----|-------------|---------------------|
| FR-ORD-1 | Queue worker must route orders to the correct adapter | Paper webhook → paper adapter; live webhook → broker adapter per `broker_type` |
| FR-ORD-2 | SEBI Algo-ID must be injected into all live Indian broker order requests | Trade row `algo_id` field non-null after live order placed |
| FR-ORD-3 | Every trade outcome (success or failure) must produce a trade row | Trade row created with status, broker_order (if filled), and error_code (if rejected) |
| FR-ORD-4 | `BROKER_MODE=mock` is server staging/CI only | Mock broker simulates HTTP for ops; not exposed as user demo mode |
| FR-ORD-5 | Broker HTTP auth headers must be redacted from logs | No auth headers appear in any log |
| FR-ORD-6 | Server must refuse startup with zero AES key in production | `config.Validate()` returns error; process exits before accepting requests |
| FR-ORD-7 | `EXECUTION_ADAPTER_MODE` supports local and HTTP sidecars | `local`: in-process; `http`: Core calls paper/zerodha sidecar ECS services |
| FR-ORD-8 | `ENABLED_ADAPTERS` gates deploy-time broker availability | Staging default `paper,zerodha`; `GET /v1/brokers/enabled` reflects config |

### 4.7 Trade Monitoring

| ID | Requirement | Acceptance Criteria |
|----|-------------|---------------------|
| FR-TRD-1 | Trade feed must push real-time updates via WebSocket | New trade row appears in dashboard within 2s of broker response |
| FR-TRD-2 | WebSocket must authenticate via JWT query param | `?token=<jwt>` is validated; invalid token closes the connection |
| FR-TRD-3 | Trade list must show structured error codes | `error_code` field present on rejected trades; must be in the published enum |

### 4.8 Billing and Plans

| ID | Requirement | Acceptance Criteria |
|----|-------------|---------------------|
| FR-BILL-1 | Platform admin can set any user's plan directly | `PUT /v1/admin/users/:id/plan` changes plan immediately |
| FR-BILL-2 | Plan limits enforced at runtime | Webhook create, credential create, paper account create, and orders/sec respect current plan |
| FR-BILL-3 | Stripe billing endpoints must be no-ops when `BILLING_ENABLED=false` | Checkout and portal endpoints return 402 or 503; no Stripe API call made |
| FR-BILL-4 | `billing_source=admin` takes precedence over Stripe subscription state | Admin override is not overwritten by Stripe webhook events |
| FR-BILL-5 | Four plan tiers: free, paper, pro, pro_plus | CHECK constraint and `internal/plan/plan.go` are source of truth |

### 4.9 Admin

| ID | Requirement | Acceptance Criteria |
|----|-------------|---------------------|
| FR-ADM-1 | Admin API requires a platform admin JWT claim | Non-admin JWT returns 403 on any `/v1/admin/*` endpoint |
| FR-ADM-2 | Admin can suspend/unsuspend any user | Suspended user's login returns 403; trading is blocked |
| FR-ADM-3 | Admin can verify any user's email manually | `PATCH /v1/admin/users/:id` with `email_verified: true` marks email as verified |

> **Removed:** Section 4.8 Organisation Management (FR-ORG-*). Org API and UI removed; legacy DB tables retained.

---

## 5. Non-Functional Requirements

### 5.1 Performance

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-PERF-1 | Webhook ingest latency (202 response) | p99 < 50ms under normal load |
| NFR-PERF-2 | Broker order placement latency (queue worker) | p99 < 2s (network-bound by broker API) |
| NFR-PERF-3 | Concurrent webhook throughput | 500 concurrent VUs sustained for 60s; < 1% error rate |
| NFR-PERF-4 | Dashboard page load | < 3s Time to Interactive on a 100Mbps connection |

### 5.2 Reliability

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-REL-1 | Monthly uptime | 99.5% for beta; 99.9% target for GA |
| NFR-REL-2 | ECS task restart on crash | ECS health check restarts failed task within 30s |
| NFR-REL-3 | No duplicate orders on restart | In-process queue is stateless; dedup prevents TradingView retry duplicates (409) |
| NFR-REL-4 | Database PITR | RDS PITR enabled with 7-day retention; recovery tested in DR drill |

### 5.3 Security

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-SEC-1 | Credential encryption | AES-256-GCM; key in AWS Secrets Manager; zero dev key rejected in production |
| NFR-SEC-2 | Transport security | TLS 1.2+ enforced via ACM + ALB; HTTP redirected to HTTPS |
| NFR-SEC-3 | Authentication tokens | JWT HS256; 5h access token TTL; refresh token rotation |
| NFR-SEC-4 | No secrets in code or images | All secrets in AWS Secrets Manager; Dockerfile has no `ENV` with secrets |
| NFR-SEC-5 | Log hygiene | No credential material, JWT secrets, or personal data in application logs |
| NFR-SEC-6 | IDOR prevention | All data access scoped to authenticated user ID |

### 5.4 Observability

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-OBS-1 | Metrics | Prometheus at `/metrics`; ingest labels include queued (accepted), deduplicated, rejected, rate_limited, queue_full |
| NFR-OBS-2 | Alerting | Alertmanager sends Telegram alert within 2min of threshold breach |
| NFR-OBS-3 | Log retention | CloudWatch logs retained for 30 days |
| NFR-OBS-4 | Healthcheck | `GET /healthz` returns 200 within 5s; used by ECS |

### 5.5 Compliance

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-COMP-1 | SEBI Algo-ID | All live Indian broker orders carry exchange-assigned Algo-ID; fail-close if missing |
| NFR-COMP-2 | No investment advice | Platform must not generate, display, or imply trading signals; execution-only |
| NFR-COMP-3 | Terms of Service | T&S and Risk Disclosure required before any public beta opens |
| NFR-COMP-4 | Data residency | All user data in AWS ap-south-1 (Mumbai); no cross-region replication |

---

## 6. Beta Release Acceptance Criteria

The following gates must be met before the beta is opened to external users:

| Gate | Description | Status |
|------|-------------|--------|
| B1 | All functional tests pass on AWS staging | Planned |
| B2 | SEBI Algo-ID sandbox validation successful with at least one Indian broker | Planned |
| B3 | Angel One NAT EIP whitelisted | Pending (ops action) |
| B4 | Dhan NAT EIP whitelisted | Pending (ops action) |
| B5 | Email verification and platform invite flows tested end-to-end on AWS | Planned |
| B6 | CORS and subdomain routing verified on staging domains | Planned |
| B7 | Terms of Service and Risk Disclosure published | Planned |

For the full beta and GA gate lists, see [milestones.md](../06-project-management/milestones.md).

---

## 7. Out of Scope

These requirements are explicitly excluded from beta and GA v1:

- Multi-user orgs (removed from product; legacy DB only)
- User-facing demo/mock broker mode (`account_mode=demo` removed)
- Investment advice, signal generation, or trading recommendations
- Options and futures order types for Indian brokers (equities only)
- Position sync / reconciliation against broker portfolio state
- Automated broker token refresh (daily manual refresh required for Zerodha)
- Mobile dashboard application
- Multi-region deployment

See [scope.md](scope.md) for the full scope boundary.
