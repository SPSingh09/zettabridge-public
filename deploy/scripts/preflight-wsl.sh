#!/usr/bin/env bash
# WSL preflight — verify staging URL, tools, and load-test webhook before test runs.
# Run from repo root on laptop/WSL after VM preflight-vm.sh.
#
# Usage:
#   export STAGING_BASE_URL=http://52.66.136.127
#   export STAGING_ADMIN_EMAIL=admin@zettaflux.com
#   export STAGING_ADMIN_PASSWORD='...'
#   ./deploy/scripts/preflight-wsl.sh
#
# Options:
#   --require-loadtest   fail if deploy/loadtest/env.local missing or ingest not 202
#   --require-admin      fail if admin creds unset (needed for smoke + functional)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REQUIRE_LOADTEST=0
REQUIRE_ADMIN=0

for arg in "$@"; do
  case "${arg}" in
    --require-loadtest) REQUIRE_LOADTEST=1 ;;
    --require-admin) REQUIRE_ADMIN=1 ;;
    -h|--help)
      echo "usage: $0 [--require-loadtest] [--require-admin]"
      exit 0
      ;;
    *)
      echo "unknown argument: ${arg}" >&2
      exit 1
      ;;
  esac
done

BASE_URL="${STAGING_BASE_URL:-${LOADTEST_BASE_URL:-}}"
ADMIN_EMAIL="${STAGING_ADMIN_EMAIL:-}"
ADMIN_PASSWORD="${STAGING_ADMIN_PASSWORD:-}"
LOADTEST_ENV="${LOADTEST_ENV_FILE:-${ROOT}/deploy/loadtest/env.local}"

CURL=(curl -4 -sS -m 30)
if [[ "${STAGING_INSECURE:-0}" == "1" ]]; then
  CURL+=( -k )
fi

failures=0
ok() { echo "    OK: $*"; }
warn() { echo "    WARN: $*" >&2; }
fail() { failures=$((failures + 1)); echo "    FAIL: $*" >&2; }

section() { echo; echo "==> $*"; }

require_cmd() {
  if command -v "$1" >/dev/null 2>&1; then
    ok "$1 found"
  else
    fail "missing command: $1"
  fi
}

section "ZettaBridge WSL preflight"

section "Required tools"
require_cmd curl
require_cmd jq
if command -v k6 >/dev/null 2>&1; then
  ok "k6 found ($(k6 version 2>/dev/null | head -1 || echo installed))"
else
  warn "k6 not found — install: sudo bash deploy/scripts/install-k6.sh (needed for load tests)"
fi

section "Staging URL"
if [[ -z "${BASE_URL}" ]]; then
  fail "STAGING_BASE_URL is required (e.g. http://52.66.136.127)"
else
  BASE_URL="${BASE_URL%/}"
  ok "STAGING_BASE_URL=${BASE_URL}"
fi

if [[ "${REQUIRE_ADMIN}" == "1" || -n "${ADMIN_EMAIL}" ]]; then
  section "Admin credentials (smoke + functional tests)"
  if [[ -z "${ADMIN_EMAIL}" || -z "${ADMIN_PASSWORD}" ]]; then
    if [[ "${REQUIRE_ADMIN}" == "1" ]]; then
      fail "STAGING_ADMIN_EMAIL and STAGING_ADMIN_PASSWORD required"
    else
      warn "admin creds unset — staging-smoke / functional-test-staging may fail"
    fi
  else
    ok "admin email set (${ADMIN_EMAIL})"
  fi
fi

