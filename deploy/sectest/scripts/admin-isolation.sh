#!/usr/bin/env bash
# Non-admin must not reach /v1/admin/* (5B.2).

set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require_cmds curl jq
require_base

echo "==> admin-isolation"

ts="$(date +%s)"
email="sectest-admin-${ts}@sectest.local"
pass="sectest-${ts}"
token="$(register_user "${email}" "${pass}")"
if [[ -z "${token}" ]]; then
  echo "    FAIL: could not register probe user" >&2
  exit 1
fi

for path in /v1/admin/users /v1/admin/orgs; do
  code="$(http_code_get "${path}" "${token}")"
  assert_code "GET ${path} as regular user" "${code}" '403'
done

finish_sectest
