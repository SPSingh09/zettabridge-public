#!/usr/bin/env bash
# Staging smoke test — run from laptop/WSL against any staging URL (Lightsail or AWS).
#
# AWS staging (primary):
#   source deploy/staging-aws.env   # sets STAGING_BASE_URL + admin creds
#   ./deploy/scripts/staging-smoke.sh
#
# Manual override:
#   export STAGING_BASE_URL=https://api.staging.zettabridge.net
#   export STAGING_ADMIN_EMAIL=admin@zettabridge.net
#   export STAGING_ADMIN_PASSWORD='...'
#   ./deploy/scripts/staging-smoke.sh
#
# Lightsail (legacy):
#   export STAGING_BASE_URL=http://52.66.136.127
#   export STAGING_ADMIN_EMAIL=admin@zettaflux.com
#
# Optional:
#   STAGING_REGISTER_ADMIN=1       register admin if login fails (first-time bootstrap)
#   STAGING_SKIP_ADMIN=1           skip admin login (market-hours disable skipped; ingest may fail off-hours)
#   STAGING_SMOKE_USER_PASSWORD=   password for ephemeral smoke user (default generated)
#   STAGING_INSECURE=1             curl -k for self-signed HTTPS
#
# Flow: ephemeral free-tier user → paper account → paper webhook → ORDER_SIGNAL ingest → poll FILLED order.
#
# Requires: curl, jq

set -euo pipefail

BASE_URL="${STAGING_BASE_URL:-}"
ADMIN_EMAIL="${STAGING_ADMIN_EMAIL:-}"
ADMIN_PASSWORD="${STAGING_ADMIN_PASSWORD:-}"
REGISTER_ADMIN="${STAGING_REGISTER_ADMIN:-0}"
SKIP_ADMIN="${STAGING_SKIP_ADMIN:-0}"
SMOKE_PASSWORD="${STAGING_SMOKE_USER_PASSWORD:-staging-smoke-$(date +%s)}"
SMOKE_EMAIL="staging-smoke-$(date +%s)@smoke.local"
SMOKE_SYMBOL="${STAGING_SMOKE_SYMBOL:-TCS}"
SMOKE_COMMENT="${STAGING_SMOKE_COMMENT:-staging-smoke}"
SMOKE_QUANTITY="${STAGING_SMOKE_QUANTITY:-10}"
SMOKE_PRICE="${STAGING_SMOKE_PRICE:-721.5}"

CURL=(curl -4 -sS -m 30)
if [[ "${STAGING_INSECURE:-0}" == "1" ]]; then
  CURL+=( -k )
fi

step=0
pass=0
failures=0

log() { echo "==> $*"; }
ok_step() { pass=$((pass + 1)); echo "    OK: $*"; }
fail_step() { failures=$((failures + 1)); echo "    FAIL: $*" >&2; }

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

json_post() {
  local path="$1" token="$2" body="$3"
  local auth=()
  if [[ -n "$token" ]]; then
    auth=( -H "Authorization: Bearer ${token}" )
  fi
  "${CURL[@]}" -X POST "${BASE_URL}${path}" \
    -H 'Content-Type: application/json' \
    "${auth[@]}" \
    -d "$body"
}

json_get() {
  local path="$1" token="$2"
  "${CURL[@]}" "${BASE_URL}${path}" \
    -H "Authorization: Bearer ${token}"
}

json_put() {
  local path="$1" token="$2" body="$3"
  "${CURL[@]}" -X PUT "${BASE_URL}${path}" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${token}" \
    -d "$body"
}

http_code() {
  local path="$1" token="$2" method="${3:-GET}" body="${4:-}"
  local auth=()
  if [[ -n "$token" ]]; then
    auth=( -H "Authorization: Bearer ${token}" )
  fi
  if [[ "$method" == "POST" ]]; then
    "${CURL[@]}" -o /dev/null -w '%{http_code}' -X POST "${BASE_URL}${path}" \
      -H 'Content-Type: application/json' \
      "${auth[@]}" \
      -d "$body"
  else
    "${CURL[@]}" -o /dev/null -w '%{http_code}' "${BASE_URL}${path}" \
      "${auth[@]}"
  fi
}

extract_token() {
  jq -r '.data.token // empty'
}

run_step() {
  step=$((step + 1))
  log "Step ${step}: $*"
}

require_cmd curl
require_cmd jq

if [[ -z "${BASE_URL}" ]]; then
  echo "STAGING_BASE_URL is required (e.g. http://52.66.136.127)" >&2
  exit 1
fi
BASE_URL="${BASE_URL%/}"

