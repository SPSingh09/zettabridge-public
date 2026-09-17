#!/usr/bin/env bash
# Lightsail / any Linux VM — pull private GHCR image and run compose (no source build).
#
# Usage (from repo):
#   ./deploy/lightsail/deploy.sh login-ghcr
#   ./deploy/lightsail/deploy.sh pull && ./deploy/lightsail/deploy.sh up
#
# Usage (on VM at /opt/zettabridge/deploy.sh):
#   ./deploy.sh login-ghcr
#   ./deploy.sh pull && ./deploy.sh up
#
# Requires on VM:
#   /opt/zettabridge/env/runtime.env
#   /opt/zettabridge/env/ghcr.env
#   /opt/zettabridge/compose/{production.base.yml,oci.staging.yml}
#   /opt/zettabridge/migrations/ + /opt/zettabridge/scripts/migrate-rds.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Repo layout: .../deploy/lightsail/deploy.sh
# VM layout:  /opt/zettabridge/deploy.sh + /opt/zettabridge/compose/
if [[ -f "${SCRIPT_DIR}/../compose/production.base.yml" ]]; then
  BASE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
  COMPOSE_DIR="${BASE_DIR}/compose"
  SCRIPTS_DIR="${BASE_DIR}/scripts"
  REPO_ROOT="$(cd "${BASE_DIR}/.." && pwd)"
elif [[ -f "${SCRIPT_DIR}/compose/production.base.yml" ]]; then
  BASE_DIR="${SCRIPT_DIR}"
  COMPOSE_DIR="${BASE_DIR}/compose"
  SCRIPTS_DIR="${BASE_DIR}/scripts"
  REPO_ROOT="${BASE_DIR}"
else
  echo "compose files not found — copy deploy/compose/*.yml to /opt/zettabridge/compose/" >&2
  exit 1
fi

COMPOSE_FILES=(
  -f "${COMPOSE_DIR}/production.base.yml"
  -f "${COMPOSE_DIR}/oci.staging.yml"
)

ENV_FILE="${ZETTABRIDGE_ENV_FILE:-/opt/zettabridge/env/runtime.env}"
GHCR_ENV="${ZETTABRIDGE_GHCR_ENV:-/opt/zettabridge/env/ghcr.env}"
export ZETTABRIDGE_ENV_FILE="${ENV_FILE}"

load_runtime_env() {
  if [[ ! -f "${ENV_FILE}" ]]; then
    echo "missing env file: ${ENV_FILE}" >&2
    exit 1
  fi
  # shellcheck disable=SC1090
  set -a
  source "${ENV_FILE}"
  set +a
  export POSTGRES_PASSWORD
}

load_ghcr_env() {
  if [[ ! -f "${GHCR_ENV}" ]]; then
    echo "missing GHCR env: ${GHCR_ENV}" >&2
    echo "copy deploy/env/ghcr.env.example → ${GHCR_ENV}" >&2
    exit 1
  fi
  # shellcheck disable=SC1090
  set -a
  source "${GHCR_ENV}"
  set +a
  if [[ -z "${ZETTABRIDGE_IMAGE:-}" ]]; then
    echo "ZETTABRIDGE_IMAGE must be set in ${GHCR_ENV}" >&2
    exit 1
  fi
  # GHCR rejects uppercase in repository paths (GitHub usernames may be mixed case).
  ZETTABRIDGE_IMAGE="$(echo "${ZETTABRIDGE_IMAGE}" | tr '[:upper:]' '[:lower:]')"
  export ZETTABRIDGE_IMAGE
  if [[ -n "${ZETTABRIDGE_DASHBOARD_IMAGE:-}" ]]; then
    ZETTABRIDGE_DASHBOARD_IMAGE="$(echo "${ZETTABRIDGE_DASHBOARD_IMAGE}" | tr '[:upper:]' '[:lower:]')"
    export ZETTABRIDGE_DASHBOARD_IMAGE
  fi
}

compose() {
  load_runtime_env
  load_ghcr_env
  docker compose "${COMPOSE_FILES[@]}" "$@"
}

cmd="${1:-}"
shift || true

case "${cmd}" in
  login-ghcr)
    load_ghcr_env
    if [[ -z "${GHCR_USER:-}" || -z "${GHCR_TOKEN:-}" ]]; then
      echo "GHCR_USER and GHCR_TOKEN required in ${GHCR_ENV}" >&2
      exit 1
    fi
    echo "${GHCR_TOKEN}" | docker login ghcr.io -u "${GHCR_USER}" --password-stdin
    echo "logged in to ghcr.io as ${GHCR_USER}"
    ;;
  pull)
    load_ghcr_env
    if [[ -n "${GHCR_USER:-}" && -n "${GHCR_TOKEN:-}" ]]; then
      echo "${GHCR_TOKEN}" | docker login ghcr.io -u "${GHCR_USER}" --password-stdin
    fi
    docker pull "${ZETTABRIDGE_IMAGE}"
    if [[ -n "${ZETTABRIDGE_DASHBOARD_IMAGE:-}" ]]; then
      docker pull "${ZETTABRIDGE_DASHBOARD_IMAGE}"
    fi
    ;;
  up)
    if [[ "${1:-}" == "--profile" ]]; then
      shift
      compose --profile "$@" up -d
    else
      compose up -d "$@"
    fi
    ;;
  down)
    compose down "$@"
    ;;
  logs)
    compose logs -f "$@"
    ;;
  ps)
    compose ps "$@"
    ;;
  migrate)
    load_runtime_env
    if [[ -z "${DATABASE_URL:-}" ]]; then
      echo "DATABASE_URL must be set in ${ENV_FILE}" >&2
      exit 1
    fi
    MIGRATE="${SCRIPTS_DIR}/migrate-rds.sh"
    if [[ ! -f "${MIGRATE}" ]]; then
      echo "missing ${MIGRATE}" >&2
      exit 1
    fi
    export ZETTABRIDGE_ROOT="${REPO_ROOT}"
    if [[ -d /opt/zettabridge/migrations ]]; then
      export ZETTABRIDGE_ROOT="/opt/zettabridge"
    fi
    "${MIGRATE}"
    ;;
  check-env)
    CHECK="${SCRIPTS_DIR}/check-env.sh"
    if [[ ! -f "${CHECK}" ]]; then
      echo "missing ${CHECK}" >&2
      exit 1
    fi
    "${CHECK}" "${ENV_FILE}"
    ;;
  *)
    echo "usage: $0 {login-ghcr|pull|up|down|logs|ps|migrate|check-env} [args...]" >&2
    exit 1
    ;;
esac
