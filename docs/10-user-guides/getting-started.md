# Getting Started with ZettaBridge
**Last updated: 2026-07-07**

This guide walks you through your first trade signal from TradingView to your broker in 10 minutes.

---

## 1. Prerequisites

Before you start:

- A TradingView account (free or paid) with alert capability
- A supported broker account with API access enabled:
  - **Zerodha**: Kite Connect developer account + API key
  - **Angel One**: SmartAPI developer account + client code
  - **Dhan**: API access enabled in the Dhan portal
  - **MetaApi MT5**: MetaApi account + auth token
- A ZettaBridge invite code (beta access required)

---

## 2. Create Your Account

```
POST /v1/auth/register
{
  "email": "you@example.com",
  "password": "SecurePassword123!",
  "invite_code": "inv_your_code"
}
```

Or register via the dashboard at `https://app.staging.zettabridge.net`.

**Email verification:** if `EMAIL_VERIFICATION_REQUIRED` is enabled, check your inbox for a verification email and click the link before proceeding.

---

## 3. Choose Paper or Live Trading

ZettaBridge supports two webhook types:

| Type | Webhook field | Plan requirement | Execution |
|------|---------------|------------------|-----------|
| **Paper** | `paper_account_id` | Any plan (Free: 1 paper account, 10 trades/mo) | Simulated fills via paper engine |
| **Live** | `broker_cred_id` | **pro** or **pro_plus** (`live_trading_allowed`) | Real broker via live adapter |

### Paper path (recommended first)

Create a paper account:

```
POST /v1/paper-accounts
{
  "label": "Nifty Paper",
  "market_profile_code": "NSE_EQ"
}
```

Then create a paper webhook:

```
POST /v1/webhooks
{
  "label": "Nifty Paper Strategy",
  "paper_account_id": "<paper-account-id>",
  "symbol": "NIFTY",
  "lot_size": 1
}
```

### Live path (pro / pro_plus)

Add a **live** broker credential. `account_mode` must be **`live`** — demo credentials were removed in migration 044.

```
POST /v1/credentials
{
  "broker_type": "zerodha",
  "raw_creds": "your_api_key:your_access_token",
  "account_label": "Zerodha Main",
  "account_mode": "live",
  "algo_id": "YOURALGO123",
  "exchange": "NSE",
  "product": "MIS"
}
```

**SEBI Algo-ID** (`algo_id`): required for all live NSE/BSE orders. Obtain from your broker. See [sebi-compliance.md](../09-compliance-and-legal/sebi-compliance.md).

**Verify your credential:**

```
POST /v1/credentials/:id/verify
→ {"valid": true, "message": "credentials verified"}
```

If this fails, check the credential format for your broker:
- Zerodha: `api_key:access_token` — see [zerodha.md](../04-integrations/zerodha.md)
- Angel One: `api_key:client_code:jwt` — see [angel-one.md](../04-integrations/angel-one.md)
- Dhan: `client_id:access_token` — see [dhan.md](../04-integrations/dhan.md)

Create a live webhook:

```
POST /v1/webhooks
{
  "label": "Nifty Live Strategy",
  "broker_cred_id": "<credential-id>",
  "symbol": "NIFTY",
  "lot_size": 1,
  "dedup_window_sec": 300
}
```

---

## 4. Create a Webhook (details)

A webhook connects a TradingView alert to either a paper account or a live broker credential — not both.

Response includes `token` — the UUID v4 that forms the ingest URL:

```
https://api.staging.zettabridge.net/v1/webhook/<your-token>
```

**Keep this URL secret.** Anyone with this URL can send trade signals to your account. If it leaks, rotate it immediately:

```
POST /v1/webhooks/:id/rotate-token
```

### Webhook guards

- `allowed_actions`: only `BUY`, `SELL`, or `CLOSE` signals pass (leave empty to allow all)
- `allowed_symbols`: only listed symbols pass (leave empty to allow any symbol)
- `dedup_window_sec`: signals with the same payload within this window return **HTTP 409** (not queued again). Default on create: 2 seconds; recommended for live: 300 seconds. Advanced guards (dedup, rate limits, trading hours) require a paid plan for **live** webhooks; paper webhooks are exempt.
- `lot_size`: multiplied with the signal's `quantity` field

---

## 5. Connect TradingView

