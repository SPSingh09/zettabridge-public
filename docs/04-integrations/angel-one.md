# Angel One (SmartAPI) Integration Guide
**Last updated: 2026-07-07**

> **Deployment note:** Live broker code exists in `livebrokers/angel.go`. Angel is **not** in default ECS `enabled_adapters` (`paper,zerodha` only). Enable in Terraform and provision an adapter before production use.

---

## 1. Credential Format

ZettaBridge credential `raw_creds` for Angel One:

```
api_key:client_code:jwt
```

| Field | Where to find | Notes |
|-------|--------------|-------|
| `api_key` | SmartAPI developer portal → app → API key | Stable |
| `client_code` | Angel One client/login ID | Stable |
| `jwt` | Session JWT from `POST https://apiconnect.angelone.in/rest/auth/angelbroking/user/v1/loginByPassword` | Session-based TTL (hours) |

**Example (never store real values):**

```json
{
  "broker_type": "angel",
  "raw_creds": "api_key_here:CLIENT001:jwt_token_here",
  "account_mode": "live",
  "algo_id": "ALGO12345",
  "exchange": "NSE",
  "product": "MIS"
}
```

---

## 2. Token Refresh (Manual)

Angel One SmartAPI JWT tokens are session-based and expire after a few hours. ZettaBridge does **not** refresh these automatically.

| When | Symptom | Action |
|------|---------|--------|
| JWT expiry | `auth_failed`, `error_code=auth_failed` on trades | Re-login to SmartAPI, obtain new JWT → `PUT /v1/credentials/:id` → verify |
| Login flow | SmartAPI `loginByPassword` API | Returns `jwtToken`; use that value in `raw_creds` |

After updating, run `POST /v1/credentials/:id/verify` before relying on webhooks.

---

## 3. SEBI Algo-ID Requirement

SEBI/NSE requires all API-originated algo orders on NSE/BSE to carry an exchange-assigned Algo ID.

| SmartAPI field | Max length | Charset |
|---------------|------------|---------|
| `ordertag` | 20 chars | Alphanumeric |

**For ≤10 OPS (typical retail):** register your algo with Angel One / NSE exchange to obtain a generic ID.

**ZettaBridge config:**

- Set `algo_id` on the credential: `PUT /v1/credentials/:id` → `{"algo_id": "ALGO12345"}`
- Or set env fallback `BROKER_ANGEL_ALGO_ID` for staging/pilot
- When `SEBI_ALGO_ID_REQUIRED=true` (default in live mode), missing ID rejects the trade with `error_code=algo_id_required`

ZettaBridge sends the `algo_id` value as the SmartAPI `ordertag` field on every live NSE/BSE order.

---

## 4. IP Whitelisting

Angel One SmartAPI requires the client's public IP to be registered in the app settings.

- Add the ZettaBridge NAT Gateway Elastic IP to the SmartAPI app's IP whitelist.
- Set the corresponding env vars so the Angel adapter can include the IP in order headers:
  - `BROKER_ANGEL_CLIENT_PUBLIC_IP` — the NAT Elastic IP (same as egress IP)
  - `BROKER_ANGEL_CLIENT_LOCAL_IP` — the ECS task local IP (typically same as the NAT IP in this context; check with `curl checkip.amazonaws.com`)
- Verify: `curl -s https://checkip.amazonaws.com` from a VPC task should show the same IP registered in SmartAPI.

Orders from an unwhitelisted IP are rejected by Angel One with an auth error.

---

## 5. Exchange and Product Settings

| Field | Values | Default |
|-------|--------|---------|
| `exchange` | `NSE`, `BSE` | `NSE` |
| `product` | `MIS` (intraday), `CNC` (delivery), `NRML` (F&O) | `MIS` |
| `order_type` | `MARKET`, `LIMIT` | `MARKET` |

---

## 6. Symbol Conventions

Use NSE/BSE equity ticker symbols:

| Signal payload | Placed on |
|----------------|-----------|
| `"symbol": "RELIANCE"` | RELIANCE on NSE |
| `"symbol": "INFY"` | INFY on NSE |
| `"symbol": "EURUSD"` | ❌ Rejected — Angel cred does not support forex |

---

## 7. Common Error Codes

| `error_code` | Likely cause | Action |
|-------------|--------------|--------|
| `auth_failed` | Expired SmartAPI JWT | Re-login SmartAPI → PUT credentials |
| `algo_id_required` | `SEBI_ALGO_ID_REQUIRED=true` and no algo_id | Set `algo_id` on credential or env fallback |
| `invalid_credentials` | Wrong format or wrong client_code/JWT | Check `api_key:client_code:jwt` format |
| `rate_limited` | Broker API rate limit exceeded | Reduce signal frequency |

---

## 8. Broker Documentation

- [SmartAPI documentation](https://smartapi.angelone.in/docs/)
- [SmartAPI developer portal](https://smartapi.angelone.in/)
