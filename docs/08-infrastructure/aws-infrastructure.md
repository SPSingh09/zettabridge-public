# AWS Infrastructure
**Last updated: 2026-07-07**

ZettaBridge runs on AWS in the `ap-south-1` (Mumbai) region. All infrastructure is Terraform-managed.

---

## 1. Architecture Overview

```
Internet
    │
    ▼
[ALB — HTTPS 443 / HTTP 80→443 redirect]
    │
    ├── api.* → ECS Fargate: Core API (port 8080)
    ├── exec-zerodha.* → ECS Fargate: zerodha-adapter sidecar (optional)
    └── app.* → ECS Fargate: Dashboard (Next.js, port 3000)
              │
              ├── [ECS Fargate: paper-adapter sidecar] — Cloud Map private DNS
              ├── [RDS PostgreSQL 15] — private subnet
              ├── [ElastiCache Redis 7] — private subnet
              └── [NAT Gateway + Elastic IP] → broker APIs

[Lightsail VM] — Prometheus + Grafana + Alertmanager
    └── scrapes Core GET /metrics via AWS_STAGING_SCRAPE_HOST (api.staging.zettabridge.net)
```

All ECS tasks run in private subnets with no public IPs. The only inbound path is the ALB. The only outbound path for broker calls is the NAT Gateway Elastic IP (static, broker-whitelistable).

---

## 2. Services

| Service | Version / Tier | Purpose |
|---------|---------------|---------|
| ECS Fargate | — | Core API + dashboard + **paper/zerodha adapter sidecars** (Terraform `enabled_adapters`, default `paper,zerodha`) |
| AWS Cloud Map | — | Private DNS for Core → adapter HTTP (`paper-adapter`, `zerodha-adapter`) |
| Lightsail VM | — | **Monitoring stack** — Prometheus scrapes ECS staging; Grafana dashboards |
| RDS PostgreSQL | 15, db.t3.medium | Primary datastore |
| ElastiCache Redis | 7, cache.t3.micro | JWT revocation, dedup, rate limiting |
| ALB | — | HTTPS termination, routing |
| ACM | — | TLS certificate (auto-renewal) |
| NAT Gateway + Elastic IP | — | Static egress for broker API calls |
| ECR | — | Docker image registry |
| Secrets Manager | — | Production secrets (JWT_SECRET, AES_KEY, DATABASE_URL, etc.) |
| KMS CMK | alias/zettabridge-staging | Encrypts Secrets Manager secrets at rest |
| CloudWatch | — | Container logs (`/ecs/zettabridge-staging`) |
| S3 | — | Terraform state backend |
| DynamoDB | — | Terraform state lock |

---

## 3. Networking

### VPC Layout

```
VPC: 10.0.0.0/16
├── Public subnets (2 AZs: ap-south-1a, ap-south-1b)
│   ├── 10.0.1.0/24 (ap-south-1a)
│   ├── 10.0.2.0/24 (ap-south-1b)
│   ├── ALB (internet-facing)
│   └── NAT Gateway + Elastic IP
│
└── Private subnets (2 AZs)
    ├── 10.0.3.0/24 (ap-south-1a)
    ├── 10.0.4.0/24 (ap-south-1b)
    ├── ECS Fargate tasks (Core + Dashboard + adapter sidecars)
    ├── RDS PostgreSQL
    └── ElastiCache Redis
```

### Security Groups

| SG | Inbound | Outbound |
|----|---------|----------|
| ALB | 0.0.0.0/0 :443, :80 | ECS backend :8080, ECS dashboard :3000 |
| ECS backend | ALB SG :8080 | RDS SG :5432, Redis SG :6379, 0.0.0.0/0 :443 (broker APIs) |
| ECS dashboard | ALB SG :3000 | ECS backend SG :8080 |
| RDS | ECS backend SG :5432 | None |
| Redis | ECS backend SG :6379 | None |

---

## 4. IAM

**ECS Task Role** (`ECSTaskRole-ZettaBridge`):

| Permission | Scope |
|-----------|-------|
| `secretsmanager:GetSecretValue` | Specific secret ARNs only |
| `kms:Decrypt` | CMK alias/zettabridge-staging only |
| `logs:CreateLogStream`, `logs:PutLogEvents` | CloudWatch log group |

No other AWS permissions. The task cannot access S3, ECR, DynamoDB, or other services.

---

## 5. Secrets Manager Layout

All production secrets are stored as a single JSON object in Secrets Manager (one secret, multiple keys):

| Key | Type | Used by |
|-----|------|---------|
| `JWT_SECRET` | String (min 32 chars) | JWT signing/verification |
| `AES_KEY` | 32-byte hex string | Broker credential encryption |
| `DATABASE_URL` | PostgreSQL DSN | `store.NewPostgres()` |
| `SMTP_USERNAME` | String | Email delivery |
| `SMTP_PASSWORD` | String | Email delivery |
| `STRIPE_SECRET_KEY` | Stripe restricted key | Billing (disabled in beta) |
| `STRIPE_WEBHOOK_SECRET` | Stripe signing secret | Stripe event verification |
| `ZERODHA_API_KEY` | String | Zerodha OAuth callback |
| `ZERODHA_API_SECRET` | String | Zerodha OAuth callback |
| `ZB_SERVICE_TOKEN` | String | Adapter ↔ Core internal API auth |

