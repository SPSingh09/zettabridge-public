# Beta Phase 3 — Application Wiring

> **Historical implementation plan.** Current product behavior is defined in [STATUS.md](../STATUS.md) and [scope.md](../01-product/scope.md). Last reviewed: 2026-07-07.

**Target: Days 5–6**
**Goal:** Dashboard auth, billing, CORS, and Stripe all working end-to-end on AWS staging URLs. Functional tests green.

Depends on: [beta-phase-2-aws-deploy.md](beta-phase-2-aws-deploy.md)

---

## Phase 8 — Dashboard and Stripe wiring

### 8.1 CORS

The backend `CORS_ORIGINS` env var must include the dashboard domain.

In ECS task definition env:
```
CORS_ORIGINS=https://app.zettabridge.<domain>
```

Verify: from browser console on `app.zettabridge.<domain>`:
```javascript
fetch('https://api.zettabridge.<domain>/healthz', {credentials: 'include'})
  .then(r => r.json()).then(console.log)
// Must not get CORS error
```

### 8.2 Stripe redirect URLs

Stripe requires exact URL matches for success/cancel redirects and the webhook endpoint.

In ECS task definition env:
```
BILLING_SUCCESS_URL=https://app.zettabridge.<domain>/billing/success?session_id={CHECKOUT_SESSION_ID}
BILLING_CANCEL_URL=https://app.zettabridge.<domain>/billing/cancel
BILLING_PORTAL_RETURN_URL=https://app.zettabridge.<domain>/billing
```

In Stripe dashboard:
- [ ] Add `https://api.zettabridge.<domain>/v1/billing/stripe-webhook` as a webhook endpoint
- [ ] Events to listen for: `checkout.session.completed`, `customer.subscription.updated`, `customer.subscription.deleted`, `invoice.payment_failed`
- [ ] Copy new `STRIPE_WEBHOOK_SECRET` to Secrets Manager `zettabridge/staging/stripe`

In `BILLING_SUCCESS_URL` template: Stripe replaces `{CHECKOUT_SESSION_ID}` automatically — no code change needed.

### 8.3 Auth cookie / JWT flow

Dashboard uses `localStorage` JWT (per 4B.2 decision). No cookie proxy needed. Verify:
- [ ] Login → token stored in localStorage `zb_token`
- [ ] 401 on expired token → redirects to `/login` (apiFetch handles this)
- [ ] `POST /v1/auth/refresh` works on AWS domain

### 8.4 Email verification URLs

When users register, the API sends a verification email containing `APP_PUBLIC_URL/verify-email?token=...`. This must point to the API domain, not the dashboard.

In ECS task definition:
```
APP_PUBLIC_URL=https://api.zettabridge.<domain>
```

The dashboard verify-email page at `/verify-email?token=` hits `GET /v1/auth/verify-email?token=` — this works because the dashboard calls the API through `NEXT_PUBLIC_API_URL`.

### 8.5 Functional test run on AWS

After all wiring is complete:
```bash
# Set FUNCTIONAL_TEST_BASE_URL to the new AWS staging API
export FUNCTIONAL_TEST_BASE_URL=https://api.zettabridge.<domain>
export FUNCTIONAL_TEST_STAGING=1

cd functional-tester
go test -tags functional -v -timeout 120s ./...
```

Expected: all subtests pass (same as Lightsail baseline).

---

## Application smoke checklist (manual)

Run through each flow in a browser on `app.zettabridge.<domain>`:

| Flow | Steps | Pass criteria |
|------|-------|--------------|
| Register | Register new account | "Check your email" shown |
| Verify email | Click link in log/email | "Email verified" shown |
| Login | Login with new account | Redirected to `/webhooks` |
| Webhook create | Create webhook with mock credential | Appears in list |
| Credential create | Add demo broker credential | Appears in list |
| Credential verify | Click verify | Returns valid/invalid inline |
| Webhook trades | Send signal via Bruno/curl | Trade row appears in WebSocket feed |
| Billing | Click upgrade | Redirects to Stripe Checkout |
| Billing success | Complete Stripe test checkout | `/billing/success` shows updated plan |
| P&L tab | Open webhook detail → P&L tab | Stat cards render (even if 0 trades) |
| Org page | `user.org_id` set → `/org` | Usage meters load |
| Admin page | Login as admin → `/admin` | Users table loads |
| Logout | Click logout | Redirected to `/login`, token cleared |

**Exit:** All flows pass. No CORS errors, no 500s, Stripe test checkout completes end-to-end.