1. Open TradingView → **Alerts** → create or edit an alert.
2. In the **Notifications** tab, enable **Webhook URL**.
3. Paste your ingest URL: `https://api.staging.zettabridge.net/v1/webhook/<your-token>`
4. In the **Message** field, enter the signal JSON:

   ```json
   {"action":"BUY","symbol":"NIFTY","quantity":1,"comment":"strategy_v2_long"}
   ```

   - `action`: `BUY`, `SELL`, or `CLOSE`
   - `symbol`: must match `allowed_symbols` (if set)
   - `quantity`: number of lots or shares
   - `comment`: optional; part of the dedup hash (use a stable value; changing it creates a new signal)

5. Save the alert.

---

## 6. Test Your First Signal

Send a test signal manually (replace with your actual token):

```bash
curl -X POST https://api.staging.zettabridge.net/v1/webhook/<your-token> \
  -H "Content-Type: application/json" \
  -d '{"action":"BUY","symbol":"NIFTY","quantity":1,"comment":"test"}'
```

Expected response (first signal):

```json
{"status": "accepted", "queued": true, "deduplicated": false, "request_id": "..."}
```

HTTP **202** means the signal was accepted. The order is placed asynchronously.

If TradingView retries the same payload within the dedup window:

```json
{"queued": false, "deduplicated": true, "request_id": "..."}
```

HTTP **409** — duplicate suppressed; this is expected, not an error.

### Check the trade result

```
GET /v1/webhooks/:id/trades
```

Look for the trade row:

| Field | Meaning |
|-------|---------|
| `status: placed` | Order submitted to broker |
| `status: filled` | Order confirmed filled |
| `status: rejected` | Order blocked (see `error_code`) |
| `broker_order` | Broker-assigned order ID |
| `error_code` | Reason for rejection (if any) |

---

## 7. Watch Trades in Real Time

Connect a WebSocket to receive live trade updates:

```
GET wss://api.staging.zettabridge.net/v1/ws/trades?token=<your-jwt>
```

Your JWT access token is included as a query parameter (standard WebSocket headers cannot carry auth).

The stream pushes trade rows as JSON whenever a trade status changes.

---

## 8. Common Issues

| Symptom | Check |
|---------|-------|
| 401 on ingest | Webhook URL token may be rotated; get the new URL from `GET /v1/webhooks/:id` |
| 403 account suspended | Contact platform admin |
| 403 webhook paused | `PUT /v1/webhooks/:id/unpause` or check auto-pause reason |
| `error_code: auth_failed` on trade | Broker token expired; see your broker's guide for refresh steps |
| `error_code: algo_id_required` | Set `algo_id` on your credential — see [sebi-compliance.md](../09-compliance-and-legal/sebi-compliance.md) |
| `error_code: guard_violation` | Signal `action` or `symbol` not in webhook's allowlist |
| `queued: false, deduplicated: true` (HTTP 409) | Same signal within dedup window — TradingView retry suppressed; not an error |
| Trade `status: rejected` with no error | Check `error_code` field; common: `rate_limited`, `invalid_credentials` |

---

## 9. Plan Limits

Four tiers — source of truth: `internal/plan/plan.go`.

| Plan | Paper accounts | Paper webhooks | Live webhooks | Live creds | Paper trades/mo | Live trading | Orders/sec |
|------|----------------|----------------|---------------|------------|-----------------|--------------|------------|
| **free** | 1 | 1 | 0 | 0 | 10 | No | 1 |
| **paper** | 3 | 5 | 0 | 0 | Unlimited | No | 5 |
| **pro** | 5 | 8 | 1 | 1 | Unlimited | Yes | 10 |
| **pro_plus** | 10 | 15 | 5 | 3 | Unlimited | Yes | 10 |

Free plan: **100 webhook ingests/day** (Redis counter). Upgrade via Stripe checkout (`paper`, `pro`, `pro_plus`) when billing is enabled, or ask a platform admin.

---

## 10. Next Steps

- [Broker integration guides](../04-integrations/) — credential formats, token refresh, IP whitelisting
- [API reference](../04-integrations/api-reference.md) — all endpoints
- [SEBI Algo-ID compliance](../09-compliance-and-legal/sebi-compliance.md) — required for live Indian broker trading
- [Security architecture](../03-security/security-architecture.md) — how your credentials are protected
