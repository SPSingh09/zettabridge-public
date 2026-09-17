#!/usr/bin/env bash
# Verify ZettaBridge metrics increment on the VM (run on Lightsail).
#
# Usage:
#   ./staging-metrics-smoke.sh
#   PUBLIC_BASE=http://52.66.136.127 ./staging-metrics-smoke.sh

set -euo pipefail

LOCAL_METRICS="${LOCAL_METRICS:-http://127.0.0.1:8080/metrics}"
LOCAL_API="${LOCAL_API:-http://127.0.0.1:8080}"
PUBLIC_BASE="${PUBLIC_BASE:-http://52.66.136.127}"

read_counter() {
  local url="$1"
  local status="$2"
  curl -fsS "${url}" 2>/dev/null | awk -v s="${status}" '$1 ~ /^zettabridge_ingest_total/ && $0 ~ s {print $2; exit}'
}

section() { echo; echo "==> $*"; }

section "Listeners (80 / 8080)"
ss -tlnp 2>/dev/null | grep -E ':80 |:8080 ' || true

section "Docker containers"
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' | grep -E 'NAMES|zettabridge|server|caddy' || docker ps

section "Localhost metrics (before)"
local_q_before="$(read_counter "${LOCAL_METRICS}" 'status="queued"' || echo 0)"
local_r_before="$(read_counter "${LOCAL_METRICS}" 'status="rejected"' || echo 0)"
echo "queued=${local_q_before} rejected=${local_r_before}"

section "POST fake webhook on localhost (should increment rejected)"
code="$(curl -sS -o /tmp/zb-smoke-local.out -w '%{http_code}' -X POST \
  "${LOCAL_API}/v1/webhook/smoke-fake-token-local" \
  -H 'Content-Type: application/json' \
  -d '{"action":"BUY","symbol":"EURUSD","lot":0.01}')"
echo "HTTP ${code} (expect 401)"
local_q_after="$(read_counter "${LOCAL_METRICS}" 'status="queued"' || echo 0)"
local_r_after="$(read_counter "${LOCAL_METRICS}" 'status="rejected"' || echo 0)"
echo "queued=${local_q_after} rejected=${local_r_after}"
if [[ "${local_r_after}" -le "${local_r_before}" ]]; then
  echo "FAIL: localhost metrics did not increment rejected"
  exit 1
fi
echo "OK: localhost metrics increment"

section "Public URL reachability"
pub_health="$(curl -sS -o /tmp/zb-smoke-health.out -w '%{http_code}' -m 10 "${PUBLIC_BASE}/healthz" || echo 000)"
echo "GET ${PUBLIC_BASE}/healthz → HTTP ${pub_health}"
if [[ "${pub_health}" != "200" ]]; then
  echo "WARN: public health failed — functional tests from WSL may not hit this VM"
fi

pub_metrics_url="${PUBLIC_BASE}/metrics"
pub_q_before="$(read_counter "${pub_metrics_url}" 'status="queued"' || echo na)"
pub_r_before="$(read_counter "${pub_metrics_url}" 'status="rejected"' || echo na)"
echo "public metrics before: queued=${pub_q_before} rejected=${pub_r_before}"

section "POST fake webhook on public URL (same process if routing OK)"
pub_code="$(curl -sS -o /tmp/zb-smoke-pub.out -w '%{http_code}' -m 10 -X POST \
  "${PUBLIC_BASE}/v1/webhook/smoke-fake-token-public" \
  -H 'Content-Type: application/json' \
  -d '{"action":"BUY","symbol":"EURUSD","lot":0.01}' || echo 000)"
echo "HTTP ${pub_code} (expect 401)"
pub_q_after="$(read_counter "${pub_metrics_url}" 'status="queued"' || echo na)"
pub_r_after="$(read_counter "${pub_metrics_url}" 'status="rejected"' || echo na)"
echo "public metrics after:  queued=${pub_q_after} rejected=${pub_r_after}"

local_r_final="$(read_counter "${LOCAL_METRICS}" 'status="rejected"' || echo 0)"
echo "localhost rejected now: ${local_r_final}"

if [[ "${pub_code}" == "000" ]]; then
  echo "FAIL: cannot POST to public base URL"
  exit 1
fi

if [[ "${local_r_final}" -le "${local_r_after}" ]]; then
  echo "FAIL: public POST did not update localhost metrics — WSL functional tests are NOT hitting this server"
  echo "     Check: Caddy on :80, SERVER_BIND, STAGING_BASE_URL, only one zettabridge stack"
  exit 1
fi

echo
echo "OK: public and localhost share the same metrics (functional tests should move counters)."
echo "While running functional tests, watch:"
echo "  watch -n2 'curl -fsS ${LOCAL_METRICS} | grep zettabridge_ingest_total'"
