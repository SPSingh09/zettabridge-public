#!/usr/bin/env bash
# Webhook token abuse: unknown token, rotate invalidates old URL, paused webhook (5B.2).

set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require_cmds curl jq
require_base

echo "==> webhook-token-probe"

payload='{"action":"BUY","symbol":"RELIANCE","lot":1,"comment":"sectest"}'

code="$(http_code_post "/v1/webhook/not-a-real-token-${RANDOM}" '' "${payload}")"
assert_code 'POST unknown webhook token' "${code}" '401'

ts="$(date +%s)"
email="sectest-wh-${ts}@sectest.local"
pass="sectest-${ts}"
token="$(register_user "${email}" "${pass}")"
if [[ -z "${token}" ]]; then
  echo "    FAIL: could not register probe user" >&2
  exit 1
fi

cred="$(json_post /v1/credentials "${token}" \
  '{"broker_type":"zerodha","account_label":"sectest","raw_creds":"api_key:sectest","exchange":"NSE","product":"MIS"}')"
cred_id="$(echo "${cred}" | jq -r '.data.id // empty')"
wh="$(json_post /v1/webhooks "${token}" \
  "{\"label\":\"sectest\",\"broker_cred_id\":\"${cred_id}\",\"symbol\":\"RELIANCE\",\"lot_size\":1,\"max_risk_pct\":0}")"
wh_id="$(echo "${wh}" | jq -r '.data.id // empty')"
wh_token="$(echo "${wh}" | jq -r '.data.token // empty')"
if [[ -z "${wh_token}" ]]; then
  echo "    FAIL: webhook create failed: $(echo "${wh}" | jq -c '.')" >&2
  exit 1
fi

code="$(http_code_post "/v1/webhook/${wh_token}" '' "${payload}")"
assert_code 'POST valid webhook token' "${code}" '202'

rot="$(json_post "/v1/webhooks/${wh_id}/rotate-token" "${token}" '{}')"
new_token="$(echo "${rot}" | jq -r '.data.token // empty')"
if [[ -z "${new_token}" ]]; then
  echo "    FAIL: rotate-token failed: $(echo "${rot}" | jq -c '.')" >&2
  exit 1
fi

code="$(http_code_post "/v1/webhook/${wh_token}" '' "${payload}")"
assert_code 'POST old token after rotate' "${code}" '401'

pause="$(json_put "/v1/webhooks/${wh_id}/pause" "${token}" '{}')"
if echo "${pause}" | jq -e '.error' >/dev/null 2>&1; then
  echo "    FAIL: pause webhook: $(echo "${pause}" | jq -c '.')" >&2
else
  code="$(http_code_post "/v1/webhook/${new_token}" '' "${payload}")"
  assert_code 'POST paused webhook' "${code}" '403'
fi

finish_sectest
