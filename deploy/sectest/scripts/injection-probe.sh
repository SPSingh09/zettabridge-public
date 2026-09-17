#!/usr/bin/env bash
# Oversized / malformed ingest payloads — must not 500 (5B.2).

set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require_cmds curl jq
require_base

echo "==> injection-probe"

ts="$(date +%s)"
email="sectest-inj-${ts}@sectest.local"
pass="sectest-${ts}"
token="$(register_user "${email}" "${pass}")"
if [[ -z "${token}" ]]; then
  echo "    FAIL: could not register probe user" >&2
  exit 1
fi

cred="$(json_post /v1/credentials "${token}" \
  '{"broker_type":"zerodha","account_label":"inj","raw_creds":"api_key:inj","exchange":"NSE","product":"MIS"}')"
cred_id="$(echo "${cred}" | jq -r '.data.id // empty')"
wh="$(json_post /v1/webhooks "${token}" \
  "{\"label\":\"inj\",\"broker_cred_id\":\"${cred_id}\",\"symbol\":\"RELIANCE\",\"lot_size\":1,\"max_risk_pct\":0}")"
wh_token="$(echo "${wh}" | jq -r '.data.token // empty')"

# Invalid action
code="$(http_code_post "/v1/webhook/${wh_token}" '' \
  '{"action":"HACK","symbol":"RELIANCE","lot":1}')"
assert_one_of 'invalid action' "${code}" '400' '403'

sql_payload="$(jq -nc '{action:"BUY",symbol:"RELIANCE",lot:1,comment:"'\'' OR 1=1; DROP TABLE users;--"}')"
code="$(http_code_post "/v1/webhook/${wh_token}" '' "${sql_payload}")"
assert_one_of 'SQL-ish comment in payload' "${code}" '202' '400' '401' '403'

# Oversized body (~8 KiB comment — must not 500)
big_comment="$(head -c 8192 /dev/zero | tr '\0' 'A')"
code="$(http_code_post "/v1/webhook/${wh_token}" '' \
  "$(jq -nc --arg c "${big_comment}" '{action:"BUY",symbol:"RELIANCE",lot:1,comment:$c}')")"
assert_one_of 'oversized JSON body' "${code}" '400' '413' '422' '202' '403' '401'

finish_sectest
