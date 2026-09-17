#!/usr/bin/env bash
# Free disk space on the Lightsail monitoring / staging VM.
#
# Usage (on VM, or via SSH):
#   /opt/zettabridge/scripts/vm-disk-cleanup.sh
#   /opt/zettabridge/scripts/vm-disk-cleanup.sh --prometheus   # reset Prometheus TSDB
#   /opt/zettabridge/scripts/vm-disk-cleanup.sh --dry-run
#
# Typical causes: Docker images, Prometheus TSDB, container logs, apt cache.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
DRY_RUN=0
RESET_PROMETHEUS=0

for arg in "$@"; do
  case "${arg}" in
    --dry-run) DRY_RUN=1 ;;
    --prometheus) RESET_PROMETHEUS=1 ;;
    -h|--help)
      sed -n '2,10p' "$0"
      exit 0
      ;;
    *)
      echo "unknown argument: ${arg}" >&2
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

section "Disk before"
df -h /
echo
if command -v docker >/dev/null 2>&1; then
  echo "Docker:"
  docker system df 2>/dev/null || true
  echo
  echo "Largest Docker dirs:"
  du -sh /var/lib/docker/* 2>/dev/null | sort -hr | head -8 || true
fi

section "Trim container logs (keeps containers running)"
if command -v docker >/dev/null 2>&1; then
  while read -r id; do
    [[ -z "${id}" ]] && continue
    log="$(docker inspect --format='{{.LogPath}}' "${id}" 2>/dev/null || true)"
    if [[ -n "${log}" && -f "${log}" ]]; then
      size="$(du -sh "${log}" 2>/dev/null | awk '{print $1}')"
      echo "  truncate ${log} (${size})"
      if [[ "${DRY_RUN}" != "1" ]]; then
        sudo truncate -s 0 "${log}" 2>/dev/null || truncate -s 0 "${log}" 2>/dev/null || true
      fi
    fi
  done < <(docker ps -q 2>/dev/null || true)
fi

section "System caches"
run "sudo apt-get clean"
run "sudo journalctl --vacuum-size=100M"

if command -v docker >/dev/null 2>&1; then
  section "Docker prune (unused images, stopped containers, dangling volumes)"
  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "  [dry-run] docker system prune -af --volumes"
  else
    docker system prune -af --volumes
  fi
fi

if [[ "${RESET_PROMETHEUS}" == "1" ]]; then
  section "Reset Prometheus TSDB (fresh metrics history)"
  if [[ -x "${SCRIPT_DIR}/monitoring-down.sh" ]]; then
    if [[ "${DRY_RUN}" == "1" ]]; then
      echo "  [dry-run] monitoring-down + remove prometheus_data volume + monitoring-up"
    else
      "${SCRIPT_DIR}/monitoring-down.sh"
      vol="$(docker volume ls --format '{{.Name}}' | grep -E 'prometheus_data$' | head -1 || true)"
      if [[ -n "${vol}" ]]; then
        docker volume rm "${vol}"
      fi
      "${SCRIPT_DIR}/monitoring-up.sh"
    fi
  else
    echo "  monitoring scripts not found under ${SCRIPT_DIR}" >&2
    exit 1
  fi
fi

section "Disk after"
df -h /

echo
echo "If still low, re-run with --prometheus to wipe metrics history."
echo "Sync updated retention limits from repo:"
echo "  ./deploy/deploy-aws-ecs/scripts/monitoring-enable-aws-scrape.sh"
