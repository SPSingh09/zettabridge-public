# ZettaBridge — Deferred Implementation Plan

Items identified during spec audits that are not being tackled immediately.

---

## Intentional Decisions (not gaps)

### algo_id stored as plaintext
`algo_id` on `broker_credentials` is kept as a plaintext column. The spec lists it under "always encrypted" but it is a public SEBI-registered identifier, not a secret. Encrypting it would add friction without any security benefit.

---

## Data Retention Cleanup (pending)

`pg_cron` was removed from `migrations/000_schema.sql` because AWS RDS only allows it in the `postgres` database, not in the app database. The four 90-day cleanup jobs (for `user_audit_log`, `ingest_log`, `trades`, `notifications`) are now unscheduled.

- **Where:** `migrations/000_schema.sql` (jobs removed), `cmd/api/main.go` (add goroutine here)
- **Required:** Add a daily background goroutine (like the existing `zerodharefresh` pattern) that runs at e.g. 03:00 UTC and deletes rows older than 90 days from each table:
  ```go
  DELETE FROM user_audit_log  WHERE created_at < NOW() - INTERVAL '90 days'
  DELETE FROM ingest_log      WHERE created_at < NOW() - INTERVAL '90 days'
  DELETE FROM trades          WHERE created_at < NOW() - INTERVAL '90 days'
  DELETE FROM notifications   WHERE created_at < NOW() - INTERVAL '90 days'
  ```

---

## Infrastructure & Reliability

### Multi-AZ RDS for production
- **What:** Spec requires "multi-AZ database failover design". `rds.tf` currently has `multi_az = false` (intentional staging cost saving). Also `skip_final_snapshot = true` and `deletion_protection = false`.
- **Where:** `deploy/deploy-aws-ecs/terraform/staging/rds.tf`
- **Required:** Before cutting a production Terraform environment, set:
  ```hcl
  multi_az            = true
  skip_final_snapshot = false
  deletion_protection = true
  ```

---

## Order Execution

### LIMIT orders for Angel One and Dhan
- **What:** `PlaceRequest.EntryExecType` and `PlaceRequest.Price` are ignored by both adapters; they always send `"ordertype": "MARKET"` (Angel) and `"orderType": "MARKET"` (Dhan).
- **Where:** `internal/livebrokers/angel.go`, `internal/livebrokers/dhan.go`
- **Required:** Read `req.EntryExecType` and `req.Price` in each adapter's `placeMarketOrder`; add LIMIT order path equivalent to Zerodha's implementation in `zerodha.go:43-55`.

### F&O auto lot-size for Dhan
- **What:** `dhanExchangeSegment` only maps `NSE_EQ` / `BSE_EQ`; no NFO/BSE_FO/CDS segment support. No per-instrument lot-size lookup for futures/options.
- **Where:** `internal/livebrokers/dhan.go:283`, `internal/livebrokers/dhan.go:42`
- **Required:** Add NFO/BSE_FO/CDS cases to `dhanExchangeSegment`; implement instrument lot-size lookup from Dhan's instrument API and use it in `resolveLotSize`.

---

## Broker Integrations — Angel One

### TOTP-based credential storage and auto session renewal
- **What:** Spec requires storing `api_key, API secret, TOTP secret` and auto-renewing the Angel JWT before market open each day. Current implementation stores `api_key:client_code:jwt` — the user must manually supply a fresh JWT daily.
- **Where:** `internal/brokercreds/creds.go` (credential format), background job needed (does not exist yet)
- **Required:**
  1. Change Angel credential format from `api_key:client_code:jwt` to `api_key:api_secret:client_code:totp_secret`.
  2. Add migration to re-encrypt existing Angel credentials (if any).
  3. Implement TOTP-based JWT generation (use a TOTP library against the stored secret).
  4. Add a background scheduler that re-logs in Angel credentials before market open (e.g., 8:50 AM IST daily) and updates `encrypted_creds` with the fresh JWT.
  5. Update `Parsed.JWT` derivation to use the generated token at execution time or cache it in Redis.

---

## Broker Integrations — Dhan

