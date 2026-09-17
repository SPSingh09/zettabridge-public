# Beta Phase 1 — AWS Foundation ✅ COMPLETE

> **Historical implementation plan.** Current product behavior is defined in [STATUS.md](../STATUS.md) and [scope.md](../01-product/scope.md). Last reviewed: 2026-07-07.

**Completed: 2026-06-19**
**Goal:** Secure AWS account, stable network with whitelistable NAT EIP, and live data/secrets services.

---

## Context
The decision to move to AWS now (not post-beta) means AWS becomes a beta blocker, not a deferred item. The broker IP whitelist must use the AWS NAT Gateway Elastic IP — not the Lightsail VM IP. No beta should open until this phase is complete.

Covers the doc's phases 0–4: release freeze → account → network → data services → secrets/config.

---

## Phase 0 — Release freeze and inventory

Before any AWS work starts:

- [x] Tag the release candidate commit: `git tag beta-rc-1 <sha>`
- [x] Record: commit SHA, backend image tag, dashboard image tag, DB migration version (latest = `018_fix_cancel_error_not_null.sql`)
- [x] Export current Lightsail `runtime.env` (secrets masked) as a baseline env map
- [x] Write rollback command: how to revert if AWS deploy fails (point DNS back to Lightsail IP)

**Exit:** A release checklist file (can be a private note) with SHA, tags, migration version, and rollback plan.

---

## Phase 1 — AWS account foundation

- [x] Confirm AWS region: `ap-south-1` (Mumbai — co-located with brokers, lowest latency)
- [x] Create/confirm IAM admin user with MFA; disable root key
- [x] Enable CloudTrail (all regions) and CloudWatch billing alert at $100
- [x] Enable AWS Budgets: monthly budget with alert at 80% and 100%
- [x] Tag strategy: `Project=zettabridge`, `Env=staging|prod`

**Exit:** AWS account is secured, cost visibility is on, CloudTrail is active.

---

## Phase 2 — Network foundation

**VPC layout (ap-south-1, 2 AZs minimum):**

```
VPC: 10.0.0.0/16

Public subnets (ALB + NAT):
  10.0.1.0/24  ap-south-1a
  10.0.2.0/24  ap-south-1b

Private subnets (ECS + RDS + Redis):
  10.0.10.0/24  ap-south-1a
  10.0.11.0/24  ap-south-1b
```

- [x] Create VPC with DNS hostnames enabled
- [x] Create 2 public subnets + Internet Gateway + public route table
- [x] Create 2 private subnets + private route table
- [x] Allocate Elastic IP for NAT Gateway
- [x] Create NAT Gateway in a public subnet; attach NAT EIP; add to private route table
- [x] Security groups:
  - `sg-alb`: inbound 443 from 0.0.0.0/0; inbound 80 for redirect
  - `sg-ecs`: inbound 8080 from `sg-alb` only; outbound 443 to internet (via NAT)
  - `sg-rds`: inbound 5432 from `sg-ecs` only
  - `sg-redis`: inbound 6379 from `sg-ecs` only
- [x] Verify: from a private-subnet EC2/task, `curl https://ifconfig.me` returns NAT EIP (not private IP)

**Exit:** NAT EIP is confirmed stable and known. This is the IP to whitelist with Angel One and Dhan.

---

## Phase 3 — Data services

**RDS PostgreSQL:**
- [x] Engine: PostgreSQL 15 (matches local dev)
- [x] Instance: `db.t4g.micro` for staging, `db.t3.small` for prod
- [x] Multi-AZ: preferred for prod; single-AZ acceptable for staging
- [x] Private subnet group (both private subnets)
- [x] SG: `sg-rds`
- [x] Automated backups: 7-day retention; backup window off-peak IST
- [x] DB name: `zettabridge`; create DB user with limited privileges (not root)
- [x] Note connection string for Secrets Manager: `postgres://USER:PASS@HOST:5432/zettabridge?sslmode=require`
- [x] Run migrations after ECS deploy (Phase 5): `./deploy/scripts/migrate-rds.sh` pointed at new `DATABASE_URL`

**ElastiCache Redis:**
- [x] Engine: Redis 7.x
- [x] Instance: `cache.t4g.micro` for staging
- [x] Private subnet group (both private subnets)
- [x] SG: `sg-redis`
- [x] No auth token for staging (VPC isolation is sufficient); add AUTH token for prod
- [x] Note connection string: `redis://HOST:6379`

**Exit:** App can connect to both services from inside the VPC only. No public endpoints.

---

## Phase 4 — Secrets and config

All secrets that currently live in `runtime.env` on the Lightsail VM move to AWS Secrets Manager.

**Secrets to create (one secret per logical group or one flat secret, either works):**

| Secret name | Contents |
|-------------|---------|
| `zettabridge/staging/core` | `JWT_SECRET`, `AES_KEY`, `METRICS_TOKEN` |
| `zettabridge/staging/database` | `DATABASE_URL` (full connection string) |
| `zettabridge/staging/redis` | `REDIS_URL` |
| `zettabridge/staging/stripe` | `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, all price IDs |
| `zettabridge/staging/broker` | Angel/Dhan/Zerodha platform keys if any; `BROKER_ANGEL_MAC_ADDRESS` |

**KMS:**
- [x] Create a Customer Managed Key (CMK) in `ap-south-1` for Secrets Manager encryption
- [x] Key alias: `alias/zettabridge-staging`
- [x] Key policy: allow ECS task role to `kms:Decrypt` and `kms:GenerateDataKey`

**IAM task role:**
- [x] Create `ECSTaskRole-ZettaBridge` with policy:
  - `secretsmanager:GetSecretValue` on `zettabridge/staging/*`
  - `kms:Decrypt` on CMK ARN
  - `logs:CreateLogStream`, `logs:PutLogEvents` for CloudWatch

Non-secret config (safe to put in ECS task definition env vars directly):
```
APP_ENV=staging
PORT=8080
BROKER_MODE=live           # set after Phase 9 smoke test
BROKER_HTTP_TIMEOUT_SEC=8
BROKER_HTTP_GET_RETRIES=2
BROKER_ANGEL_CLIENT_LOCAL_IP=<NAT-EIP>
BROKER_ANGEL_CLIENT_PUBLIC_IP=<NAT-EIP>
SEBI_ALGO_ID_REQUIRED=false   # auto-true when BROKER_MODE=live
WORKER_COUNT=20
QUEUE_BUFFER=500
EMAIL_VERIFICATION_REQUIRED=false
EMAIL_PROVIDER=log
METRICS_ENABLED=true
BILLING_ENABLED=true
CORS_ORIGINS=https://app.zettabridge.<domain>
APP_PUBLIC_URL=https://api.zettabridge.<domain>
```

**Exit:** No plaintext secrets in the repo, shell history, or image. ECS task role can read all secrets.
