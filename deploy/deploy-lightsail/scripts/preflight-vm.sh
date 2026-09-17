#!/usr/bin/env bash
# VM preflight — restart app/redis, verify health, metrics, and Prometheus scrape.
# Run on the Lightsail/OCI VM before WSL smoke / functional / load / pen tests.
#
# Usage (on VM):
#   /opt/zettabridge/scripts/preflight-vm.sh
#   /opt/zettabridge/scripts/preflight-vm.sh --skip-restart
#   /opt/zettabridge/scripts/preflight-vm.sh --reset-prometheus
#   /opt/zettabridge/scripts/preflight-vm.sh --fix-monitoring
#
# From WSL (after scp or sync):
#   ssh ubuntu@STATIC_IP '/opt/zettabridge/scripts/preflight-vm.sh'

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${SCRIPT_DIR}/../compose/production.base.yml" ]]; then
  BASE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
elif [[ -f "/opt/zettabridge/compose/production.base.yml" ]]; then
  BASE_DIR="/opt/zettabridge"
  SCRIPT_DIR="${BASE_DIR}/scripts"
else
  echo "compose files not found — expected ${SCRIPT_DIR}/../compose or /opt/zettabridge/compose" >&2
  exit 1
fi

COMPOSE_DIR="${BASE_DIR}/compose"
ENV_FILE="${ZETTABRIDGE_ENV_FILE:-${BASE_DIR}/env/runtime.env}"
GHCR_ENV="${ZETTABRIDGE_GHCR_ENV:-${BASE_DIR}/env/ghcr.env}"
SKIP_RESTART=0
RESET_PROMETHEUS=0
FIX_MONITORING=0
REQUIRE_MONITORING=0

for arg in "$@"; do
  case "${arg}" in
    --skip-restart) SKIP_RESTART=1 ;;
    --reset-prometheus) RESET_PROMETHEUS=1 ;;
    --fix-monitoring) FIX_MONITORING=1 ;;
    --require-monitoring) REQUIRE_MONITORING=1 ;;
    -h|--help)
      cat <<EOF
usage: $0 [options]

  --skip-restart        do not restart redis/server
  --reset-prometheus    wipe Prometheus TSDB and restart monitoring
  --fix-monitoring      run monitoring-down.sh && monitoring-up.sh
  --require-monitoring  fail if monitoring-doctor fails (default: warn only)
EOF
      exit 0
      ;;
    *)
      echo "unknown argument: ${arg}" >&2
      exit 1
      ;;
  esac
done

COMPOSE=(docker compose -f "${COMPOSE_DIR}/production.base.yml" -f "${COMPOSE_DIR}/oci.staging.yml")
LOCAL_API="${LOCAL_API:-http://127.0.0.1:8080}"
LOCAL_METRICS="${LOCAL_METRICS:-${LOCAL_API}/metrics}"
WAIT_SECS="${PREFLIGHT_WAIT_SECS:-90}"

failures=0
ok() { echo "    OK: $*"; }
warn() { echo "    WARN: $*" >&2; }
fail() { failures=$((failures + 1)); echo "    FAIL: $*" >&2; }

section() { echo; echo "==> $*"; }

load_runtime_env() {
  if [[ ! -f "${ENV_FILE}" ]]; then
    echo "missing ${ENV_FILE}" >&2
    exit 1
  fi
  # shellcheck disable=SC1090
  set -a
  source "${ENV_FILE}"
  set +a
  export POSTGRES_PASSWORD
}

load_compose_env() {
  load_runtime_env
  if [[ -f "${GHCR_ENV}" ]]; then
    # shellcheck disable=SC1090
    set -a
    source "${GHCR_ENV}"
    set +a
    export ZETTABRIDGE_IMAGE
  fi
}

http_code() {
  local url="$1"
  local method="${2:-GET}"
  local body="${3:-}"
  local code
  if [[ "${method}" == "POST" ]]; then
    code="$(curl -4 -sS -m 15 -o /dev/null -w '%{http_code}' -X POST "${url}" \
      -H 'Content-Type: application/json' \
      -d "${body}" 2>/dev/null)" || code=""
  else
    code="$(curl -4 -sS -m 15 -o /dev/null -w '%{http_code}' "${url}" 2>/dev/null)" || code=""
  fi
  echo "${code:-000}"
}

wait_for_server() {
  local elapsed=0
  while (( elapsed < WAIT_SECS )); do
    local code
    code="$(http_code "${LOCAL_API}/healthz")"
    if [[ "${code}" == "200" ]]; then
      ok "server ready after ${elapsed}s"
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  fail "server not ready after ${WAIT_SECS}s (last healthz HTTP $(http_code "${LOCAL_API}/healthz"))"
  return 1
}

read_ingest_metric() {
  local status="$1"
  curl -4 -fsS -m 10 "${LOCAL_METRICS}" 2>/dev/null \
    | awk -v s="${status}" '$1 ~ /^zettabridge_ingest_total/ && $0 ~ s {print $2; exit}' \
    || echo "0"
}

section "ZettaBridge VM preflight"
echo "  BASE_DIR=${BASE_DIR}"
echo "  skip_restart=${SKIP_RESTART} reset_prometheus=${RESET_PROMETHEUS} fix_monitoring=${FIX_MONITORING}"

load_compose_env

