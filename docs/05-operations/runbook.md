# Operations Runbook
**Last updated: 2026-07-07**

On-call and platform-admin guide for ZettaBridge. Covers incident response, account management, log hygiene, and egress verification.

Related: [observability.md](observability.md) · [developer-guide.md](../07-engineering/developer-guide.md) · [security-architecture.md](../03-security/security-architecture.md)

---

## 1. Severity Guide

| Severity | Examples | First action |
|----------|----------|--------------|
| **P0** | Duplicate live orders, suspected credential leak, mass `auth_failed` | Stop trading path immediately — suspend user or rotate webhook token; audit trades |
| **P1** | Spike in `rate_limited`, broker HTTP 5xx, single user cannot trade | Check metrics + broker status; verify token/IP whitelist |
| **P2** | Elevated ingest rejections, dedup-only replays | Review webhook guards and TradingView alert config |

---

## 2. Incident: Duplicate or Unexpected Orders

TradingView retries on timeout. ZettaBridge deduplicates when `dedup_window_sec > 0` on the webhook (paid plans for **live** webhooks; paper webhooks exempt from advanced-guard plan gate).

### Symptoms

- Two broker orders for one TradingView alert
- Trade list shows two rows with the same `signal_key`
- `zettabridge_dedup_hits_total` not incrementing on replays

### Investigation

1. `GET /v1/webhooks/:id` — confirm `dedup_window_sec > 0` (recommended: 300 for live). Free plan: advanced guards limited on live webhooks.
2. `GET /v1/webhooks/:id/trades` — compare `signal_key`, `comment`, `created_at` on duplicate rows.
3. Verify replay response: should be **HTTP 409** `{ "queued": false, "deduplicated": true }`, not 202 with `queued: true`.
4. Check for distinct `comment` values — different comment hashes bypass dedup intentionally.
5. Window expired — identical payload after `dedup_window_sec` → new order by design.

### Remediation

| Action | When | How |
|--------|------|-----|
| Enable dedup | Window was 0 | `PUT /v1/webhooks/:id` → `dedup_window_sec: 300` |
| Rotate webhook URL token | Token leaked or unknown-source abuse | `POST /v1/webhooks/:id/rotate-token` → update TradingView alert URL |
| Pause webhook | Stop all trading immediately | `PUT /v1/webhooks/:id` → `status: paused` |
| Suspend user | Account compromise or policy violation | Admin `PATCH /v1/admin/users/:id` → `status: suspended` |

---

## 3. Incident: Credential Compromise or Leak

Broker secrets are sent as `raw_creds` on POST/PUT only, encrypted at rest with `AES_KEY`. They are never returned by the API and never stored on trade rows.

### If a broker session token is exposed (pasted in chat, committed to git)

1. **Revoke at broker** — Regenerate Kite / SmartAPI / Dhan / MetaApi token in the broker portal immediately.
2. **Delete or update in ZettaBridge** — `DELETE /v1/credentials/:id` or `PUT /v1/credentials/:id` with new `raw_creds`.
3. **Audit trades** — `GET /v1/webhooks/:id/trades` for the window since exposure; note `broker_order` IDs for broker-side cancellation if needed.
4. **Verify** — `POST /v1/credentials/:id/verify` after updating.

### If webhook URL token is exposed

Anyone with the URL can send signals. Rotate immediately:

```
POST /v1/webhooks/:id/rotate-token
Authorization: Bearer <owner JWT>
```

Update every TradingView alert to the new token. The old token returns 401 on ingest.

### If platform `AES_KEY` may be compromised

All stored credentials must be treated as decryptable by the attacker.

**Rotation procedure (no in-app re-encrypt in v1):**

1. Schedule maintenance window; pause or suspend affected webhooks.
2. Generate new key: `openssl rand -hex 32`
3. Update `AES_KEY` in AWS Secrets Manager.
4. Force-new-deployment on ECS task (picks up new secret).
5. **All users must re-submit credentials** — existing ciphertext cannot be decrypted with the new key. Communicate to users via email/notification to `PUT /v1/credentials/:id` with their `raw_creds`.
6. Retire old key material from secrets store and any CI logs.

Production refuses startup with the all-zero dev `AES_KEY` when `APP_ENV=production` (`config.Validate`).

### If `JWT_SECRET` may be compromised

1. Rotate `JWT_SECRET` in AWS Secrets Manager.
2. Force-new-deployment on ECS.
3. All active access and refresh tokens invalidate; users must re-login.
4. Review admin user audit log (`GET /v1/admin/users/:id/audit`) for any admin actions taken by a forged token.

---

## 4. Suspended Users

Compliance gates block trading even when webhook tokens are still known.

| Entity | Admin action | Ingest response | Worker outcome |
|--------|--------------|-----------------|----------------|
| User `status: suspended` | `PATCH /v1/admin/users/:id` `{"status":"suspended"}` | **403** `account suspended` | Trade `rejected`, `error_code=account_suspended` |

- **Unsuspend** — set `status: active` on the same admin endpoint; ingest resumes on next signal.
- **Webhook token** — suspension does not rotate the token; rotate separately if the URL was leaked.

---

## 5. NAT Egress and IP Whitelist Verification

Live Indian brokers require a static outbound IP. Misconfiguration shows as `auth_failed`, connection errors, or order rejections.

### Verify egress IP

From a task in the same VPC/subnet (or a temporary debug ECS task):

```bash
curl -s https://checkip.amazonaws.com
```

