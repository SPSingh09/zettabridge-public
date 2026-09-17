# SEBI Algo-ID Compliance
**Last updated: 2026-07-07**

Reference: NSE circular [INVG67858](https://nsearchives.nseindia.com/content/circulars/INVG67858.pdf)

---

## 1. Regulatory Requirement

From August 2025, SEBI/NSE require all API-originated algorithmic orders on NSE/BSE to carry an **exchange-assigned unique identifier** (Algo ID). This applies to:

- All API clients placing orders via broker APIs (Kite Connect, SmartAPI, DhanHQ, etc.)
- Both buy and sell sides of every order
- All product types: MIS, CNC, NRML

ZettaBridge is an API bridge: every webhook-triggered NSE/BSE order is an algo order in regulator terms. Live Indian broker execution without Algo-ID tagging is a compliance violation.

---

## 2. Who Needs an Algo ID

| Scenario | Requirement |
|----------|-------------|
| API client placing ≤10 orders/sec | Generic Algo ID from broker/exchange |
| API client placing >10 orders/sec | Unique registered Algo ID (requires registration as Algo Provider) |

**Most ZettaBridge users fall in the ≤10 OPS category** and can obtain a generic ID from their broker.

---

## 3. Obtaining an Algo ID

Each Indian broker has its own process:

### Zerodha (Kite Connect)

- Register via the Kite Connect developer console or contact Kite support.
- Generic ID available for retail clients (≤10 OPS).
- The ID is sent as the `tag` field on every order (max 20 alphanumeric characters).
- Docs: [Kite Connect `tag` parameter](https://kite.trade/docs/connect/v3/orders/#placing-orders)

### Angel One (SmartAPI)

- Register your algo with Angel One / NSE exchange via the SmartAPI portal.
- Contact SmartAPI support if a generic ID is not automatically issued.
- The ID is sent as the `ordertag` field on every order (max 20 alphanumeric characters).

### Dhan

- Contact Dhan support to obtain a generic Algo ID for ≤10 OPS usage.
- For >10 OPS, register as an Algo Provider with Dhan/exchange to receive a unique exchange-assigned ID.
- The ID is sent as the `correlationId` field on every order (max 30 alphanumeric characters).
- Confirmed by Dhan support: `correlationId` is the correct request field for SEBI compliance.

---

## 4. Configuring Algo-ID in ZettaBridge

### Per-credential (recommended)

Set `algo_id` when creating or updating a broker credential:

```http
POST /v1/credentials
Authorization: Bearer <jwt>
Content-Type: application/json

{
  "broker_type": "zerodha",
  "raw_creds": "api_key:access_token",
  "account_mode": "live",
  "algo_id": "YOURALGO123",
  "exchange": "NSE",
  "product": "MIS"
}
```

Or update an existing credential:

```http
PUT /v1/credentials/:id
Authorization: Bearer <jwt>
Content-Type: application/json

{"algo_id": "YOURALGO123"}
```

The `algo_id` is returned in credential GET responses and is visible in the dashboard.

### Platform-level fallback (staging/pilot)

Set environment variables for a platform-wide default:

```bash
BROKER_ZERODHA_ALGO_ID=STAGINGALGO123
BROKER_ANGEL_ALGO_ID=STAGINGALGO456
BROKER_DHAN_ALGO_ID=STAGINGALGO789
```

Precedence: credential-level `algo_id` → env fallback → reject (if enforcement on).

---

## 5. Enforcement Behaviour

Controlled by `SEBI_ALGO_ID_REQUIRED` env var:

| Setting | When active | Behaviour |
|---------|------------|-----------|
| `true` (default when `BROKER_MODE=live`) | Live Indian broker orders | Reject trade with `error_code=algo_id_required` if no Algo ID resolvable |
| `false` | Dev/CI/staging without IDs | Orders placed without the tag (non-compliant; only for testing) |

Enforcement applies to `broker_type` in `{zerodha, angel, dhan}` on **live broker credentials** when `BROKER_MODE=live`. MT5/forex and **paper webhooks** are not subject to Indian Algo-ID tagging.

### Fail-close trade flow

```
Signal ingest → guard check → queue → maporder.Build
  → algo.Resolve(credential, config)
     ├── paper webhook destination → skip (paper engine)
     ├── broker_type: mt5_cloud → skip (return "")
     ├── cred.algo_id set → use it
     ├── env fallback set → use it
     └── SEBI_ALGO_ID_REQUIRED=true → reject with error_code=algo_id_required
  → PlaceOrder with AlgoID set → broker order placed with tag
  → trade row saved with algo_id snapshot
```

---

## 6. Audit Trail

The `trades` table records the `algo_id` used at execution time:

```sql
SELECT id, signal, symbol, broker_order, algo_id, created_at
FROM trades
WHERE user_id = $1
  AND status IN ('placed', 'filled')
ORDER BY created_at DESC;
```

`trades.algo_id` is a snapshot — it reflects the ID used when the order was placed, independent of any subsequent credential updates.

---

## 7. Pre-Live Checklist

Before flipping `BROKER_MODE=live` in production:

- [ ] Obtained Algo ID from each broker used (Zerodha, Angel, Dhan)
- [ ] Set `algo_id` on all live credentials
- [ ] Confirmed with `POST /v1/credentials/:id/verify` that credentials pass broker validation
- [ ] Set `SEBI_ALGO_ID_REQUIRED=true` in environment
- [ ] Placed one sandbox order per broker and verified the algo tag appears in the broker order book
- [ ] NAT Gateway Elastic IP whitelisted with all broker portals (required for Dhan; recommended for Angel)

---

## 8. Scope Limitation

ZettaBridge stores and forwards the Algo ID provided by the operator — it does not synthesize, validate, or register IDs on behalf of users. Each user is responsible for:

- Registering with their broker to obtain a valid Algo ID
- Ensuring their OPS rate does not exceed their registration tier without re-registering
- Compliance with any other SEBI/exchange requirements applicable to their trading activity

ZettaBridge is a technical execution bridge. It is not a registered investment advisor, broker, or SEBI-registered entity.
