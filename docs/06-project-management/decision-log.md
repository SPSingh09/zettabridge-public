# Decision Log
**Last updated: 2026-07-07**

Record of significant product and technical decisions made during ZettaBridge's development. Each entry captures the context, the options considered, the choice made, and the rationale, so that future contributors understand why the system is built the way it is.

**Status:** Active · Superseded · Revisit

---

## DEC-001 — Go as the backend language

| Field | Detail |
|-------|--------|
| **Date** | 2026-Q1 |
| **Status** | Active |
| **Decision** | Use Go for the backend API and queue worker |
| **Context** | Needed a language for a latency-sensitive webhook processing pipeline that would handle broker HTTP calls with concurrency. |
| **Alternatives** | Python (FastAPI), Node.js (Fastify), Rust |
| **Rationale** | Go's goroutine model maps naturally to a queue worker pattern; low latency per-request; strong HTTP client support; single binary deployment simplifies Docker images and ECS task definitions. The team already had Go experience. |

---

## DEC-002 — Fiber as the HTTP framework

| Field | Detail |
|-------|--------|
| **Date** | 2026-Q1 |
| **Status** | Active |
| **Decision** | Use Go Fiber (`github.com/gofiber/fiber`) as the HTTP framework |
| **Context** | Needed an HTTP framework for the Go backend. |
| **Alternatives** | net/http + Chi, Echo, Gin |
| **Rationale** | Fiber's middleware chain and group-based routing aligned well with the required structure (unauthed ingest + authed API groups). Comparable performance to alternatives. |

---

## DEC-003 — In-process queue over a message broker

| Field | Detail |
|-------|--------|
| **Date** | 2026-Q1 |
| **Status** | Active (revisit at multi-instance ECS scale) |
| **Decision** | Use an in-process Go channel queue backed by Redis for dedup, rather than NATS or Kafka |
| **Context** | Needed a queue for decoupling webhook ingest from broker API calls. Expected scale: hundreds of concurrent signals, not millions. |
| **Alternatives** | NATS JetStream, Apache Kafka, Redis Streams, AWS SQS |
| **Rationale** | In-process queue eliminates network hop overhead for the common case (single ECS task). Redis is already required for dedup and rate limiting. NATS/Kafka add ops complexity for a scale target that a single Go process handles easily. Fan-out to Redis pub/sub can be added when multi-instance ECS is needed (post-GA). |

---

## DEC-004 — PostgreSQL for persistent data

| Field | Detail |
|-------|--------|
| **Date** | 2026-Q1 |
| **Status** | Active |
| **Decision** | Use PostgreSQL 15 on AWS RDS as the primary database |
| **Context** | Needed a database for users, webhooks, credentials, trades, orgs, and billing state. |
| **Alternatives** | MySQL, DynamoDB, CockroachDB |
| **Rationale** | Relational model fits trade audit requirements well (joins across users → webhooks → trades). SQL migrations are straightforward. RDS PostgreSQL is a fully managed service with PITR, Multi-AZ, and automated backups. Team has PostgreSQL experience. |

---

## DEC-005 — AWS ap-south-1 (Mumbai) as primary region

| Field | Detail |
|-------|--------|
| **Date** | 2026-Q1 |
| **Status** | Active |
| **Decision** | Deploy to `ap-south-1` (Mumbai) as the primary and only region |
| **Context** | ZettaBridge places orders on Indian brokers (Zerodha, Angel One, Dhan). Order placement latency is a product quality factor. |
| **Alternatives** | ap-southeast-1 (Singapore), eu-west-1 (Ireland), us-east-1 |
| **Rationale** | Mumbai minimises round-trip latency to NSE/BSE broker API servers. Indian brokers also require a static IP for whitelisting — the NAT Gateway EIP in Mumbai satisfies this. |

---

## DEC-006 — Broker adapter pattern (interface-based)

| Field | Detail |
|-------|--------|
| **Date** | 2026-Q1 |
| **Status** | Active |
| **Decision** | Define a `broker.Broker` interface and implement one adapter per broker; queue worker calls the interface only |
| **Context** | Needed to support multiple brokers (Zerodha, Angel, Dhan, MT5) plus mock mode without spreading broker-specific logic into the queue worker. |
| **Alternatives** | Switch statements in worker, plugin system, generic REST adapter |
| **Rationale** | Interface-based adapters allow the worker to be broker-agnostic. Each adapter is independently testable with httptest. New brokers are added by implementing the interface without changing worker logic. Mock adapters share the same interface, so functional tests use the same code paths as production. |

---

## DEC-007 — Per-user broker credential storage (not platform-level)

| Field | Detail |
|-------|--------|
| **Date** | 2026-Q1 |
| **Status** | Active |
| **Decision** | Each user stores their own broker credentials (API key, access token) in ZettaBridge; the platform does not hold a single shared broker account |
| **Context** | Designing how ZettaBridge holds broker authentication state. |
| **Alternatives** | Platform holds one broker account per broker type and sub-routes orders; OAuth platform-level session |
| **Rationale** | Per-user credentials: (1) avoid single-point-of-failure for all users on one account; (2) required by broker ToS (each API key tied to one account); (3) confirmed impossible for Zerodha OAuth (DEC-011); (4) simpler regulatory position — each user's Algo-ID maps to their own broker account. |

