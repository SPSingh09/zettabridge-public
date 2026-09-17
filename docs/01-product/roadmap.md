# Product Roadmap
**Last updated: 2026-07-07**
**Migrated from:** `docs/roadmap.md` (2026-06-23)

Living plan for remaining work. Status baseline synced with [STATUS.md](../STATUS.md) and migrations through **046**.

**Related:** [milestones.md](../06-project-management/milestones.md) · [pending-work.md](../06-project-management/pending-work.md) · [completed-work.md](../06-project-management/completed-work.md)

---

## Current baseline

| Area | Completion | Notes |
|------|------------|-------|
| Phase 1 — Foundation & core pipeline | **100%** | Fiber, in-process queue, Redis, PG, MT5, docker-compose |
| Phase 2 — Multi-tenant & security | **100%** | Email verification, JWT, plan tiers; org product **removed** |
| Phase 3 — Indian broker integrations | **~98%** | SL/TP brackets done; CancelOrder done; Zerodha WS deferred post-GA |
| Phase 4A — Backend product APIs | **100%** | P&L, WebSocket, Grafana, Telegram alerts, paper accounts |
| Phase 4B — Customer-facing product | **~90%** | Full dashboard shipped; paper tier + adapter split done; Stripe disabled |
| Phase 5A — Staging infrastructure | **100%** | ECS Core + paper/zerodha sidecars, RDS, Redis, NAT, ALB |
| Phase 5B — Security hardening | **~20%** | Load test scripts exist; pen test and KMS envelope pending |
| Phase 5C — Launch | **~15%** | Paper tier live; live broker flip pending NAT whitelist + Algo-ID sandbox |

### Already shipped

Webhook ingest, worker queue, paper engine + paper adapter, live broker adapters (Zerodha on ECS; Angel/Dhan/MT5 in code), JWT + refresh, credential encryption, webhook guards, four-plan model (`free`/`paper`/`pro`/`pro_plus`), dedup (409), compliance gates, admin panel, credential verify, Prometheus `/metrics`, Grafana on Lightsail VM, Telegram alerting, Stripe billing code, full Next.js dashboard, AWS ECS staging with HTTP adapter split, email verification, SEBI Algo-ID, SL/TP brackets, CancelOrder, migration 001–**046**.

### Removed / superseded

- **Org/household product** — `/v1/orgs/*` removed; dashboard redirects; legacy DB only (migration 044 detached org_id)
- **Demo credential mode** — `account_mode=demo` removed (migration 044); paper via `paper_account_id`
- **Individual/Household plan names** — replaced by four-tier model (migration 046)
- **User-facing mock broker** — `BROKER_MODE=mock` is server staging/CI config only

---

## Guiding principles

1. **Infra before beta** — staging/prod must exist before load tests, pen tests, and real users.
2. **Backend APIs before UI** — P&L and WebSocket endpoints before the dashboard consumes them.
3. **Observability before scale** — Grafana and alerts on staging before 500-webhook load tests.
4. **Billing before paid beta** — Stripe (or manual admin billing for closed beta only).
5. **Indian compliance before Indian live scale** — SEBI Algo-ID before marketing live NSE/BSE.
6. **Paper before live** — Paper tier and paper adapter validate pipeline before live broker scale.

---

## Master sprint order

| Sprint | Phase | Deliverables | Status |
|--------|-------|--------------|--------|
| S1 | 3.1 | Email verification | Done |
| S2 | 5A | Staging AWS (ECS, RDS, Redis, NAT, ALB, Secrets Manager) | Done |
| S3 | 4A | Grafana dashboard JSON + alert rules on staging | Done |
| S4 | 5B | Load test (500 webhooks) + pen test fixes | Pending |
| S5 | 4A | P&L aggregation API + WebSocket trade push | Done |
| S6 | 4B | Stripe billing | Done (disabled) |
| S7–S9 | 4B | Dashboard MVP (React/Next.js) | Done |
| S10 | 3.2 | SEBI Algo-ID tagging | Done (sandbox validation pending) |
| S11 | 5C | Closed beta — paper + live traders | In Progress |
| S12 | 3.3 | Live SL/TP bracket orders | Done |
| ~~S13~~ | ~~3.4~~ | ~~CancelOrder API~~ | Done |
| S14 | 5B | AWS KMS envelope encryption + credential rotation | Pending |
| S15 | 4B + 5C | Paper tier + execution adapter split | **Done** |
| S16 | 5C | Live broker flip on staging | In Progress (blocked: NAT whitelist) |
| Later | 3.5 | Zerodha WebSocket, XM/FBS-specific flows | Deferred |

---

## Phase 3 — Trading & registration gaps

**Goal:** Close remaining core backend work.

| # | Item | Priority | Status |
|---|------|----------|--------|
| 3.1 | Email verification on registration | P1 | Done |
| 3.2 | SEBI Algo-ID tagging | P1 | Done (sandbox validation pending) |
| 3.3 | Live SL/TP / bracket orders (Indian brokers) | P2 | Done |
| 3.4 | CancelOrder API | P2 | Done |
| 3.5 | Zerodha WebSocket real-time feed | P3 | Deferred post-GA |
| 3.6 | Paper trading engine + paper tier | P1 | **Done** |
| 3.7 | Execution adapter split (local/http sidecars) | P1 | **Done** |

