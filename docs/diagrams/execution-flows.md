# Execution Flow Diagrams

**Last updated:** July 2026

How signals move from ingest to paper or Zerodha Kite Publisher outcomes.

[← Back to diagram index](README.md)

---

## 1. Webhook ingest sequence

Synchronous validation at ingest. Processing continues asynchronously after HTTP 202.

```mermaid
flowchart TB
    SRC["Signal source<br/>TradingView · Python · AI bot"]
    TOKEN["Token lookup<br/>Hash match · webhook config"]
    VALID["Hot-path validation<br/>action · comment · hours · rate limit"]
    RESP{"Ingest response"}
    OK["202 Accepted"]
    ERR["400 / 401 / 403"]
    DEDUP["409 dedup"]
    LIMIT["429 / 503"]
    WORKER["Worker processing<br/>symbol · qty · workflow routing"]
    OUT["Dashboard outcome<br/>queued → filled / rejected / pending_confirmation"]

    SRC -->|"POST /v1/webhook/{token}"| TOKEN
    TOKEN --> VALID
    VALID -->|"dedup check"| RESP
    RESP --> OK
    RESP --> ERR
    RESP --> DEDUP
    RESP --> LIMIT
    OK -->|"enqueue job"| WORKER
    WORKER --> OUT
```

**Notes**

- Ingest is **not** synchronous order execution — 202 means accepted and queued.
- Rejections at ingest return 4xx immediately; some validation (e.g. symbol) may fail **after** 202.
- Save `request_id` from the 202 response for support and dashboard correlation.

---

## 2. Paper vs Zerodha Kite Publisher

Same webhook ingest path. Destination is determined by **webhook configuration** — not the signal payload.

```mermaid
flowchart TB
    IN["POST webhook<br/>202 + request_id"]
    ROUTE{"Route by webhook destination"}

    subgraph Paper["Paper webhook"]
        PE["Paper engine<br/>Simulated balance & positions"]
        PO["Paper order / fill"]
        PS["filled / rejected"]
        PE --> PO --> PS
    end

    subgraph Publisher["Publisher webhook"]
        VQ["Validate & queue"]
        PC["pending_confirmation"]
        HAND["Dashboard handoff<br/>no redirect in 202 response"]
        CONF["User confirms on Zerodha"]
        PO2["submitted / filled / rejected"]
        VQ --> PC --> HAND --> CONF --> PO2
    end

    IN --> ROUTE
    ROUTE -->|paper webhook| PE
    ROUTE -->|Publisher webhook| VQ
```

**Notes**

- One webhook URL links to exactly one destination (paper account **or** broker configuration).
- Publisher never returns a Zerodha redirect URL in the webhook HTTP response.
- Paper outcomes resolve inside ZettaBridge; Publisher requires user action on Zerodha.

---

## 3. Kite Publisher confirmation flow

ZettaBridge never stores your Zerodha login password. Live orders require your confirmation on Zerodha.

```mermaid
sequenceDiagram
    participant S as Signal source
    participant Z as ZettaBridge
    participant U as User

    S->>Z: 1. POST JSON to webhook URL
    Z->>S: 2. Validate token & payload → HTTP 202 + request_id
    Z->>Z: 3. Queue job · trade status → queued
    Z->>Z: 4. Prepare Publisher handoff · pending_confirmation
    U->>Z: 5. Open trade in dashboard
    U->>U: 6. Review & confirm order on Zerodha
    Z->>Z: 7. Record submitted / filled / rejected in audit log
```

**Step reference**

| Step | Actor | Action |
|------|-------|--------|
| 1 | Signal source | POST JSON to webhook URL |
| 2 | ZettaBridge | Validate token & payload → HTTP 202 + `request_id` |
| 3 | ZettaBridge | Queue job · trade status → `queued` |
| 4 | ZettaBridge | Prepare Publisher handoff · `pending_confirmation` |
| 5 | User | Open trade in dashboard |
| 6 | User | Review & confirm order on Zerodha |
| 7 | ZettaBridge | Record `submitted` / `filled` / `rejected` in audit log |

---

## 4. Trade status lifecycle

Some rejections occur after HTTP 202 — check `error_code` on rejected trades.

```mermaid
stateDiagram-v2
    [*] --> queued: HTTP 202 accepted

    queued --> pending_confirmation: Publisher workflow
    queued --> filled: Paper fill
    queued --> rejected: Validation / workflow error
    queued --> cancelled: User or system

    pending_confirmation --> submitted: User confirmed on Zerodha
    pending_confirmation --> rejected
    pending_confirmation --> cancelled

    submitted --> filled
    submitted --> rejected

    filled --> [*]
    rejected --> [*]
    cancelled --> [*]
```

**Status reference**

| Status | Meaning |
|--------|---------|
| `queued` | Signal accepted & queued |
| `pending_confirmation` | Publisher only — awaiting user on Zerodha |
| `submitted` | Submission recorded |
| `filled` | Paper fill or broker confirmation |
| `rejected` | Validation or workflow error |
| `cancelled` | User or system cancelled |

**Typical Publisher path:** `queued` → `pending_confirmation` → `submitted` → `filled` (or `rejected` at any stage).

Poll status via **`GET /v1/trades/{id}`**.
