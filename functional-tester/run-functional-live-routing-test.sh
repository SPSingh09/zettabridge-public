#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"
ensure_go_on_path

BASE_URL="${FUNCTIONAL_TEST_BASE_URL:-http://localhost:8080}"
ADMIN_EMAIL="${FUNCTIONAL_TEST_ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${FUNCTIONAL_TEST_ADMIN_PASSWORD:-admin123}"
COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.live.yml)

export FUNCTIONAL_TEST_BASE_URL="$BASE_URL"
export FUNCTIONAL_TEST_ADMIN_EMAIL="$ADMIN_EMAIL"
export FUNCTIONAL_TEST_ADMIN_PASSWORD="$ADMIN_PASSWORD"
export FUNCTIONAL_TEST_LIVE_ROUTING=1

cd "$ROOT_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required for live routing functional test" >&2
  exit 1
fi

echo "Starting postgres + redis..."
"${COMPOSE[@]}" up -d postgres redis

if [[ "${FUNCTIONAL_TEST_SKIP_MIGRATIONS:-0}" != "1" ]]; then
  echo "Wiping database schema for a clean migration run..."
  "${COMPOSE[@]}" exec -T postgres psql -q -U postgres -d zettabridge -v ON_ERROR_STOP=1 <<'SQL'
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO public;
GRANT ALL ON SCHEMA public TO postgres;
SQL
fi

echo "Building and starting server with BROKER_MODE=live..."
"${COMPOSE[@]}" build server
"${COMPOSE[@]}" up -d server

for _ in $(seq 1 60); do
  if curl -sf "${BASE_URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if ! curl -sf "${BASE_URL}/healthz" >/dev/null 2>&1; then
  echo "server did not become healthy at ${BASE_URL}/healthz" >&2
  "${COMPOSE[@]}" logs server --tail 40 >&2 || true
  exit 1
fi

if [[ "${FUNCTIONAL_TEST_SKIP_MIGRATIONS:-0}" != "1" && -n "${ADMIN_EMAIL}" ]]; then
  seed_functional_admin "${SCRIPT_DIR}" "${ADMIN_EMAIL}" "${ADMIN_PASSWORD}" "${COMPOSE[@]}"
fi

exec go test -tags functional -count=1 ./functional-tester -run TestFreeUserBlockedFromLiveWhenBrokerLive -v
