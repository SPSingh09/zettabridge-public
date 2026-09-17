#!/usr/bin/env bash
# /metrics exposure when METRICS_TOKEN unset vs set (5B.2).

set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require_cmds curl
require_base

echo "==> metrics-auth"

code="$(http_code_get /metrics '')"
if [[ -n "${SECTEST_METRICS_TOKEN:-}" ]]; then
  assert_code 'GET /metrics without token (token configured)' "${code}" '401'
  code="$("${CURL[@]}" -o /dev/null -w '%{http_code}' "${BASE_URL}/metrics" \
    -H "Authorization: Bearer ${SECTEST_METRICS_TOKEN}")"
  assert_code 'GET /metrics with valid token' "${code}" '200'
else
  # Staging: /metrics may be localhost-only, disabled, or open on public URL.
  assert_one_of 'GET /metrics without token (open scrape)' "${code}" '200' '401' '404'
  if [[ "${code}" == "200" ]]; then
    echo "    NOTE: METRICS_TOKEN unset — metrics publicly reachable (ok on staging sidecar scrape via VM localhost)"
  elif [[ "${code}" == "404" ]]; then
    echo "    NOTE: /metrics not exposed on public URL (expected — Prometheus scrapes server:8080 on VM)"
  fi
fi

finish_sectest