### Order callback webhook endpoint
- **What:** Spec states "Order callbacks: real-time order status via DhanHQ webhook callback". Dhan can POST real-time order status updates to a ZettaBridge endpoint. Currently trade status is only known at placement time (submitted/rejected).
- **Where:** New endpoint needed, e.g. `POST /v1/internal/dhan/callback`
- **Required:**
  1. Add public (or HMAC-verified) Dhan callback endpoint.
  2. On callback, find the trade by `orderId`, update `status` / `fill_price` in DB.
  3. Push update via `tradepush.Hub` so dashboard WebSocket reflects the fill in real time.

---

## Broker Integrations — General

### Yellow "session renewal in progress" connection status
- **What:** The spec defines a Yellow indicator meaning "session renewal in progress — briefly unavailable". No backend state emits this; only Green (active), Red (expired/paused), and Grey (not configured) are derivable today.
- **Required:** When a renewal job runs (Angel), set a short-lived Redis flag per credential that the status endpoint reads and returns as `renewing`. Map to Yellow in the frontend.

---

## Billing — Pre-Launch Checklist

All items below must be resolved before setting `billing_enabled = true` in terraform.

### 1. Annual billing price IDs
- **What:** Spec promises 15–20% discount on annual billing. Only monthly price IDs exist (`StripePriceIndividual`, `StripePriceHousehold`). No annual variants in config or checkout.
- **Where:** `internal/config/config.go`, `internal/billing/plans.go`, `internal/billing/stripe_client.go`, `internal/billing/checkout.go`
- **Required:**
  1. Create annual Stripe prices in Stripe Dashboard for Individual and Household (15–20% off monthly rate).
  2. Add `StripePriceIndividualAnnual` / `StripePriceHouseholdAnnual` fields to `Config` and load from env.
  3. Add `interval` field to `CheckoutRequest` (`"monthly"` | `"annual"`).
  4. Route to the correct price ID in `PriceIDForOffer` based on interval.
  5. Register both annual price IDs in `PlanFromStripePriceID` so webhook can identify the plan.

### 2. Upgrade flow must update existing subscription (not create a new one)
- **What:** Upgrading from Individual → Household (or any plan change for an active subscriber) goes through `CreateCheckoutSession` which creates a **new** Stripe subscription. The old subscription stays active → double billing. The correct flow is to update the existing subscription's price item.
- **Where:** `internal/billing/checkout.go`, `internal/billing/stripe_client.go`
- **Required:**
  1. In `CreateCheckout`, check if user already has a Stripe subscription ID.
  2. If they do, call a new `StripeGateway.UpdateSubscription(subscriptionID, newPriceID)` instead of `CreateCheckoutSession`.
  3. Stripe applies proration automatically on subscription item update.
  4. Use `stripe.SubscriptionParams` with `Items[0].Price = newPriceID` and `ProrationBehavior = "create_prorations"`.

### 3. Downgrade should take effect at end of billing period
- **What:** Spec says "Downgrades take effect at end of current billing period; access continues until then." Currently `handleSubscriptionUpdated` applies any plan change immediately via `Apply()` regardless of timing.
- **Where:** `internal/billing/webhook.go`, `internal/billing/stripe_client.go`
- **Required:**
  1. When downgrading (new plan rank < current plan rank), schedule the Stripe subscription update with `ProrationBehavior = "none"` and a subscription schedule or `BillingCycleAnchor = "unchanged"` so the price switch happens at next renewal.
  2. In `handleSubscriptionUpdated`, only call `Apply(newPlan)` when the subscription's `current_period_start` changes (i.e., the new billing period has actually begun).

### 4. Post-cancellation audit export access (30-day grace window)
- **What:** Spec says config and logs are retained 30 days after subscription ends and user can export during that period. After cancellation, user is downgraded to Free → `CanViewAuditLogs(Free)` blocks all audit access → user cannot retrieve their own data.
- **Where:** `internal/plan/plan.go`, `internal/store/models.go`, `internal/handler/audit.go`
- **Required:**
  1. Add `SubscriptionEndedAt *time.Time` column to `users` table (migration).
  2. Set this field in `handleSubscriptionDeleted` when reverting to Free.
  3. In `CanViewAuditLogs`, also return `nil` if `user.SubscriptionEndedAt != nil && time.Since(*user.SubscriptionEndedAt) < 30*24*time.Hour`.

