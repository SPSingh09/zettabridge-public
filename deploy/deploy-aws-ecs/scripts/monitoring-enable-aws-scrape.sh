#!/usr/bin/env bash
# Add AWS staging as a second Prometheus scrape target on the Lightsail monitoring VM.
#
# Automates the four manual steps:
#   1. Sync updated monitoring files to the VM
#   2. Patch /opt/zettabridge/env/monitoring.env with AWS_STAGING_SCRAPE_HOST
#   3. Restart the monitoring stack
#   4. Verify both scrape targets are UP
#
# Usage:
#   ./deploy/scripts/monitoring-enable-aws-scrape.sh
#   ./deploy/scripts/monitoring-enable-aws-scrape.sh 52.66.136.127
#
# Environment overrides:
#   VM_HOST                    Lightsail VM IP (default: 52.66.136.127)
#   VM_USER                    SSH user (default: ubuntu)
#   AWS_STAGING_SCRAPE_HOST    API hostname to scrape (default: api.staging.zettabridge.net)
#   AWS_STAGING_METRICS_TOKEN  Bearer token if METRICS_TOKEN is set on ECS (default: empty)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"

VM_HOST="${1:-${VM_HOST:-52.66.136.127}}"
VM_USER="${VM_USER:-ubuntu}"
REMOTE="${VM_USER}@${VM_HOST}"
AWS_HOST="${AWS_STAGING_SCRAPE_HOST:-api.staging.zettabridge.net}"
AWS_TOKEN="${AWS_STAGING_METRICS_TOKEN:-}"

step() {
  echo
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  $*"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

ok()   { echo "  ✓ $*"; }
fail() { echo "  ✗ $*" >&2; exit 1; }

echo "ZettaBridge — enable AWS staging scrape on Lightsail monitoring"
echo "  VM:              ${REMOTE}"
echo "  AWS scrape host: ${AWS_HOST}"
echo "  metrics token:   ${AWS_TOKEN:+(set)}"
echo "  metrics token:   ${AWS_TOKEN:-<empty — no bearer auth>}"

# ── 1. Sync monitoring files ────────────────────────────────────────────────
step "Step 1/4 — Sync monitoring files to VM"
bash "${ROOT}/deploy/deploy-lightsail/scripts/sync-monitoring-to-vm.sh" "${VM_HOST}"
ok "monitoring files synced"

# ── 2. Patch monitoring.env on VM ───────────────────────────────────────────
step "Step 2/4 — Patch monitoring.env on VM"

ssh "${REMOTE}" bash <<EOF
set -euo pipefail
ENV_FILE=/opt/zettabridge/env/monitoring.env

if [[ ! -f "\${ENV_FILE}" ]]; then
  echo "monitoring.env not found — creating from example"
  cp /opt/zettabridge/env/monitoring.env.example "\${ENV_FILE}"
  chmod 600 "\${ENV_FILE}"
fi

# Upsert AWS_STAGING_SCRAPE_HOST
if grep -q '^AWS_STAGING_SCRAPE_HOST=' "\${ENV_FILE}"; then
  sed -i 's|^AWS_STAGING_SCRAPE_HOST=.*|AWS_STAGING_SCRAPE_HOST=${AWS_HOST}|' "\${ENV_FILE}"
  echo "  updated AWS_STAGING_SCRAPE_HOST"
elif grep -q '^#.*AWS_STAGING_SCRAPE_HOST=' "\${ENV_FILE}"; then
  sed -i 's|^#.*AWS_STAGING_SCRAPE_HOST=.*|AWS_STAGING_SCRAPE_HOST=${AWS_HOST}|' "\${ENV_FILE}"
  echo "  uncommented AWS_STAGING_SCRAPE_HOST"
else
  echo "AWS_STAGING_SCRAPE_HOST=${AWS_HOST}" >> "\${ENV_FILE}"
  echo "  appended AWS_STAGING_SCRAPE_HOST"
fi

# Upsert AWS_STAGING_METRICS_TOKEN (only if provided)
if [[ -n "${AWS_TOKEN}" ]]; then
  if grep -q '^AWS_STAGING_METRICS_TOKEN=' "\${ENV_FILE}"; then
    sed -i 's|^AWS_STAGING_METRICS_TOKEN=.*|AWS_STAGING_METRICS_TOKEN=${AWS_TOKEN}|' "\${ENV_FILE}"
    echo "  updated AWS_STAGING_METRICS_TOKEN"
  elif grep -q '^#.*AWS_STAGING_METRICS_TOKEN=' "\${ENV_FILE}"; then
    sed -i 's|^#.*AWS_STAGING_METRICS_TOKEN=.*|AWS_STAGING_METRICS_TOKEN=${AWS_TOKEN}|' "\${ENV_FILE}"
    echo "  uncommented AWS_STAGING_METRICS_TOKEN"
  else
    echo "AWS_STAGING_METRICS_TOKEN=${AWS_TOKEN}" >> "\${ENV_FILE}"
    echo "  appended AWS_STAGING_METRICS_TOKEN"
  fi
fi

echo "  monitoring.env patched"
EOF

ok "monitoring.env patched on VM"

# ── 3. Restart monitoring stack ─────────────────────────────────────────────
step "Step 3/4 — Restart monitoring stack"
ssh "${REMOTE}" /opt/zettabridge/scripts/monitoring-up.sh
ok "monitoring stack restarted"

# ── 4. Verify both targets are UP ───────────────────────────────────────────
step "Step 4/4 — Verify Prometheus targets"

echo "  Waiting for Prometheus to be ready..."
ssh "${REMOTE}" bash <<'EOF'
for i in $(seq 1 20); do
  if curl -sf http://127.0.0.1:9090/-/ready >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
EOF

# Query targets via SSH and parse locally
targets_json="$(ssh "${REMOTE}" \
  "curl -sS http://127.0.0.1:9090/api/v1/targets")"

if [[ -z "${targets_json}" ]]; then
  fail "Could not reach Prometheus API on VM"
fi

echo
echo "  Scrape targets:"
echo "${targets_json}" | jq -r \
  '.data.activeTargets[] | "  job=\(.labels.job)  health=\(.health)  url=\(.scrapeUrl)"'

lightsail_health="$(echo "${targets_json}" | jq -r \
  '.data.activeTargets[] | select(.labels.job=="zettabridge") | .health' | head -1)"
aws_health="$(echo "${targets_json}" | jq -r \
  '.data.activeTargets[] | select(.labels.job=="zettabridge-aws-staging") | .health' | head -1)"

echo
[[ "${lightsail_health}" == "up" ]] && ok "zettabridge (Lightsail) — UP" \
  || echo "  ⚠  zettabridge (Lightsail) — ${lightsail_health:-not found}"
[[ "${aws_health}" == "up" ]]       && ok "zettabridge-aws-staging (AWS) — UP" \
  || echo "  ⚠  zettabridge-aws-staging (AWS) — ${aws_health:-not found} (may need ~30s on first scrape)"

# ── Summary ─────────────────────────────────────────────────────────────────
echo
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Done. Open Grafana via SSH tunnel:"
echo
echo "    ssh -N -L 3001:127.0.0.1:3001 ${REMOTE}"
echo
echo "  Then open: http://localhost:3001"
echo "  Filter by cluster label: lightsail-mumbai | aws-ecs-staging"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