---

## DEC-008 — AES-256-GCM for credential encryption at rest

| Field | Detail |
|-------|--------|
| **Date** | 2026-Q1 |
| **Status** | Active (upgrade to KMS envelope encryption planned as S6) |
| **Decision** | Encrypt `raw_creds` using AES-256-GCM with a single `AES_KEY` from environment / Secrets Manager |
| **Context** | Broker API tokens and keys must not be stored in plaintext. |
| **Alternatives** | Hashicorp Vault, AWS KMS direct encryption, no encryption |
| **Rationale** | AES-256-GCM is a standard authenticated encryption scheme; straightforward to implement; key lives in AWS Secrets Manager (not in code or image). KMS envelope encryption is the planned upgrade path post-beta to allow zero-downtime key rotation. |

---

## DEC-009 — SEBI Algo-ID: per-credential field with environment fallback

| Field | Detail |
|-------|--------|
| **Date** | 2026-06 |
| **Status** | Active |
| **Decision** | Store `algo_id` per `broker_credentials` row; allow platform-level `BROKER_*_ALGO_ID` env var as fallback; fail-close when required and missing |
| **Context** | SEBI requires every live NSE/BSE API order to carry an exchange-assigned Algo-ID. |
| **Alternatives** | Per-webhook override, webhook comment field as proxy, single platform Algo-ID |
| **Rationale** | Per-credential is the right granularity because each credential belongs to one user who obtained their own Algo-ID from the broker. The env fallback supports staging and early-bird pilots where all traffic shares one test ID. Fail-close ensures compliance cannot be bypassed in production. |

---

## DEC-010 — Dhan Algo-ID via `correlationId` field

| Field | Detail |
|-------|--------|
| **Date** | 2026-06 |
| **Status** | Active |
| **Decision** | Pass SEBI Algo-ID as the `correlationId` field in Dhan v2 order requests |
| **Context** | Dhan v2 public docs showed `correlationId` as "user tracking" and `algoId` only in responses — the request field for Algo-ID compliance was ambiguous. |
| **Alternatives** | `algoId` request field (not confirmed), broker-side registration only (no request field) |
| **Rationale** | Dhan support explicitly confirmed that `correlationId` is the correct request field for passing the SEBI Algo-ID. This is documented and ZettaBridge implements accordingly. The Dhan support correspondence should be archived as evidence. |

---

## DEC-011 — Zerodha: per-user API key required (multi-user OAuth not possible)

| Field | Detail |
|-------|--------|
| **Date** | 2026-06-23 |
| **Status** | Active |
| **Decision** | Each ZettaBridge user must supply their own Kite Connect API key + secret. Platform-level shared OAuth login is not possible. |
| **Context** | Explored using a single Kite Connect API key to authenticate multiple ZettaBridge users through an OAuth flow, similar to how other platforms work. |
| **Alternatives** | Platform-level Kite Connect OAuth with session management per user |
| **Rationale** | Kite Connect confirmed directly: one API key allows one user session at a time. There is no multi-user authorization model in their API. This is a hard constraint with no workaround. The scope document and broker-guide must document this prominently. |

---

## DEC-012 — Payment gateway deferred (not an engineering decision)

| Field | Detail |
|-------|--------|
| **Date** | 2026-06-23 |
| **Status** | Active |
| **Decision** | Stripe and Razorpay payment gateways cannot be enabled at this time. Beta billing handled via admin plan overrides only. |
| **Context** | Stripe billing code is complete and production-ready. Explored enabling it for beta. |
| **Alternatives** | Proceed with Stripe in test mode only; use Razorpay instead |
| **Rationale** | Stripe is invite-only for new business accounts in India. Razorpay requires a registered business entity in India. Both constraints apply equally. Business entity registration is a prerequisite. The code remains in place (`BILLING_ENABLED=false`) and can be enabled immediately once unblocked. |

---

## DEC-013 — Dashboard: Next.js with App Router

| Field | Detail |
|-------|--------|
| **Date** | 2026-06 |
| **Status** | Active |
| **Decision** | Use Next.js 14 (App Router) with TypeScript and Tailwind CSS for the dashboard |
| **Context** | Needed a frontend framework for the self-serve dashboard. |
| **Alternatives** | Vite + React SPA, Remix, SvelteKit, HTMX + Go templates |
| **Rationale** | Next.js App Router provides SSR for auth pages and RSC for data fetching with good TypeScript defaults. Tailwind avoids a design-system dependency for MVP. shadcn/ui provides accessible pre-built primitives. Monorepo layout (`dashboard/` at repo root) avoids cross-repo coordination overhead. |

---

## DEC-014 — JWT stored in localStorage (not httpOnly cookie)

