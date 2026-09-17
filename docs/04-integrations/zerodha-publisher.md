# Zerodha Kite Publisher Integration
**Last updated: 2026-07-07**

**Status:** **Implemented** when `ZERODHA_PUBLISHER_ENABLED=true`. Callback on Core (`GET /v1/publisher/callback`) when `EXECUTION_ADAPTER_MODE=local`; on Zerodha adapter when `http`. Requires Pro/Pro Plus live webhook + publisher execution mode credential.

---

## What is Kite Publisher?

Kite Publisher is Zerodha's hosted order form. Instead of placing orders via the Kite Connect REST API (which requires SEBI Algo-ID registration as an algorithmic strategy), users can be redirected to a Kite-hosted page where they manually confirm the order. This sidesteps the algo-trading compliance requirement for retail users.

## Flow

```
ZettaBridge dashboard          Kite Publisher (Zerodha)
       │                                │
       │  POST /v1/publisher/order      │
       │──────────────────────────────> │  (backend creates record,
       │  ← { redirect_url, order_id }  │   builds Kite URL with params)
       │                                │
       │  browser redirect ──────────────────────────────────────────> kite.zerodha.com
       │                                                                       │
       │                                                         user reviews │
       │                                                         and confirms │
       │                                                                       │
       │  GET /v1/publisher/callback?status=success&order_id=...&webhook_id=... │
       │ <─────────────────────────────────────────────────────────────────────┘
       │
       │  GET /v1/publisher/orders/:id  (poll for final status)
```

## Planned API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/v1/publisher/order` | JWT | Create publisher order, returns Kite redirect URL |
| GET | `/v1/publisher/orders/:id` | JWT | Poll status of a publisher order |
| GET | `/v1/publisher/callback` | Public | Kite redirects here after user action |

## Publisher Order Params

These are forwarded to Kite's order form URL:

| Field | Values | Notes |
|-------|--------|-------|
| `variety` | `regular`, `co`, `amo` | Order variety |
| `exchange` | `NSE`, `BSE`, `NFO`, `MCX` | Exchange |
| `tradingsymbol` | e.g. `RELIANCE` | Instrument symbol |
| `transaction_type` | `BUY`, `SELL` | Direction |
| `order_type` | `MARKET`, `LIMIT`, `SL`, `SL-M` | Order type |
| `product` | `CNC`, `MIS`, `NRML` | Product code |
| `quantity` | integer | Number of units |
| `price` | decimal | Required for LIMIT orders |
| `trigger_price` | decimal | Required for SL / SL-M orders |

## Callback Query Params

Zerodha appends these when redirecting back to `/v1/publisher/callback`:

| Param | Values | Notes |
|-------|--------|-------|
| `status` | `success`, `cancelled` | Whether user confirmed or cancelled |
| `order_id` | Zerodha order ID string | Only present on `success` |
| `webhook_id` | ZettaBridge webhook UUID | Passed through from initial request |

## Prerequisites

- Webhook must have a live Zerodha credential (`account_mode=live`)
- Zerodha access token must be from today's login (expires daily at 6:00 AM IST)
- No SEBI Algo-ID required (Kite Publisher is a manual order flow, not algorithmic)

## Bruno Collection

See `bruno/ZettaBridge/Zerodha Publisher/` for request stubs covering all four endpoints.

## Constraint

Kite Publisher is a browser-redirect flow and cannot be used from server-side signals (TradingView webhooks). It is intended for manual order entry from the ZettaBridge dashboard.
