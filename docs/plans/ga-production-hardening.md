# GA — Production Hardening

> **Historical implementation plan.** Current product behavior is defined in [STATUS.md](../STATUS.md) and [scope.md](../01-product/scope.md). Last reviewed: 2026-07-07.

**Target: Parallel to controlled beta, completed before public GA**
**Goal:** Harden ZettaBridge for full-fledged production: security, reliability, observability, and operational maturity.

Depends on: Beta phases 1–4 complete and controlled beta running.

---

## P0 — Security review and pen test
**Must be clean before public GA.**

Scope for external pen test:
- Auth: JWT forgery, session fixation, token replay, refresh abuse
- Webhook tokens: enumeration, token reuse after rotation
- Broker credential encryption: AES key exposure paths, secret logging, response scrubbing
- Billing: Stripe webhook signature bypass, plan downgrade race
- Admin / RBAC: horizontal privilege escalation (user A accessing user B's resources), org role bypass
- Rate limiting: webhook ingest flood, API endpoint abuse
- SSRF / injection: webhook URL, comment field, symbol field, TradingView payload
- TLS: cipher suite review, HSTS headers

Deliverable: test report with critical/high findings closed or risk-accepted before GA.

Internal pre-pen-test hardening:
- [ ] Add `Strict-Transport-Security`, `X-Content-Type-Options`, `X-Frame-Options` response headers (Fiber middleware)
- [ ] Confirm all `secret`/`key`/`password` fields are scrubbed from logs
- [ ] Confirm broker credentials are never returned in any API response (already masked via `json:"-"` on `EncryptedCreds`)
- [ ] Verify rate limiting covers `/v1/auth/login`, `/v1/auth/register`, and webhook ingest endpoints

---

## P0 — Observability and oncall

**Minimum alerts before GA (CloudWatch Alarms + SNS → email/PagerDuty):**

| Alarm | Metric | Threshold |
|-------|--------|-----------|
| High order rejection rate | Custom metric `order_rejected_total` | > 5% of submitted in 5 min window |
| Queue depth spike | Custom metric `queue_depth` | > 100 for 5 min |
| Backend 5xx rate | ALB `HTTPCode_Target_5XX_Count` | > 10 in 1 min |
| p95 API latency | ALB `TargetResponseTime p95` | > 1000ms for 5 min |
| ECS service unhealthy | ECS `ServiceCount` < desired | Any |
| RDS CPU | RDS `CPUUtilization` | > 80% for 10 min |
| Redis memory | ElastiCache `DatabaseMemoryUsagePercentage` | > 80% |
| Broker auth failure | Custom metric `broker_auth_error_total` | > 3 in 5 min |

**Grafana dashboards:**
- [ ] Order throughput (ingested / submitted / filled / rejected) over time
- [ ] Broker error breakdown by broker type and error code
- [ ] Queue depth and processing latency
- [ ] API p50/p95/p99 latency by endpoint
- [ ] Active WebSocket connections

**Oncall runbook** (markdown file per incident type):
- `ORDER_REJECTED` spike: steps to identify broker, check credential validity, check SEBI Algo-ID
- `QUEUE_DEPTH` spike: check worker count, Redis connectivity, broker rate limits
- `5XX` spike: check ECS logs, DB connectivity, migration state
- Rollback to `BROKER_MODE=mock`: 5-minute recovery procedure

---

## P0 — Load test

Target: simulate NFP / high-volatility market event (many concurrent TradingView alerts).

```bash
# k6 load test scripts are in deploy/loadtest/k6/
# Update BASE_URL to AWS staging before running
k6 run --vus 500 --duration 60s deploy/loadtest/k6/webhook-ingest.js
```

Scenarios to validate:
- [ ] 500 concurrent webhook ingest requests over 60s → no 5xx, queue drains within 30s
- [ ] Broker rate-limit simulation: mock broker returns 429 → retry/backoff behavior correct
- [ ] Queue saturation: fill queue buffer (500) → excess requests get 429, existing queue drains correctly
- [ ] Spike-and-recover: burst 1000 RPS for 10s, then normal → system recovers without manual intervention

---

## P0 — Disaster recovery

**RDS:**
- [ ] Enable PITR (Point-in-Time Recovery) — already on with automated backups
- [ ] DR drill: restore RDS to a point-in-time in a test subnet; confirm app connects and migrations are at expected version
- [ ] Document RTO target: < 1 hour for RDS restore

**Redis:**
- [ ] Redis recovery plan: ElastiCache is cache-only (no durable state); on Redis loss, the system degrades gracefully (rate limits reset, dedup window resets, queued trades may re-run)
- [ ] Document expected behavior on Redis outage

**Application:**
- [ ] ECS service auto-recovery is already configured (desired count = 1; ECS restarts failed tasks)
- [ ] Document manual failover: if primary AZ fails, ECS places tasks in secondary AZ automatically

**Evidence archive:**
- [ ] Create S3 bucket `zettabridge-ops-archive` for test screenshots, load test reports, pen test evidence, DB restore drill records

---

## P1 — Infrastructure as Code

Replace manual AWS console work with repeatable IaC.

**Tool:** Terraform (recommended for AWS maturity; CDK is a valid alternative)

**Modules to write:**
- `vpc` — VPC, subnets, IGW, NAT Gateway, EIP, route tables, SGs
- `rds` — RDS PostgreSQL, subnet group, parameter group, backups
- `elasticache` — Redis cluster, subnet group
- `secrets` — Secrets Manager secrets, KMS key, IAM policy
- `ecr` — Repositories with immutable tags and scan-on-push
- `ecs` — Cluster, task definitions, services, task role, execution role
- `alb` — ALB, listeners, target groups, health checks
- `dns` — Route 53 records (ACM cert validation + API/app aliases)

State storage: S3 backend with DynamoDB lock table.

---

## P1 — CI/CD and release controls

**Pipeline stages:**
1. `test` — `go test ./...` + `npm run build` (dashboard)
2. `build` — Docker build + push to ECR with commit SHA tag
3. `staging-deploy` — Update ECS task def + force deploy to staging
4. `staging-test` — Run `make staging-functional-test` against AWS staging
5. `staging-approve` — Manual approval gate before prod
6. `prod-deploy` — Update ECS task def + force deploy to prod (blue/green)

**Blue/green deploy:**
- Use ECS CodeDeploy deployment group with `BLUE_GREEN` deployment type
- Shift traffic in 10% increments over 5 minutes; auto-rollback on health check failure
- Pre-traffic Lambda hook: run smoke test against new task before shifting traffic

---

## P1 — KMS envelope encryption and credential versioning

Current state: AES-256-GCM key lives in `AES_KEY` env var (now in Secrets Manager).

Upgrade path:
- [ ] Wrap `AES_KEY` with KMS CMK (envelope encryption): store encrypted DEK in DB; decrypt at runtime using KMS
- [ ] Add `key_version` column to `broker_credentials` table (migration)
- [ ] Dual-DEK rotation: generate new DEK, re-encrypt all credentials, mark old DEK as deprecated
- [ ] No downtime: read old DEK for existing records, write new DEK for new records during rotation window

This is 5B.3 + 5B.4 from the original roadmap — now unblocked on AWS because Secrets Manager + KMS are in place.

---

## P1 — AWS WAF

Protect auth, webhook, and admin endpoints at the ALB layer.

- [ ] Enable AWS WAF on the staging ALB
- [ ] Attach managed rule groups: `AWSManagedRulesCommonRuleSet`, `AWSManagedRulesKnownBadInputsRuleSet`
- [ ] Custom rule: rate limit `POST /v1/auth/login` to 20 req/5min per IP
- [ ] Custom rule: rate limit `POST /v1/webhooks/*` to 100 req/sec per IP (supplement app-level rate limiting)
- [ ] Log WAF decisions to S3 for audit

---

## P2 — Email / Telegram trade alerts (4A.4)

User-facing alerts on trade fill, rejection, and cancellation.

- Email: reuse existing SMTP provider; template per event type
- Telegram: bot token + chat ID stored per user in preferences table (new migration needed)
- Configurable per user: opt-in, per-event-type toggle
- Backend: fan-out after trade status update in queue worker

---

## P2 — Grafana JSON source datasource (4A.3)

Endpoint: `GET /v1/admin/metrics/grafana` (already stubbed or easy to add)
Returns Grafana's SimpleJSON format for trade volume, broker breakdown, rejection rates.
Used by ops dashboards without requiring Prometheus scraping.

---

## P2 — Multi-org switcher polish

When a user is a member of multiple orgs (via `GET /v1/orgs`), the NavBar shows a dropdown instead of a single `/org` link. The active org ID is stored in `localStorage` as `zb_active_org` and passed as a context to org-scoped API calls.

Backend change needed: org-scoped routes currently derive org from the URL param. The dashboard already parameterizes by `user.org_id` — extend to allow override from localStorage selection.

---

## P3 — Product backlog (post-GA)

| Item | Notes |
|------|-------|
| Mobile-friendly dashboard | Responsive layout for tablet/phone |
| Richer P&L analytics | Time-series P&L chart (requires trade history timestamps), drawdown, Sharpe ratio |
| Strategy marketplace | Shared webhook templates — well beyond v1 |
| Multi-region expansion | Only when latency or compliance demands it |
| Reserved Instances / Savings Plans | After 3 months of stable EC2/RDS usage pattern |

---

## GA Go / No-Go Gates

| Gate | Owner | Status |
|------|-------|--------|
| Pen test clean (critical/high closed) | Security | [ ] |
| CloudWatch/Grafana alerts live and tested | Ops | [ ] |
| Load test: 500 concurrent webhooks passed | QA | [ ] |
| DR drill: RDS restore verified | Ops | [ ] |
| Blue/green deploy tested | DevOps | [ ] |
| KMS/DEK rotation implemented | Backend | [ ] |
| AWS WAF enabled | Ops | [ ] |
| Support SOP and incident templates ready | Product | [ ] |