| Field | Detail |
|-------|--------|
| **Date** | 2026-06 |
| **Status** | Active (revisit for post-GA if XSS risk materialises) |
| **Decision** | Dashboard stores JWT in `localStorage` (`zb_token`), not an httpOnly cookie |
| **Context** | Dashboard needs to authenticate API calls. Go API uses `Authorization: Bearer` header. |
| **Alternatives** | httpOnly cookie with Next.js API route proxy for every call |
| **Rationale** | `localStorage` is simpler for MVP — no need for a Next.js proxy layer for every API call. XSS risk is mitigated by CSP headers (planned as S1). The httpOnly cookie approach doubles infrastructure complexity. Revisit in post-GA hardening if a strict CSP cannot be maintained. |

---

## DEC-015 — Subdomain routing (api.* + app.*) over path-based routing

| Field | Detail |
|-------|--------|
| **Date** | 2026-06 |
| **Status** | Active |
| **Decision** | Use separate subdomains for API (`api.staging.zettabridge.net`) and dashboard (`app.staging.zettabridge.net`) rather than path-based routing on one domain |
| **Context** | ALB supports both host-header routing (subdomains) and path-based routing from a single domain. |
| **Alternatives** | `zettabridge.net/api/` → backend, `zettabridge.net/` → dashboard |
| **Rationale** | Subdomain routing produces a cleaner CORS story (explicit allowed origins) and avoids path-prefix complexity in the Go API. ACM certificates can be issued per subdomain. Each service gets independent health checks and target groups on the ALB. |

---

## DEC-016 — Plan naming: Free / Individual / Household

| Field | Detail |
|-------|--------|
| **Date** | 2026-06-21 |
| **Status** | Superseded by DEC-020 |
| **Decision** | Rename plans from Free / Pro / Enterprise to Free / Individual / Household |
| **Context** | Original plan names were generic SaaS names (Pro/Enterprise). ZettaBridge targets retail algo traders, not businesses. |
| **Alternatives** | Keep Free/Pro/Enterprise; use Free/Standard/Family |
| **Rationale** | "Individual" and "Household" better reflected the user segment at the time. Superseded July 2026 by four-tier model (DEC-020). Migration 020 applied to RDS 2026-06-21. |

---

## DEC-020 — Four-plan model and org removal

| Field | Detail |
|-------|--------|
| **Date** | 2026-07 |
| **Status** | Active |
| **Decision** | Replace Free/Individual/Household with **Free / Paper / Pro / Pro Plus**; remove multi-user org product and `account_mode=demo` |
| **Context** | Product targeted solo algo traders. Org/Household added complexity without demand. Demo mode duplicated paper trading. Adapter split needed clearer paper vs live execution paths. |
| **Alternatives** | Keep Household for teams; keep demo mode for free live simulation; retain 3-plan model with paper as feature flag |
| **Rationale** | Four tiers map cleanly to use cases: try (Free), simulate (Paper), one live webhook (Pro), multi-webhook live (Pro Plus). Paper accounts + paper webhooks replace demo credentials. Org API removed; legacy DB retained. `BROKER_MODE=mock` remains server-only for staging/CI. Migrations 044 (org detach, live-only creds) and 046 (plan CHECK). Source: `internal/plan/plan.go`, [STATUS.md](../STATUS.md). |

---

## DEC-017 — Monorepo layout (dashboard/ at repo root)

| Field | Detail |
|-------|--------|
| **Date** | 2026-06 |
| **Status** | Active |
| **Decision** | House the Next.js dashboard inside `dashboard/` at the Go repo root (monorepo), not in a separate repository |
| **Context** | Dashboard MVP needed to ship quickly alongside API changes. |
| **Alternatives** | Separate `zettabridge-ui` repository |
| **Rationale** | Monorepo avoids cross-repo coordination during the fast MVP build. API type shapes and Bruno collection are co-located. Single CI pipeline. No cross-repo release coordination overhead. Can be extracted later if team grows. |

---

## DEC-018 — Cancel subscription immediately (not at period end)

| Field | Detail |
|-------|--------|
| **Date** | 2026-06 |
| **Status** | Active |
| **Decision** | When a Stripe subscription is cancelled (via Portal or `customer.subscription.deleted` webhook), downgrade user to Free immediately, not at end of billing period |
| **Context** | Stripe supports both immediate and end-of-period cancellation policies. |
| **Alternatives** | Downgrade at end of billing period (more user-friendly) |
| **Rationale** | Immediate downgrade is simpler to reason about in v1 — no "grace period" state to manage. Also aligns with the admin billing override model where plan state is always current. Can be changed to end-of-period post-GA if churn data shows it hurts retention. |

---

## DEC-019 — Telegram for platform alerts (email deferred)

| Field | Detail |
|-------|--------|
| **Date** | 2026-06 |
| **Status** | Active |
| **Decision** | Use Alertmanager → Telegram for platform ops alerts; email alerting deferred |
| **Context** | Platform operators needed alerting for auth spikes, ingest rejected, 5xx, and service down events. |
| **Alternatives** | PagerDuty, email only, Slack, SNS → email |
| **Rationale** | Telegram bot is zero-cost, zero-setup for a small ops team. Alertmanager integration is straightforward. Email alerting adds SMTP configuration overhead for no additional value at the current team size. CloudWatch SNS → email is planned for GA. |
