#!/usr/bin/env bash
# Tier 2 — full functional suite against staging (no local Docker).
#
# AWS staging (primary):
#   cp deploy/deploy-aws-ecs/staging-aws.env.example deploy/deploy-aws-ecs/staging-aws.env
#   ./functional-tester/run-functional-staging.sh
#
# STAGING_BASE_URL is optional if terraform output api_url is available (same as aws-deploy.sh).
#
# Manual override:
#   export STAGING_BASE_URL=https://api.staging.zettabridge.net
#   export STAGING_ADMIN_EMAIL=admin@zettabridge.net
#   export STAGING_ADMIN_PASSWORD='...'
#   ./functional-tester/run-functional-staging.sh
#
# Lightsail (legacy):
#   export STAGING_BASE_URL=http://52.66.136.127
#   export STAGING_ADMIN_EMAIL=admin@zettaflux.com
#
# Optional:
#   STAGING_RUN_LIVE_ROUTING=1     also run TestFreeUserMockRoutingWhenBrokerLive (BROKER_MODE=live)
#   STAGING_LIVE_ROUTING_ONLY=1    skip full suite; live routing test only
#   STAGING_FUNCTIONAL_TIMEOUT=15m go test timeout (default 15m)
#   FUNCTIONAL_TEST_EMAIL_VERIFY=1 include email verification test (staging must require verify)
#
# Requires: go, curl, jq
# Prerequisite: admin user must exist on the target DB (created via BOOTSTRAP_ADMIN_EMAIL on first deploy)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Auto-load staging env file so the script works without manually sourcing it first.
if [[ -f "${ROOT_DIR}/deploy/deploy-aws-ecs/staging-aws.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${ROOT_DIR}/deploy/deploy-aws-ecs/staging-aws.env"
  set +a
fi

# shellcheck disable=SC1091
source "${ROOT_DIR}/deploy/deploy-aws-ecs/scripts/common.sh"

BASE_URL="${STAGING_BASE_URL:-${FUNCTIONAL_TEST_BASE_URL:-}}"
if [[ -z "${BASE_URL}" ]]; then
  BASE_URL="$(zb_tf_output api_url)"
fi
if [[ -z "${BASE_URL}" ]]; then
  BASE_URL="https://api.staging.zettabridge.net"
  echo "NOTE: STAGING_BASE_URL unset — using default ${BASE_URL}" >&2
fi
BASE_URL="${BASE_URL%/}"
ADMIN_EMAIL="${STAGING_ADMIN_EMAIL:-${FUNCTIONAL_TEST_ADMIN_EMAIL:-}}"
ADMIN_PASSWORD="${STAGING_ADMIN_PASSWORD:-${FUNCTIONAL_TEST_ADMIN_PASSWORD:-}}"
METRICS_TOKEN="${STAGING_METRICS_TOKEN:-${FUNCTIONAL_TEST_METRICS_TOKEN:-}}"
RUN_LIVE_ROUTING="${STAGING_RUN_LIVE_ROUTING:-0}"
LIVE_ROUTING_ONLY="${STAGING_LIVE_ROUTING_ONLY:-0}"
TEST_TIMEOUT="${STAGING_FUNCTIONAL_TIMEOUT:-15m}"

export FUNCTIONAL_TEST_BASE_URL="${BASE_URL%/}"
export FUNCTIONAL_TEST_ADMIN_EMAIL="$ADMIN_EMAIL"
export FUNCTIONAL_TEST_ADMIN_PASSWORD="$ADMIN_PASSWORD"
export FUNCTIONAL_TEST_METRICS_TOKEN="${METRICS_TOKEN}"
export FUNCTIONAL_TEST_SKIP_MIGRATIONS=1
export FUNCTIONAL_TEST_SKIP_SERVER_REBUILD=1
export FUNCTIONAL_TEST_STAGING=1
export FUNCTIONAL_TEST_ENABLED_ADAPTERS="${STAGING_ENABLED_ADAPTERS:-paper,zerodha}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

preflight() {
  local code body token role login_body

  echo "==> Preflight: GET ${FUNCTIONAL_TEST_BASE_URL}/healthz"
  code="$(curl -4 -sS -o /tmp/zb-staging-health.json -w '%{http_code}' \
    -m 30 "${FUNCTIONAL_TEST_BASE_URL}/healthz")"
  body="$(cat /tmp/zb-staging-health.json)"
  if [[ "$code" != "200" ]] || [[ "$(echo "$body" | jq -r '.data.status // .status // empty')" != "ok" ]]; then
    echo "healthz failed: HTTP ${code} body=${body}" >&2
    exit 1
  fi
  echo "    OK healthz"

  if [[ -z "$ADMIN_EMAIL" || -z "$ADMIN_PASSWORD" ]]; then
    echo "STAGING_ADMIN_EMAIL and STAGING_ADMIN_PASSWORD are required" >&2
    exit 1
  fi

  echo "==> Preflight: admin login (${ADMIN_EMAIL})"
  login_body="$(curl -4 -sS -m 30 -X POST "${FUNCTIONAL_TEST_BASE_URL}/v1/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}")"
  token="$(echo "$login_body" | jq -r '.data.token // empty')"
  if [[ -z "$token" ]]; then
    echo "admin login failed: $(echo "$login_body" | jq -c '.')" >&2
    echo "Register admin once, restart with BOOTSTRAP_ADMIN_EMAIL, then re-run." >&2
    exit 1
  fi

  role="$(curl -4 -sS -m 30 "${FUNCTIONAL_TEST_BASE_URL}/v1/me" \
    -H "Authorization: Bearer ${token}" | jq -r '.data.role // empty')"
  if [[ "$role" != "admin" ]]; then
    echo "admin role check failed: got role=${role} (expected admin)" >&2
    exit 1
  fi
  echo "    OK admin login role=admin"
  echo
}

run_go_test() {
  local -a args=("$@")
  # -count=1 disables go test cache — otherwise a prior PASS reruns in ~1s with no HTTP traffic.
  go test -tags functional -count=1 ./functional-tester -v -timeout "$TEST_TIMEOUT" "${args[@]}"
}

require_cmd go
require_cmd curl
require_cmd jq

if [[ -z "$BASE_URL" ]]; then
  echo "STAGING_BASE_URL is required (set in deploy/deploy-aws-ecs/staging-aws.env or terraform output api_url)" >&2
  exit 1
fi

echo "ZettaBridge functional staging (Tier 2)"
echo "  BASE_URL=${FUNCTIONAL_TEST_BASE_URL}"
echo "  ADMIN=${ADMIN_EMAIL}"
echo "  TIMEOUT=${TEST_TIMEOUT}"
echo

preflight

if [[ "$LIVE_ROUTING_ONLY" == "1" ]]; then
  echo "==> Live routing test only (BROKER_MODE=live staging)"
  export FUNCTIONAL_TEST_LIVE_ROUTING=1
  run_go_test -run TestFreeUserMockRoutingWhenBrokerLive
  exit $?
fi

echo "==> Full functional suite (creates ephemeral users/orgs on staging DB)"
if ! run_go_test; then
  exit 1
fi

if [[ "$RUN_LIVE_ROUTING" == "1" ]]; then
  echo
  echo "==> Live routing test (BROKER_MODE=live)"
  export FUNCTIONAL_TEST_LIVE_ROUTING=1
  run_go_test -run TestFreeUserMockRoutingWhenBrokerLive
fi
