#!/usr/bin/env bash
# Render Prometheus + Alertmanager configs and start the monitoring sidecar.
#
# Usage:
#   cp deploy/env/monitoring.env.example deploy/env/monitoring.env
#   # Edit TELEGRAM_* and GRAFANA_ADMIN_PASSWORD
#   ./deploy/scripts/monitoring-up.sh
#
# On Lightsail VM:
#   cp deploy/env/monitoring.env.example /opt/zettabridge/env/monitoring.env
#   /opt/zettabridge/scripts/monitoring-up.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
if [[ -d "${ROOT_DIR}/prometheus" ]]; then
  ZETTABRIDGE_ROOT="${ZETTABRIDGE_ROOT:-${ROOT_DIR}}"
else
  ZETTABRIDGE_ROOT="${ZETTABRIDGE_ROOT:-$(cd "${ROOT_DIR}/.." && pwd)}"
fi
if [[ -n "${MONITORING_ENV_FILE:-}" ]]; then
  ENV_FILE="${MONITORING_ENV_FILE}"
elif [[ -f "${ROOT_DIR}/env/monitoring.env" ]]; then
  ENV_FILE="${ROOT_DIR}/env/monitoring.env"
else
  ENV_FILE="${ZETTABRIDGE_ROOT}/env/monitoring.env"
fi
RENDER_DIR="${MONITORING_RENDER_DIR:-${ROOT_DIR}/monitoring/rendered}"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "missing ${ENV_FILE} — copy deploy/env/monitoring.env.example" >&2
  exit 1
fi

# shellcheck disable=SC1090
set -a
source "${ENV_FILE}"
set +a

: "${OBS_ENVIRONMENT:=staging}"
: "${OBS_CLUSTER:=lightsail-mumbai}"
: "${OBS_ALERTS_FILE:=zettabridge-staging.yml}"
: "${ZETTABRIDGE_SCRAPE_TARGET:=host.docker.internal:8080}"
: "${GRAFANA_ADMIN_USER:=admin}"
: "${GRAFANA_ADMIN_PASSWORD:=admin}"

# VM (oci.staging): API is published as 127.0.0.1:8080 — Prometheus in Docker cannot
# reach loopback via host.docker.internal. Join the app compose network instead.
ZETTABRIDGE_DOCKER_NETWORK="${ZETTABRIDGE_DOCKER_NETWORK:-}"
if [[ -z "${ZETTABRIDGE_DOCKER_NETWORK}" ]]; then
  for candidate in zettabridge_internal zettabridge_default; do
    if docker network inspect "${candidate}" >/dev/null 2>&1; then
      ZETTABRIDGE_DOCKER_NETWORK="${candidate}"
      break
    fi
  done
fi
if [[ -n "${ZETTABRIDGE_DOCKER_NETWORK}" && "${ZETTABRIDGE_SCRAPE_TARGET}" == "host.docker.internal:8080" ]]; then
  ZETTABRIDGE_SCRAPE_TARGET="server:8080"
  echo "note: using ${ZETTABRIDGE_SCRAPE_TARGET} via docker network ${ZETTABRIDGE_DOCKER_NETWORK}" >&2
fi
export ZETTABRIDGE_SCRAPE_TARGET ZETTABRIDGE_DOCKER_NETWORK

if [[ -z "${TELEGRAM_BOT_TOKEN:-}" || -z "${TELEGRAM_STAGING_CHAT_ID:-}" ]]; then
  echo "note: Telegram unset — Grafana + Prometheus only (alerts not delivered)" >&2
  TELEGRAM_ENABLED=0
else
  TELEGRAM_ENABLED=1
fi
if [[ -z "${TELEGRAM_ONCALL_CHAT_ID:-}" ]]; then
  export TELEGRAM_ONCALL_CHAT_ID="${TELEGRAM_STAGING_CHAT_ID:-0}"
fi

mkdir -p "${RENDER_DIR}"

if ! command -v envsubst >/dev/null 2>&1; then
  echo "envsubst required (install gettext)" >&2
  exit 1
fi

envsubst '${OBS_ENVIRONMENT} ${OBS_CLUSTER} ${OBS_ALERTS_FILE} ${ZETTABRIDGE_SCRAPE_TARGET}' \
  < "${ROOT_DIR}/prometheus/prometheus.yml.template" \
  > "${RENDER_DIR}/prometheus.yml"

# Append AWS staging scrape job when AWS_STAGING_SCRAPE_HOST is set.
# This lets the existing Lightsail Prometheus also scrape the ECS backend over HTTPS
# without standing up a separate monitoring instance inside the VPC.
if [[ -n "${AWS_STAGING_SCRAPE_HOST:-}" ]]; then
  cat >> "${RENDER_DIR}/prometheus.yml" <<EOF

  - job_name: zettabridge-aws-staging
    metrics_path: /metrics
    scheme: https
    static_configs:
      - targets: ["${AWS_STAGING_SCRAPE_HOST}"]
        labels:
          cluster: aws-ecs-staging
EOF
  if [[ -n "${AWS_STAGING_METRICS_TOKEN:-}" ]]; then
    printf '    bearer_token: "%s"\n' "${AWS_STAGING_METRICS_TOKEN}" \
      >> "${RENDER_DIR}/prometheus.yml"
  fi
  echo "note: added aws-staging scrape job → https://${AWS_STAGING_SCRAPE_HOST}/metrics" >&2
fi

if [[ "${TELEGRAM_ENABLED}" == "1" ]]; then
  envsubst '${TELEGRAM_BOT_TOKEN} ${TELEGRAM_STAGING_CHAT_ID} ${TELEGRAM_ONCALL_CHAT_ID}' \
    < "${ROOT_DIR}/alertmanager/alertmanager.yml.template" \
    > "${RENDER_DIR}/alertmanager.yml"
else
  cp "${ROOT_DIR}/alertmanager/alertmanager.noop.yml.template" "${RENDER_DIR}/alertmanager.yml"
fi

export MONITORING_RENDER_DIR="${RENDER_DIR}"
export GRAFANA_ADMIN_USER GRAFANA_ADMIN_PASSWORD

COMPOSE=(docker compose -f "${ROOT_DIR}/compose/monitoring.yml")
if [[ -n "${ZETTABRIDGE_DOCKER_NETWORK}" ]]; then
  COMPOSE+=(-f "${ROOT_DIR}/compose/monitoring.network.yml")
fi
"${COMPOSE[@]}" up -d --force-recreate prometheus grafana

echo
echo "Monitoring stack started (localhost only):"
echo "  Prometheus:    http://127.0.0.1:9090"
echo "  Grafana:       http://127.0.0.1:3001  (user: ${GRAFANA_ADMIN_USER})"
echo "  Alertmanager:  http://127.0.0.1:9093"
echo "  Scrape target: ${ZETTABRIDGE_SCRAPE_TARGET}"
echo "  environment:   ${OBS_ENVIRONMENT}  alerts: ${OBS_ALERTS_FILE}"
echo
echo "SSH tunnel from laptop:  ssh -N -L 3001:127.0.0.1:3001 ubuntu@YOUR_VM"
echo "Docs: docs/observability.md"
