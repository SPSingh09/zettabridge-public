# Data Flows
**Last updated: 2026-07-07**

Sequence diagrams for all major system flows. Actors: **Client** (dashboard browser or TradingView), **ALB** (AWS Load Balancer), **API** (Go Fiber server), **Queue** (in-process worker), **Broker** (live broker API), **DB** (PostgreSQL), **Redis** (ElastiCache), **WS Hub** (in-process WebSocket hub).

---

## 1. Webhook Ingest → Order Execution (primary flow)

This is the critical path. Ingest returns **202** when accepted; **409** when deduplicated within the dedup window.

```
TradingView         ALB         API (handler)       Redis        Queue Worker      Broker         DB           WS Hub
     │               │                │               │                │              │             │              │
     │ POST /v1/      │                │               │                │              │             │              │
     │ webhook/:token │                │               │                │              │             │              │
     │───────────────►│                │               │                │              │             │              │
     │               │ HTTPS forward  │               │                │              │             │              │
     │               │───────────────►│               │                │              │             │              │
     │               │                │               │                │              │             │              │
     │               │                │ token lookup  │                │              │             │              │
     │               │                │──────────────────────────────────────────────────────────►│              │
     │               │                │ webhook+cred ◄─────────────────────────────────────────────              │
     │               │                │               │                │              │             │              │
     │               │                │ suspend check │                │              │             │              │
     │               │                │  (user)      │                │              │             │              │
     │               │                │──────────────────────────────────────────────────────────►│              │
     │               │                │ ok ◄──────────────────────────────────────────────────────              │
     │               │                │               │                │              │             │              │
     │               │                │ guard check   │                │              │             │              │
     │               │                │ (rate, symbol,│                │              │             │              │
     │               │                │  action, dedup│                │              │             │              │
     │               │                │──────────────►│                │              │             │              │
     │               │                │    SETNX ok? ◄│                │              │             │              │
     │               │                │  (dup → 409) │                │              │             │              │
     │               │                │               │                │              │             │              │
     │               │                │   enqueue     │                │              │             │              │
     │               │                │──────────────────────────────►│              │             │              │
     │               │                │               │                │              │             │              │
     │               │ 202 Accepted   │               │                │              │             │              │
     │◄──────────────────────────────◄│               │                │              │             │              │
     │  (or 409 if deduplicated)      │               │                │              │             │              │
     │               │                │               │                │              │             │              │
     │               │                │               │                │ route paper  │             │              │
     │               │                │               │                │ or live      │             │              │
     │               │                │               │                │ adapter      │             │              │
     │               │                │               │                │              │             │              │
     │               │                │               │                │ resolve      │             │              │
     │               │                │               │                │ algo_id      │             │              │
     │               │                │               │                │              │             │              │
     │               │                │               │                │ PlaceOrder() │             │              │
     │               │                │               │                │─────────────►│             │              │
     │               │                │               │                │ broker_order_id◄───────────              │
     │               │                │               │                │              │             │              │
     │               │                │               │                │ INSERT trade │             │              │
     │               │                │               │                │─────────────────────────►│              │
     │               │                │               │                │ trade row ◄───────────────              │
     │               │                │               │                │              │             │              │
     │               │                │               │                │ Notify(userID, trade)      │              │
     │               │                │               │                │────────────────────────────────────────►│
     │               │                │               │                │              │             │             │
```

**Key points:**
- **202** is returned when the signal is accepted and enqueued. TradingView gets a fast response.
- **409 Conflict** is returned when dedup detects a duplicate within `dedup_window_sec` — body includes `deduplicated: true`, `queued: false`. Signal is **not** re-queued.
- If the channel is full (backpressure), ingest returns **503** — metric `queue_full`.
- Per-webhook rate limit exceeded returns **429** — metric `rate_limited`.
- Dedup check is a Redis `SETNX`; duplicate within window → 409 (not 202).
- Prometheus: `zettabridge_ingest_total{status="queued"}` means accepted; `deduplicated`, `rate_limited`, and `queue_full` are separate labels.
- Paper webhooks route to paper adapter; live webhooks decrypt credential and route to live adapter.
- On broker failure: trade row is inserted with `status=rejected` and `error_code`.

---

## 2. User Registration + Email Verification

