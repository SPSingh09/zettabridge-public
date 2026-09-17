# Observability — Grafana + Prometheus + Alertmanager
**Last updated: 2026-07-07**

ZettaBridge exposes Prometheus metrics at `GET /metrics`. This document covers the VM sidecar monitoring stack used in staging and the AWS ECS path for production.

---

## 1. Metrics Reference

| Metric | Labels | Meaning |
|--------|--------|---------|
| `zettabridge_ingest_total` | `status` (`queued`, `deduplicated`, `rejected`, `rate_limited`, `queue_full`) | Signals received at ingest |
| `zettabridge_trades_total` | `status`, `error_code`, `broker_type` | Worker-persisted trade outcomes |
| `zettabridge_dedup_hits_total` | — | Duplicate signals suppressed within dedup window |
| `zettabridge_broker_http_duration_seconds` | `broker`, `op` | Outbound broker HTTP latency histogram |
| `zettabridge_billing_checkout_total` | — | Checkout sessions created |
| `zettabridge_billing_webhook_total` | `status` | Stripe webhook requests by outcome |

Labels are low-cardinality — no `user_id` or `symbol` to avoid metric explosion.

### Grafana dashboard mapping (ZettaBridge Overview)

| Panel | PromQL source | Load-test signal |
|-------|---------------|------------------|
| Ingest accepted (in range) | `increase(zettabridge_ingest_total{status="queued"}[$__range])` | HTTP 202 accepted into worker queue (formerly labeled “queued”) |
| Ingest rejected (in range) | `increase(...{status="rejected"}[$__range])` | guard/validation only; 429 → `rate_limited` |
| Trades submitted (in range) | `increase(zettabridge_trades_total{status="submitted"}[$__range])` | mock broker smoke/target; publisher callback transitions |
| Trades rejected (in range) | `increase(...{status="rejected"}[$__range])` | worker/gateway rejections; flat on mock smoke |
| Trades persisted /s | `rate(zettabridge_trades_total[5m])` (all statuses) | live rate during run |
| Dedup hits (in range) | `increase(zettabridge_dedup_hits_total[$__range])` | dedup load test |
| Ingest deduplicated (in range) | `increase(...{status="deduplicated"}[$__range])` | HTTP 409 at ingest |
| Ingest rate by status | stacked `rate(zettabridge_ingest_total[5m])` + `dedup_hits` | live breakdown |
| Broker HTTP latency | `zettabridge_broker_http_duration_seconds` | Core → adapter (`http`) or in-process Kite (`local`) |

Use the **Cluster** variable (`lightsail-mumbai` vs `aws-ecs-staging`) when Prometheus scrapes both VM and ECS.

---

## 2. Stack Layout

```
deploy/deploy-lightsail/
  prometheus/prometheus.yml.template
  prometheus/alerts/zettabridge-staging.yml
  prometheus/alerts/zettabridge-production.yml
  alertmanager/alertmanager.yml.template
  grafana/dashboards/zettabridge.json
  grafana/provisioning/
  compose/monitoring.yml
  scripts/monitoring-up.sh
  env/monitoring.env.example
```

---

## 3. VM Sidecar Setup (Staging)

### Prerequisites

- ZettaBridge running with `METRICS_ENABLED=true` (default)
- API bound to the docker internal network so Prometheus can scrape `server:8080`
- Docker + Compose on the VM

### Step 1 — Configure Telegram

