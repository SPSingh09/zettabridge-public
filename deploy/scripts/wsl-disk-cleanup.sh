#!/usr/bin/env bash
# Free disk space on WSL / dev laptop (Docker, Go cache, build artifacts).
#
# Usage (from repo root):
#   ./deploy/scripts/wsl-disk-cleanup.sh           # report + safe cleanup
#   ./deploy/scripts/wsl-disk-cleanup.sh --docker  # also prune unused Docker data
#   ./deploy/scripts/wsl-disk-cleanup.sh --all     # aggressive (includes go mod cache)
#   ./deploy/scripts/wsl-disk-cleanup.sh --dry-run # show only, no deletes
#
# After aggressive Docker cleanup, restart dev stack:
#   docker compose up -d

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DRY_RUN=0
PRUNE_DOCKER=0
AGGRESSIVE=0

for arg in "$@"; do
  case "${arg}" in
    --dry-run) DRY_RUN=1 ;;
    --docker) PRUNE_DOCKER=1 ;;
    --all) PRUNE_DOCKER=1; AGGRESSIVE=1 ;;
    -h|--help)
      sed -n '2,12p' "$0"
      exit 0
      ;;
    *)
      echo "unknown argument: ${arg} (try --help)" >&2
      exit 1
      ;;
  esac
done

run() {
  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "  [dry-run] $*"
  else
    echo "  $*"
    eval "$@"
  fi
}

section() { echo; echo "==> $*"; }

human() {
  du -sh "$1" 2>/dev/null | awk '{print $1 "\t" $2}' || echo "? $1"
}

section "Disk before"
df -h / /home 2>/dev/null || df -h /
echo
echo "Top under \$HOME:"
du -sh "${HOME}"/* 2>/dev/null | sort -hr | head -12 || true

if command -v docker >/dev/null 2>&1; then
  section "Docker usage"
  docker system df 2>/dev/null || true
fi

section "Safe cleanup (always)"
run "rm -rf '${ROOT}/deploy/loadtest/results/'*"
run "rm -rf /tmp/zb-loadtest-*.json /tmp/zb-preflight-*.json 2>/dev/null || true"
run "find '${ROOT}' -name '*.test' -type f -delete 2>/dev/null || true"
if command -v go >/dev/null 2>&1; then
  run "go clean -cache -testcache"
fi
if [[ -d "${ROOT}/dashboard/.next" ]]; then
  run "rm -rf '${ROOT}/dashboard/.next'"
fi
run "sudo apt-get clean 2>/dev/null || true"
run "journalctl --user --vacuum-size=50M 2>/dev/null || true"

if [[ "${PRUNE_DOCKER}" == "1" ]] && command -v docker >/dev/null 2>&1; then
  section "Docker prune (unused images, stopped containers, dangling volumes)"
  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "  [dry-run] docker system prune -af --volumes"
    docker system df 2>/dev/null || true
  else
    docker system prune -af --volumes
  fi
fi

if [[ "${AGGRESSIVE}" == "1" ]]; then
  section "Aggressive cleanup"
  if command -v go >/dev/null 2>&1; then
    run "go clean -modcache"
  fi
  if [[ -d "${ROOT}/dashboard/node_modules" ]]; then
    echo "  dashboard/node_modules: $(human "${ROOT}/dashboard/node_modules")"
    run "rm -rf '${ROOT}/dashboard/node_modules'"
  fi
fi

section "Disk after"
df -h / /home 2>/dev/null || df -h /

echo
echo "If WSL still shows full but Windows has free space, compact the VHD (PowerShell as Admin):"
echo "  wsl --shutdown"
echo "  Optimize-VHD -Path \"\$env:LOCALAPPDATA\\Packages\\CanonicalGroupLimited.Ubuntu24.04_*/LocalState\\ext4.vhdx\" -Mode Full"
echo "  (Or: Settings → System → Storage → Temporary files → clear old WSL/cache)"
echo
echo "Lightsail VM full? Run on the VM:"
echo "  /opt/zettabridge/scripts/vm-disk-cleanup.sh"
