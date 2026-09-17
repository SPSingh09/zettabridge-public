# Product Diagrams

**Last updated:** July 2026  
**Scope:** Closed beta — paper trading and Zerodha Kite Publisher only.  
**Audience:** Product, engineering, support, and marketing alignment.

Visual reference for ZettaBridge customer-facing flows. These diagrams mirror the diagrams on [zettabridge-web](https://github.com/SPSingh09/zettabridge-web) and reflect **current product behaviour**, not future integrations (no public Kite Connect OAuth promises).

For internal infrastructure (Core vs adapters, staging AWS, Terraform), see [architecture-diagrams.md](../architecture-diagrams.md). For ASCII sequence diagrams of system actors, see [data-flows.md](../02-architecture/data-flows.md).

---

## How to render

| Method | Steps |
|--------|--------|
| **GitHub / GitLab** | Push — Mermaid blocks render in markdown preview |
| **VS Code / Cursor** | Install “Markdown Preview Mermaid Support”, open preview |
| **mermaid.live** | Copy a ` ```mermaid ` block → [mermaid.live](https://mermaid.live) → Export PNG/SVG |
| **CLI** | `npx @mermaid-js/mermaid-cli -i diagram.mmd -o diagram.png` |

---

## Catalog

### Customer architecture & onboarding

| # | Diagram | File |
|---|---------|------|
| — | Enterprise execution stack (homepage) | [customer-architecture.md](customer-architecture.md#1-enterprise-execution-stack) |
| — | Three steps: signal to outcome (homepage) | [customer-architecture.md](customer-architecture.md#2-three-steps-signal-to-outcome) |

### Execution flows

| # | Diagram | File |
|---|---------|------|
| 1 | Webhook ingest sequence | [execution-flows.md](execution-flows.md#1-webhook-ingest-sequence) |
| 2 | Paper vs Zerodha Kite Publisher fork | [execution-flows.md](execution-flows.md#2-paper-vs-zerodha-kite-publisher) |
| 3 | Kite Publisher confirmation flow | [execution-flows.md](execution-flows.md#3-kite-publisher-confirmation-flow) |
| 4 | Trade status lifecycle | [execution-flows.md](execution-flows.md#4-trade-status-lifecycle) |

### Configuration & onboarding

| # | Diagram | File |
|---|---------|------|
| 5 | First-time setup map | [configuration.md](configuration.md#1-first-time-setup-map) |
| 6 | Webhook configuration model | [configuration.md](configuration.md#2-webhook-configuration-model) |
| 7 | Signal payload resolution | [configuration.md](configuration.md#3-signal-payload-resolution) |
| 8 | Supported workflows matrix | [configuration.md](configuration.md#4-supported-workflows-matrix) |

### Trust, security & compliance

| # | Diagram | File |
|---|---------|------|
| 9 | Data boundary — what we store | [trust-and-compliance.md](trust-and-compliance.md#1-data-boundary--what-we-store) |
| 10 | Security path (customer-facing) | [trust-and-compliance.md](trust-and-compliance.md#2-security-path-customer-facing) |
| 11 | Compliance role boundary | [trust-and-compliance.md](trust-and-compliance.md#3-compliance-role-boundary) |
| 12 | Audit trail tracing (`request_id`) | [trust-and-compliance.md](trust-and-compliance.md#4-audit-trail-tracing) |

### Operations & plans

| # | Diagram | File |
|---|---------|------|
| 13 | Deduplication & retry behaviour | [operations.md](operations.md#1-deduplication--retry-behaviour) |
| 14 | Plan limits overview | [operations.md](operations.md#2-plan-limits-overview) |

---

## Product facts (all diagrams)

- Webhook ingest returns **HTTP 202** with `{ "status": "accepted", "queued": true, "request_id": "..." }`
- **No `redirect_url`** in the webhook response for Kite Publisher
- Publisher: user confirms via dashboard when trade status is `pending_confirmation`
- Trade status API: **`GET /v1/trades/{id}`** (not `/v1/orders/{id}`)
- Lot: webhook default wins when `lot_size > 0`; signal must send `lot` when default is 0
- Symbol validation may occur **after** 202 (trade `rejected` in dashboard)
- Publisher webhooks: **BUY / SELL only** at configuration level
- `required_comment` mismatch → **401** at ingest
- Default rate limits: **60/min** per webhook, **2s** dedup window, free plan **100 req/day**

---

## Related docs

- [STATUS.md](../STATUS.md) — canonical product snapshot
- [architecture-diagrams.md](../architecture-diagrams.md) — internal infra Mermaid diagrams
- [data-flows.md](../02-architecture/data-flows.md) — ASCII actor sequence diagrams
- [zerodha-publisher.md](../04-integrations/zerodha-publisher.md) — Publisher handoff detail
- [api-reference.md](../04-integrations/api-reference.md) — endpoints and error codes
