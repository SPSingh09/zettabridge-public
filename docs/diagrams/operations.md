# Operations & Plans Diagrams

**Last updated:** July 2026

Retry behaviour, deduplication, and plan limit overview.

[← Back to diagram index](README.md)

---

## 1. Deduplication & retry behaviour

409 means the duplicate was suppressed — the original signal may still be processing.

```mermaid
flowchart TD
    POST["POST /v1/webhook/{token}"]
    DEDUP{"Within dedup window?<br/>default 2s"}
    DUP["409 — duplicate suppressed<br/>do not retry"]
    PROC["Process signal"]
    RESP{"HTTP status"}

    POST --> DEDUP
    DEDUP -->|yes, same fingerprint| DUP
    DEDUP -->|no| PROC
    PROC --> RESP

    RESP --> S202["202 — accepted, check dashboard"]
    RESP --> S400["400 — fix payload"]
    RESP --> S401["401 — fix token / comment"]
    RESP --> S403["403 — fix config / hours / action"]
    RESP --> S429["429 — backoff, 60/min per webhook"]
    RESP --> S503["503 — retry shortly, queue full"]
```

| HTTP | Retry? | Notes |
|------|--------|-------|
| **202** | No | Accepted — check dashboard for final trade status |
| **400** | Fix payload | Malformed JSON or missing `action` |
| **401** | Fix token / comment | Invalid token or `required_comment` mismatch |
| **403** | Fix config | Paused webhook, action not allowed, outside hours |
| **409** | No | Duplicate suppressed within dedup window (default **2s**) |
| **429** | Yes + backoff | **60/min** per webhook; free plan **100/day** |
| **503** | Yes + delay | Queue full — retry shortly |

**Defaults**

- Dedup window: **2 seconds**
- Per-webhook rate limit: **60 requests/minute**
- Free plan daily ingest cap: **100 requests/day**

---

## 2. Plan limits overview

Live broker workflows require eligible plan + closed-beta approval. Per broker configuration cap: **10 orders/sec**.

```mermaid
flowchart TB
    subgraph Free["Free"]
        F1["Paper: 1 account · 1 webhook"]
        F2["Live: —"]
        F3["100 ingests/day · 1 order/sec"]
    end

    subgraph Paper["Paper"]
        P1["Paper: 3 accounts · 5 webhooks"]
        P2["Live: —"]
        P3["5 orders/sec · fair-use ingests"]
    end

    subgraph Pro["Pro"]
        R1["Paper: 5 accounts · 8 webhooks"]
        R2["Live: 1 live webhook · 1 broker config"]
        R3["10 orders/sec · closed beta live"]
    end

    subgraph ProPlus["Pro Plus"]
        PP1["Paper: 10 accounts · 15 webhooks"]
        PP2["Live: 5 live webhooks · 3 broker configs"]
        PP3["10 orders/sec · multi-product where enabled"]
    end
```

| Plan | Paper | Live | Limits |
|------|-------|------|--------|
| **Free** | 1 account · 1 webhook | — | 100 ingests/day · 1 order/sec |
| **Paper** | 3 accounts · 5 webhooks | — | 5 orders/sec · fair-use ingests |
| **Pro** | 5 accounts · 8 webhooks | 1 live webhook · 1 broker config | 10 orders/sec · closed beta live |
| **Pro Plus** | 10 accounts · 15 webhooks | 5 live webhooks · 3 broker configs | 10 orders/sec · multi-product where enabled |

**Notes**

- Live Zerodha Publisher requires **Pro** or **Pro Plus** plus closed-beta approval.
- Plan gates are enforced at webhook creation and at ingest routing.
- See [payments.md](../04-integrations/payments.md) for billing detail.
