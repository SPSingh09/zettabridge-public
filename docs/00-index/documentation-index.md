# Documentation Index
**Last updated: 2026-07-07**

Master index of all ZettaBridge documentation. Directories follow the `NN-topic/` convention.

**Canonical product snapshot:** [STATUS.md](../STATUS.md) (last updated 2026-07-07) — current plans, execution model, API surface, and known gaps.

---

## 00 — Index

| File | Description |
|------|-------------|
| [STATUS.md](../STATUS.md) | **Canonical snapshot** — current shipped product (plans, adapters, API, deployment) |
| [documentation-index.md](documentation-index.md) | This file — master index |
| [glossary.md](glossary.md) | Key terms and definitions |

---

## 01 — Product

| File | Description |
|------|-------------|
| [product-vision.md](../01-product/product-vision.md) | Problem statement, target personas, 7 principles, vision |
| [prd.md](../01-product/prd.md) | User journeys, functional requirements, NFRs (solo accounts; paper + live) |
| [scope.md](../01-product/scope.md) | In-scope / out-of-scope feature boundaries |
| [feature-catalogue.md](../01-product/feature-catalogue.md) | All shipped features with completion status |
| [roadmap.md](../01-product/roadmap.md) | Phase plan: beta milestones, GA hardening, post-GA |

---

## 02 — Architecture

| File | Description |
|------|-------------|
| [system-architecture.md](../02-architecture/system-architecture.md) | System diagram, request lifecycle, API surface, trade-offs |
| [database-design.md](../02-architecture/database-design.md) | Full schema, ER diagram, all migrations consolidated |
| [data-flows.md](../02-architecture/data-flows.md) | 9 ASCII sequence diagrams: ingest, auth, WebSocket, OAuth, billing |
| [hld.md](../02-architecture/hld.md) | High-level design (original) |

### Product diagrams (customer-facing)

| File | Description |
|------|-------------|
| [diagrams/README.md](../diagrams/README.md) | **Index** — all 14 product diagrams + homepage architecture |
| [diagrams/customer-architecture.md](../diagrams/customer-architecture.md) | Execution stack + three-step onboarding flow |
| [diagrams/execution-flows.md](../diagrams/execution-flows.md) | Webhook ingest, Paper vs Publisher, confirmation, trade status |
| [diagrams/configuration.md](../diagrams/configuration.md) | Setup map, webhook model, payload rules, workflows matrix |
| [diagrams/trust-and-compliance.md](../diagrams/trust-and-compliance.md) | Data boundary, security path, compliance roles, audit trail |
| [diagrams/operations.md](../diagrams/operations.md) | Dedup/retry guide, plan limits |

---

## Presentations

| File | Description |
|------|-------------|
| [presentations/README.md](../presentations/README.md) | Zerodha demo deck — export instructions |
| [presentations/ZettaBridge_Zerodha_Demo_Final.marp.md](../presentations/ZettaBridge_Zerodha_Demo_Final.marp.md) | **Zerodha demo** — Marp source (export to PPTX/PDF) |

---

## 03 — Security

| File | Description |
|------|-------------|
| [security-architecture.md](../03-security/security-architecture.md) | AES-256-GCM encryption, JWT design, transport security, secrets management, rate limiting |
| [threat-model.md](../03-security/threat-model.md) | 7 threat actors, 3 trust boundaries, 12 threat scenarios T1–T12, OWASP Top 10 coverage |

---

## 04 — Integrations

| File | Description |
|------|-------------|
| [api-reference.md](../04-integrations/api-reference.md) | All endpoints grouped by tag; auth patterns; error codes; OpenAPI spec location |
| [zerodha.md](../04-integrations/zerodha.md) | Kite Connect credential format, token refresh, OAuth constraint, SEBI Algo-ID |
| [zerodha-publisher.md](../04-integrations/zerodha-publisher.md) | Kite Publisher redirect flow — planned endpoints, params, callback |
| [angel-one.md](../04-integrations/angel-one.md) | SmartAPI credential format, token refresh, IP whitelist, SEBI ordertag |
| [dhan.md](../04-integrations/dhan.md) | Dhan credential format, API access, IP whitelist, SEBI correlationId |
| [payments.md](../04-integrations/payments.md) | Four-tier plans (Free/Paper/Pro/Pro Plus), Stripe checkout, admin override, billing ops |

---

## 05 — Operations