if [[ "${FIX_MONITORING}" == "1" ]]; then
  section "Fix monitoring stack (down + up)"
  if [[ -x "${SCRIPT_DIR}/monitoring-down.sh" && -x "${SCRIPT_DIR}/monitoring-up.sh" ]]; then
    "${SCRIPT_DIR}/monitoring-down.sh"
    "${SCRIPT_DIR}/monitoring-up.sh"
    sleep 3
  else
    fail "monitoring scripts missing — run sync-monitoring-to-vm.sh from WSL"
  fi
fi

if [[ "${RESET_PROMETHEUS}" == "1" ]]; then
  section "Reset Prometheus TSDB (optional fresh charts)"
  if [[ -x "${SCRIPT_DIR}/monitoring-down.sh" ]]; then
    "${SCRIPT_DIR}/monitoring-down.sh"
    vol="$(docker volume ls --format '{{.Name}}' | grep -E 'prometheus_data$' | head -1 || true)"
    if [[ -n "${vol}" ]]; then
      docker volume rm "${vol}"
      ok "removed volume ${vol}"
    else
      warn "no prometheus_data volume found"
    fi
    "${SCRIPT_DIR}/monitoring-up.sh"
    sleep 3
  else
    fail "monitoring-down.sh not found — sync scripts to VM first"
  fi
fi

if [[ "${SKIP_RESTART}" == "0" ]]; then
  section "Restart redis + server (fresh metrics, dedup, rate-limit windows)"
  if ! "${COMPOSE[@]}" restart redis server; then
    fail "docker compose restart failed — try: cd ${BASE_DIR} && ./deploy.sh up"
  fi
  section "Wait for server /healthz"
  wait_for_server || true
else
  section "Skipping restart (--skip-restart)"
fi

section "App containers"
if ! "${COMPOSE[@]}" ps --status running 2>/dev/null | grep -q server; then
  fail "server container not running — run: cd ${BASE_DIR} && ./deploy.sh up"
else
  ok "server container running"
fi
"${COMPOSE[@]}" ps --format 'table {{.Name}}\t{{.Status}}' 2>/dev/null | grep -E 'NAME|server|redis|postgres|caddy' || true

section "GET /healthz"
health_code="$(http_code "${LOCAL_API}/healthz")"
health_body="$(curl -4 -fsS -m 15 "${LOCAL_API}/healthz" 2>/dev/null || true)"
if [[ "${health_code}" == "200" ]] && echo "${health_body}" | grep -qE '"status"[[:space:]]*:[[:space:]]*"ok"'; then
  ok "healthz HTTP 200"
else
  fail "healthz HTTP ${health_code} body=${health_body:-empty}"
  echo "    hint: docker compose -f ${COMPOSE_DIR}/production.base.yml -f ${COMPOSE_DIR}/oci.staging.yml logs server --tail=40" >&2
fi

section "Metrics baseline (/metrics on localhost)"
if curl -4 -fsS -m 10 "${LOCAL_METRICS}" >/dev/null 2>&1; then
  q="$(read_ingest_metric 'status="queued"')"
  r="$(read_ingest_metric 'status="rejected"')"
  d="$(read_ingest_metric 'status="deduplicated"')"
  echo "    ingest queued=${q} rejected=${r} deduplicated=${d}"
  ok "metrics endpoint reachable"
else
  fail "cannot GET ${LOCAL_METRICS} — check METRICS_ENABLED and SERVER_BIND in oci.staging.yml"
fi

section "Monitoring stack"
if [[ -x "${SCRIPT_DIR}/monitoring-doctor.sh" ]]; then
  if "${SCRIPT_DIR}/monitoring-doctor.sh"; then
    ok "monitoring-doctor passed"
  elif [[ "${REQUIRE_MONITORING}" == "1" ]]; then
    fail "monitoring-doctor reported problems (use --fix-monitoring or monitoring-down && monitoring-up)"
  else
    warn "monitoring-doctor reported problems — tests can run; fix Grafana with --fix-monitoring"
  fi
else
  warn "monitoring-doctor.sh not found — run sync-monitoring-to-vm.sh from WSL"
fi

section "Ingest probe (fake token → rejected should increment)"
r_before="$(read_ingest_metric 'status="rejected"')"
r_before=$((r_before + 0))
probe_code="$(http_code "${LOCAL_API}/v1/webhook/preflight-fake-token" POST \
  '{"action":"BUY","symbol":"RELIANCE","lot":1}')"
r_after="$(read_ingest_metric 'status="rejected"')"
r_after=$((r_after + 0))
if [[ "${probe_code}" == "401" ]] && (( r_after > r_before )); then
  ok "ingest + metrics live (HTTP 401, rejected counter +1)"
else
  fail "ingest probe HTTP ${probe_code}, rejected ${r_before} → ${r_after}"
  if [[ "${probe_code}" == "000" ]]; then
    echo "    hint: nothing listening on ${LOCAL_API} — check: ss -tlnp | grep 8080" >&2
  fi
fi

echo
if [[ "${failures}" -gt 0 ]]; then
  echo "VM preflight FAILED (${failures} check(s)). Fix before running WSL tests." >&2
  exit 1
fi

echo "VM preflight passed."
echo "Next (WSL): ./deploy/scripts/preflight-wsl.sh"
echo "  then: staging-smoke, make functional-test-staging, loadtest, sectest"
