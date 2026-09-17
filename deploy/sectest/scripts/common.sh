#!/usr/bin/env bash
# Shared helpers for 5B.2 pen-test scripts.

set -euo pipefail

SECTEST_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BASE_URL="${SECTEST_BASE_URL:-${STAGING_BASE_URL:-}}"
if [[ -f "${SECTEST_ENV_FILE:-${SECTEST_ROOT}/env.local}" ]]; then
  # shellcheck disable=SC1090
  set -a
  source "${SECTEST_ENV_FILE:-${SECTEST_ROOT}/env.local}"
  set +a
  BASE_URL="${SECTEST_BASE_URL:-${STAGING_BASE_URL:-${BASE_URL:-}}}"
fi

CURL=(curl -4 -sS -m 30)
if [[ "${STAGING_INSECURE:-0}" == "1" ]]; then
  CURL+=( -k )
fi

SECTEST_CHECKS=0
SECTEST_FAILURES=0

require_base() {
  if [[ -z "${BASE_URL}" ]]; then
    echo "SECTEST_BASE_URL or STAGING_BASE_URL is required" >&2
    exit 1
  fi
  BASE_URL="${BASE_URL%/}"
}

require_cmds() {
  for cmd in "$@"; do
    if ! command -v "${cmd}" >/dev/null 2>&1; then
      echo "missing required command: ${cmd}" >&2
      exit 1
    fi
  done
}

json_post() {
  local path="$1" token="$2" body="$3"
  local auth=()
  [[ -n "${token}" ]] && auth=( -H "Authorization: Bearer ${token}" )
  "${CURL[@]}" -X POST "${BASE_URL}${path}" \
    -H 'Content-Type: application/json' \
    "${auth[@]}" \
    -d "${body}"
}

json_get() {
  local path="$1" token="$2"
  local auth=()
  [[ -n "${token}" ]] && auth=( -H "Authorization: Bearer ${token}" )
  "${CURL[@]}" "${BASE_URL}${path}" "${auth[@]}"
}

json_put() {
  local path="$1" token="$2" body="$3"
  "${CURL[@]}" -X PUT "${BASE_URL}${path}" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${token}" \
    -d "${body}"
}

json_patch() {
  local path="$1" token="$2" body="$3"
  "${CURL[@]}" -X PATCH "${BASE_URL}${path}" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${token}" \
    -d "${body}"
}

json_delete() {
  local path="$1" token="$2"
  "${CURL[@]}" -X DELETE "${BASE_URL}${path}" \
    -H "Authorization: Bearer ${token}"
}

http_code_post() {
  local path="$1" token="$2" body="$3"
  local auth=()
  [[ -n "${token}" ]] && auth=( -H "Authorization: Bearer ${token}" )
  "${CURL[@]}" -o /dev/null -w '%{http_code}' -X POST "${BASE_URL}${path}" \
    -H 'Content-Type: application/json' \
    "${auth[@]}" \
    -d "${body}"
}

http_code_get() {
  local path="$1" token="$2"
  local auth=()
  [[ -n "${token}" ]] && auth=( -H "Authorization: Bearer ${token}" )
  "${CURL[@]}" -o /dev/null -w '%{http_code}' "${BASE_URL}${path}" "${auth[@]}"
}

register_user() {
  local email="$1" password="$2"
  json_post /v1/auth/register '' "{\"email\":\"${email}\",\"password\":\"${password}\"}" >/dev/null || true
  local login
  login="$(json_post /v1/auth/login '' "{\"email\":\"${email}\",\"password\":\"${password}\"}")"
  echo "${login}" | jq -r '.data.token // empty'
}

admin_token() {
  if [[ -z "${STAGING_ADMIN_EMAIL:-}" || -z "${STAGING_ADMIN_PASSWORD:-}" ]]; then
    return 1
  fi
  local login
  login="$(json_post /v1/auth/login '' \
    "{\"email\":\"${STAGING_ADMIN_EMAIL}\",\"password\":\"${STAGING_ADMIN_PASSWORD}\"}")"
  echo "${login}" | jq -r '.data.token // empty'
}

user_id_for_token() {
  local token="$1"
  json_get /v1/me "${token}" | jq -r '.data.id // empty'
}

bump_user_plan() {
  local user_id="$1" plan="$2" admin_tok="$3"
  json_put "/v1/admin/users/${user_id}/plan" "${admin_tok}" "{\"plan\":\"${plan}\"}" >/dev/null
}

assert_code() {
  local name="$1" got="$2" want="$3"
  SECTEST_CHECKS=$((SECTEST_CHECKS + 1))
  if [[ "${got}" == "${want}" ]]; then
    echo "    OK: ${name} (HTTP ${got})"
  else
    SECTEST_FAILURES=$((SECTEST_FAILURES + 1))
    echo "    FAIL: ${name} — expected HTTP ${want}, got ${got}" >&2
  fi
}

assert_one_of() {
  local name="$1" got="$2"
  shift 2
  SECTEST_CHECKS=$((SECTEST_CHECKS + 1))
  for want in "$@"; do
    if [[ "${got}" == "${want}" ]]; then
      echo "    OK: ${name} (HTTP ${got})"
      return 0
    fi
  done
  SECTEST_FAILURES=$((SECTEST_FAILURES + 1))
  echo "    FAIL: ${name} — expected one of [$*], got ${got}" >&2
}

finish_sectest() {
  [[ "${SECTEST_FAILURES}" -eq 0 ]]
}
