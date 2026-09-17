#!/usr/bin/env bash
# Apply ZettaBridge SQL migrations to RDS (or any Postgres).
#
# Usage:
#   export DATABASE_URL='postgres://user:pass@host:5432/zettabridge?sslmode=require'
#   ./deploy/scripts/migrate-rds.sh
#
# Requires: psql (postgresql-client)

set -euo pipefail

ROOT_DIR="${ZETTABRIDGE_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL is required" >&2
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "psql not found — install postgresql-client" >&2
  exit 1
fi

MIGRATIONS=(
  000_schema.sql
)

for migration in "${MIGRATIONS[@]}"; do
  path="${ROOT_DIR}/migrations/${migration}"
  if [[ ! -f "${path}" ]]; then
    echo "missing migration file: ${path}" >&2
    exit 1
  fi
  echo "==> ${migration}"
  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -q -f "${path}"
done

echo "Migrations complete."