ECS task secrets are injected via the `secrets` block in the task definition (not baked into the image).

---

## 6. Terraform Layout

```
deploy/deploy-aws-ecs/terraform/staging/
  main.tf          — no content; resources in separate files
  vpc.tf           — VPC, subnets, IGW, NAT Gateway, route tables
  ecs.tf           — ECS cluster, Core task definition, service
  adapters.tf      — Conditional paper + zerodha adapter sidecars, Cloud Map
  alb.tf           — ALB, listeners, target groups, ACM certificate
  rds.tf           — RDS PostgreSQL instance, subnet group, parameter group
  elasticache.tf   — ElastiCache Redis cluster and subnet group
  ecr.tf           — ECR repository with scan-on-push
  secrets.tf       — Secrets Manager secret + KMS CMK
  iam.tf           — ECS task role + task execution role
  security_groups.tf — All security groups
  dashboard.tf     — Dashboard ECS service
  locals.tf        — Name prefixes, CIDR lists, AZ list
  variables.tf     — Input variables
  outputs.tf       — ALB DNS name, ECR URI, RDS endpoint, etc.
  providers.tf     — AWS provider ~>5.0, region ap-south-1
  versions.tf      — Terraform + provider version locks
  terraform.tfvars         — Active values (gitignored)
  terraform.tfvars.example — Template with all required variables
```

### S3 Backend

```hcl
terraform {
  backend "s3" {
    bucket         = "zettabridge-tfstate"
    key            = "staging/terraform.tfstate"
    region         = "ap-south-1"
    dynamodb_table = "zettabridge-tfstate-lock"
    encrypt        = true
  }
}
```

---

## 7. Terraform Workflow

```bash
cd deploy/deploy-aws-ecs/terraform/staging

# One-time init (or after provider version change)
terraform init

# Plan (review changes before apply)
terraform plan

# Apply
terraform apply

# Destroy staging (careful)
terraform destroy
```

**Key variables** (`terraform.tfvars`):

| Variable | Example | Notes |
|----------|---------|-------|
| `aws_region` | `ap-south-1` | |
| `vpc_cidr` | `10.0.0.0/16` | |
| `broker_mode` | `mock` | `mock` for CI/smoke; `live` for real broker HTTP |
| `execution_adapter_mode` | `http` | `local` or `http` (staging uses `http` + sidecars) |
| `enabled_adapters` | `paper,zerodha` | Comma-separated; Angel/Dhan/MT5 code exists but not provisioned until added here |
| `bootstrap_admin_email` | `admin@example.com` | First admin account |
| `email_provider` | `smtp` | |
| `sebi_algo_id_required` | `true` | Set false for staging without algo IDs |
| `billing_enabled` | `false` | Set true + add price IDs to enable Stripe |
| `cors_origins` | `https://app.staging.zettabridge.net` | |

---

## 8. ECR and Docker Images

```bash
# Authenticate
aws ecr get-login-password --region ap-south-1 | \
  docker login --username AWS --password-stdin <account>.dkr.ecr.ap-south-1.amazonaws.com

# Build and push API server
docker build -t zettabridge-server .
docker tag zettabridge-server:latest <ECR_URI>:latest
docker push <ECR_URI>:latest

# Force ECS to pull the new image
aws ecs update-service \
  --cluster zettabridge-staging \
  --service zettabridge-server \
  --force-new-deployment
```

ECR has scan-on-push enabled. Image vulnerabilities appear in the ECR console.

The staging release script (`deploy/scripts/staging-release.sh`) automates these steps.

---

## 9. Rollback

Immediate rollback for any deployment issue:

```bash
# Option 1: Roll ECS back to the previous task definition revision
aws ecs update-service \
  --cluster zettabridge-staging \
  --service zettabridge-server \
  --task-definition zettabridge-server:<previous-revision>

# Option 2: Switch to mock broker mode (no live calls)
# Set BROKER_MODE=mock in terraform.tfvars → terraform apply
```

---

## 10. DNS and TLS

- DNS is managed by Cloudflare (DNS-only, no proxy/orange-cloud).
- TLS certificate is issued and renewed by AWS ACM, attached to the ALB HTTPS listener.
- ALB redirects HTTP → HTTPS via a listener rule.
- Custom domain: `api.staging.zettabridge.net` → ALB DNS name; `exec-zerodha.staging.zettabridge.net` → zerodha adapter (when enabled).

---

## 11. Monitoring (Lightsail)

Observability runs on a **Lightsail VM**, not inside ECS:

- Prometheus + Grafana + Alertmanager: `deploy/deploy-lightsail/`
- Enable ECS scrape: set `AWS_STAGING_SCRAPE_HOST=api.staging.zettabridge.net` in `monitoring.env`
- Script: `deploy/deploy-aws-ecs/scripts/monitoring-enable-aws-scrape.sh`
- Grafana dashboard: **ZettaBridge Overview** (`deploy/deploy-lightsail/grafana/dashboards/zettabridge.json`)

See [observability.md](../05-operations/observability.md).
