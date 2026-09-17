#!/usr/bin/env bash
# Staging release from WSL — deploy new GHCR image to VM and run tests.
#
# Run AFTER GitHub Actions "Publish Docker image" is green.
#
# One-time WSL env (add to ~/.bashrc or deploy/staging.env):
#   export STAGING_SSH_HOST=52.66.136.127
#   export STAGING_BASE_URL=http://52.66.136.127
#   export STAGING_ADMIN_EMAIL=admin@zettaflux.com
#   export STAGING_ADMIN_PASSWORD='...'
#
# Usage:
#   ./deploy/scripts/staging-release.sh              # deploy + smoke (~2 min)
#   ./deploy/scripts/staging-release.sh --full       # + functional + load smoke + sectest
#   ./deploy/scripts/staging-release.sh --test-only  # skip deploy (VM already updated)
#   ./deploy/scripts/staging-release.sh --deploy-only
#   ./deploy/scripts/staging-release.sh --sync       # also sync monitoring files to VM
#
# Optional: source deploy/staging.env if present (gitignored)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${ROOT}"

if [[ -f "${ROOT}/deploy/deploy-lightsail/staging.env" ]]; then
  # shellcheck disable=SC1091
  set -a
  source "${ROOT}/deploy/deploy-lightsail/staging.env"
  set +a
fi

DEPLOY=1
TEST=1
FULL=0
SYNC=0

for arg in "$@"; do
  case "${arg}" in
    --deploy-only) TEST=0 ;;
    --test-only) DEPLOY=0 ;;
    --full) FULL=1 ;;
    --sync) SYNC=1 ;;
    -h|--help)
      sed -n '2,20p' "$0"
      exit 0
      ;;
    *)
      echo "unknown option: ${arg}" >&2
      exit 1
      ;;
  esac
done

STAGING_SSH_HOST="${STAGING_SSH_HOST:-}"
STAGING_SSH_USER="${STAGING_SSH_USER:-ubuntu}"
STAGING_BASE_URL="${STAGING_BASE_URL:-}"

if [[ -z "${STAGING_SSH_HOST}" && -n "${STAGING_BASE_URL}" ]]; then
  STAGING_SSH_HOST="$(echo "${STAGING_BASE_URL}" | sed -E 's#^https?://([^/:]+).*#\1#')"
fi
if [[ -z "${STAGING_BASE_URL}" && -n "${STAGING_SSH_HOST}" ]]; then
  STAGING_BASE_URL="http://${STAGING_SSH_HOST}"
fi

REMOTE="${STAGING_SSH_USER}@${STAGING_SSH_HOST}"

require_env() {
  local missing=0
  [[ -z "${STAGING_SSH_HOST}" ]] && echo "missing STAGING_SSH_HOST (or STAGING_BASE_URL)" >&2 && missing=1
  [[ -z "${STAGING_BASE_URL}" ]] && echo "missing STAGING_BASE_URL" >&2 && missing=1
  [[ -z "${STAGING_ADMIN_EMAIL:-}" ]] && echo "missing STAGING_ADMIN_EMAIL" >&2 && missing=1
  [[ -z "${STAGING_ADMIN_PASSWORD:-}" ]] && echo "missing STAGING_ADMIN_PASSWORD" >&2 && missing=1
  (( missing == 0 )) || exit 1
}

step() {
  echo
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  $*"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

remote() {
  ssh -o BatchMode=yes -o ConnectTimeout=15 "${REMOTE}" "$@"
}

require_env

step "Staging release → ${REMOTE} (${STAGING_BASE_URL})"

if [[ "${SYNC}" == "1" ]]; then
  step "[WSL] Sync monitoring files to VM"
  bash "${ROOT}/deploy/deploy-lightsail/scripts/sync-monitoring-to-vm.sh" "${STAGING_SSH_HOST}"
fi

if [[ "${DEPLOY}" == "1" ]]; then
  step "[WSL] Sync migrations + scripts to VM"
  rsync -az --checksum "${ROOT}/migrations/" "${REMOTE}:/opt/zettabridge/migrations/"
  rsync -az --checksum "${ROOT}/deploy/deploy-lightsail/scripts/" "${REMOTE}:/opt/zettabridge/scripts/"

  step "[VM via SSH] Pull new image + restart app"
  remote bash -s <<'REMOTE_DEPLOY'
set -euo pipefail
cd /opt/zettabridge
./deploy.sh pull
./deploy.sh up
./deploy.sh migrate
echo "deploy OK"
REMOTE_DEPLOY

  step "[VM via SSH] Preflight (restart redis/server, fix monitoring)"
  remote bash -s <<'REMOTE_PREFLIGHT'
set -euo pipefail
if [[ -x /opt/zettabridge/scripts/preflight-vm.sh ]]; then
  /opt/zettabridge/scripts/preflight-vm.sh --fix-monitoring
else
  echo "preflight-vm.sh missing — scp from repo or run sync-monitoring-to-vm.sh" >&2
  exit 1
fi
REMOTE_PREFLIGHT
else
  step "[skip] Deploy (--test-only)"
fi

if [[ "${TEST}" == "0" ]]; then
  echo
  echo "Deploy complete (--deploy-only)."
  exit 0
fi

export STAGING_BASE_URL="${STAGING_BASE_URL%/}"

step "[WSL] Preflight checks"
bash "${ROOT}/deploy/scripts/preflight-wsl.sh" --require-admin

step "[WSL] Smoke test"
bash "${ROOT}/deploy/scripts/staging-smoke.sh"

if [[ "${FULL}" == "1" ]]; then
  step "[WSL] Functional tests"
  bash "${ROOT}/functional-tester/run-functional-staging.sh"

  step "[WSL] Load test (smoke — 10 VU)"
  if [[ ! -f "${ROOT}/deploy/loadtest/env.local" ]]; then
    bash "${ROOT}/deploy/loadtest/setup-loadtest-webhook.sh"
  fi
  bash "${ROOT}/deploy/scripts/preflight-wsl.sh" --require-admin --require-loadtest
  bash "${ROOT}/deploy/loadtest/run-loadtest.sh" smoke

  step "[WSL] Pen test"
  bash "${ROOT}/deploy/sectest/run-sectest.sh"
fi

step "Done"
echo "  Smoke: PASS"
[[ "${FULL}" == "1" ]] && echo "  Full suite: completed"
echo
echo "Grafana (optional): ssh -N -L 3000:127.0.0.1:3000 ${REMOTE}"
echo "  → http://localhost:3000 → ZettaBridge Overview → Last 15 minutes"