1. Create a bot with [@BotFather](https://t.me/BotFather) → save **bot token**.
2. Create a Telegram group **"ZettaBridge staging alerts"** → add the bot → send a message.
3. Get chat ID: `https://api.telegram.org/bot<TOKEN>/getUpdates` → note `chat.id`.
4. (Production) Create a separate **"ZettaBridge oncall"** group with the same bot.

### Step 2 — Sync monitoring files

From WSL (repo root):

```bash
./deploy/scripts/sync-monitoring-to-vm.sh YOUR_VM_STATIC_IP
```

On the VM:

```bash
cp /opt/zettabridge/env/monitoring.env.example /opt/zettabridge/env/monitoring.env
chmod 600 /opt/zettabridge/env/monitoring.env
```

Edit `/opt/zettabridge/env/monitoring.env`:

```bash
OBS_ENVIRONMENT=staging
OBS_CLUSTER=aws-ecs-ap-south-1
OBS_ALERTS_FILE=zettabridge-staging.yml
ZETTABRIDGE_SCRAPE_TARGET=server:8080

TELEGRAM_BOT_TOKEN=123456:ABC...
TELEGRAM_STAGING_CHAT_ID=-1001234567890
TELEGRAM_ONCALL_CHAT_ID=-1001234567890   # same as staging until production

GRAFANA_ADMIN_PASSWORD=choose-a-strong-password
MONITORING_RENDER_DIR=/opt/zettabridge/monitoring/rendered
```

### Step 3 — Start monitoring

```bash
/opt/zettabridge/scripts/monitoring-up.sh
```

Services bind localhost only (do not expose to public IP):

| Service | URL |
|---------|-----|
| Grafana | http://127.0.0.1:3001 |
| Prometheus | http://127.0.0.1:9090 |
| Alertmanager | http://127.0.0.1:9093 |

### Step 4 — Access via SSH tunnel

From WSL:

```bash
ssh -N -L 3001:127.0.0.1:3001 -L 9090:127.0.0.1:9090 ubuntu@<VM_IP>
```

Open http://localhost:3001 → login → dashboard **ZettaBridge Overview**.

### Step 5 — Verify scrape

```bash
curl -sS http://127.0.0.1:9090/api/v1/targets | \
  jq '.data.activeTargets[] | {job: .labels.job, health, lastError}'
# health should be "up"

curl -sS http://127.0.0.1:8080/metrics | head
```

Generate traffic to populate panels:

```bash
./deploy/scripts/staging-smoke.sh
```

Grafana ingest/trade panels should update within ~1 minute.

**Troubleshooting: Scrape shows DOWN**

Prometheus runs in Docker and cannot reach the host loopback via `localhost`. Re-sync and restart:

```bash
./deploy/scripts/sync-monitoring-to-vm.sh YOUR_VM_STATIC_IP
/opt/zettabridge/scripts/monitoring-down.sh
/opt/zettabridge/scripts/monitoring-up.sh
```

### Step 6 — Test Telegram alerting

```bash
/opt/zettabridge/scripts/monitoring-test-alert.sh
```

Check the staging Telegram group for `ZettaBridgeTestAlert`.

**Down alert test** (pages staging chat — use sparingly):

```bash
cd /opt/zettabridge
docker compose -f compose/production.base.yml -f compose/oci.staging.yml stop server
# wait ~2–3 min → ZettaBridgeDown fires in Telegram
docker compose -f compose/production.base.yml -f compose/oci.staging.yml start server
```

---

## 4. Stop / Restart Monitoring

```bash
/opt/zettabridge/scripts/monitoring-down.sh
/opt/zettabridge/scripts/monitoring-up.sh
```

---

## 5. Alert Routing

Prometheus external labels drive alert routing:

```yaml
external_labels:
  environment: staging   # or production
  cluster: aws-ecs-ap-south-1
```

| `environment` | Telegram receiver |
|---------------|-------------------|
| `staging` | `telegram-staging` → staging group |
| `production` | `telegram-oncall` → oncall group |

---

## 6. Alert Rules Summary

| Alert | Staging threshold | Production threshold |
|-------|------------------|----------------------|
| `ZettaBridgeDown` | 2 min | 2 min |
| `IngestRejectSpike` | >50% for 15 min | >20% for 5 min |
| `TradeAuthFailedSpike` | >0.5/s for 10 min | >0.1/s for 5 min |
| `TradeRateLimited` | >1/s for 15 min | >0.5/s for 5 min |
| `BrokerHTTPLatencyHigh` | p95 >10 s for 10 min | p95 >5 s for 5 min |
| `DedupStorm` | info, 15 min | info, 10 min |

Full rules in `deploy/prometheus/alerts/zettabridge-staging.yml` and `zettabridge-production.yml`.

Alert response procedures: see [runbook.md](runbook.md).

---

## 7. AWS ECS (Production Path)

When Terraform staging/prod is applied, reuse the same dashboard and alert files:

1. Update `monitoring.env`:

   ```bash
   OBS_ENVIRONMENT=production
   OBS_CLUSTER=aws-ecs-ap-south-1
   OBS_ALERTS_FILE=zettabridge-production.yml
   ZETTABRIDGE_SCRAPE_TARGET=<ECS-private-IP-or-service-discovery>:8080
   ```

2. If `METRICS_TOKEN` is set in Secrets Manager, add `bearer_token` to the Prometheus scrape config template.

3. Run Prometheus/Grafana on a VPC-internal host (sidecar EC2, ECS task, or Amazon Managed Prometheus).

4. Route `environment=production` alerts to the oncall Telegram chat only.
