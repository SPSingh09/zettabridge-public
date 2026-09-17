#!/usr/bin/env bash
# Copy monitoring stack files to a Lightsail/OCI VM (/opt/zettabridge layout).
#
# Usage (from repo root on WSL):
#   ./deploy/scripts/sync-monitoring-to-vm.sh 52.66.136.127
#
# Then on VM:
#   cp /opt/zettabridge/env/monitoring.env.example /opt/zettabridge/env/monitoring.env
#   nano /opt/zettabridge/env/monitoring.env
#   /opt/zettabridge/scripts/monitoring-up.sh

set -euo pipefail

HOST="${1:-${MONITORING_SYNC_HOST:-52.66.136.127}}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REMOTE="${MONITORING_SYNC_USER:-ubuntu}@${HOST}"

echo "==> Creating directories on ${REMOTE}"
ssh "${REMOTE}" 'mkdir -p /opt/zettabridge/{prometheus/alerts,alertmanager,grafana/{dashboards,provisioning/datasources,provisioning/dashboards},compose,scripts,env,monitoring/rendered}'

echo "==> Copying prometheus, alertmanager, grafana"
scp -r "${ROOT}/prometheus/"* "${REMOTE}:/opt/zettabridge/prometheus/"
scp -r "${ROOT}/alertmanager/"* "${REMOTE}:/opt/zettabridge/alertmanager/"
scp -r "${ROOT}/grafana/"* "${REMOTE}:/opt/zettabridge/grafana/"

echo "==> Copying compose + scripts + env example"
scp "${ROOT}/compose/monitoring.yml" "${REMOTE}:/opt/zettabridge/compose/monitoring.yml"
scp "${ROOT}/compose/monitoring.network.yml" "${REMOTE}:/opt/zettabridge/compose/monitoring.network.yml"
scp "${ROOT}/env/monitoring.env.example" "${REMOTE}:/opt/zettabridge/env/monitoring.env.example"
scp "${ROOT}/scripts/monitoring-up.sh" \
    "${ROOT}/scripts/monitoring-down.sh" \
    "${ROOT}/scripts/monitoring-test-alert.sh" \
    "${ROOT}/scripts/monitoring-doctor.sh" \
    "${ROOT}/scripts/monitoring-grafana-reset.sh" \
    "${ROOT}/scripts/staging-metrics-smoke.sh" \
    "${ROOT}/scripts/preflight-vm.sh" \
    "${ROOT}/scripts/vm-disk-cleanup.sh" \
    "${REMOTE}:/opt/zettabridge/scripts/"

ssh "${REMOTE}" 'chmod +x /opt/zettabridge/scripts/monitoring-*.sh /opt/zettabridge/scripts/staging-*.sh /opt/zettabridge/scripts/preflight-vm.sh /opt/zettabridge/scripts/vm-disk-cleanup.sh 2>/dev/null || true'

echo
echo "Done. On the VM:"
echo "  cp /opt/zettabridge/env/monitoring.env.example /opt/zettabridge/env/monitoring.env"
echo "  chmod 600 /opt/zettabridge/env/monitoring.env"
echo "  nano /opt/zettabridge/env/monitoring.env   # TELEGRAM_*, GRAFANA_ADMIN_PASSWORD"
echo "  /opt/zettabridge/scripts/monitoring-up.sh"