```
Browser             ALB         API             DB             SMTP
   │                 │           │               │               │
   │ POST /v1/auth/register      │               │               │
   │────────────────►│           │               │               │
   │                 │──────────►│               │               │
   │                 │           │ hash password │               │
   │                 │           │ INSERT user   │               │
   │                 │           │──────────────►│               │
   │                 │           │ user.id ◄─────│               │
   │                 │           │               │               │
   │                 │           │ gen token     │               │
   │                 │           │ hash token    │               │
   │                 │           │ INSERT        │               │
   │                 │           │ email_verif   │               │
   │                 │           │──────────────►│               │
   │                 │           │               │               │
   │                 │           │ send verify   │               │
   │                 │           │ email ────────────────────────►
   │ 201 Created ◄───────────────│               │               │
   │                 │           │               │               │
   │ (clicks link)   │           │               │               │
   │ GET /v1/auth/verify-email?token=...         │               │
   │────────────────►│           │               │               │
   │                 │──────────►│               │               │
   │                 │           │ hash(token)   │               │
   │                 │           │ SELECT        │               │
   │                 │           │ email_verif   │               │
   │                 │           │──────────────►│               │
   │                 │           │ row ◄─────────│               │
   │                 │           │               │               │
   │                 │           │ UPDATE user   │               │
   │                 │           │ email_verified_at = NOW()     │
   │                 │           │──────────────►│               │
   │                 │           │ UPDATE verif  │               │
   │                 │           │ consumed_at   │               │
   │                 │           │──────────────►│               │
   │ 200 OK ◄────────────────────│               │               │
```

When `EMAIL_VERIFICATION_REQUIRED=false` (local dev), the login gate is skipped and users can log in immediately.

---

## 3. Login + Token Refresh

```
Browser             API             Redis           DB
   │                 │               │               │
   │ POST /v1/auth/login             │               │
   │────────────────►│               │               │
   │                 │ SELECT user   │               │
   │                 │──────────────────────────────►│
   │                 │ user ◄────────────────────────│
   │                 │               │               │
   │                 │ bcrypt compare│               │
   │                 │ sign JWT      │               │
   │                 │ (access_token, refresh_token) │
   │                 │               │               │
   │                 │ INSERT        │               │
   │                 │ refresh_tokens│               │
   │                 │──────────────────────────────►│
   │                 │               │               │
   │ {access_token, refresh_token} ◄─│               │
   │                 │               │               │
   │ (access expires — 5h)           │               │
   │                 │               │               │
   │ POST /v1/auth/refresh           │               │
   │────────────────►│               │               │
   │                 │ hash(token)   │               │
   │                 │ SELECT refresh_tokens         │
   │                 │──────────────────────────────►│
   │                 │ row ◄─────────────────────────│
   │                 │ sign new access_token         │
   │ {new access_token} ◄────────────│               │
   │                 │               │               │
   │ POST /v1/auth/logout            │               │
   │────────────────►│               │               │
   │                 │ SETEX JWT     │               │
   │                 │ (revocation)  │               │
   │                 │──────────────►│               │
   │                 │ UPDATE refresh│               │
   │                 │ _tokens       │               │
   │                 │ revoked_at    │               │
   │                 │──────────────────────────────►│
   │ 200 OK ◄────────│               │               │
```

JWT revocation on logout: the raw JWT is stored in Redis with a TTL matching the token's expiry. The `CheckRevoked` middleware checks this on every authenticated request.

---

## 4. WebSocket Trade Feed

```
Dashboard           ALB         API (WS)       WS Hub        Queue Worker
   │                 │           │               │                │
   │ GET /v1/ws/trades?token=<jwt>               │                │
   │────────────────►│           │               │                │
   │                 │──────────►│               │                │
   │                 │           │ validate JWT  │                │
   │                 │           │ (query param) │                │
   │                 │           │               │                │
   │   WebSocket upgrade handshake               │                │
   │◄───────────────────────────►│               │                │
   │                 │           │               │                │
   │                 │           │ Register      │                │
   │                 │           │ conn(userID)──►│                │
   │                 │           │               │                │
   │                 │           │               │  (trade placed)│
   │                 │           │               │◄───────────────│
   │                 │           │               │  Notify(uid,t) │
   │                 │           │               │                │
   │ {trade event} ◄────────────────────────────│                │
   │                 │           │               │                │
   │ (disconnect)    │           │               │                │
   │────────────────►│           │               │                │
   │                 │──────────►│               │                │
   │                 │           │ Unregister    │                │
   │                 │           │ conn(userID)──►│               │
```

The WebSocket hub is in-process. Multiple browser tabs for the same user each get their own connection registered under the same `userID`. All connections receive the same trade events for that user.

---

## 5. Credential Add + Verify