echo "ZettaBridge staging smoke"
echo "  BASE_URL=${BASE_URL}"
echo "  SMOKE_USER=${SMOKE_EMAIL}"
echo

# ── 1. Health ────────────────────────────────────────────────────────────────
run_step "GET /healthz"
code="$(http_code /healthz '' GET)"
body="$("${CURL[@]}" "${BASE_URL}/healthz")"
if [[ "$code" == "200" ]] && [[ "$(echo "$body" | jq -r '.data.status // .status // empty')" == "ok" ]]; then
  ok_step "healthz 200 status=ok"
else
  fail_step "healthz want 200 status=ok, got HTTP ${code} body=${body}"
fi

# ── 2. Admin login (optional) ───────────────────────────────────────────────
ADMIN_TOKEN=""
if [[ "${SKIP_ADMIN}" == "1" ]]; then
  log "Skipping admin checks (STAGING_SKIP_ADMIN=1)"
else
  if [[ -z "${ADMIN_EMAIL}" || -z "${ADMIN_PASSWORD}" ]]; then
    fail_step "set STAGING_ADMIN_EMAIL and STAGING_ADMIN_PASSWORD, or STAGING_SKIP_ADMIN=1"
  else
    run_step "Admin login (${ADMIN_EMAIL})"
    login_body="$(json_post /v1/auth/login '' \
      "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}")"
    ADMIN_TOKEN="$(echo "$login_body" | extract_token)"

    if [[ -z "${ADMIN_TOKEN}" && "${REGISTER_ADMIN}" == "1" ]]; then
      log "    Admin login failed — registering (STAGING_REGISTER_ADMIN=1)"
      json_post /v1/auth/register '' \
        "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}" >/dev/null || true
      echo "    NOTE: restart server with BOOTSTRAP_ADMIN_EMAIL set, then re-run smoke" >&2
      login_body="$(json_post /v1/auth/login '' \
        "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}")"
      ADMIN_TOKEN="$(echo "$login_body" | extract_token)"
    fi

    if [[ -z "${ADMIN_TOKEN}" ]]; then
      fail_step "admin login failed: $(echo "$login_body" | jq -c '.')"
    else
      ok_step "admin login"

      run_step "GET /v1/me (admin role)"
      me="$(json_get /v1/me "$ADMIN_TOKEN")"
      role="$(echo "$me" | jq -r '.data.role // empty')"
      if [[ "$role" == "admin" ]]; then
        ok_step "role=admin"
      else
        fail_step "expected role=admin, got role=${role} (register admin user and restart with BOOTSTRAP_ADMIN_EMAIL?)"
      fi
    fi
  fi
fi

# ── 2b. Disable market-hours enforcement (admin, for deterministic paper fills) ─
if [[ -n "${ADMIN_TOKEN}" ]]; then
  run_step "Admin: disable market-hours enforcement"
  mh="$(json_put /v1/admin/settings/market-hours-enforcement "$ADMIN_TOKEN" '{"enabled":false}')"
  if echo "$mh" | jq -e '.data.enabled == false' >/dev/null 2>&1; then
    ok_step "market-hours enforcement disabled"
  else
    fail_step "disable market hours: $(echo "$mh" | jq -c '.')"
  fi
fi

# ── 3. Smoke user register + login ───────────────────────────────────────────
run_step "Register smoke user"
reg="$(json_post /v1/auth/register '' \
  "{\"email\":\"${SMOKE_EMAIL}\",\"password\":\"${SMOKE_PASSWORD}\"}")"
reg_err="$(echo "$reg" | jq -r '.error // empty')"
if [[ -n "$reg_err" ]]; then
  fail_step "register: ${reg_err}"
else
  ok_step "registered ${SMOKE_EMAIL}"
fi

run_step "Login smoke user"
login="$(json_post /v1/auth/login '' \
  "{\"email\":\"${SMOKE_EMAIL}\",\"password\":\"${SMOKE_PASSWORD}\"}")"
USER_TOKEN="$(echo "$login" | extract_token)"
if [[ -z "${USER_TOKEN}" ]]; then
  fail_step "smoke user login: $(echo "$login" | jq -c '.')"
  echo
  echo "smoke finished: ${pass} passed, ${failures} failed" >&2
  exit 1
fi
ok_step "smoke user login"

run_step "GET /v1/me (smoke user)"
me="$(json_get /v1/me "$USER_TOKEN")"
USER_ID="$(echo "$me" | jq -r '.data.id // empty')"
if [[ -n "$USER_ID" ]]; then
  ok_step "user id=${USER_ID}"
