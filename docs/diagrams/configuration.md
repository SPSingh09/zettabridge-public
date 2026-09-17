# Configuration & Onboarding Diagrams

**Last updated:** July 2026

Setup order, webhook model, payload rules, and supported workflows.

[← Back to diagram index](README.md)

---

## 1. First-time setup map

Recommended: validate with paper trading before enabling a live Publisher webhook.

```mermaid
flowchart TD
    S1["1. Create ZettaBridge account"]
    S2["2. Create paper account OR Zerodha broker configuration"]
    S3["3. Create webhook — link destination + set defaults"]
    S4["4. Copy webhook URL into TradingView / script"]
    S5["5. Send test signal → check logs (202)"]
    S6["6. Publisher only: confirm on Zerodha via dashboard"]

    S1 --> S2 --> S3 --> S4 --> S5 --> S6
```

**Checklist**

1. Create ZettaBridge account
2. Create paper account **or** Zerodha broker configuration
3. Create webhook — link destination + set defaults
4. Copy webhook URL into TradingView / script / bot
5. Send test signal → verify **202** in logs
6. **Publisher only:** confirm on Zerodha via dashboard

---

## 2. Webhook configuration model

Each webhook links to exactly one destination. One webhook URL = one token.

```mermaid
flowchart TB
    ACCT["User account"]
    WH["Webhook<br/>token · defaults · guards"]
    DEST{"Exactly one destination"}
    PAPER["Paper account<br/>paper webhook"]
    BROKER["Broker configuration<br/>live Publisher webhook"]

    ACCT --> WH
    WH --> DEST
    DEST --> PAPER
    DEST --> BROKER

    DEF["Defaults: symbol · lot · product · order_type<br/>allowed_actions · required_comment · dedup · rate limits"]
    WH -.-> DEF
```

**Configuration fields (defaults & guards)**

| Category | Examples |
|----------|----------|
| Identity | Hashed token, webhook name |
| Destination | Paper account ID **or** broker configuration ID |
| Defaults | `symbol`, `lot`, `product`, `order_type` |
| Guards | `allowed_actions`, `required_comment`, trading hours |
| Limits | Dedup window, per-webhook rate limit |

---

## 3. Signal payload resolution

Only `action` is always required at ingest. Other fields fall back to webhook defaults.

```mermaid
flowchart LR
    SIG["Incoming JSON"] --> ACT{"action present?"}
    ACT -->|no| E400["400 at ingest"]
    ACT -->|yes| LOT{"lot resolution"}
    LOT -->|"default lot > 0"| USE_DEF["Use webhook default lot"]
    LOT -->|"default lot = 0"| NEED["Signal must send positive lot"]
    USE_DEF --> SYM["symbol · comment · price checks"]
    NEED --> SYM
    SYM --> Q202["202 Accepted → async processing"]
    SYM --> E401["401 if required_comment mismatch"]
    SYM --> E403["403 if action/hours not allowed"]
```

**Field rules**

| Field | Rule |
|-------|------|
| `action` | **Required.** `BUY` / `SELL` / `CLOSE`. Publisher webhooks: `BUY` & `SELL` only at config level. |
| `lot` | If webhook default `lot > 0`, default wins. If default is `0`, signal must send positive `lot`. |
| `symbol` | When provided, must match allowed symbol list. May reject **after** 202. |
| `comment` | When `required_comment` is set, must match exactly — else **401** at ingest. |
| `price` | Omit or `0` for market-style orders, subject to webhook settings. |

---

## 4. Supported workflows matrix

Zerodha OAuth and Kite Connect are **not** available on the public platform today.

| Workflow | Auth | User action | Status |
|----------|------|-------------|--------|
| Paper engine | None | None | **Available** |
| Zerodha Kite Publisher | No OAuth session stored for Publisher handoff | Confirm on Zerodha | **Closed beta** |

```mermaid
flowchart LR
    subgraph Available["Available today"]
        P["Paper engine<br/>No broker credentials"]
    end

    subgraph Beta["Closed beta"]
        Z["Zerodha Kite Publisher<br/>User confirms on Zerodha"]
    end

    WH["Webhook ingest<br/>POST → 202"] --> P
    WH --> Z

    style Available fill:#e8f5e9,stroke:#4caf50
    style Beta fill:#e3f2fd,stroke:#2196f3
```

**Out of scope (public platform today)**

- Kite Connect OAuth live order flow
- Angel One, Dhan, MT5 live adapters (in repo, not publicly deployed)
- Discretionary trading or investment advice
