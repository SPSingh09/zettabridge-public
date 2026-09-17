#!/usr/bin/env bash
# Run all 5B.2 pen-test scripts against staging.
#
# Usage:
#   export STAGING_BASE_URL=http://52.66.136.127
#   export STAGING_ADMIN_EMAIL=... STAGING_ADMIN_PASSWORD=...  # webhook-flood pro bump
#   ./deploy/sectest/run-sectest.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SECTEST_ENV_FILE:-${ROOT}/env.local}"
if [[ -f "${ENV_FILE}" ]]; then
  # shellcheck disable=SC1090
  set -a
  source "${ENV_FILE}"
  set +a
fi
if [[ -f "${ROOT}/../staging.env" ]]; then
  # shellcheck disable=SC1091
  set -a
  source "${ROOT}/../staging.env"
  set +a
fi

export SECTEST_ENV_FILE="${ENV_FILE}"

BASE_URL="${SECTEST_BASE_URL:-${STAGING_BASE_URL:-}}"
if [[ -z "${BASE_URL}" ]]; then
  echo "SECTEST_BASE_URL or STAGING_BASE_URL is required" >&2
  exit 1
fi

SCRIPTS=(
  jwt-tamper.sh
  admin-isolation.sh
  webhook-token-probe.sh
  idor-probe.sh
  webhook-flood.sh
  injection-probe.sh
  metrics-auth.sh
)

SCRIPT_FAILURES=0
FAILED_NAMES=()

echo "ZettaBridge pen test (5B.2)"
echo "  BASE_URL=${BASE_URL%/}"
echo

for script in "${SCRIPTS[@]}"; do
  if bash "${ROOT}/scripts/${script}"; then
    echo "    → ${script} PASS"
  else
    SCRIPT_FAILURES=$((SCRIPT_FAILURES + 1))
    FAILED_NAMES+=("${script}")
    echo "    → ${script} FAIL" >&2
  fi
  echo
done

echo "Pen test summary: $(( ${#SCRIPTS[@]} - SCRIPT_FAILURES ))/${#SCRIPTS[@]} scripts passed"
if [[ "${SCRIPT_FAILURES}" -gt 0 ]]; then
  echo "Failed: ${FAILED_NAMES[*]}" >&2
  exit 1
fi
echo "All scripts passed."
