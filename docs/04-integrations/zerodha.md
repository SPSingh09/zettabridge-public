# Zerodha (Kite Connect) Integration Guide
**Last updated: 2026-07-07**

---

## 1. Credential Format

ZettaBridge credential `raw_creds` for Zerodha:

```
api_key:access_token
```

| Field | Where to find | Notes |
|-------|--------------|-------|
| `api_key` | Kite Connect developer console → app → API key | Stable; does not rotate |
| `access_token` | Generated daily via Kite login flow | Expires at ~3:00 AM IST next day |

**Example (never store real values):**

```json
{
  "broker_type": "zerodha",
  "raw_creds": "api_key_here:access_token_here",
  "account_mode": "live",
  "algo_id": "YOURALGO123",
  "exchange": "NSE",
  "product": "MIS"
}
```

---

## 2. Token Refresh (Manual)

ZettaBridge does **not** refresh Kite tokens automatically. Users must `PUT /v1/credentials/:id` when the token expires.

| When | Symptom | Action |
|------|---------|--------|
| Daily (~3 AM IST) | `auth_failed`, `error_code=auth_failed` on trades | Regenerate `access_token` in Kite developer console → `PUT /v1/credentials/:id` → verify |
| After re-login | New access_token issued | Same PUT flow |

After updating, always run `POST /v1/credentials/:id/verify` to confirm the new token works before the next trade.

---

## 3. Zerodha OAuth (Per-User Kite Login)

Kite Connect access tokens are bound to the **API key owner's account**. The Kite Connect API does not support third-party OAuth where multiple end-users can log in under a single developer app and trade their own accounts independently.

**Implication for ZettaBridge:** Each ZettaBridge user who wants to trade via Zerodha must obtain their **own Kite Connect API key** (register a Kite Connect app in the developer console) and generate their own access token.

The Zerodha OAuth callback in ZettaBridge (`GET /v1/auth/zerodha/callback`) is a convenience flow to exchange a Kite login code for an access token — it still requires the user's own `api_key`.

---

## 4. SEBI Algo-ID Requirement

SEBI/NSE requires all API-originated algo orders on NSE/BSE to carry an exchange-assigned Algo ID from August 2025.

| Field | Kite API key | Max length | Charset |
|-------|-------------|------------|---------|
| `tag` | `tag` in order payload | 20 chars | Alphanumeric |

**For ≤10 OPS (typical retail):** obtain a generic Algo ID from Zerodha support or register in the Kite developer console.

**ZettaBridge config:**

- Set `algo_id` on the credential: `PUT /v1/credentials/:id` → `{"algo_id": "YOURALGO123"}`
- Or set env fallback `BROKER_ZERODHA_ALGO_ID` for staging/pilot
- When `SEBI_ALGO_ID_REQUIRED=true` (default in live mode), missing ID rejects the trade with `error_code=algo_id_required`

ZettaBridge sends the `algo_id` value as the Kite `tag` field on every live NSE/BSE order.

---

## 5. Exchange and Product Settings

| Field | Values | Default |
|-------|--------|---------|
| `exchange` | `NSE`, `BSE` | `NSE` |
| `product` | `MIS` (intraday), `CNC` (delivery) | `MIS` |
| `order_type` | `MARKET`, `LIMIT` | `MARKET` |

Set `exchange` and `product` on the credential, not per-signal. Symbol names follow NSE/BSE equity tickers (`RELIANCE`, `NIFTY50`, `TCS`).

---

## 6. IP Whitelisting

Kite Connect apps may optionally enable IP whitelisting. If enabled:

- Add the ZettaBridge NAT Gateway Elastic IP to the Kite app's allowed IPs in the developer console.
- Verify egress IP: `curl -s https://checkip.amazonaws.com` from a VPC task.

IP whitelisting is not mandatory for Kite Connect but is recommended for production.

---

## 7. Symbol Conventions

Use NSE/BSE equity ticker symbols (not the `-EQ` suffix):

| Signal payload | Order placed on |
|----------------|-----------------|
| `"symbol": "RELIANCE"` | RELIANCE-EQ on NSE |
| `"symbol": "NIFTY"` | NIFTY index (F&O) |
| `"symbol": "EURUSD"` | ❌ Rejected — Zerodha cred does not support forex |

Use `allowed_symbols` on the webhook to restrict which symbols are accepted.

---

## 8. Rate Limits

Per-user worker caps (`internal/plan/plan.go`):

| Plan | Orders/sec (enforced) |
|------|------------------------|
| free | 1/s |
| paper | 5/s |
| pro | 10/s |
| pro_plus | 10/s |

Per live broker credential: **10 OPS** (`BrokerCredOrdersPerSecCap`). Zerodha also enforces broker-side limits.

Free plan users cannot attach live Zerodha credentials (`LiveAllowed=false`). Paper simulation uses paper accounts, not Zerodha demo routing.

---

## 9. Common Error Codes

| `error_code` | Likely cause | Action |
|-------------|--------------|--------|
| `auth_failed` | Expired access_token (daily rotation) | Regenerate token → PUT credentials |
| `algo_id_required` | `SEBI_ALGO_ID_REQUIRED=true` and no algo_id | Set `algo_id` on credential or env fallback |
| `invalid_credentials` | Wrong format or non-existent API key | Check `api_key:access_token` format |
| `rate_limited` | >10 OPS to Zerodha | Reduce signal frequency |

---

## 10. Broker Documentation

- [Kite Connect API docs](https://kite.trade/docs/connect/v3/)
- [Kite Connect developer console](https://developers.kite.trade/)
- [SEBI Algo-ID tag field — Kite forum](https://kite.trade/forum/discussion/15924)
