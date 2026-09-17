# ZettaBridge Architecture Diagrams

**Exported:** 2026-07-07  
**Scope:** Staging — **paper + Zerodha live** via HTTP adapters. Angel/Dhan/MT5/FYERS live broker **not deployed**.  
**Source:** [STATUS.md](STATUS.md) + current implementation (Phase 0, adapter split partial).

---

## How to export as PNG / SVG

These diagrams use [Mermaid](https://mermaid.js.org/). To render or export:

| Method | Steps |
|--------|--------|
| **GitHub / GitLab** | Push this file — Mermaid renders in the markdown preview |
| **VS Code / Cursor** | Install “Markdown Preview Mermaid Support”, open preview |
| **mermaid.live** | Copy a ` ```mermaid ` block → paste at [mermaid.live](https://mermaid.live) → Export PNG/SVG |
| **CLI** | `npx @mermaid-js/mermaid-cli -i diagram.mmd -o diagram.png` |

---

## 1. Logical overview (two service classes)

High-level split: Core vs Execution Adapters vs market-data on Core.

```mermaid
flowchart TB
    subgraph Core["ZettaBridge Core"]
        WH[Webhook ingest]
        CR[Credential config API]
        RR[Risk / guards / plan / algo-id]
        ON[Order normalizer]
        ER[Execution router / orchestrator]
        AU[Trades / audit / WS / dashboard]
        PE[Paper engine — cohosted on staging]
    end

    subgraph Deployed["Execution adapters — deployed day 0"]
        ZA[Zerodha Adapter ECS]
    end

    subgraph Future["In repo, not deployed"]
        AA[Angel Adapter]
        DA[Dhan Adapter]
        MA[MT5 Adapter]
        FYL[FYERS live broker adapter — Phase 5]
    end

    subgraph CoreMarketData["Core — market data not split"]
        FY[FYERS LTP OAuth + SnapshotJob]
    end

    TV[TradingView] --> WH
    WH --> RR --> ON --> ER
    CR -. encrypted creds in PG .-> ER
    ER --> PE
    ER --> ZA
    ER -. disabled .-> AA & DA & MA & FYL
    PE & ZA -->|ExecutionOutcome| AU
    ZA -->|async status| AU
    FY -. LTP for paper snapshots .-> PE

    style Future fill:#f5f5f5,stroke:#999,stroke-dasharray: 5 5
```

---

## 2. Staging AWS (Zerodha live target)

`execution_adapter_mode=http`, `paper_adapter_cohosted=true`, `enabled_adapters=paper,zerodha`.

```mermaid
flowchart TB
    subgraph External["External"]
        TV[TradingView webhooks]
        DASH[Dashboard / users]
        KITE[Kite Connect API]
        KITE_OAUTH[Kite OAuth / Publisher browser]
        FYERS_MD[FYERS quotes API]
    end

    subgraph DNS["Cloudflare DNS"]
        API_HOST[api.staging.zettabridge.net]
        APP_HOST[app.staging.zettabridge.net]
        EXEC_HOST[exec-zerodha.staging.zettabridge.net]
    end

    subgraph AWS["AWS ap-south-1"]
        ALB[Application Load Balancer]

        subgraph Private["Private subnets"]
            CORE[Core ECS<br/>/zettabridge :8080]
            ZAD[Zerodha adapter ECS<br/>/app/zerodha-adapter :8092]
            RDS[(PostgreSQL RDS)]
            REDIS[(Redis ElastiCache)]
        end

        NAT[NAT Gateway — static EIP]
        SD[Cloud Map<br/>zettabridge-staging.local]
    end

    TV --> API_HOST --> ALB
    DASH --> APP_HOST --> ALB
    DASH --> API_HOST
    KITE_OAUTH --> EXEC_HOST --> ALB

    ALB -->|host api.*| CORE
    ALB -->|host app.*| CORE
    ALB -->|host exec-zerodha.*| ZAD

    CORE --> RDS
    CORE --> REDIS
    ZAD --> RDS
    ZAD --> REDIS

    CORE -->|POST /v1/orders<br/>X-ZB-Service-Token| ZAD
    ZAD --> NAT --> KITE

    CORE -->|FYERS admin OAuth LTP| FYERS_MD
    FYERS_MD -.-> CORE

    ZAD --- SD
    CORE --- SD

    subgraph NotDeployed["Not deployed"]
        PA_ECS[paper-adapter ECS]
        OTHER[angel / dhan / mt5 adapters]
    end

    style NotDeployed fill:#f5f5f5,stroke:#999,stroke-dasharray: 5 5
```

---

## 3. Order execution path (inside Core)

```mermaid
flowchart LR
    WH[Webhook ingest] --> Q[Queue workers]
    Q --> ORCH[Orchestrator]
    ORCH --> ROUTER[Router<br/>ENABLED_ADAPTERS]

    ROUTER -->|Free / Paper / paper webhook| PAPER[PaperRunner<br/>paperengine in Core]
    ROUTER -->|Pro / Pro Plus + broker_cred| HTTP[HTTP adapter client]

    HTTP -->|POST /v1/orders| ZA[zerodha-adapter]
    ZA --> LIVE[local Live executor]
    LIVE --> KITE[Kite Connect API]

    PAPER --> PG[(PostgreSQL)]
    ZA --> PG
    ORCH --> TRADES[Trades / WS / metrics]
```

**Plan gates (four tiers):**

| Plan | Paper webhooks | Live webhooks |
|------|----------------|---------------|
| Free | 1 | 0 |
| Paper | 5 | 0 |
| Pro | 8 | 1 |
| Pro Plus | 15 | 5 |

Live Zerodha requires Pro/Pro Plus + `broker_cred_id` when `EXECUTION_ADAPTER_MODE=http` and `BROKER_MODE=live`.

---

## 4. Local dev (`make compose-http`)

```mermaid
flowchart LR
    subgraph Compose["docker compose --profile adapters"]
        SRV[server :8080 Core]
        ZA[zerodha-adapter :8092]
        PA[paper-adapter :8091]
        PG[(postgres)]
        RD[(redis)]
    end

    SRV --> PG
    SRV --> RD
    ZA --> PG
    ZA --> RD
    PA --> PG
    PA --> RD

    SRV -->|EXECUTION_ADAPTER_MODE=http| ZA
    SRV -->|PAPER_ADAPTER_URL optional| PA
```

**Note:** Staging cohosts paper in Core (`paper_adapter_cohosted=true`). Local compose may still run a paper sidecar when `PAPER_ADAPTER_URL` is set.

---

## 5. Zerodha OAuth connect flow (http mode)

Core does not receive Kite `request_token` on the public internet — only the adapter does.

```mermaid
sequenceDiagram
    participant U as User browser
    participant D as Dashboard
    participant C as Core API
    participant A as Zerodha Adapter
    participant K as Kite

    U->>D: Connect Zerodha
    D->>C: GET /v1/credentials/zerodha/connect?id=...
    C->>C: Verify cred scope, build Kite login URL
    C-->>U: Redirect to Kite (callback = adapter URL)
    U->>K: Kite login
    K->>A: GET /v1/credentials/zerodha/callback?request_token=...
    A->>A: Exchange token, encrypt session in PG
    A->>U: Redirect to app.staging / dashboard
    Note over C,A: Session stored in shared PostgreSQL
```

Publisher handoff: synchronous `handoff` in order response → user confirms basket on Kite → Kite redirects to **adapter** `/v1/publisher/callback`.

---

## 6. Three-layer adapter gating

```mermaid
flowchart LR
    TF[Terraform enabled_adapters] --> ECS[Which ECS services exist]
    CORE[Core ENABLED_ADAPTERS + URLs] --> ROUTER[Execution router]
    API[GET /v1/brokers/enabled] --> UI[Dashboard credential form]
    ECS --> CORE
    ROUTER -->|only enabled| AD[Adapter instances]
    UI -->|only enabled types| USER[User]
```

| Layer | Mechanism | Day-0 value |
|-------|-----------|-------------|
| Terraform | `enabled_adapters` → ECS `for_each` | `["paper","zerodha"]` → **zerodha-adapter ECS only** |
| Core runtime | `ENABLED_ADAPTERS` + adapter URLs | Router rejects disabled brokers |
| Product API | `/v1/brokers/enabled` | Dashboard hides Angel/Dhan/MT5 |

---

## 7. Implementation phases (timeline)

```mermaid
flowchart LR
    P0[Phase 0<br/>Product cleanup] --> P1[Phase 1<br/>Orchestrator]
    P1 --> P2[Phase 2<br/>Adapter packages]
    P2 --> P3[Phase 3<br/>Compose HTTP]
    P3 --> P4[Phase 4<br/>Terraform ECS]
    P4 --> P15[§15 Cutover<br/>DNS Kite NAT]
    P4 --> P5[Phase 5<br/>FYERS live broker]

    style P5 fill:#f5f5f5,stroke:#999
```

---

## 8. Responsibility matrix (quick reference)

```mermaid
flowchart TB
    subgraph CoreOwns["Core owns"]
        C1[TradingView webhooks]
        C2[Auth / Stripe / billing]
        C3[Credential CRUD encrypted]
        C4[Plan gate Free/Paper/Pro/Pro Plus]
        C5[Paper engine staging]
        C6[FYERS LTP snapshots]
        C7[Trades WS metrics]
    end

    subgraph AdapterOwns["Zerodha adapter owns"]
        A1[Kite PlaceOrder / Cancel]
        A2[Kite OAuth + Publisher callbacks]
        A3[NAT egress to Kite]
        A4[Decrypt cred at execution]
        A5[Adapter idempotency Redis]
    end

    subgraph NotNow["Not deployed now"]
        N1[Angel Dhan MT5 adapters]
        N2[FYERS live broker]
    end

    style NotNow fill:#f5f5f5,stroke:#999,stroke-dasharray: 5 5
```

---

## 9. Config reference (day-0 staging)

```env
ENABLED_ADAPTERS=paper,zerodha
EXECUTION_ADAPTER_MODE=http
paper_adapter_cohosted=true                    # Terraform variable
ZERODHA_ADAPTER_URL=https://exec-zerodha.staging.zettabridge.net
ZB_SERVICE_TOKEN=...                           # Core ↔ adapter
BROKER_MODE=live                               # after NAT IP whitelisted
```

---

## Related docs

- [STATUS.md](STATUS.md) — canonical product snapshot
- [diagrams/README.md](diagrams/README.md) — customer-facing product diagrams (webhook flows, Publisher, compliance)
- [scope.md](01-product/scope.md) — in/out of scope
- [deploy/deploy-aws-ecs/terraform/staging/README.md](../deploy/deploy-aws-ecs/terraform/staging/README.md) — Phase 4 Terraform