Compare to:
- Dhan developer portal → IP whitelist
- Angel SmartAPI app → registered IP
- ECS env vars: `BROKER_ANGEL_CLIENT_PUBLIC_IP`, `BROKER_ANGEL_CLIENT_LOCAL_IP` (both must match NAT Elastic IP)

### Checklist after infra change

| Step | Pass criteria |
|------|---------------|
| NAT Gateway + Elastic IP attached | `curl checkip.amazonaws.com` shows expected IP |
| Dhan whitelist updated | Test verify or order succeeds |
| Angel env vars updated | Startup log shows correct `public_ip` / `local_ip` |
| Zerodha (if IP lock enabled) | Kite app whitelist includes NAT IP |
| MetaApi MT5 | Account provisioned; verify returns equity |

**Rollback:** set `BROKER_MODE=mock` and redeploy — webhooks keep working; no live broker calls.

---

## 6. Monitoring and Alerting

Scrape `GET /metrics` (optional `METRICS_TOKEN` bearer). Key series:

| Metric | Alert on |
|--------|----------|
| `zettabridge_ingest_total{status="rejected"}` | Sustained spike — guard violations, validation failures, suspension (distinct from rate limiting) |
| `zettabridge_ingest_total{status="rate_limited"}` | Per-webhook or free-plan daily ingest throttle — expected during rate-limit load tests |
| `zettabridge_ingest_total{status="deduplicated"}` | Healthy during TradingView retries; drop to zero while TV reports retries may indicate dedup misconfiguration |
| `zettabridge_ingest_total{status="queue_full"}` | Ingest queue saturated — lower k6 VUs or raise `QUEUE_BUFFER` |
| `zettabridge_trades_total{status="rejected",error_code="auth_failed"}` | Token expiry wave — send user comms; see broker guides |
| `zettabridge_trades_total{error_code="rate_limited"}` | Plan or broker throttle — tune limits or broker quota |
| `zettabridge_broker_http_duration_seconds` | p95 high — broker or adapter degradation (on `http` mode this is Core→adapter latency) |
| `zettabridge_dedup_hits_total` | Drop to zero while TV reports retries — dedup misconfigured |
| `zettabridge_billing_checkout_total` | Flatline during billing period — Stripe keys or price IDs wrong |
| `zettabridge_billing_webhook_total{status="signature_invalid"}` | Wrong `STRIPE_WEBHOOK_SECRET` or replay attempt |

Alert routing and Grafana setup: see [observability.md](observability.md).

---

## 7. Stripe Billing (Ops)

When `BILLING_ENABLED=true`, subscription state is driven by Stripe webhooks unless overridden by platform admin.

| Scenario | v1 behaviour | Ops action |
|----------|-------------|------------|
| **Comp / manual override** | `PUT /v1/admin/users/:id/plan` sets `billing_source=admin`; Stripe webhooks do not downgrade | Use for pilots, refunds, manual enterprise tiers |
| **Admin sets free on Stripe subscriber** | Plan downgrades in PG; `billing_source=admin`; Stripe subscription stays active | Cancel subscription in Stripe Dashboard to stop future invoices |
| **Refund** | No automatic downgrade on `charge.refunded` (not handled in v1) | Manually adjust plan via admin API if needed |
| **Dispute / chargeback** | No automatic suspension in v1 | Review in admin; suspend account if warranted |
| **`invoice.payment_failed`** | Logged only; Stripe retries per dunning settings | Follow up with customer; downgrade happens on `customer.subscription.deleted` |
| **Duplicate webhook** | Idempotent on `stripe_webhook_events.id` | None — safe to replay from Stripe Dashboard |

---

## 8. Log Hygiene

The following must never appear in logs:

| Data | Protection |
|------|-----------|
| `raw_creds` or decrypted credential material | `json:"-"` tag on model field; never logged |
| `JWT_SECRET` or `AES_KEY` | Secrets Manager injection; never in code or env output |
| Broker `Authorization`, `auth-token`, `access-token`, `x-api-key` headers | `internal/livebrokers/httpclient` auto-redaction |
| Headers containing `token` or `secret` | Catch-all redaction rule |

What does appear in logs:
- Fiber access log: method, path, status, latency (no bodies)
- Broker HTTP: `broker_http: broker=… op=… method=… path=… status=… latency_ms=…`
- Startup: `broker_mode`, worker count, Angel egress IPs (not secrets)

### Operator checklist

- [ ] CloudWatch log retention aligned with compliance policy
- [ ] Log IAM restricted to ops roles; no public raw-log export
- [ ] Debug logging that dumps request bodies is disabled in production
- [ ] Support staff must never ask users to paste `raw_creds` in tickets — PUT via API only

---

## 9. Production Security Baseline

| Control | Status in v1 |
|---------|--------------|
| Credentials encrypted at rest (`AES_KEY`) | Yes — zero-key blocked on startup in production |
| Webhook URL as capability token | Yes — rotate via `POST /v1/webhooks/:id/rotate-token` |
| Plan + user RBAC on credential CRUD | Yes |
| Ingest + worker compliance gates | Yes — suspended user |
| Signal dedup (paid live webhooks; paper exempt from guard gate) | Yes — Redis SETNX; ingest returns **409** on duplicate |
| Broker POST never retried | Yes — idempotency before retry |
| OAuth / automatic broker token refresh | No — manual PUT only |
| HTTP security headers | Pending (GA gate S1) |
| Log scrub audit | Pending (GA gate S2) |
| Pen test | Pending (GA gate S5) |
