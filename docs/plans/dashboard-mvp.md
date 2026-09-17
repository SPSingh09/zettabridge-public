# Phase 4B.2 — Dashboard MVP

> **Historical implementation plan.** Current product behavior is defined in [STATUS.md](../STATUS.md) and [scope.md](../01-product/scope.md). Last reviewed: 2026-07-07.

Self-serve UI for ZettaBridge: auth flows, webhook/credential management, and live trade monitoring — replacing Bruno for end users.

**Roadmap:** [roadmap.md](../roadmap.md#pr-4b2--dashboard-mvp) · **Status:** Planned  
**Depends on:** 4A.1 (P&L API ✓), 4A.2 (WebSocket push ✓), 4B.1 (Stripe billing ✓)  
**Sprints:** S7–S9

---

## Why this is needed

All backend APIs are complete. Right now users need Bruno or raw `curl` to register, configure webhooks, and watch trades. The dashboard is the product surface that:

- Removes the onboarding friction blocking closed beta (5C.1)
- Provides the checkout redirect target for Stripe (currently 404 at `/billing/success`)
- Makes the system self-serve without platform admin involvement

---

## Goals (MVP)

| Goal | MVP behavior |
|------|-------------|
| Auth | Register, verify email, login, logout |
| Profile | View plan, billing source, usage limits |
| Billing | Upgrade via Stripe Checkout; manage via Portal |
| Webhooks | List, create, update, pause/resume, rotate token, delete |
| Credentials | List, create (masked), verify, delete |
| Trades | Live feed via WebSocket; per-webhook history |
| Error states | Unverified email gate, plan limit blocks, loading/empty |

### Non-goals (deferred to 4B.3)

- Org switcher, org invites, org usage meters
- P&L charts and win-rate display
- Platform admin ops view
- Password change / account settings
- Notification preferences (Telegram alert setup)

---

## Tech stack

| Choice | Recommendation | Rationale |
|--------|---------------|-----------|
| Framework | **Next.js 14 (App Router)** | SSR for auth pages; RSC for data fetching; good TypeScript defaults |
| Language | **TypeScript** | Type-safe API client generation matches Go response shapes |
| Styling | **Tailwind CSS** | Utility-first; no design-system dependency for MVP |
| Data fetching | **SWR** | Lightweight, stale-while-revalidate for webhook/credential lists |
| WebSocket | Native `WebSocket` API | Direct connection to `/v1/ws/trades`; no extra library needed |
| Auth state | Cookie-stored JWT (`httpOnly`) **or** `localStorage` | See [Auth approach](#auth-approach) |
| Build | `next build` → Docker image | Same `Dockerfile`-style output, deploy alongside API |

### Repo layout

Two options — pick before coding:

| Option | Layout | Trade-off |
|--------|--------|-----------|
| **Monorepo** (recommended) | `dashboard/` at repo root | One CI pipeline, shared types possible |
| Separate repo | `zettabridge-ui` | Cleaner separation; harder to keep API types in sync |

Recommendation: **monorepo** for MVP — avoids cross-repo coordination overhead during the fast MVP build.

```
dashboard/
  app/                    # Next.js App Router pages
    (auth)/               # login, register, verify-email
    (app)/                # authenticated layout
      page.tsx            # overview / redirect to /webhooks
      webhooks/
      credentials/
      billing/
  components/             # shared UI
  lib/
    api.ts                # typed fetch wrapper (base: NEXT_PUBLIC_API_URL)
    auth.ts               # JWT storage + refresh helper
    ws.ts                 # WebSocket trade client
  public/
  Dockerfile
  next.config.ts
```

---

## Auth approach

The Go API issues JWTs from `POST /v1/auth/login`. Two options:

| Option | Pros | Cons |
|--------|------|------|
| **`localStorage` JWT** (recommended for MVP) | Simple; no Next.js server-side proxy needed; works with current `Authorization: Bearer` flow | XSS-accessible (mitigated by CSP headers) |
| `httpOnly` cookie proxy | XSS-safe | Requires Next.js API route as proxy for every call; doubles infra complexity |

Recommendation: **`localStorage`** for MVP with a strict CSP header. Move to httpOnly cookies in 4B.3 if required.

**Token refresh:** `POST /v1/auth/refresh` is already implemented. The API client intercepts 401 responses, attempts refresh, retries once, then redirects to login.

---

## Page map

### Unauthenticated

| Route | Component | API calls |
|-------|-----------|-----------|
| `/login` | LoginForm | `POST /v1/auth/login` |
| `/register` | RegisterForm | `POST /v1/auth/register` |
| `/verify-email` | VerifyEmailBanner | `GET /v1/auth/verify-email?token=` |
| `/resend-verification` | ResendForm | `POST /v1/auth/resend-verification` |
| `/invites/:token` | InvitePreview | `GET /v1/invites/:token` → `POST /v1/invites/accept` |
| `/billing/success` | BillingSuccess | `GET /v1/me` (poll until plan updated) |
| `/billing/cancel` | BillingCancel | static |

### Authenticated (shared layout: nav + plan badge)

| Route | Component | API calls |
|-------|-----------|-----------|
| `/` | → redirect to `/webhooks` | |
| `/webhooks` | WebhookList | `GET /v1/webhooks` |
| `/webhooks/new` | WebhookForm | `POST /v1/webhooks` |
| `/webhooks/:id` | WebhookDetail | `GET /v1/webhooks/:id/trades`, WS `/v1/ws/trades` |
| `/webhooks/:id/edit` | WebhookForm | `PUT /v1/webhooks/:id` |
| `/credentials` | CredentialList | `GET /v1/credentials` |
| `/credentials/new` | CredentialForm | `POST /v1/credentials` |
| `/credentials/:id` | CredentialDetail | `POST /v1/credentials/:id/verify` |
| `/billing` | BillingOverview | `GET /v1/billing/plans`, `POST /v1/billing/checkout`, `POST /v1/billing/portal` |
| `/me` | ProfilePage | `GET /v1/me` |

---

## Key component specs

### WebhookList

- Shows: name, status (active/paused), broker type, trade count, token (masked `tok_…****`)
- Actions: create (→ `/webhooks/new`), pause/resume toggle, rotate token (confirm dialog), delete (confirm)
- Plan limits: show "upgrade to add more" when `webhooks.length >= maxWebhooks`

### WebhookDetail

- Tabs: **Trades** | **Settings** | **Guards**
- **Trades tab:** WebSocket connection to `GET /v1/ws/trades`; filters to this webhook's token; live scroll with pause; shows `symbol`, `side`, `quantity`, `status`, `error_code`, timestamp
- Fallback: if WS disconnects, poll `GET /v1/webhooks/:id/trades` every 30s
- **Settings tab:** guard fields (`allowed_symbols`, `allowed_actions`, `dedup_window_sec`, `rate_limit_per_sec`), URL copy button for TradingView
- **Guards tab:** editable guard config

### CredentialList

- Shows: broker type, account mode (live/demo), exchange, product, masked key
- Verify button → `POST /v1/credentials/:id/verify` → show result inline

### BillingOverview

- Current plan badge, billing source (`free` / `stripe` / `admin`)
- Plan cards for Pro + Enterprise tiers (prices from `GET /v1/billing/plans`)
- Upgrade button → POST checkout → redirect to Stripe URL
- "Manage subscription" button → POST portal → redirect (only if `billing_source=stripe`)
- Disabled state when `BILLING_ENABLED=false` (API returns 503)

### UnverifiedEmailBanner

- Shown at top of all authenticated pages when `GET /v1/me` has `email_verified=false`
- "Resend verification email" inline action

---

## API client design

Single typed wrapper in `lib/api.ts`:

```ts
// Base: NEXT_PUBLIC_API_URL (e.g. http://localhost:8080 dev, https://api.example.com prod)
async function apiFetch<T>(path: string, opts?: RequestInit): Promise<T>
```

- Attaches `Authorization: Bearer <token>` from `localStorage`
- On 401: tries `POST /v1/auth/refresh`, retries once, then `router.push('/login')`
- On 503: surfaces "billing not enabled" message for billing endpoints

Types derived from API response shapes (manually for MVP; codegen in 4B.3).

---

## WebSocket trade feed

```ts
// lib/ws.ts
class TradeSocket {
  connect(token: string): void   // opens ws://…/v1/ws/trades?token=Bearer <jwt>
  disconnect(): void
  onTrade(cb: (trade: Trade) => void): void
  onError(cb: (err: Event) => void): void
}
```

- JWT passed as `Authorization` header isn't possible in browser WebSocket. Use query param: `?token=<jwt>` or sub-protocol header. Check existing `wsTradesUpgrade` handler for how auth is currently expected.
- Reconnect with exponential backoff (1s, 2s, 4s, max 30s)
- Show "Live" / "Reconnecting…" / "Disconnected" badge on webhook detail

---

## Environment variables

| Variable | Dev | Prod |
|----------|-----|------|
| `NEXT_PUBLIC_API_URL` | `http://localhost:8080` | `https://api.example.com` |
| `NEXT_PUBLIC_BILLING_ENABLED` | `false` | `true` |
| `NEXT_PUBLIC_APP_URL` | `http://localhost:3000` | `https://app.example.com` |

The API already uses `APP_PUBLIC_URL` for email verify links and Stripe redirect URLs. The dashboard URL goes in `BILLING_SUCCESS_URL` (or `NEXT_PUBLIC_APP_URL/billing/success`).

---

## Docker / deploy

```dockerfile
# dashboard/Dockerfile
FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:20-alpine
WORKDIR /app
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
EXPOSE 3000
CMD ["node", "server.js"]
```

Add `dashboard` service to `docker-compose.yml`:

```yaml
dashboard:
  build:
    context: ./dashboard
  ports:
    - "3000:3000"
  environment:
    NEXT_PUBLIC_API_URL: "http://server:8080"
  depends_on:
    - server
```

For OCI/AWS: deploy as a second Fargate service or Lightsail container alongside the API; share the same ALB with path-based routing (`/` → dashboard, `/v1` → API).

---

## WebSocket auth

`GET /v1/ws/trades` already accepts `?token=<jwt>` via `middlewareBearerOrQuery` in `internal/handler/ws_trades.go:48`. No backend change needed — the dashboard WS client just passes the JWT as a query param:

```ts
new WebSocket(`${WS_BASE_URL}/v1/ws/trades?token=${jwt}`)
```

No spike required.

---

## Implementation phases (S7–S9)

### Phase A — Foundation + Auth (S7 start, ~3 days)

- [ ] `npm create next-app dashboard` with TypeScript + Tailwind
- [ ] `lib/api.ts` — typed fetch wrapper, JWT refresh logic
- [ ] Login page + form validation
- [ ] Register page (show "check your email" on success)
- [ ] Verify email page (reads `?token=` from URL)
- [ ] Resend verification page
- [ ] Authenticated layout shell (nav: Webhooks | Credentials | Billing | Profile)
- [ ] `GET /v1/me` → display name, plan badge
- [ ] UnverifiedEmailBanner component
- [ ] `docker-compose.yml` dashboard service

### Phase B — Webhooks + Credentials (~4 days)

- [ ] WebhookList page (list, pause/resume, delete)
- [ ] WebhookForm (create + edit): name, broker type, guards fields
- [ ] Rotate token confirm dialog → update displayed token
- [ ] TradingView webhook URL copy button
- [ ] CredentialList page (masked display)
- [ ] CredentialForm: broker type, account mode, raw creds, exchange, product, algo_id
- [ ] Verify credential inline result

### Phase C — Trades + Billing (~3 days)

- [ ] WebSocket client (`lib/ws.ts`) + WS auth query-param spike
- [ ] WebhookDetail trades tab (live feed + fallback polling)
- [ ] Trade row: symbol, action, qty, status, error_code, timestamp
- [ ] BillingOverview page: plan cards, upgrade CTA, portal link
- [ ] `/billing/success` and `/billing/cancel` landing pages
- [ ] Plan limit guards: "upgrade to unlock" state in WebhookForm + CredentialForm

### Phase D — Polish + integration (~2 days)

- [ ] Invite accept flow (`/invites/:token`)
- [ ] Loading skeletons on list pages (no content flash)
- [ ] Error boundaries + toast notifications for API errors
- [ ] Empty states (no webhooks, no credentials, no trades)
- [ ] CORS: add `NEXT_PUBLIC_APP_URL` to `CORS_ORIGINS` on API server
- [ ] Stripe `APP_PUBLIC_URL` / `BILLING_SUCCESS_URL` wired to dashboard URL
- [ ] `make dev-dashboard` target (runs both API + dashboard in parallel)
- [ ] Bruno: add note that dashboard now covers manual flows

---

## Acceptance criteria

- [ ] User can register → verify email → login without Bruno
- [ ] Webhook create/pause/resume/rotate/delete cycle works end-to-end
- [ ] Credential create → verify shows success/failure inline
- [ ] Live trades appear in WebhookDetail within 2s of webhook ingest
- [ ] Billing page shows correct plan; upgrade redirects to Stripe Checkout
- [ ] `/billing/success` polls `GET /v1/me` and shows updated plan after webhook delivers
- [ ] Unverified email gate visible; resend works
- [ ] Dashboard deploys via `docker-compose up` alongside API
- [ ] CORS and `APP_PUBLIC_URL` wired so email verify links + Stripe redirects land on dashboard
- [ ] No hardcoded API URLs (all via `NEXT_PUBLIC_API_URL`)

---

## Effort estimate

| Slice | Size |
|-------|------|
| Phase A — Auth + foundation | 3 days |
| Phase B — Webhooks + credentials | 4 days |
| Phase C — Trades + billing | 3 days |
| Phase D — Polish + integration | 2 days |
| **Total** | **~12 days** (3 sprints, S7–S9) |

---

## Open decisions (pick before coding)

| # | Question | Recommendation |
|---|----------|---------------|
| 1 | Monorepo vs separate repo | **Monorepo** — `dashboard/` at root |
| 2 | JWT storage: localStorage vs httpOnly cookie | **localStorage** for MVP; cookie in 4B.3 |
| 3 | WS auth: query param vs ticket | **Query param** — handler already supports `?token=` (resolved) |
| 4 | Dashboard domain: same ALB path-routing vs separate subdomain | **Subdomain** (`app.example.com`) — cleaner CORS story |
| 5 | Styling: Tailwind only vs component library (shadcn/ui) | **shadcn/ui** on top of Tailwind — pre-built accessible primitives without design-system lock-in |

---

## Related

- [roadmap.md](../roadmap.md) — S7–S9 sprint slot
- [stripe-billing.md](stripe-billing.md) — Stripe redirect URLs, billing_source logic
- [plans-and-billing.md](../plans-and-billing.md) — plan limits referenced in UI
- [email-verification.md](email-verification.md) — verify flow the UI implements
- [operations-security.md](../operations-security.md) — CORS configuration
- `GET /v1/ws/trades` — WebSocket auth needs verification before building client