else
  fail_step "GET /v1/me missing id"
fi

# ── 4. Paper account (free tier: 1 account, no broker credentials) ───────────
run_step "POST /v1/paper-accounts"
paper="$(json_post /v1/paper-accounts "$USER_TOKEN" \
  '{"label":"smoke-paper","starting_balance":100000,"exchange":"NSE","default_product":"MIS"}')"
PAPER_ACCOUNT_ID="$(echo "$paper" | jq -r '.data.id // empty')"
if [[ -n "$PAPER_ACCOUNT_ID" ]]; then
  ok_step "paper account id=${PAPER_ACCOUNT_ID}"
else
  fail_step "create paper account: $(echo "$paper" | jq -c '.')"
fi

# ── 5. Paper webhook ─────────────────────────────────────────────────────────
run_step "POST /v1/webhooks (paper destination)"
wh="$(json_post /v1/webhooks "$USER_TOKEN" \
  "{\"label\":\"smoke-paper\",\"paper_account_id\":\"${PAPER_ACCOUNT_ID}\",\"allowed_symbols\":[\"${SMOKE_SYMBOL}\"],\"required_comment\":\"${SMOKE_COMMENT}\",\"default_order_type\":\"MARKET\"}")"
WEBHOOK_ID="$(echo "$wh" | jq -r '.data.id // empty')"
WEBHOOK_TOKEN="$(echo "$wh" | jq -r '.data.token // empty')"
if [[ -n "$WEBHOOK_ID" && -n "$WEBHOOK_TOKEN" ]]; then
  ok_step "webhook id=${WEBHOOK_ID}"
else
  fail_step "create webhook: $(echo "$wh" | jq -c '.')"
fi

# ── 6. Ingest paper ORDER_SIGNAL ─────────────────────────────────────────────
run_step "POST /v1/webhook/:token (paper ORDER_SIGNAL BUY)"
ingest_payload="$(jq -nc \
  --arg comment "$SMOKE_COMMENT" \
  --arg symbol "$SMOKE_SYMBOL" \
  --argjson qty "$SMOKE_QUANTITY" \
  --argjson price "$SMOKE_PRICE" \
  '{comment:$comment,type:"ORDER_SIGNAL",symbol:$symbol,action:"BUY",quantity:$qty,order_type:"MARKET",price:$price}')"
ingest="$("${CURL[@]}" -w '\n%{http_code}' -X POST "${BASE_URL}/v1/webhook/${WEBHOOK_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d "$ingest_payload")"
ingest_body="$(echo "$ingest" | head -n -1)"
ingest_code="$(echo "$ingest" | tail -n 1)"
queued="$(echo "$ingest_body" | jq -r '.queued // empty')"
if [[ "$ingest_code" == "202" && "$queued" == "true" ]]; then
  ok_step "ingest 202 queued=true"
else
  fail_step "ingest want 202 queued=true, got HTTP ${ingest_code} body=${ingest_body}"
fi

# ── 7. Paper orders → FILLED (free tier cannot list webhook trade audit) ─────
run_step "GET /v1/paper-accounts/:id/orders → FILLED"
order_ok=0
status=""
for _ in $(seq 1 30); do
  orders="$(json_get "/v1/paper-accounts/${PAPER_ACCOUNT_ID}/orders" "$USER_TOKEN")"
  status="$(echo "$orders" | jq -r '.data.orders[0].status // empty')"
  order_id="$(echo "$orders" | jq -r '.data.orders[0].id // empty')"
  if [[ "$status" == "FILLED" && -n "$order_id" ]]; then
    order_ok=1
    ok_step "paper order FILLED id=${order_id}"
    break
  fi
  if [[ "$status" == "REJECTED" ]]; then
    reason="$(echo "$orders" | jq -r '.data.orders[0].reason // .data.orders[0].reject_reason // .data.orders[0].error // empty')"
    fail_step "paper order REJECTED: ${reason:-$(echo "$orders" | jq -c '.data.orders[0] // empty')}"
    break
  fi
  sleep 0.5
done
if [[ "$order_ok" -eq 0 && "$status" != "REJECTED" ]]; then
  fail_step "timed out waiting for FILLED paper order (last: $(echo "${orders:-}" | jq -c '.data.orders[0] // empty'))"
fi

# ── Summary ──────────────────────────────────────────────────────────────────
echo
if [[ "$failures" -eq 0 ]]; then
  echo "staging smoke: ALL PASSED (${pass} checks)"
  exit 0
fi
echo "staging smoke: ${failures} failed, ${pass} passed" >&2
exit 1
