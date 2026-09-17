#!/usr/bin/env bash
# Reset Grafana state so provisioned datasource UID + dashboard match.
# Run on VM after sync when panels show "No data" but Prometheus queries work.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
RENDER_DIR="${MONITORING_RENDER_DIR:-${ROOT_DIR}/monitoring/rendered}"

export MONITORING_RENDER_DIR="${RENDER_DIR}"

COMPOSE=(docker compose -f "${ROOT_DIR}/compose/monitoring.yml")
if [[ -f "${ROOT_DIR}/compose/monitoring.network.yml" ]] \
  && docker network inspect zettabridge_internal >/dev/null 2>&1; then
  COMPOSE+=(-f "${ROOT_DIR}/compose/monitoring.network.yml")
  export ZETTABRIDGE_DOCKER_NETWORK="${ZETTABRIDGE_DOCKER_NETWORK:-zettabridge_internal}"
fi

echo "==> Stopping monitoring stack"
"${COMPOSE[@]}" down

echo "==> Removing Grafana volume (cached datasource UIDs / old dashboards)"
vol="$(docker volume ls --format '{{.Name}}' | grep -E 'grafana_data$' | head -1 || true)"
if [[ -n "${vol}" ]]; then
  docker volume rm "${vol}"
else
  echo "no grafana_data volume found (ok on first run)"
fi

echo "==> Starting monitoring stack"
"${SCRIPT_DIR}/monitoring-up.sh"

echo
echo "Grafana reset complete."
echo "  1. SSH tunnel: ssh -N -L 3000:127.0.0.1:3000 ubuntu@YOUR_IP"
echo "  2. Open http://localhost:3000 → Dashboards → ZettaBridge → ZettaBridge Overview"
echo "  3. Hard refresh browser (Ctrl+Shift+R)"
