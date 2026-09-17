#!/usr/bin/env bash
# Quick health check for VM monitoring stack (run on Lightsail VM).
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${SCRIPT_DIR}/../compose/monitoring.yml" ]]; then
  BASE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
elif [[ -f "/opt/zettabridge/compose/monitoring.yml" ]]; then
  BASE_DIR="/opt/zettabridge"
else
  BASE_DIR="/opt/zettabridge"
fi

RENDER_DIR="${MONITORING_RENDER_DIR:-${BASE_DIR}/monitoring/rendered}"
COMPOSE_DIR="${BASE_DIR}/compose"

APP_NETWORK="${ZETTABRIDGE_DOCKER_NETWORK:-}"
if [[ -z "${APP_NETWORK}" ]]; then
  for candidate in zettabridge_internal zettabridge_default; do
    if docker network inspect "${candidate}" >/dev/null 2>&1; then
      APP_NETWORK="${candidate}"
      break
    fi
  done
fi

fail=0

section() {
  echo
  echo "==> $*"
}

prom_query() {
  curl -sS -G 'http://127.0.0.1:9090/api/v1/query' --data-urlencode "query=$1"
}

section "Monitoring containers"
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' \
  | grep -E 'NAMES|monitoring|prometheus|grafana|alertmanager' || {
  echo "no monitoring containers running"
  fail=1
}

section "Prometheus ready"
if curl -fsS http://127.0.0.1:9090/-/ready >/dev/null 2>&1; then
  echo "ready"
else
  echo "NOT READY — check: docker compose -f ${COMPOSE_DIR}/monitoring.yml logs prometheus"
  fail=1
fi

section "Rendered prometheus.yml (scrape target)"
if [[ -f "${RENDER_DIR}/prometheus.yml" ]]; then
  grep -A5 'scrape_configs:' "${RENDER_DIR}/prometheus.yml" || true
else
  echo "missing ${RENDER_DIR}/prometheus.yml — run monitoring-up.sh"
  fail=1
fi

section "Prometheus scrape targets"
targets_json="$(curl -sS http://127.0.0.1:9090/api/v1/targets 2>/dev/null || true)"
if [[ -z "${targets_json}" ]]; then
  echo "could not fetch /api/v1/targets"
  fail=1
elif command -v jq >/dev/null 2>&1; then
  count="$(echo "${targets_json}" | jq '.data.activeTargets | length')"
  echo "activeTargets count: ${count}"
  if [[ "${count}" == "0" ]]; then
    echo "WARNING: no scrape targets — re-run monitoring-up.sh"
    fail=1
  else
    echo "${targets_json}" | jq '.data.activeTargets[] | {job: .labels.job, instance: .labels.instance, health, scrapeUrl, lastError}'
  fi
else
  echo "${targets_json}"
fi

section "Prometheus query up{job=\"zettabridge\"}"
up_json="$(prom_query 'up{job="zettabridge"}' 2>/dev/null || true)"
if [[ -z "${up_json}" ]]; then
  echo "query failed"
  fail=1
elif command -v jq >/dev/null 2>&1; then
  echo "${up_json}" | jq '.data.result'
  val="$(echo "${up_json}" | jq -r '.data.result[0].value[1] // empty')"
  if [[ "${val}" != "1" ]]; then
    echo "WARNING: scrape not up (want value 1, got '${val:-none}')"
    echo "         Is the app server running on 127.0.0.1:8080? Run preflight-vm.sh first."
    fail=1
  fi
else
  echo "${up_json}"
fi

section "Prometheus query zettabridge_ingest_total"
ingest_json="$(prom_query 'sum(zettabridge_ingest_total)' 2>/dev/null || true)"
if command -v jq >/dev/null 2>&1 && [[ -n "${ingest_json}" ]]; then
  echo "${ingest_json}" | jq '.data.result'
else
  echo "${ingest_json:-query failed}"
fi

section "App /metrics (host loopback)"
if curl -fsS http://127.0.0.1:8080/metrics 2>/dev/null | grep -E '^zettabridge_ingest_total' | head -5; then
  :
else
  echo "(no zettabridge_ingest metrics on 127.0.0.1:8080 — server may still be starting)"
fi

section "Prometheus container networks"
prom="$(docker ps --format '{{.Names}}' | grep -E 'prometheus' | head -1 || true)"
if [[ -n "${prom}" ]]; then
  nets="$(docker inspect "${prom}" --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}')"
  echo "${nets}"
  if [[ -n "${APP_NETWORK}" ]]; then
    if ! echo "${nets}" | grep -q "${APP_NETWORK}"; then
      echo "WARNING: not on ${APP_NETWORK} — cannot scrape server:8080"
      fail=1
    fi
  else
    echo "WARNING: no zettabridge app network found — is the app stack running?"
    fail=1
  fi
  if ! echo "${nets}" | grep -q 'monitoring'; then
    echo "WARNING: not on monitoring network — Grafana cannot reach Prometheus"
    fail=1
  fi
else
  echo "prometheus container not found"
  fail=1
fi

section "Grafana → Prometheus (from Grafana container)"
graf="$(docker ps --format '{{.Names}}' | grep -E 'grafana' | head -1 || true)"
if [[ -n "${graf}" ]]; then
  if docker exec "${graf}" wget -qO- http://prometheus:9090/-/ready >/dev/null 2>&1; then
    echo "OK"
  else
    echo "FAIL — Grafana cannot reach prometheus:9090"
    fail=1
  fi
else
  echo "grafana container not found"
  fail=1
fi

section "Grafana dashboard file"
if [[ -n "${graf}" ]]; then
  docker exec "${graf}" ls -la /var/lib/grafana/dashboards/ 2>/dev/null || true
fi

if [[ "${fail}" -ne 0 ]]; then
  section "Prometheus logs (last 20 lines)"
  export MONITORING_RENDER_DIR="${RENDER_DIR}"
  compose=(docker compose -f "${COMPOSE_DIR}/monitoring.yml")
  if [[ -f "${COMPOSE_DIR}/monitoring.network.yml" && -n "${APP_NETWORK}" ]]; then
    compose+=(-f "${COMPOSE_DIR}/monitoring.network.yml")
    export ZETTABRIDGE_DOCKER_NETWORK="${APP_NETWORK}"
  fi
  "${compose[@]}" logs prometheus --tail=20 2>/dev/null || true
  echo
  echo "Fix (WSL): ./deploy/scripts/sync-monitoring-to-vm.sh YOUR_IP"
  echo "Fix (VM):  ${BASE_DIR}/scripts/monitoring-down.sh && ${BASE_DIR}/scripts/monitoring-up.sh"
  echo "           ensure app is up first: cd ${BASE_DIR} && ./deploy.sh up"
  exit 1
fi

echo
echo "All checks passed. In browser: Dashboards → ZettaBridge → ZettaBridge Overview"
