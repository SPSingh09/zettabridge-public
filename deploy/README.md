# Deployment

ZettaBridge runs as **one Docker image** everywhere. Cloud choice is wiring only — see [docs/plans/pluggable-deployment.md](../docs/plans/pluggable-deployment.md).

| Platform | Path | When to use |
|----------|------|-------------|
| **Local / CI** | [`docker-compose.yml`](../docker-compose.yml) | Dev, `make functional-test` |
| **OCI Always Free** | [`oci/`](oci/) | $0 staging / early beta |
| **Lightsail / VPS + GHCR** | [`lightsail/`](lightsail/) | Private repo, pull image (no build on VM) |
| **Any VM bootstrap** | [`vm/bootstrap.sh`](vm/bootstrap.sh) | Docker + packages (run once on fresh Ubuntu) |
| **AWS managed** | [`terraform/staging/`](terraform/staging/) | Scale, load tests, managed RDS/Redis |

## Shared pieces

| Path | Purpose |
|------|---------|
| [`env/runtime.env.example`](env/runtime.env.example) | Canonical env vars (both clouds) |
| [`scripts/migrate-rds.sh`](scripts/migrate-rds.sh) | Apply SQL migrations to any Postgres |
| [`scripts/check-env.sh`](scripts/check-env.sh) | Pre-deploy validation |
| [`scripts/staging-smoke.sh`](scripts/staging-smoke.sh) | HTTP smoke test against staging URL |
| [`../functional-tester/run-functional-staging.sh`](../functional-tester/run-functional-staging.sh) | Full API functional suite against staging |
| [`scripts/monitoring-up.sh`](scripts/monitoring-up.sh) | Prometheus + Grafana + Alertmanager sidecar |
| [`grafana/dashboards/zettabridge.json`](grafana/dashboards/zettabridge.json) | Grafana dashboard (4A.3) |
| [`prometheus/alerts/`](prometheus/alerts/) | Alert rules — staging vs production (4A.4) |

## Observability (4A.3 + 4A.4)

VM sidecar on Lightsail/OCI — localhost-only Grafana, Telegram alerts by `environment` label:

```bash
# From laptop (repo root)
./deploy/scripts/sync-monitoring-to-vm.sh YOUR_STATIC_IP

# On VM
cp /opt/zettabridge/env/monitoring.env.example /opt/zettabridge/env/monitoring.env
# Edit TELEGRAM_* + GRAFANA_ADMIN_PASSWORD
/opt/zettabridge/scripts/monitoring-up.sh
```

Full guide: [docs/observability.md](../docs/observability.md)

## Staging smoke (Tier 1)

From WSL after deploy:

```bash
export STAGING_BASE_URL=http://YOUR_STATIC_IP
export STAGING_ADMIN_EMAIL=admin@zettaflux.com
export STAGING_ADMIN_PASSWORD='your-admin-password'
./deploy/scripts/staging-smoke.sh
```

Or: `make staging-smoke` (same env vars).

## Staging functional suite (Tier 2)

Full API regression (~44 endpoints) against staging — no local Docker:

```bash
export STAGING_BASE_URL=http://YOUR_STATIC_IP
export STAGING_ADMIN_EMAIL=admin@zettaflux.com
export STAGING_ADMIN_PASSWORD='your-admin-password'
./functional-tester/run-functional-staging.sh
```

Or: `make functional-test-staging`. For `BROKER_MODE=live` staging, add `STAGING_RUN_LIVE_ROUTING=1`.

**Note:** Creates many ephemeral users/orgs on the staging DB — use a disposable staging environment only.

## Lightsail + private GHCR

Full guide: [lightsail/README.md](lightsail/README.md)

## OCI quick start

```bash
# On the VM (after setup-vm.sh)
cp deploy/env/oci.staging.example /opt/zettabridge/env/runtime.env
./deploy/scripts/check-env.sh /opt/zettabridge/env/runtime.env
./deploy/oci/deploy.sh build
./deploy/oci/deploy.sh up
./deploy/oci/deploy.sh migrate
curl -sS http://127.0.0.1:8080/healthz
```

Full runbook: [oci/README.md](oci/README.md)

## AWS quick start

```bash
cd deploy/terraform/staging && terraform plan
./deploy/terraform/tf.sh plan
export DATABASE_URL='postgres://...'
./deploy/scripts/migrate-rds.sh
```

## OCI → AWS migration

Operator checklist: [docs/plans/pluggable-deployment.md#migration-oci--aws-operator-runbook](../docs/plans/pluggable-deployment.md)

**Critical:** copy the same `AES_KEY` — broker credentials are encrypted with it.
