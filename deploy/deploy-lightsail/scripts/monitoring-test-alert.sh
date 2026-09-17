#!/usr/bin/env bash
# Send a test alert through Alertmanager (staging Telegram chat).
#
# Usage:
#   ./deploy/scripts/monitoring-test-alert.sh
#
# Requires monitoring stack running and monitoring.env configured.

set -euo pipefail

AM_URL="${ALERTMANAGER_URL:-http://127.0.0.1:9093}"
ENVIRONMENT="${OBS_ENVIRONMENT:-staging}"

payload="$(cat <<EOF
[
  {
    "labels": {
      "alertname": "ZettaBridgeTestAlert",
      "severity": "warning",
      "environment": "${ENVIRONMENT}"
    },
    "annotations": {
      "summary": "Manual test alert from monitoring-test-alert.sh",
      "description": "If you see this in Telegram staging chat, Alertmanager routing works."
    }
  }
]
EOF
)"

code="$(curl -4 -sS -o /tmp/zb-am-test.out -w '%{http_code}' -X POST \
  "${AM_URL}/api/v2/alerts" \
  -H 'Content-Type: application/json' \
  -d "${payload}")"

if [[ "${code}" != "200" ]]; then
  echo "Alertmanager POST failed: HTTP ${code}" >&2
  cat /tmp/zb-am-test.out >&2
  exit 1
fi

echo "Test alert sent to Alertmanager (check Telegram staging chat)."
