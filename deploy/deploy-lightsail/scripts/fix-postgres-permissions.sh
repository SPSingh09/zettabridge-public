#!/usr/bin/env bash
# Fix Postgres data dir ownership on VM (bootstrap chowned it to ubuntu by mistake).
#
# Run on VM as root or with sudo:
#   sudo bash /opt/zettabridge/scripts/fix-postgres-permissions.sh
#
# Or from WSL:
#   scp deploy/scripts/fix-postgres-permissions.sh ubuntu@IP:/tmp/
#   ssh ubuntu@IP 'sudo bash /tmp/fix-postgres-permissions.sh'

set -euo pipefail

DATA_DIR="${ZETTABRIDGE_DATA_DIR:-/opt/zettabridge/data}/postgres"
PG_UID=999
PG_GID=999

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

if [[ ! -d "${DATA_DIR}" ]]; then
  echo "creating ${DATA_DIR}"
  install -d -m 0700 -o "${PG_UID}" -g "${PG_GID}" "${DATA_DIR}"
else
  echo "==> chown ${PG_UID}:${PG_GID} ${DATA_DIR}"
  chown -R "${PG_UID}:${PG_GID}" "${DATA_DIR}"
  chmod 700 "${DATA_DIR}"
fi

echo "==> restart postgres container"
if [[ -f /opt/zettabridge/env/runtime.env ]]; then
  # shellcheck disable=SC1091
  set -a
  source /opt/zettabridge/env/runtime.env
  set +a
  export POSTGRES_PASSWORD
fi

if [[ -f /opt/zettabridge/compose/production.base.yml ]]; then
  cd /opt/zettabridge
  docker compose -f compose/production.base.yml -f compose/oci.staging.yml restart postgres
  echo "waiting for postgres..."
  sleep 5
  for _ in $(seq 1 30); do
    if docker compose -f compose/production.base.yml -f compose/oci.staging.yml exec -T postgres \
      pg_isready -U zettabridge -d zettabridge >/dev/null 2>&1; then
      echo "postgres ready"
      exit 0
    fi
    sleep 2
  done
  echo "postgres not ready — check: docker compose logs postgres --tail=30" >&2
  exit 1
fi

echo "Done. Run: cd /opt/zettabridge && ./deploy.sh migrate"