```
Dashboard           API             DB           Broker
   │                 │               │               │
   │ POST /v1/credentials            │               │
   │ {broker_type, raw_creds, ...}   │               │
   │────────────────►│               │               │
   │                 │ AES-256-GCM   │               │
   │                 │ encrypt       │               │
   │                 │ raw_creds     │               │
   │                 │               │               │
   │                 │ INSERT        │               │
   │                 │ broker_credentials            │
   │                 │──────────────►│               │
   │                 │ cred.id ◄─────│               │
   │                 │               │               │
   │ {id, broker_type, ...} ◄────────│               │
   │ (raw_creds NOT in response)     │               │
   │                 │               │               │
   │ POST /v1/credentials/:id/verify │               │
   │────────────────►│               │               │
   │                 │ SELECT cred   │               │
   │                 │──────────────►│               │
   │                 │ cred ◄────────│               │
   │                 │               │               │
   │                 │ AES decrypt   │               │
   │                 │ in memory     │               │
   │                 │               │               │
   │                 │ VerifyCredential()            │
   │                 │──────────────────────────────►│
   │                 │ ok / error ◄──────────────────│
   │                 │               │               │
   │                 │ UPDATE status │               │
   │                 │ (active/error)│               │
   │                 │──────────────►│               │
   │ {status, message} ◄─────────────│               │
```

`raw_creds` is decrypted in API process memory only during verify. It is never written to logs or included in any response.

---

## 6. Org Invite Flow

**Removed from product (2026).** All `/v1/orgs/*` routes removed from router. Legacy org tables remain in DB. Dashboard org pages redirect to `/webhooks`. Platform (closed-beta) invites use separate platform invite routes — not org membership.

---

## 7. Zerodha OAuth (per-user)

Each ZettaBridge user goes through their own Kite Connect OAuth flow. The platform has one Kite API key; each user gets their own access token.

```
User (browser)      API             Zerodha Kite Connect
     │               │                       │
     │ GET /v1/credentials                   │
     │ /zerodha/connect                      │
     │──────────────►│                       │
     │               │ gen login_url         │
     │               │ (api_key + checksum)  │
     │ redirect ◄────│                       │
     │               │                       │
     │ (logs in at Kite)                     │
     │──────────────────────────────────────►│
     │◄──────────────────────────────────────│
     │ redirect to ZERODHA_CALLBACK_URL      │
     │ ?request_token=...&status=success     │
     │──────────────►│                       │
     │               │ POST /session/token   │
     │               │ (api_key, request_token, checksum)
     │               │──────────────────────►│
     │               │ {access_token} ◄──────│
     │               │                       │
     │               │ encrypt access_token  │
     │               │ UPDATE credential     │
     │               │ (or INSERT new row)   │
     │               │                       │
     │ redirect to   │                       │
     │ ZERODHA_FRONTEND_URL ◄────────────────│
```

**Constraint:** Zerodha access tokens expire daily. Users must re-run this OAuth flow (or use `PUT /v1/credentials/:id` with a manually obtained token) each trading day. Automated refresh is deferred post-GA.

---

## 8. Stripe Checkout (disabled — for reference)

When `BILLING_ENABLED=true`, the self-serve upgrade flow is:

```
User (browser)      API             Stripe
     │               │               │
     │ POST /v1/billing/checkout     │
     │ {price_id: "price_..."}       │
     │──────────────►│               │
     │               │ CreateCheckoutSession()
     │               │──────────────►│
     │               │ {session_url} ◄│
     │ redirect ◄────│               │
     │               │               │
     │ (pays at Stripe)              │
     │──────────────────────────────►│
     │               │               │
     │ redirect to /billing/success  │
     │               │               │
     │               │  POST /v1/webhooks/stripe (async)
     │               │◄──────────────│ checkout.session.completed
     │               │               │
     │               │ SELECT stripe_webhook_events (idempotency)
     │               │ INSERT if not exists          │
     │               │ UPDATE users.plan             │
     │               │ UPDATE users.billing_source='stripe'
     │               │               │
     │ GET /v1/me    │               │
     │──────────────►│               │
     │ {plan: "pro"} ◄───────────────│
```

`BILLING_ENABLED=false` (current): checkout and portal endpoints return 402/503 without calling Stripe. Admin plan override (`PUT /v1/admin/users/:id/plan`) is the only billing path active today.

---

## 9. Admin Plan Override

```
Admin (browser)     API             DB
     │               │               │
     │ PUT /v1/admin │               │
     │ /users/:id    │               │
     │ /plan         │               │
     │ {plan: "pro"}          │
     │──────────────►│               │
     │               │ RequirePlatformAdmin() middleware
     │               │ (checks JWT role=admin claim)
     │               │               │
     │               │ UPDATE users  │
     │               │ SET plan = 'pro',
     │               │ billing_source = 'admin'
     │               │──────────────►│
     │ 200 OK ◄──────│               │
```

`billing_source = 'admin'` prevents subsequent Stripe webhook events from changing the plan — the Stripe event handler checks `billing_source` before updating.
