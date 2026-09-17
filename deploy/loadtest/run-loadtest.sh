#!/usr/bin/env bash
# Run k6 webhook load tests against staging (5B.1).
#
# Usage:
#   ./deploy/loadtest/setup-loadtest-webhook.sh   # one-time
#   ./deploy/loadtest/run-loadtest.sh smoke
#   ./deploy/loadtest/run-loadtest.sh target
#
# Scenarios: smoke | ramp | target | dedup | soak | bracket | rate-limit-free | rate-limit-paid
#
# Requires: k6 (https://grafana.com/docs/k6/latest/set-up/install-k6/)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "${ROOT}/../.." && pwd)"
SCENARIO="${1:-smoke}"

# Auto-load staging credentials + URL.
if [[ -f "${REPO}/deploy/deploy-aws-ecs/staging-aws.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${REPO}/deploy/deploy-aws-ecs/staging-aws.env"
  set +a
fi
# shellcheck disable=SC1091
source "${REPO}/deploy/deploy-aws-ecs/scripts/common.sh"

ENV_FILE="${LOADTEST_ENV_FILE:-${ROOT}/env.local}"
if [[ -f "${ENV_FILE}" ]]; then
  # shellcheck disable=SC1090
  set -a
  source "${ENV_FILE}"
  set +a
fi

LOADTEST_BASE_URL="${LOADTEST_BASE_URL:-${STAGING_BASE_URL:-}}"
if [[ -z "${LOADTEST_BASE_URL}" ]]; then
  LOADTEST_BASE_URL="$(zb_tf_output api_url)"
fi
if [[ -z "${LOADTEST_BASE_URL}" ]]; then
  LOADTEST_BASE_URL="https://api.staging.zettabridge.net"
  echo "NOTE: LOADTEST_BASE_URL unset — using default ${LOADTEST_BASE_URL}" >&2
fi
LOADTEST_WEBHOOK_TOKEN="${LOADTEST_WEBHOOK_TOKEN:-}"

if [[ -z "${LOADTEST_WEBHOOK_TOKEN}" ]]; then
  echo "LOADTEST_WEBHOOK_TOKEN is required — run: ${ROOT}/setup-loadtest-webhook.sh" >&2
  exit 1
fi

if ! command -v k6 >/dev/null 2>&1; then
  echo "k6 not found — install: https://grafana.com/docs/k6/latest/set-up/install-k6/" >&2
  echo "  or: sudo bash deploy/scripts/install-k6.sh" >&2
  exit 1
fi

RESULTS_DIR="${LOADTEST_RESULTS_DIR:-${ROOT}/results}"
mkdir -p "${RESULTS_DIR}"
export LOADTEST_BASE_URL="${LOADTEST_BASE_URL%/}"
export LOADTEST_WEBHOOK_TOKEN
export LOADTEST_DEDUP_WEBHOOK_TOKEN="${LOADTEST_DEDUP_WEBHOOK_TOKEN:-${LOADTEST_WEBHOOK_TOKEN}}"
export LOADTEST_RL_FREE_TOKEN="${LOADTEST_RL_FREE_TOKEN:-${LOADTEST_WEBHOOK_TOKEN}}"
export LOADTEST_RL_PAID_TOKEN="${LOADTEST_RL_PAID_TOKEN:-${LOADTEST_WEBHOOK_TOKEN}}"
export LOADTEST_BRACKET_WEBHOOK_TOKEN="${LOADTEST_BRACKET_WEBHOOK_TOKEN:-${LOADTEST_WEBHOOK_TOKEN}}"
export LOADTEST_SYMBOL="${LOADTEST_SYMBOL:-RELIANCE}"
export LOADTEST_RESULTS_DIR="${RESULTS_DIR}"

loadtest_user_token() {
  if [[ -n "${LOADTEST_USER_TOKEN:-}" ]]; then
    echo "${LOADTEST_USER_TOKEN}"
    return
  fi
  if [[ -z "${LOADTEST_USER_EMAIL:-}" || -z "${LOADTEST_USER_PASSWORD:-}" ]]; then
    return 1
  fi
  curl -4 -sS -m 30 -X POST "${LOADTEST_BASE_URL}/v1/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"${LOADTEST_USER_EMAIL}\",\"password\":\"${LOADTEST_USER_PASSWORD}\"}" \
    | jq -r '.data.token // empty'
}

patch_webhook() {
  local token="$1" webhook_id="$2" body="$3"
  curl -4 -sS -m 30 -X PUT "${LOADTEST_BASE_URL}/v1/webhooks/${webhook_id}" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${token}" \
    -d "${body}" >/dev/null
}

reset_ingest_webhook_limits() {
  local token="$1" webhook_id="$2"
  [[ -z "${token}" || -z "${webhook_id}" ]] && return 0
  echo "==> Reset ingest webhook limits (rate_limit_per_min defaults to 60 if unset)"
  patch_webhook "${token}" "${webhook_id}" '{"rate_limit_per_sec":0,"rate_limit_per_min":0,"dedup_window_sec":0}'
}

DEDUP_WEBHOOK_PATCH='{"rate_limit_per_sec":0,"rate_limit_per_min":0,"dedup_window_sec":60}'

resolve_dedup_webhook_id() {
  local ut="${1:-}"
  if [[ -n "${LOADTEST_DEDUP_WEBHOOK_ID:-}" ]]; then
    echo "${LOADTEST_DEDUP_WEBHOOK_ID}"
    return 0
  fi
  [[ -z "${ut}" ]] && return 1
  curl -4 -sS -m 30 "${LOADTEST_BASE_URL}/v1/webhooks" \
    -H "Authorization: Bearer ${ut}" \
    | jq -r '.data[] | select(.label == "loadtest-dedup") | .id' | head -1
}

prepare_dedup_webhook() {
  local ut="${1:-}"
  [[ "${SCENARIO}" != "dedup" ]] && return 0
  if [[ -z "${ut}" ]]; then
    echo "WARN: no load-test user token — cannot PATCH dedup webhook (re-run setup-loadtest-webhook.sh)" >&2
    return 0
  fi

  local dedup_id
  dedup_id="$(resolve_dedup_webhook_id "${ut}" || true)"
  if [[ -n "${dedup_id}" && "${LOADTEST_DEDUP_WEBHOOK_TOKEN}" != "${LOADTEST_WEBHOOK_TOKEN}" ]]; then
    echo "==> Configure loadtest-dedup webhook (${dedup_id}): dedup on, rate limits off"
    patch_webhook "${ut}" "${dedup_id}" "${DEDUP_WEBHOOK_PATCH}"
    return 0
  fi

  if [[ "${LOADTEST_DEDUP_WEBHOOK_TOKEN}" == "${LOADTEST_WEBHOOK_TOKEN}" && -n "${LOADTEST_WEBHOOK_ID:-}" ]]; then
    echo "==> Configure main webhook for dedup scenario (pro fallback): dedup on, rate limits off"
    patch_webhook "${ut}" "${LOADTEST_WEBHOOK_ID}" "${DEDUP_WEBHOOK_PATCH}"
  fi
}

preflight_dedup_webhook() {
  [[ "${SCENARIO}" != "dedup" ]] && return 0
  local token="${LOADTEST_DEDUP_WEBHOOK_TOKEN}"
  local url="${LOADTEST_BASE_URL}/v1/webhook/${token}"
  local comment="dedup-preflight-$(date +%s)-${RANDOM}"
  local payload
  payload="$(jq -nc --arg sym "${LOADTEST_SYMBOL}" --arg c "${comment}" \
    '{action:"BUY",symbol:$sym,lot:1,comment:$c}')"

  echo "==> Dedup preflight (expect 202 then 409 on identical payload)"
  local code1 code2
  code1="$(curl -4 -sS -m 30 -o /tmp/zb-dedup-preflight-1.json -w '%{http_code}' \
    -X POST "${url}" -H 'Content-Type: application/json' -d "${payload}")"
  code2="$(curl -4 -sS -m 30 -o /tmp/zb-dedup-preflight-2.json -w '%{http_code}' \
    -X POST "${url}" -H 'Content-Type: application/json' -d "${payload}")"
  echo "    first=${code1} replay=${code2}"
  if [[ "${code1}" != "202" ]]; then
    echo "dedup preflight FAILED: first request HTTP ${code1}" >&2
    cat /tmp/zb-dedup-preflight-1.json >&2
    echo >&2
    exit 1
  fi
  if [[ "${code2}" != "409" ]]; then
    echo "dedup preflight FAILED: replay HTTP ${code2} (expected 409)" >&2
    cat /tmp/zb-dedup-preflight-2.json >&2
    echo "  hint: dedup_window_sec may be 0 or rate_limit_per_min=60 still active — re-run setup-loadtest-webhook.sh" >&2
    exit 1
  fi
}

# When pro plan allows only one live webhook, temporarily tune the main webhook for specialized scenarios.
ensure_ingest_webhook_ready() {
  local ut="${1:-}"
  case "${SCENARIO}" in
    smoke|target|ramp|soak|bracket)
      reset_ingest_webhook_limits "${ut}" "${LOADTEST_WEBHOOK_ID:-}"
      ;;
  esac
}

maybe_prepare_webhook() {
  local ut="${1:-}"
  local wh_id="${LOADTEST_WEBHOOK_ID:-}"
  [[ -z "${ut}" || -z "${wh_id}" ]] && return 0

  case "${SCENARIO}" in
    dedup)
      ;;
    rate-limit-free)
      if [[ "${LOADTEST_RL_FREE_TOKEN}" == "${LOADTEST_WEBHOOK_TOKEN}" ]]; then
        echo "==> Temporarily setting rate_limit_per_sec=1 on main webhook (pro fallback)"
        patch_webhook "${ut}" "${wh_id}" '{"rate_limit_per_sec":1,"dedup_window_sec":0}'
      fi
      ;;
    rate-limit-paid)
      if [[ "${LOADTEST_RL_PAID_TOKEN}" == "${LOADTEST_WEBHOOK_TOKEN}" ]]; then
        echo "==> Temporarily setting rate_limit_per_sec=10 on main webhook (pro fallback)"
        patch_webhook "${ut}" "${wh_id}" '{"rate_limit_per_sec":10,"dedup_window_sec":0}'
      fi
      ;;
  esac
}

