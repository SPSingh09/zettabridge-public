# ZettaBridge AWS Terraform

> **Run Terraform from `staging/`, not this directory.**  
> If you see `initialized in an empty directory`, you are in the wrong folder.

```bash
cd deploy/terraform/staging   # required
terraform init
terraform validate
terraform apply
```

Or from repo root, use the wrapper (forwards all args to `staging/`):

```bash
./deploy/terraform/tf.sh init
./deploy/terraform/tf.sh validate
./deploy/terraform/tf.sh plan
```

Staging-only scaffold with **smallest practical instance sizes** and **one NAT Gateway** (budget-friendly).

**Plan:** [docs/plans/phase-5a-staging-infra.md](../../docs/plans/phase-5a-staging-infra.md)

## What this creates (staging)

| Resource | Size / notes |
|----------|----------------|
| VPC | 2 AZs, 1 NAT Gateway + Elastic IP (~$35/mo) |
| RDS PostgreSQL 16 | `db.t4g.micro`, 20 GB gp3, single-AZ |
| ElastiCache Redis 7 | `cache.t4g.micro`, 1 node |
| ECS Fargate | 1 task, 0.5 vCPU / 1 GB |
| ALB | HTTP and/or HTTPS (ACM + Route53 optional) |
| ECR | `zettabridge-staging` repository |
| Secrets Manager | `JWT_SECRET`, `AES_KEY`, `DATABASE_URL`, optional `METRICS_TOKEN` |

## Prerequisites

1. AWS account + [AWS CLI](https://aws.amazon.com/cli/) configured (`aws configure`)
2. [Terraform](https://developer.hashicorp.com/terraform/install) ≥ 1.5
3. Docker (to build and push the app image to ECR)
4. **Optional for HTTPS:** a domain + Route53 hosted zone (`api.staging.yourdomain.com`)

### AWS credentials (fix `No valid credential sources found`)

Terraform uses the **same credentials as the AWS CLI**. On your laptop/WSL there is no EC2 metadata service (IMDS), so you must configure credentials explicitly.

**1. Install AWS CLI v2** (if missing):

```bash
aws --version
```

**2. Create an IAM user** (AWS Console → IAM → Users → Create):

- Programmatic access only (no console required for Terraform)
- Attach policy: **`AdministratorAccess`** for first staging apply (or a tighter custom policy later)
- Save **Access key ID** + **Secret access key** (shown once)

**3. Configure the CLI:**

```bash
aws configure
# AWS Access Key ID:     AKIA...
# AWS Secret Access Key: ...
# Default region name:   ap-south-1
# Default output format: json
```

**4. Verify before Terraform:**

```bash
aws sts get-caller-identity
```

You should see your account ID and user ARN. If this fails, Terraform will fail too.

**5. Re-run plan:**

```bash
cd deploy/terraform/staging
terraform plan
```

**Named profile** (optional):

```bash
aws configure --profile zettabridge
export AWS_PROFILE=zettabridge
terraform plan
```

**SSO / IAM Identity Center** (if your org uses it):

```bash
aws configure sso
aws sso login --profile your-profile
export AWS_PROFILE=your-profile
terraform plan
```

**Environment variables** (CI or temporary):

```bash
export AWS_ACCESS_KEY_ID=AKIA...
export AWS_SECRET_ACCESS_KEY=...
export AWS_REGION=ap-south-1
terraform plan
```

Never commit access keys to git. Rotate keys if exposed.

**Billing guardrail (recommended):** AWS Console → Billing → **Budgets** → create alert at $20–50/month while learning.

## Quick start

```bash
cd deploy/terraform/staging

cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars (region, domain, bootstrap email, etc.)

terraform init
terraform plan
terraform apply
```

### 1. Push container image (before or right after first apply)

```bash
# From repo root — use outputs from: terraform output ecr_repository_url
AWS_REGION=ap-south-1
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
ECR="${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/zettabridge-staging"

aws ecr get-login-password --region "$AWS_REGION" | \
  docker login --username AWS --password-stdin "${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"

docker build -t zettabridge:staging .
docker tag zettabridge:staging "${ECR}:staging"
docker push "${ECR}:staging"

# Force ECS to pull the new image
aws ecs update-service \
  --cluster zettabridge-staging \
  --service zettabridge-staging \
  --force-new-deployment \
  --region "$AWS_REGION"
```

### 2. Run database migrations

```bash
# DATABASE_URL is in Secrets Manager — fetch once for migrate (do not commit)
aws secretsmanager get-secret-value \
  --secret-id zettabridge/staging/app \
  --query SecretString --output text | jq -r .DATABASE_URL

export DATABASE_URL='postgres://...'
../../scripts/migrate-rds.sh
```

### 3. Smoke test

```bash
terraform output api_url
curl "$(terraform output -raw api_url)/healthz"
```

### 4. NAT egress IP (broker whitelist)

```bash
terraform output nat_public_ip
# Register at Dhan / Angel / Zerodha; set matches BROKER_ANGEL_CLIENT_* on redeploy
# (Terraform sets these from the NAT EIP automatically)
```

## HTTPS options

| Mode | Set in `terraform.tfvars` |
|------|---------------------------|
| **HTTP only** (no domain yet) | `enable_https = false` |
| **HTTPS + ACM** | `enable_https = true`, `api_domain_name`, `route53_zone_id` |
| **Bring your own cert** | `enable_https = true`, `acm_certificate_arn = "arn:aws:acm:..."` |

After HTTPS is enabled, enable HSTS on the ALB (one-time):

```bash
ALB_ARN=$(terraform output -raw alb_arn)
aws elbv2 modify-load-balancer-attributes \
  --load-balancer-arn "$ALB_ARN" \
  --attributes \
    Key=routing.http.response.strict_transport_security.enabled,Value=true \
    Key=routing.http.response.strict_transport_security.header_value,Value="max-age=31536000; includeSubDomains"
```

## Budget tips

- **Staging only** — do not apply prod until beta-ready (~$90–120/mo).
- **One NAT** — already configured (largest fixed cost).
- **Stop ECS service** when not testing: set `ecs_desired_count = 0` in tfvars and `terraform apply` (RDS/Redis/NAT still bill).
- **Free tier:** new AWS accounts may get partial RDS/ElastiCache credits — check Billing → Free tier.

## State backend (recommended before team use)

Default is **local** `terraform.tfstate`. For shared state, uncomment the S3 backend in `staging/versions.tf` and create:

- S3 bucket (versioning on)
- DynamoDB table for state lock

## Layout

```
deploy/
  scripts/migrate-rds.sh
  terraform/
    README.md
    staging/          ← apply here
      *.tf
      terraform.tfvars.example
```

## Destroy staging (stop charges)

```bash
cd deploy/terraform/staging
terraform destroy
```

Release the Elastic IP if destroy fails on dependencies — check EC2 → Elastic IPs.

## Related

- [deploy-production.md](../../docs/deploy-production.md) — env reference and rollout
- [operations-security.md](../../docs/operations-security.md) — NAT verification
