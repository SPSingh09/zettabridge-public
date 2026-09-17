# Milestone Register
**Last updated: 2026-07-07**

Single source of truth for all project milestones. Update Status and Evidence columns as items progress. Current product: [STATUS.md](../STATUS.md).

**Status values:** Proposed · Planned · In Progress · Blocked · Testing · Done · Deferred · Cancelled

---

## Milestone Table

| ID | Phase | Item | Status | Priority | Owner | Target | Dependency | Evidence |
|----|-------|------|--------|----------|-------|--------|------------|----------|
| P0.1 | Phase 0 | Four-plan model (Free/Paper/Pro/Pro Plus, migration 046) | Done | Critical | Surya | 2026-07 | — | plan.go, migration 046 |
| P0.2 | Phase 0 | Org product removed from API/UI; demo mode removed | Done | Critical | Surya | 2026-07 | — | router.go, migration 044 |
| P0.3 | Phase 0 | Paper accounts + paper webhook destination | Done | Critical | Surya | 2026-07 | — | paper handlers, dashboard |
| EA.1 | Adapters | Execution adapter split (Core + paper/zerodha sidecars on ECS) | Done | Critical | Surya | 2026-07 | P0.1 | deploy-aws-ecs |
| EA.2 | Adapters | Angel/Dhan/MT5 ECS provisioning + E2E | Planned | High | Surya | TBD | EA.1 | enabled_adapters |
| P1.1 | Foundation | Webhook ingest endpoint + TradingView JSON parsing | Done | Critical | Surya | 2026-06 | — | Bruno, functional tests |
| P1.2 | Foundation | Redis-backed in-process queue + configurable worker pool | Done | Critical | Surya | 2026-06 | — | docker-compose, functional tests |
| P1.3 | Foundation | Queue worker: validate → dispatch → trade insert | Done | Critical | Surya | 2026-06 | P1.2 | functional tests |
| P1.4 | Foundation | Mock broker factory for all broker types | Done | Critical | Surya | 2026-06 | — | functional tests |
| P1.5 | Foundation | JWT auth: login + refresh token rotation | Done | Critical | Surya | 2026-06 | — | Bruno auth |
| P1.6 | Foundation | AES-256-GCM credential encryption at rest | Done | Critical | Surya | 2026-06 | — | credenc unit tests |
| P1.7 | Foundation | Webhook guards: symbol/action filters + rate limit | Done | High | Surya | 2026-06 | P1.1 | Bruno failure modes |
| P1.8 | Foundation | Signal deduplication (Redis SETNX, pro+) | Done | High | Surya | 2026-06 | P1.2 | Bruno, ops-security.md |
| P1.9 | Foundation | Plan limits enforcement (webhooks, creds, orders/sec) | Done | High | Surya | 2026-06 | — | functional tests |
| P1.10 | Foundation | Prometheus /metrics endpoint | Done | Medium | Surya | 2026-06 | — | /metrics scrape |
| P2.1 | Multi-tenant | Enterprise org model + invite flow *(removed Phase 0)* | Cancelled | — | Surya | — | — | legacy DB only |
| P2.2 | Multi-tenant | Admin panel endpoints (list, plan override, suspend) | Done | High | Surya | 2026-06 | P2.1 | Bruno admin |
| P2.3 | Multi-tenant | Compliance gates (suspended user/org blocks ingest + worker) | Done | High | Surya | 2026-06 | P2.2 | functional tests |
| P2.4 | Multi-tenant | Token revocation middleware (CheckRevoked) | Done | High | Surya | 2026-06 | P1.5 | functional tests |
| P2.5 | Multi-tenant | billing_source field (free/stripe/admin; admin not overwritten) | Done | High | Surya | 2026-06 | — | internal/billing/ |
| P3.1 | Phase 3 | Email verification (register gate, verify, resend, invite auto-verify) | Done | Critical | Surya | 2026-06 | P1.5 | functional tests |
| P3.2 | Phase 3 | SEBI Algo-ID tagging (Zerodha/Angel/Dhan, fail-close, audit column) | Done | Critical | Surya | 2026-06 | P3.1 | unit tests, adapter tests |
| P3.2.S | Phase 3 | SEBI Algo-ID sandbox validation on live broker | Blocked | Critical | Surya | TBD | B4.3, B4.7 | Broker order details |
| P3.3 | Phase 3 | SL/TP bracket orders (Zerodha CO, Angel ROBO, MT5 SL/TP) | Done | High | Surya | 2026-06 | P3.2 | integration tests |
| P3.4 | Phase 3 | CancelOrder API (DELETE /v1/webhooks/:id/trades/:tradeId) | Done | High | Surya | 2026-06 | P1.3 | Bruno, functional tests |
| P3.5 | Phase 3 | Zerodha WebSocket tick feed (LTP cache) | Deferred | Low | Surya | Post-GA | P3.3 | — |
| P4A.1 | Phase 4A | P&L aggregation API (per-webhook + org rollup) | Done | High | Surya | 2026-06 | P1.3 | Bruno P&L |
| P4A.2 | Phase 4A | WebSocket live trade push (/v1/ws/trades) | Done | High | Surya | 2026-06 | P1.5 | Dashboard WS feed |
| P4A.3 | Phase 4A | Grafana dashboard JSON (ingest, trades, latency, dedup) | Done | Medium | Surya | 2026-06 | P1.10 | deploy/grafana/ |
| P4A.4 | Phase 4A | Telegram alerting via Alertmanager | Done | Medium | Surya | 2026-06 | P4A.3 | alertmanager config |
| P4B.1 | Phase 4B | Stripe billing code (checkout, portal, webhook handler, admin override) | Done | High | Surya | 2026-06 | P5A.5 | Bruno billing, internal/billing/ |
| P4B.2 | Phase 4B | Dashboard MVP (auth, webhooks, creds, paper accounts, trades, billing, admin, P&L) | Done | Critical | Surya | 2026-06 | P4A.1, P4A.2, P4B.1 | app.staging.zettabridge.net |
| P4B.3 | Phase 4B | Dashboard remaining (P&L charts, admin ops view) | Planned | Medium | Surya | TBD | P4B.2 | — |
| P5A.0 | AWS Infra | AWS account secured (IAM, MFA, CloudTrail, billing alerts) | Done | Critical | Surya | 2026-06-19 | — | AWS console |
| P5A.1 | AWS Infra | VPC: subnets, IGW, NAT Gateway, Elastic IP, security groups | Done | Critical | Surya | 2026-06-19 | P5A.0 | Terraform state |
| P5A.2 | AWS Infra | RDS PostgreSQL 15 (private, automated backups) | Done | Critical | Surya | 2026-06-19 | P5A.1 | RDS console |
| P5A.3 | AWS Infra | ElastiCache Redis 7 (private subnet) | Done | Critical | Surya | 2026-06-19 | P5A.1 | ElastiCache console |
| P5A.4 | AWS Infra | Secrets Manager + KMS CMK + ECS task IAM role | Done | Critical | Surya | 2026-06-19 | P5A.1 | Secrets Manager console |
| P5A.5 | AWS Infra | ALB + ACM TLS (api.staging + app.staging live) | Done | Critical | Surya | 2026-06-19 | P5A.1 | curl /healthz → 200 |
| P5A.6 | AWS Infra | ECS Fargate services (backend + dashboard) live on staging | Done | Critical | Surya | 2026-06-19 | P5A.2, P5A.3, P5A.4 | ECS console |
| P5A.7 | AWS Infra | ECR repositories (immutable tags, scan-on-push) | Done | Critical | Surya | 2026-06-19 | P5A.0 | ECR console |
| PR.M | Release | Plan rename: Free/Individual/Household (migration 020) | Superseded | — | Surya | 2026-06-21 | — | Replaced by P0.1 (046) |
| B3.1 | Beta | Stripe webhook endpoint added for staging URL | Planned | High | Surya | TBD | P4B.1, P5A.5 | Stripe Dashboard |
| B3.2 | Beta | JWT login/refresh flow verified on AWS staging domain | Planned | Critical | Surya | TBD | P5A.6 | Manual browser test |
| B3.3 | Beta | Functional tests green on AWS staging | Planned | Critical | Surya | TBD | B3.2 | CI pass |
| B3.4 | Beta | Manual smoke checklist passed (register → trade → billing) | Planned | Critical | Surya | TBD | B3.2 | Checklist record |
| B4.1 | Beta | NAT EIP whitelisted with Angel One portal | Blocked | Critical | Surya | TBD | P5A.1 | Angel portal |
| B4.2 | Beta | NAT EIP whitelisted with Dhan portal | Blocked | Critical | Surya | TBD | P5A.1 | Dhan portal |
| B4.3 | Beta | ECS live flip: BROKER_MODE=live + Angel env vars updated | Blocked | Critical | Surya | TBD | B4.1, B4.2 | CloudWatch startup log |
| B4.4 | Beta | Real Pro-plan credential verified (valid: true) | Blocked | Critical | Surya | TBD | B4.3 | API response |
| B4.5 | Beta | Live order end-to-end smoke test (status=submitted, real broker order ID) | Blocked | Critical | Surya | TBD | B4.4 | CloudWatch + broker portal |
| B4.6 | Beta | SL/TP bracket order verified on live broker | Blocked | High | Surya | TBD | B4.5 | Broker portal |
| B4.7 | Beta | SEBI Algo-ID confirmed in broker order details | Blocked | Critical | Surya | TBD | B4.3 | Broker order details |
| B4.8 | Beta | Ops drills: token rotate, suspend/unsuspend, ECS redeploy | Planned | High | Surya | TBD | B4.3 | Drill record |
| B4.9 | Beta | Beta Go/No-Go checklist signed off | Blocked | Critical | Surya | TBD | B4.5, B4.7, B4.8 | Signed checklist |
| BETA.OPEN | Beta | Closed beta open to first traders | Blocked | Critical | Surya | TBD | B4.9, B3.3 | — |
| S1 | Hardening | Security response headers (HSTS, X-Content-Type-Options, X-Frame-Options) | Planned | High | Surya | TBD | — | Browser check |
| S2 | Hardening | Log scrubbing audit (secret/key/password fields) | Planned | High | Surya | TBD | — | Log review |
| S3 | Hardening | Rate limiting verified on auth + ingest endpoints | Planned | High | Surya | TBD | — | Bruno failure modes |
| S4 | Hardening | Load test: 500 concurrent webhooks, p99 < 100ms | Planned | High | Surya | TBD | P5A.6 | k6 report |
| S5 | Hardening | Penetration test (JWT, IDOR, ingest flood, SSRF) | Planned | High | Surya | TBD | S1, S2, S3 | Pen test report |
| S6 | Hardening | AWS KMS envelope encryption (KMS-wrapped DEK, key_version column) | Planned | Medium | Surya | TBD | P5A.4 | Code + migration |
| S7 | Hardening | Credential versioning + zero-downtime rotation | Planned | Medium | Surya | TBD | S6 | Runbook |
| G1 | GA | CloudWatch alarms wired and tested (all 8 alarm types) | Planned | High | Surya | TBD | P5A.6 | CloudWatch console |
| G2 | GA | AWS WAF enabled on ALB (managed rules + custom rate limits) | Planned | High | Surya | TBD | P5A.5 | WAF console |
| G3 | GA | CI/CD pipeline (test → build → staging → approve → prod blue/green) | Planned | High | Surya | TBD | P5A.6 | Pipeline run |
| G4 | GA | Blue/green deploy tested (CodeDeploy, auto-rollback) | Planned | High | Surya | TBD | G3 | Deployment record |
| G5 | GA | Disaster recovery drill (RDS PITR restore, RTO < 1 hour) | Planned | High | Surya | TBD | P5A.2 | DR record |
| G6 | GA | Oncall runbooks complete (rejection spike, queue depth, 5xx, rollback) | Planned | High | Surya | TBD | — | docs/08-operations/ |
| G7 | GA | Terraform IaC for production environment | Planned | Medium | Surya | TBD | G3 | terraform apply |
| PAY.1 | Business | Business entity registration in India | Blocked | High | Surya | TBD | — (legal process) | Registration certificate |
| PAY.2 | Business | Stripe India onboarding approval | Blocked | High | Surya | TBD | PAY.1 | Stripe email |
| PAY.3 | Business | Enable BILLING_ENABLED=true + STRIPE_PRICE_PAPER/PRO/PRO_PLUS in Terraform | Blocked | High | Surya | TBD | PAY.1, PAY.2 | /billing/plans API |
| GA.PUB | GA | Public beta / GA launch | Planned | Critical | Surya | TBD | S4, S5, G1, G2, G3, G5 | Launch announcement |

---

## Deferred Items

| ID | Phase | Item | Status | Notes |
|----|-------|------|--------|-------|
| P3.5 | Phase 3 | Zerodha WebSocket tick feed | Deferred | REST polling sufficient for beta |
| DEF.2 | Post-GA | Full mark-to-market P&L | Deferred | Requires position-sync beyond trades table |
| DEF.3 | Post-GA | Automatic broker OAuth token refresh | Deferred | Manual PUT documented in broker-guide |
| DEF.4 | Post-GA | NATS / Kafka message queue | Deferred | In-process queue meets scale targets through GA |
| DEF.5 | Post-GA | Mobile-responsive dashboard polish | Deferred | Post-launch UX iteration |
| DEF.6 | Post-GA | Richer P&L analytics (Sharpe, drawdown) | Deferred | Post-launch product iteration |
