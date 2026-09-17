# Dhan Integration Guide
**Last updated: 2026-07-07**

> **Deployment note:** Live broker code exists in `livebrokers/dhan.go`. Dhan is **not** in default ECS `enabled_adapters` (`paper,zerodha` only). Enable in Terraform before production use.

---

## 1. Credential Format

ZettaBridge credential `raw_creds` for Dhan:

```
client_id:access_token
```

| Field | Where to find | Notes |
|-------|--------------|-------|
| `client_id` | Dhan account / developer portal | Stable |
| `access_token` | [web.dhan.co](https://web.dhan.co) → API token section | Typically ~24-hour TTL |

**Example (never store real values):**

```json
{
  "broker_type": "dhan",
  "raw_creds": "client_id_here:access_token_here",
  "account_mode": "live",
  "algo_id": "DHANALGOID",
  "exchange": "NSE",
  "product": "MIS"
}
```

---

## 2. API Access Requirements

Dhan requires explicit API / algo access to be enabled on the account before orders can be placed programmatically.

1. Log in to [web.dhan.co](https://web.dhan.co).
2. Navigate to the API section and enable API trading access.
3. Whitelist the ZettaBridge NAT Elastic IP in the Dhan developer portal (see section 4 — this is required).
4. Obtain the access token for use in `raw_creds`.

---

## 3. Token Refresh (Manual)

Dhan access tokens have a ~24-hour TTL. ZettaBridge does **not** refresh these automatically.

| When | Symptom | Action |
|------|---------|--------|
| Token expiry | `auth_failed`, `error_code=auth_failed` on trades | Generate a new access token from Dhan portal → `PUT /v1/credentials/:id` → verify |

After updating, run `POST /v1/credentials/:id/verify` before relying on webhooks.

---

## 4. IP Whitelisting (Required)

Dhan **requires** the client's public egress IP to be whitelisted in the developer portal. Orders from non-whitelisted IPs are rejected.

- Add the ZettaBridge NAT Gateway Elastic IP to the Dhan developer portal → IP whitelist.
- Verify current egress IP: `curl -s https://checkip.amazonaws.com` from a VPC ECS task.
- After any infra change that recreates the NAT Gateway (Elastic IP changes), update the whitelist immediately.

---

## 5. SEBI Algo-ID Requirement

SEBI/NSE requires all API-originated algo orders on NSE/BSE to carry an exchange-assigned Algo ID.

**Confirmed by Dhan support:** the correct request field for passing the SEBI Algo ID on Dhan orders is `correlationId`.

| Dhan field | Max length | Charset | Notes |
|-----------|------------|---------|-------|
| `correlationId` | 30 chars | Alphanumeric | Doubles as SEBI Algo ID for compliance; also used as user tracking |

**Obtaining Algo ID from Dhan:**
- For **≤10 OPS** (typical retail): a generic Algo ID is available via Dhan support. Contact support with your client ID and API access details to obtain it.
- For **>10 OPS**: register as an Algo Provider with Dhan/exchange to receive a unique exchange-assigned ID.

**ZettaBridge config:**

- Set `algo_id` on the credential: `PUT /v1/credentials/:id` → `{"algo_id": "DHANALGOID"}`
- Or set env fallback `BROKER_DHAN_ALGO_ID` for staging/pilot
- When `SEBI_ALGO_ID_REQUIRED=true` (default in live mode), missing ID rejects the trade with `error_code=algo_id_required`

ZettaBridge sends the `algo_id` value as `correlationId` on every live Dhan order.

---

## 6. Exchange and Product Settings

| Field | Values | Default |
|-------|--------|---------|
| `exchange` | `NSE`, `BSE` | `NSE` |
| `product` | `INTRADAY`, `CNC` | `INTRADAY` |
| `order_type` | `MARKET`, `LIMIT` | `MARKET` |

---

## 7. Symbol Conventions

Use NSE/BSE equity ticker symbols:

| Signal payload | Placed on |
|----------------|-----------|
| `"symbol": "RELIANCE"` | RELIANCE on NSE |
| `"symbol": "SBIN"` | SBIN on NSE |
| `"symbol": "EURUSD"` | ❌ Rejected — Dhan cred does not support forex |

---

## 8. Common Error Codes

| `error_code` | Likely cause | Action |
|-------------|--------------|--------|
| `auth_failed` | Expired access_token or unwhitelisted IP | Refresh token; verify IP whitelist in Dhan portal |
| `algo_id_required` | `SEBI_ALGO_ID_REQUIRED=true` and no algo_id | Set `algo_id` on credential or obtain from Dhan support |
| `invalid_credentials` | Wrong format | Check `client_id:access_token` format |
| `rate_limited` | Broker API rate limit | Reduce signal frequency |

---

## 9. Broker Documentation

- [DhanHQ API v2 documentation](https://dhanhq.co/docs/v2/)
- [DhanHQ web portal](https://web.dhan.co)