if [[ -n "${BASE_URL}" ]]; then
  section "GET ${BASE_URL}/healthz"
  code="$("${CURL[@]}" -o /tmp/zb-preflight-health.json -w '%{http_code}' "${BASE_URL}/healthz")"
  if [[ "${code}" == "200" ]]; then
    status="$(jq -r '.data.status // .status // empty' /tmp/zb-preflight-health.json 2>/dev/null || true)"
    if [[ "${status}" == "ok" ]]; then
      ok "public healthz 200 status=ok"
    else
      fail "healthz 200 but status!=ok (${status})"
    fi
  else
    fail "healthz HTTP ${code}"
  fi

  section "Public routing (fake webhook → 401)"
  wh_code="$("${CURL[@]}" -o /tmp/zb-preflight-wh.json -w '%{http_code}' -X POST \
    "${BASE_URL}/v1/webhook/preflight-wsl-fake-token" \
    -H 'Content-Type: application/json' \
    -d '{"action":"BUY","symbol":"RELIANCE","lot":1}')"
  if [[ "${wh_code}" == "401" ]]; then
    ok "POST /v1/webhook/:token returns 401 for bad token"
  else
    fail "expected HTTP 401 for fake webhook, got ${wh_code}"
  fi
fi

section "Load-test webhook (deploy/loadtest/env.local)"
if [[ -f "${LOADTEST_ENV}" ]]; then
  # shellcheck disable=SC1090
  set -a
  source "${LOADTEST_ENV}"
  set +a
  LOADTEST_BASE_URL="${LOADTEST_BASE_URL:-${BASE_URL}}"
  LOADTEST_WEBHOOK_TOKEN="${LOADTEST_WEBHOOK_TOKEN:-}"
  if [[ -z "${LOADTEST_WEBHOOK_TOKEN}" ]]; then
    if [[ "${REQUIRE_LOADTEST}" == "1" ]]; then
      fail "LOADTEST_WEBHOOK_TOKEN empty in ${LOADTEST_ENV}"
    else
      warn "LOADTEST_WEBHOOK_TOKEN empty — run: ./deploy/loadtest/setup-loadtest-webhook.sh"
    fi
  elif [[ -n "${LOADTEST_BASE_URL}" ]]; then
    ingest_code="$("${CURL[@]}" -o /tmp/zb-preflight-ingest.json -w '%{http_code}' -X POST \
      "${LOADTEST_BASE_URL%/}/v1/webhook/${LOADTEST_WEBHOOK_TOKEN}" \
      -H 'Content-Type: application/json' \
      -d "{\"action\":\"BUY\",\"symbol\":\"${LOADTEST_SYMBOL:-RELIANCE}\",\"lot\":1,\"comment\":\"preflight-wsl\"}")"
    queued="$(jq -r '.queued // false' /tmp/zb-preflight-ingest.json 2>/dev/null || echo false)"
    if [[ "${ingest_code}" == "202" && "${queued}" == "true" ]]; then
      ok "load-test webhook ingest HTTP 202 queued=true"
    else
      msg="$(jq -c '.' /tmp/zb-preflight-ingest.json 2>/dev/null || cat /tmp/zb-preflight-ingest.json)"
      if [[ "${REQUIRE_LOADTEST}" == "1" ]]; then
        fail "load-test ingest HTTP ${ingest_code} body=${msg}"
      else
        warn "load-test ingest HTTP ${ingest_code} — re-run setup-loadtest-webhook.sh before k6"
      fi
    fi
  fi
else
  if [[ "${REQUIRE_LOADTEST}" == "1" ]]; then
    fail "missing ${LOADTEST_ENV} — run: ./deploy/loadtest/setup-loadtest-webhook.sh"
  else
    warn "no ${LOADTEST_ENV} — run setup-loadtest-webhook.sh before load tests"
  fi
fi

section "Grafana (optional)"
echo "    For live metrics during tests, open a tunnel in another terminal:"
echo "    ssh -N -L 3000:127.0.0.1:3000 -L 9090:127.0.0.1:9090 ubuntu@YOUR_STATIC_IP"
echo "    Then http://localhost:3000 → ZettaBridge Overview → Last 15 minutes"

echo
if [[ "${failures}" -gt 0 ]]; then
  echo "WSL preflight FAILED (${failures} check(s))." >&2
  exit 1
fi

echo "WSL preflight passed."
echo "Suggested order:"
echo "  1. VM:  /opt/zettabridge/scripts/preflight-vm.sh"
echo "  2. WSL: ./deploy/scripts/staging-smoke.sh"
echo "  3. WSL: make functional-test-staging"
echo "  4. WSL: ./deploy/loadtest/run-loadtest.sh smoke"
echo "  5. WSL: make sectest-staging"
