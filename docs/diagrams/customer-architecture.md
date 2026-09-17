# Customer Architecture Diagrams

**Last updated:** July 2026

Homepage-style architecture and onboarding flows — customer-facing view of the execution stack.

[← Back to diagram index](README.md)

---

## 1. Enterprise execution stack

Every signal traverses a hardened infrastructure path — from Cloudflare at the edge to the audit database at the end.

Internal validation and routing optimized for low latency. Broker, exchange, and user-confirmation latency may vary.

```mermaid
flowchart TB
    subgraph Inbound["Inbound path"]
        SRC["Signal Source<br/>TradingView · AI Bot · Python"]
        CF["Cloudflare<br/>DDoS · WAF · TLS termination"]
        API["API Gateway<br/>Auth · Rate-limit · Webhook validation"]
        ENG["Execution Engine<br/>Dedup · Algo-ID · Order routing"]
    end

    subgraph Cache["State"]
        REDIS["Redis Cache<br/>Dedup window · Session state"]
    end

    subgraph Outbound["Outbound path"]
        BROKER["Broker Order Flow<br/>Paper engine · Zerodha Kite Publisher"]
        MARKET["Market access<br/>Paper sim · Zerodha user-confirmed"]
        AUDIT["Audit Database<br/>RDS PostgreSQL · Every event logged"]
    end

    SRC --> CF --> API --> ENG
    ENG <--> REDIS
    ENG --> BROKER --> MARKET
    BROKER --> AUDIT
    ENG --> AUDIT

    ZB["ZettaBridge<br/>Execution Engine"]
    ENG -.-> ZB
```

**Stack layers**

| Layer | Description |
|-------|-------------|
| Signal Source | TradingView · AI Bot · Python |
| Cloudflare | DDoS · WAF · TLS termination |
| API Gateway | Auth · Rate-limit · Webhook validation |
| Execution Engine | Dedup · Algo-ID · Order routing |
| Redis Cache | Dedup window · Session state |
| Broker Order Flow | Paper engine · Zerodha Kite Publisher |
| Market access | Paper sim · Zerodha (user-confirmed) |
| Audit Database | RDS PostgreSQL · Every event logged |

**Key metrics (customer-facing)**

| Metric | Value |
|--------|-------|
| Internal processing | Fast (async after 202) |
| Credential encryption | AES-256-GCM |
| Default dedup window | 2 s |

For deployment topology (ECS, adapters, NAT), see [architecture-diagrams.md](../architecture-diagrams.md).

---

## 2. Three steps: signal to outcome

End-user journey from webhook setup to paper fill or Zerodha Publisher confirmation.

```mermaid
flowchart LR
    subgraph Step1["01 — Set up your webhook"]
        SETUP["Create paper account or Zerodha broker configuration"]
        WH["Create webhook in dashboard"]
        COPY["Copy URL into TradingView / bot / script"]
        SETUP --> WH --> COPY
    end

    subgraph Step2["02 — Signal received HTTP 202"]
        FIRE["Alert fires · POST /v1/webhook/{token}"]
        VAL["ZettaBridge validates payload"]
        ACC["202 Accepted + request_id · queued"]
        FIRE --> VAL --> ACC
    end

    subgraph Step3["03 — Paper fill or Zerodha confirmation"]
        PAPER["Paper: filled / rejected"]
        PUB["Publisher: pending_confirmation"]
        CONF["User confirms on Zerodha"]
        FINAL["submitted / filled / rejected"]
        PAPER
        PUB --> CONF --> FINAL
    end

    Step1 --> Step2 --> Step3
```

**Step detail**

### 01 — Set up your webhook

Create a paper account or Zerodha broker configuration, then create a webhook in the dashboard. Copy the URL into TradingView, your AI bot, or a Python script.

```json
POST /v1/webhook/{token}
{
  "action": "BUY",
  "symbol": "RELIANCE",
  "lot": 1
}
```

### 02 — Signal received — HTTP 202

When your alert fires, ZettaBridge validates the payload and returns **202 Accepted** with a `request_id`. The signal is queued for processing — not executed synchronously in the response.

```json
HTTP 202 Accepted
{
  "status": "accepted",
  "request_id": "req_...",
  "queued": true
}
```

### 03 — Paper fill or Zerodha confirmation

Paper webhooks update your paper account. Zerodha Kite Publisher webhooks move to `pending_confirmation` — you complete the order on Zerodha via the dashboard. Full audit trail is logged.

```
Paper:        filled / rejected
Publisher:    pending_confirmation → user confirms on Zerodha
```

---

## Sequence view (all three steps)

```mermaid
sequenceDiagram
    participant U as User
    participant TV as Signal source
    participant Z as ZettaBridge
    participant ZD as Zerodha

    U->>Z: Configure webhook + destination
    U->>TV: Paste webhook URL
    TV->>Z: POST signal
    Z->>TV: 202 Accepted + request_id

    alt Paper webhook
        Z->>Z: Paper engine fill
        Z->>U: Dashboard: filled / rejected
    else Publisher webhook
        Z->>U: Dashboard: pending_confirmation
        U->>ZD: Confirm order on Zerodha
        ZD->>Z: Outcome callback / status
        Z->>U: Dashboard: submitted / filled / rejected
    end
```
