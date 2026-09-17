#!/usr/bin/env bash
# JWT tamper / missing Bearer probes (5B.2).

set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require_cmds curl jq
require_base

echo "==> jwt-tamper"

code="$(http_code_get /v1/me '')"
assert_code 'GET /v1/me without Bearer' "${code}" '401'

code="$("${CURL[@]}" -o /dev/null -w '%{http_code}' "${BASE_URL}/v1/me" \
  -H 'Authorization: Bearer not-a-jwt')"
assert_code 'GET /v1/me garbage JWT' "${code}" '401'

code="$("${CURL[@]}" -o /dev/null -w '%{http_code}' "${BASE_URL}/v1/me" \
  -H 'Authorization: Bearer eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJhYmMiLCJyb2xlIjoidXNlciJ9.')"
assert_code 'GET /v1/me alg=none JWT' "${code}" '401'

# Valid user token, then tamper last character
ts="$(date +%s)"
email="sectest-jwt-${ts}@sectest.local"
pass="sectest-${ts}"
token="$(register_user "${email}" "${pass}")"
if [[ -z "${token}" ]]; then
  echo "    FAIL: could not register probe user" >&2
  exit 1
fi

tampered="${token%?}X"
code="$(http_code_get /v1/me "${tampered}")"
assert_code 'GET /v1/me tampered signature' "${code}" '401'

finish_sectest
