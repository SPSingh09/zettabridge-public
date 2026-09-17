# Beta Phase 2 — AWS Container Deploy ✅ COMPLETE

> **Historical implementation plan.** Current product behavior is defined in [STATUS.md](../STATUS.md) and [scope.md](../01-product/scope.md). Last reviewed: 2026-07-07.

**Completed: 2026-06-19**
**Goal:** Backend and dashboard containers running in private ECS subnets, reachable via HTTPS through ALB.

Depends on: [beta-phase-1-aws-foundation.md](beta-phase-1-aws-foundation.md)

---

## Phase 5 — Container registry (ECR)

- [x] Create ECR repository: `zettabridge-backend`
- [x] Create ECR repository: `zettabridge-dashboard`
- [x] Enable image scanning on push (basic scanning, free tier)
- [x] Enable immutable tags (prevents overwriting a released tag)

**Build and push:**
```bash
# Authenticate
aws ecr get-login-password --region ap-south-1 \
  | docker login --username AWS --password-stdin <account>.dkr.ecr.ap-south-1.amazonaws.com

# Backend
docker build -t zettabridge-backend:beta-rc-1 .
docker tag zettabridge-backend:beta-rc-1 \
  <account>.dkr.ecr.ap-south-1.amazonaws.com/zettabridge-backend:beta-rc-1
docker push <account>.dkr.ecr.ap-south-1.amazonaws.com/zettabridge-backend:beta-rc-1

# Dashboard
cd dashboard
docker build -t zettabridge-dashboard:beta-rc-1 .
docker tag zettabridge-dashboard:beta-rc-1 \
  <account>.dkr.ecr.ap-south-1.amazonaws.com/zettabridge-dashboard:beta-rc-1
docker push <account>.dkr.ecr.ap-south-1.amazonaws.com/zettabridge-dashboard:beta-rc-1
```

**Exit:** Both image tags are visible in ECR console with `beta-rc-1` tag.

---

## Phase 6 — ECS services

**Cluster:**
- [x] Create ECS cluster: `zettabridge-staging`
- [x] Capacity: Fargate (serverless, no EC2 to manage)
- [x] Enable Container Insights (CloudWatch)

**CloudWatch log groups:**
- [x] `/ecs/zettabridge-backend`
- [x] `/ecs/zettabridge-dashboard`
- Retention: 30 days

**Backend task definition:**
```
Family:          zettabridge-backend
CPU:             512  (0.5 vCPU)
Memory:          1024 (1 GB)
Network:         awsvpc
Task role:       ECSTaskRole-ZettaBridge
Container:       zettabridge-backend
  Image:         <ecr>/zettabridge-backend:beta-rc-1
  Port:          8080
  Secrets:       DATABASE_URL, REDIS_URL, JWT_SECRET, AES_KEY, STRIPE_* (from Secrets Manager)
  Environment:   APP_ENV, PORT, BROKER_MODE, WORKER_COUNT, CORS_ORIGINS, APP_PUBLIC_URL, ...
  HealthCheck:   curl -f http://localhost:8080/healthz || exit 1
  Logs:          awslogs → /ecs/zettabridge-backend
```

**Dashboard task definition:**
```
Family:          zettabridge-dashboard
CPU:             256
Memory:          512
Container:       zettabridge-dashboard
  Image:         <ecr>/zettabridge-dashboard:beta-rc-1
  Port:          3000
  Environment:   NEXT_PUBLIC_API_URL=https://api.zettabridge.<domain>
                 NEXT_PUBLIC_BILLING_ENABLED=true
                 NEXT_PUBLIC_APP_URL=https://app.zettabridge.<domain>
  HealthCheck:   curl -f http://localhost:3000/ || exit 1
  Logs:          awslogs → /ecs/zettabridge-dashboard
```

**ECS services:**
- [x] Backend service: desired count = 1, rolling update (min 100%, max 200%), private subnets, `sg-ecs`
- [x] Dashboard service: desired count = 1, private subnets, `sg-ecs`
- [x] Wait for both services to reach `RUNNING` state in CloudWatch logs

**Run migrations before opening traffic:**
```bash
# Run from local with DATABASE_URL exported from Secrets Manager
export DATABASE_URL=$(aws secretsmanager get-secret-value \
  --secret-id zettabridge/staging/database --query SecretString --output text | jq -r .DATABASE_URL)
./deploy/scripts/migrate-rds.sh
```

**Exit:** Both ECS services stable and healthy. CloudWatch logs show `startup: broker_mode=... workers=20`.

---

## Phase 7 — ALB, TLS, and DNS

**Target load balancer domains:**
- `api.zettabridge.<domain>` → ECS backend (port 8080)
- `app.zettabridge.<domain>` → ECS dashboard (port 3000)

**ACM certificates:**
- [x] Request certificate for `api.staging.zettabridge.net` (DNS validation via Cloudflare)
- [x] Request certificate for `app.staging.zettabridge.net` (DNS validation via Cloudflare)
- [x] Wait for both to reach `Issued` status

**Application Load Balancer:**
- [x] Create ALB: `zettabridge-staging-alb`, internet-facing, `sg-alb`, both public subnets
- [x] HTTP listener (port 80): redirect all to HTTPS 443
- [x] HTTPS listener (port 443): TLS policy `ELBSecurityPolicy-TLS13-1-2-2021-06` (TLS 1.2+)

**Target groups:**
- [x] `tg-backend`: protocol HTTP, port 8080, health check `GET /healthz` → 200
- [x] `tg-dashboard`: protocol HTTP, port 3000, health check `GET /` → 200

**HTTPS listener rules (in order):**
1. Host header `api.zettabridge.<domain>` → forward to `tg-backend`
2. Host header `app.zettabridge.<domain>` → forward to `tg-dashboard`
3. Default: 404 fixed response

**Attach ECS services to target groups:**
- [x] Backend service: add `tg-backend` as load balancer target
- [x] Dashboard service: add `tg-dashboard` as load balancer target

**DNS (Cloudflare — zettabridge.net uses Cloudflare Registrar, not Route 53):**
- [x] `api.staging.zettabridge.net` → CNAME to ALB DNS (DNS-only, grey cloud)
- [x] `app.staging.zettabridge.net` → CNAME to ALB DNS (DNS-only, grey cloud)

**Verification:**
```bash
curl -s https://api.zettabridge.<domain>/healthz   # → {"status":"ok"}
curl -I https://app.zettabridge.<domain>/           # → 200
curl -I http://api.zettabridge.<domain>/healthz     # → 301 redirect to HTTPS
```

**Exit:** Both domains resolve over HTTPS, health endpoints return 200, HTTP redirects to HTTPS.