---

## Phase 4A — Backend product APIs

All items Done. No remaining work.

| # | Item | Status |
|---|------|--------|
| 4A.1 | P&L aggregation API | Done |
| 4A.2 | WebSocket trade push | Done |
| 4A.3 | Grafana dashboard JSON | Done |
| 4A.4 | Email / Telegram alerting | Done (email deferred) |
| 4A.5 | Paper accounts API | Done |

---

## Phase 5A — Staging infrastructure

All items Done. Sidecar adapters provisioned on ECS.

| # | Item | Status |
|---|------|--------|
| 5A.1 | AWS ECS + ECR (ap-south-1) | Done |
| 5A.2 | RDS PostgreSQL + migrations (through 046) | Done |
| 5A.3 | ElastiCache Redis | Done |
| 5A.4 | NAT Gateway + Elastic IP | Done |
| 5A.5 | ALB + ACM TLS | Done |
| 5A.6 | Secrets Manager (AES_KEY, JWT, etc.) | Done |
| 5A.7 | Paper + Zerodha adapter sidecars (`EXECUTION_ADAPTER_MODE=http`) | Done |

**Remaining ops action:** Dhan + Angel One IP whitelist on staging NAT EIP (portal action, not code).

---

## Phase 5B — Security hardening & validation

**Goal:** Prove the system under load and abuse before GA.

| # | Item | Priority | Status |
|---|------|----------|--------|
| 5B.1 | Load test — 500 concurrent webhooks (k6) | P1 | Pending |
| 5B.2 | Penetration test — webhook + auth endpoints | P1 | Pending |
| 5B.3 | AWS KMS envelope encryption | P2 | Pending |
| 5B.4 | Credential versioning / zero-downtime rotation | P2 | Pending |

k6 scripts exist at `deploy/loadtest/k6/`. Security test scripts at `deploy/sectest/scripts/`.

---

## Phase 4B — Customer-facing product

| # | Item | Priority | Status |
|---|------|----------|--------|
| 4B.1 | Stripe billing | P0 | Done (code); disabled (payment gateway blocked) |
| 4B.2 | Dashboard MVP (React/Next.js) | P0 | Done |
| 4B.3 | Paper accounts UI | P1 | Done |
| 4B.4 | P&L charts | P2 | Planned |

**Stripe unblock path:** Business registration → Stripe India application → enable `BILLING_ENABLED=true` + `STRIPE_PRICE_PAPER/PRO/PRO_PLUS` in Terraform tfvars.

---

## Phase 5C — Launch

| # | Item | Priority | Status |
|---|------|----------|--------|
| 5C.1 | Closed beta — paper + live traders | P0 | In Progress |
| 5C.2 | Flip staging → live brokers (Zerodha sidecar) | P1 | Blocked (NAT whitelist + Algo-ID sandbox) |
| 5C.3 | Public beta / GA | P1 | Blocked (5C.1 feedback + pen test clean) |

### Beta exit criteria

- [ ] `make functional-test` green on release branch
- [ ] Staging load test passed (500 VUs, p99 < 100ms)
- [ ] Grafana alerts wired and tested on staging (Lightsail VM scrapes ECS)
- [ ] Ops runbook drill (rotate token, suspend user)
- [ ] Dhan + Angel whitelist verified on prod NAT EIP
- [ ] SEBI Algo-ID sandbox validation complete
- [ ] Terms of Service and Risk Disclosure published

---

## Deferred post-GA (v1.1+)

| Item | Reason deferred |
|------|----------------|
| Zerodha WebSocket | REST + poll sufficient for beta |
| XM / FBS dedicated flows | Same MetaApi `mt5_cloud` path works |
| HashiCorp Vault | KMS + Secrets Manager sufficient on AWS |
| Full mark-to-market P&L | Needs position sync; beyond v1 trades table |
| Automated broker token refresh | Manual PUT documented; Zerodha daily refresh is a known user action |
| NATS / SQS message queue | In-process queue meets current scale target |
| Additional Indian brokers on ECS | Angel/Dhan/MT5 code exists; enable via `ENABLED_ADAPTERS` |
| Options and futures order types | Equities-only for v1 |
| Mobile dashboard | Post-GA |
| Multi-region deployment | Post-GA |
| Multi-user orgs (re-launch) | Removed from v1; would be v2+ if revisited |

---

## Revision history

| Date | Change |
|------|--------|
| 2026-07-07 | Four-plan model (046); org removed; paper tier + adapter split marked done; dedup 409 |
| 2026-06-23 | Migrated to `docs/01-product/roadmap.md`; updated completion percentages |
| 2026-06-21 | Plan rename (Free/Individual/Household) — **superseded by 046** |
| 2026-06-20 | Beta Phase 1 + 2 marked complete; staging live |
