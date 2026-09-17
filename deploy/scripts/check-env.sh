#!/usr/bin/env bash
# Validate a runtime env file before deploy (mirrors key production rules).
#
# Usage:
#   ./deploy/scripts/check-env.sh deploy/env/oci.staging.env
#   ./deploy/scripts/check-env.sh /opt/zettabridge/env/runtime.env

set -euo pipefail

ENV_FILE="${1:-}"
if [[ -z "${ENV_FILE}" || ! -f "${ENV_FILE}" ]]; then
  echo "usage: $0 <env-file>" >&2
  exit 1
fi

# shellcheck disable=SC1090
set -a
source "${ENV_FILE}"
set +a

errors=0

require_nonempty() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "ERROR: ${name} is required" >&2
    errors=$((errors + 1))
  fi
}

require_nonempty POSTGRES_PASSWORD
require_nonempty JWT_SECRET
require_nonempty AES_KEY
require_nonempty APP_PUBLIC_URL
require_nonempty CORS_ORIGINS

if [[ -n "${JWT_SECRET:-}" && ${#JWT_SECRET} -lt 32 ]]; then
  echo "ERROR: JWT_SECRET must be at least 32 characters" >&2
  errors=$((errors + 1))
fi

if [[ -n "${AES_KEY:-}" ]]; then
  if [[ ! "${AES_KEY}" =~ ^[0-9a-fA-F]{64}$ ]]; then
    echo "ERROR: AES_KEY must be 64 hex characters (32 bytes)" >&2
    errors=$((errors + 1))
  elif [[ "${AES_KEY}" =~ ^0+$ ]]; then
    echo "ERROR: AES_KEY must not be all zeros" >&2
    errors=$((errors + 1))
  fi
fi

broker_mode="${BROKER_MODE:-mock}"
broker_mode="$(echo "${broker_mode}" | tr '[:upper:]' '[:lower:]')"
if [[ "${broker_mode}" != "mock" && "${broker_mode}" != "live" ]]; then
  echo "ERROR: BROKER_MODE must be mock or live" >&2
  errors=$((errors + 1))
fi

if [[ "${broker_mode}" == "live" ]]; then
  sebi_required="${SEBI_ALGO_ID_REQUIRED:-false}"
  sebi_required="$(echo "${sebi_required}" | tr '[:upper:]' '[:lower:]')"
  if [[ "${sebi_required}" == "true" || "${sebi_required}" == "1" || "${sebi_required}" == "yes" ]]; then
    require_nonempty BROKER_ANGEL_CLIENT_LOCAL_IP
    require_nonempty BROKER_ANGEL_CLIENT_PUBLIC_IP
  fi
fi

if [[ "${errors}" -gt 0 ]]; then
  echo "check-env: ${errors} error(s)" >&2
  exit 1
fi

echo "check-env: OK (${ENV_FILE})"
