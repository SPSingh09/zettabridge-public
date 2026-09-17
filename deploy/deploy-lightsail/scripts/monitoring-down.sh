#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RENDER_DIR="${MONITORING_RENDER_DIR:-${ROOT_DIR}/monitoring/rendered}"

export MONITORING_RENDER_DIR="${RENDER_DIR}"

COMPOSE=(docker compose -f "${ROOT_DIR}/compose/monitoring.yml")
if [[ -n "${ZETTABRIDGE_DOCKER_NETWORK:-}" ]]; then
  COMPOSE+=(-f "${ROOT_DIR}/compose/monitoring.network.yml")
elif docker network inspect zettabridge_internal >/dev/null 2>&1; then
  COMPOSE+=(-f "${ROOT_DIR}/compose/monitoring.network.yml")
  export ZETTABRIDGE_DOCKER_NETWORK=zettabridge_internal
elif docker network inspect zettabridge_default >/dev/null 2>&1; then
  COMPOSE+=(-f "${ROOT_DIR}/compose/monitoring.network.yml")
  export ZETTABRIDGE_DOCKER_NETWORK=zettabridge_default
fi
"${COMPOSE[@]}" down

echo "Monitoring stack stopped."