maybe_restore_webhook() {
  local ut="${1:-}"
  local wh_id="${LOADTEST_WEBHOOK_ID:-}"
  [[ -z "${ut}" || -z "${wh_id}" ]] && return 0
  case "${SCENARIO}" in
    dedup|rate-limit-free|rate-limit-paid)
      if [[ "${LOADTEST_DEDUP_WEBHOOK_TOKEN}" == "${LOADTEST_WEBHOOK_TOKEN}" \
         || "${LOADTEST_RL_FREE_TOKEN}" == "${LOADTEST_WEBHOOK_TOKEN}" \
         || "${LOADTEST_RL_PAID_TOKEN}" == "${LOADTEST_WEBHOOK_TOKEN}" ]]; then
        patch_webhook "${ut}" "${wh_id}" '{"rate_limit_per_sec":0,"rate_limit_per_min":0,"dedup_window_sec":0}'
      fi
      ;;
  esac
}

K6_SCRIPT=""
case "${SCENARIO}" in
  smoke)
    export LOADTEST_VUS="${LOADTEST_VUS:-10}"
    export LOADTEST_DURATION="${LOADTEST_DURATION:-1m}"
    export LOADTEST_SLEEP="${LOADTEST_SLEEP:-0.5}"
    export LOADTEST_PROFILE=smoke
    K6_SCRIPT="${ROOT}/k6/webhook-ingest.js"
    ;;
  ramp)
    export LOADTEST_VUS="${LOADTEST_VUS:-20}"
    export LOADTEST_SLEEP="${LOADTEST_SLEEP:-0.05}"
    K6_SCRIPT="${ROOT}/k6/webhook-ramp.js"
    ;;
  target)
    export LOADTEST_VUS="${LOADTEST_VUS:-40}"
    export LOADTEST_DURATION="${LOADTEST_DURATION:-3m}"
    export LOADTEST_SLEEP="${LOADTEST_SLEEP:-0.1}"
    export LOADTEST_PROFILE=load
    K6_SCRIPT="${ROOT}/k6/webhook-ingest.js"
    ;;
  dedup)
    export LOADTEST_VUS="${LOADTEST_VUS:-50}"
    export LOADTEST_DURATION="${LOADTEST_DURATION:-2m}"
    K6_SCRIPT="${ROOT}/k6/dedup-replay.js"
    ;;
  bracket)
    export LOADTEST_VUS="${LOADTEST_VUS:-5}"
    export LOADTEST_DURATION="${LOADTEST_DURATION:-1m}"
    export LOADTEST_SLEEP="${LOADTEST_SLEEP:-0.1}"
    export LOADTEST_PROFILE=smoke
    K6_SCRIPT="${ROOT}/k6/bracket-order-ingest.js"
    ;;
  soak)
    export LOADTEST_VUS="${LOADTEST_VUS:-100}"
    export LOADTEST_DURATION="${LOADTEST_DURATION:-15m}"
    export LOADTEST_SLEEP="${LOADTEST_SLEEP:-0.1}"
    export LOADTEST_PROFILE=load
    K6_SCRIPT="${ROOT}/k6/webhook-ingest.js"
    ;;
  rate-limit-free)
    export LOADTEST_PLAN=free
    export LOADTEST_VUS="${LOADTEST_VUS:-5}"
    export LOADTEST_DURATION="${LOADTEST_DURATION:-30s}"
    K6_SCRIPT="${ROOT}/k6/rate-limit-enforce.js"
    ;;
  rate-limit-paid)
    export LOADTEST_PLAN=paid
    export LOADTEST_VUS="${LOADTEST_VUS:-20}"
    export LOADTEST_DURATION="${LOADTEST_DURATION:-30s}"
    K6_SCRIPT="${ROOT}/k6/rate-limit-enforce.js"
    ;;
  *)
    echo "usage: $0 {smoke|ramp|target|dedup|soak|bracket|rate-limit-free|rate-limit-paid}" >&2
    exit 1
    ;;
esac

echo "ZettaBridge load test (5B.1)"
echo "  scenario=${SCENARIO}"
echo "  base=${LOADTEST_BASE_URL}"
echo "  results=${RESULTS_DIR}"
if [[ -n "${LOADTEST_USER_PLAN:-}" ]]; then
  echo "  loadtest_user_plan=${LOADTEST_USER_PLAN}"
fi
if [[ -n "${LOADTEST_PROFILE:-}" ]]; then
  echo "  profile=${LOADTEST_PROFILE} sleep=${LOADTEST_SLEEP:-0.05}s"
fi
echo

UT="$(loadtest_user_token || true)"
ensure_ingest_webhook_ready "${UT}"
prepare_dedup_webhook "${UT}"
maybe_prepare_webhook "${UT}"
preflight_dedup_webhook
trap 'maybe_restore_webhook "${UT}"' EXIT

k6 run "${K6_SCRIPT}"

echo
echo "Done. Check Grafana (Last 15 min) and re-run functional-test-staging after a full target run."
