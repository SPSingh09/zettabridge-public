#!/usr/bin/env bash
# High RPS burst against rate-limited webhook — expect 429, server stays healthy (5B.2).

set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require_cmds curl jq
require_base

echo "==> webhook-flood"

ts="$(date +%s)"
email="sectest-flood-${ts}@sectest.local"
pass="sectest-${ts}"
token="$(register_user "${email}" "${pass}")"
if [[ -z "${token}" ]]; then
  echo "    FAIL: could not register probe user" >&2
  exit 1
fi

user_id="$(user_id_for_token "${token}")"
admin_tok="$(admin_token || true)"
if [[ -n "${admin_tok}" && -n "${user_id}" ]]; then
  bump_user_plan "${user_id}" "individual" "${admin_tok}"
  ok_msg="individual plan"
else
  ok_msg="free plan (rate limit via PUT may fail without admin)"
fi
echo "    user ${user_id:-unknown} (${ok_msg})"

cred="$(json_post /v1/credentials "${token}" \
  '{"broker_type":"zerodha","account_label":"flood","raw_creds":"api_key:flood","exchange":"NSE","product":"MIS"}')"
cred_id="$(echo "${cred}" | jq -r '.data.id // empty')"
wh="$(json_post /v1/webhooks "${token}" \
  "{\"label\":\"flood\",\"broker_cred_id\":\"${cred_id}\",\"symbol\":\"RELIANCE\",\"lot_size\":1,\"max_risk_pct\":0,\"rate_limit_per_sec\":5}")"
wh_id="$(echo "${wh}" | jq -r '.data.id // empty')"
wh_token="$(echo "${wh}" | jq -r '.data.token // empty')"
if [[ -z "${wh_token}" ]]; then
  echo "    FAIL: webhook create failed: $(echo "${wh}" | jq -c '.')" >&2
  exit 1
fi

# Ensure rate limit applied (create may ignore on older server builds).
json_put "/v1/webhooks/${wh_id}" "${token}" '{"rate_limit_per_sec":5}' >/dev/null || true

payload='{"action":"BUY","symbol":"RELIANCE","lot":1,"comment":"flood"}'
url="${BASE_URL}/v1/webhook/${wh_token}"
burst="${SECTEST_FLOOD_BURST:-25}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

for i in $(seq 1 "${burst}"); do
  curl -4 -sS -m 10 -o "${tmpdir}/${i}.json" -w '%{http_code}' \
    -X POST "${url}" -H 'Content-Type: application/json' -d "${payload}" \
    > "${tmpdir}/${i}.code" &
done
wait

accepted=0
limited=0
other=0
for i in $(seq 1 "${burst}"); do
  c="$(tr -d '\n' < "${tmpdir}/${i}.code")"
  case "${c}" in
    202) accepted=$((accepted + 1)) ;;
    429) limited=$((limited + 1)) ;;
    *) other=$((other + 1)) ;;
  esac
done

SECTEST_CHECKS=$((SECTEST_CHECKS + 1))
if [[ "${limited}" -ge 1 ]]; then
  echo "    OK: rate limit triggered (${limited}/${burst} HTTP 429, ${accepted} accepted)"
elif [[ "${other}" -ge 1 ]]; then
  SECTEST_FAILURES=$((SECTEST_FAILURES + 1))
  echo "    FAIL: expected HTTP 429, got accepted=${accepted} other=${other} (503=queue full)" >&2
else
  SECTEST_FAILURES=$((SECTEST_FAILURES + 1))
  echo "    FAIL: expected at least one HTTP 429, got accepted=${accepted}" >&2
fi

health="$(http_code_get /healthz '')"
assert_code 'GET /healthz after flood' "${health}" '200'

finish_sectest