| File | Description |
|------|-------------|
| [runbook.md](../05-operations/runbook.md) | Incident response, credential compromise, suspension, NAT egress, log hygiene, billing ops |
| [observability.md](../05-operations/observability.md) | Prometheus metrics, Grafana/Alertmanager setup, Telegram alerts, alert rules |

---

## 06 — Project Management

| File | Description |
|------|-------------|
| [completed-work.md](../06-project-management/completed-work.md) | Shipped features and PRs |
| [pending-work.md](../06-project-management/pending-work.md) | Open work items |
| [milestones.md](../06-project-management/milestones.md) | Milestone definitions and status |
| [risks.md](../06-project-management/risks.md) | Risk register |
| [decision-log.md](../06-project-management/decision-log.md) | Architectural and product decisions |
| [action-items.md](../06-project-management/action-items.md) | Current action items |

---

## 07 — Engineering

| File | Description |
|------|-------------|
| [developer-guide.md](../07-engineering/developer-guide.md) | Prerequisites, docker-compose setup, env vars, Make commands, adding a broker adapter |
| [repository-guide.md](../07-engineering/repository-guide.md) | Directory tree, all `internal/` packages, import conventions |
| [technology-stack.md](../07-engineering/technology-stack.md) | Go packages, frontend, databases, AWS services, CI/testing tooling |
| [testing-strategy.md](../07-engineering/testing-strategy.md) | Test pyramid, unit/functional/load/security tests, billing tests |
| [configuration.md](../07-engineering/configuration.md) | Environment variable reference |

---

## 08 — Infrastructure

| File | Description |
|------|-------------|
| [aws-infrastructure.md](../08-infrastructure/aws-infrastructure.md) | ECS Fargate, RDS, Redis, ALB, NAT, ECR, Secrets Manager, Terraform layout, deploy workflow |

---

## 09 — Compliance and Legal

| File | Description |
|------|-------------|
| [sebi-compliance.md](../09-compliance-and-legal/sebi-compliance.md) | SEBI Algo-ID regulatory requirement, obtaining IDs from brokers, ZettaBridge enforcement |

---

## 10 — User Guides

| File | Description |
|------|-------------|
| [getting-started.md](../10-user-guides/getting-started.md) | End-to-end: account → credentials → webhook → TradingView → first trade |

---

## Architecture

| File | Description |
|------|-------------|
| [architecture-diagrams.md](../architecture-diagrams.md) | Mermaid diagrams: Core vs execution adapters, staging AWS, plan gates |
| [diagrams/README.md](../diagrams/README.md) | Product/customer-facing Mermaid diagrams (mirrors marketing site) |

---

## Legacy / Plans

Historical implementation plans in `docs/plans/` — superseded by [STATUS.md](../STATUS.md) for current behavior. Each file has a banner noting this.

| File | Feature |
|------|---------|
| [beta-phase-1-aws-foundation.md](../plans/beta-phase-1-aws-foundation.md) | AWS VPC, ECS, RDS, Redis, ALB, ACM |
| [beta-phase-2-aws-deploy.md](../plans/beta-phase-2-aws-deploy.md) | ECR, Terraform, deploy pipeline |
| [beta-phase-3-app-wiring.md](../plans/beta-phase-3-app-wiring.md) | App config, secrets, ECS integration |
| [beta-phase-4-live-broker-launch.md](../plans/beta-phase-4-live-broker-launch.md) | Live broker adapters, P&L, WebSocket |
| [dashboard-mvp.md](../plans/dashboard-mvp.md) | Next.js 14 dashboard |
| [ga-production-hardening.md](../plans/ga-production-hardening.md) | GA security gates, load test, pen test |
| [sebi-algo-id.md](../plans/sebi-algo-id.md) | SEBI Algo-ID implementation detail |
| [stripe-billing.md](../plans/stripe-billing.md) | Stripe subscription billing |
| [KITE_PUBLISHER.md](../plans/KITE_PUBLISHER.md) | Kite Publisher execution mode |

---

## OpenAPI Spec

The machine-readable API spec lives at:

- `swagger/swagger.json` — OpenAPI 2.0 JSON
- `swagger/swagger.yaml` — OpenAPI 2.0 YAML

Regenerate with `make swagger`. See [developer-guide.md](../07-engineering/developer-guide.md#10-openapi-spec-swagger) for annotation workflow.