### 5. 30-day data deletion job after cancellation
- **What:** Spec requires permanent deletion of all user data 30 days after subscription ends. No scheduled deletion job exists.
- **Where:** New background job needed (e.g. `internal/billing/purge.go`)
- **Required:**
  1. Query users where `subscription_ended_at < NOW() - 30 days` and `plan = 'free'` and `billing_source != 'stripe'` (or a dedicated `pending_deletion` flag).
  2. Delete webhooks, broker credentials, trades, audit logs, and finally the user row (or anonymize PII fields).
  3. Run as a daily background goroutine started in `cmd/api/main.go`, similar to `zerodharefresh`.
  4. Send a "your data will be deleted in 7 days" email at day 23 as a warning.

### 6. Proactive plan limit alerts
- **What:** Spec says "alert sent before limit is hit" for webhook tokens and broker connections. No such notification exists; users only learn they're at the limit when the next create call is rejected.
- **Where:** `internal/handler/webhooks.go`, `internal/handler/broker_creds.go`
- **Required:**
  1. After a successful webhook/broker create, check if `current_count == max_allowed - 1` (one slot left).
  2. If so, send an in-app notification (and Telegram if connected) warning the user they are at `N/N` of their plan limit.

---

## Security — Pre-Production Launch

### Multi-factor authentication (MFA)
- **What:** Spec marks MFA as "Planned — production launch". No TOTP/MFA fields exist anywhere in the auth flow today.
- **Where:** New: `internal/handler/auth.go` (enroll/verify endpoints), `internal/store/models.go` (user schema), migration needed
- **Required:**
  1. Add `totp_secret` (encrypted) and `mfa_enabled bool` columns to `users` table (migration).
  2. Add `POST /v1/auth/mfa/enroll` — generates a TOTP secret, returns a QR code URI (use `github.com/pquerna/otp`).
  3. Add `POST /v1/auth/mfa/confirm` — user submits first TOTP code to activate; stores encrypted secret.
  4. Modify login: if `mfa_enabled`, return a short-lived `mfa_pending` token instead of the full JWT; require a second `POST /v1/auth/mfa/verify` call with the TOTP code to exchange for the full JWT.
  5. Add `DELETE /v1/auth/mfa` — disables MFA after TOTP re-verification.
  6. Dashboard: add MFA setup card to `/me` settings page.

### AWS KMS hardware-backed key management
- **What:** Spec marks KMS as "Planned". Currently AES-256-GCM encryption uses a static 32-byte key injected via `AES_KEY` env var from Secrets Manager. KMS provides hardware-backed key material and automatic key rotation without the key ever leaving AWS.
- **Where:** `internal/credenc/`, `deploy/deploy-aws-ecs/terraform/staging/secrets.tf`, new `kms.tf`
- **Required:**
  1. Create a KMS CMK in Terraform (`aws_kms_key`), grant the ECS task role `kms:Decrypt` / `kms:GenerateDataKey`.
  2. Replace `AES_KEY` env var with KMS-envelope encryption: generate a data key per credential via `GenerateDataKey`, store the encrypted data key alongside the ciphertext, decrypt on read.
  3. Update `credenc.Encrypt` / `credenc.Decrypt` to use the KMS envelope pattern.
  4. Write a one-time migration that re-encrypts all existing `encrypted_creds` rows under the new KMS envelope.
  5. Remove `AES_KEY` from Secrets Manager and tfvars after migration.

---

## Stripe Dashboard Config Checklist (no code changes)

Before enabling billing:
- Enable **UPI** and **Bank transfer** payment methods in Stripe Dashboard (India).
- Enable **Stripe Tax** and configure tax registration for India (GST).
- Set Stripe billing portal to **"Cancel at end of billing period"** (not immediate cancellation).
- Create **annual price IDs** for Individual and Household (15–20% below monthly).
- Configure **Smart Retries** for failed payments before subscription deletion.

---

## Planned Broker Integrations (future)

Per spec section 8:

| Broker | API | Markets |
|---|---|---|
| Upstox | Upstox API v3 | NSE, BSE, MCX |
| ICICI Direct | Breeze Connect | NSE, BSE, MCX, Currency |
| Fyers | Fyers API v3 | NSE, BSE, MCX, CDS |
| cTrader | cTrader Open API | Forex, CFDs |
