#!/usr/bin/env bash
# IDOR: user B cannot access user A webhooks, credentials, or trades (5B.2).

set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require_cmds curl jq
require_base

echo "==> idor-probe"

ts="$(date +%s)"
pass="sectest-${ts}"

token_a="$(register_user "sectest-a-${ts}@sectest.local" "${pass}")"
token_b="$(register_user "sectest-b-${ts}@sectest.local" "${pass}")"
if [[ -z "${token_a}" || -z "${token_b}" ]]; then
  echo "    FAIL: could not register probe users" >&2
  exit 1
fi

cred_a="$(json_post /v1/credentials "${token_a}" \
  '{"broker_type":"zerodha","account_label":"a","raw_creds":"api_key:a","exchange":"NSE","product":"MIS"}')"
cred_id="$(echo "${cred_a}" | jq -r '.data.id // empty')"

wh_a="$(json_post /v1/webhooks "${token_a}" \
  "{\"label\":\"a\",\"broker_cred_id\":\"${cred_id}\",\"symbol\":\"RELIANCE\",\"lot_size\":1,\"max_risk_pct\":0}")"
wh_id="$(echo "${wh_a}" | jq -r '.data.id // empty')"
wh_token="$(echo "${wh_a}" | jq -r '.data.token // empty')"

json_post "/v1/webhook/${wh_token}" '' \
  '{"action":"BUY","symbol":"RELIANCE","lot":1,"comment":"idor-setup"}' >/dev/null || true

# Free-tier user B gets 403 (no audit) before webhook lookup; pro+ gets 404 — both deny access.
code="$(http_code_get "/v1/webhooks/${wh_id}/trades" "${token_b}")"
assert_one_of "GET /v1/webhooks/${wh_id}/trades as user B" "${code}" '404' '403'

code="$("${CURL[@]}" -o /dev/null -w '%{http_code}' -X PUT "${BASE_URL}/v1/webhooks/${wh_id}" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${token_b}" \
  -d '{"label":"hijack"}')"
assert_code "PUT /v1/webhooks/${wh_id} as user B" "${code}" '404'

code="$("${CURL[@]}" -o /dev/null -w '%{http_code}' -X DELETE "${BASE_URL}/v1/webhooks/${wh_id}" \
  -H "Authorization: Bearer ${token_b}")"
assert_one_of "DELETE /v1/webhooks/${wh_id} as user B" "${code}" '404' '403'

code="$("${CURL[@]}" -o /dev/null -w '%{http_code}' -X PUT "${BASE_URL}/v1/credentials/${cred_id}" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${token_b}" \
  -d '{"account_label":"hijack"}')"
assert_code "PUT /v1/credentials/${cred_id} as user B" "${code}" '404'

finish_sectest
