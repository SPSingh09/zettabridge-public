# AWS ECS staging deployment

Script-driven deploy for **Core + Zerodha adapter sidecar** (Phase 4). Paper trading runs cohosted in Core.

## Prerequisites

- AWS CLI configured (`aws sts get-caller-identity`)
- Docker
- Terraform ≥ 1.5
- `jq`, `curl`
- `terraform.tfvars` in `terraform/staging/` (copy from `terraform.tfvars.example`)

One-time env file:

```bash
cp deploy/deploy-aws-ecs/staging-aws.env.example deploy/deploy-aws-ecs/staging-aws.env
# edit STAGING_ADMIN_PASSWORD
```

## Commands (from repo root)

| Script | What it does |
|--------|----------------|
| **`deploy-staging-full.sh`** | Terraform apply → ECS migrations → build/push → deploy all services → smoke |
| **`terraform-apply.sh`** | `terraform init/validate/plan/apply` only |
| **`aws-deploy.sh`** | Build/push ECR image → redeploy Core + adapters + dashboard → smoke |
| **`run-migrations.sh`** | One-off ECS task `MIGRATE_ONLY=true` |
| **`deploy-staging.sh`** | Alias for `aws-deploy.sh` |

### Full staging deploy (recommended)

```bash
./deploy/deploy-aws-ecs/scripts/deploy-staging-full.sh
```

Skip Terraform if infra already applied:

```bash
./deploy/deploy-aws-ecs/scripts/deploy-staging-full.sh --skip-infra
```

App-only redeploy (after code change):

```bash
./deploy/deploy-aws-ecs/scripts/aws-deploy.sh
```

**Clean redeploy** (stop all ECS tasks first — fixes stuck PRIMARY/ACTIVE rollouts):

```bash
./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --reset
```

Or manually:

```bash
./deploy/deploy-aws-ecs/scripts/staging-reset.sh          # scale all to 0
./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --reset   # build/push + start adapters → Core
```

Redeploy only Zerodha adapter (same image tag):

```bash
./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --adapters-only
```

### Terraform only

Terraform updates ECS task definitions (env vars, secrets wiring) but **does not build or push** the application Docker image. After infra-only apply you must redeploy the app or connect-info / adapter routing will stay on stale code.

```bash
./deploy/deploy-aws-ecs/scripts/terraform-apply.sh --plan-only
./deploy/deploy-aws-ecs/scripts/terraform-apply.sh --auto-approve
# infra + image + ECS rollout in one step:
./deploy/deploy-aws-ecs/scripts/terraform-apply.sh --auto-approve --with-deploy
```

Or via wrapper:

```bash
./deploy/deploy-aws-ecs/terraform/tf.sh plan
./deploy/deploy-aws-ecs/terraform/tf.sh apply
```

## What gets deployed

With `execution_adapter_mode=http` and `enabled_adapters=["paper","zerodha"]`:

| ECS service | Image | Notes |
|-------------|-------|--------|
| `zettabridge-staging` | `zettabridge-staging:staging` | Core API + paper engine |
| `zettabridge-staging-zerodha-adapter` | same image, entrypoint `/app/zerodha-adapter` | Live Zerodha |
| `zettabridge-staging-dashboard` | dashboard image | Optional |

Angel/Dhan/MT5 adapters are **not** created unless added to `enabled_adapters` in tfvars.

## Makefile shortcuts

```bash
make staging-deploy-full   # full pipeline
make staging-deploy         # app deploy only
make staging-infra          # terraform apply
make staging-smoke-aws      # smoke only (loads staging-aws.env)
make staging-smoke          # smoke only (export STAGING_* manually)
```

## Logs

```bash
aws logs tail /ecs/zettabridge-staging --follow --region ap-south-1
```
