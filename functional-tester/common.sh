# Shared helpers for functional test runner scripts.

ensure_go_on_path() {
  if command -v go >/dev/null 2>&1; then
    return 0
  fi
  local dir
  for dir in /usr/local/go/bin "${HOME}/go/bin"; do
    if [[ -x "${dir}/go" ]]; then
      export PATH="${dir}:${PATH}"
      return 0
    fi
  done
  echo "go is required on PATH for functional tests (install Go or add /usr/local/go/bin to PATH)" >&2
  exit 1
}

# seed_functional_admin inserts the functional-test admin user via psql variables.
# Args: script_dir admin_email admin_password [docker compose args...]
seed_functional_admin() {
  local script_dir="$1" admin_email="$2" admin_password="$3"
  shift 3
  local compose=(docker compose)
  if [[ $# -gt 0 ]]; then
    compose=("$@")
  fi
  "${compose[@]}" exec -T postgres psql -q -U postgres -d zettabridge \
    -v ON_ERROR_STOP=1 \
    -v "admin_email=${admin_email}" \
    -v "admin_password=${admin_password}" \
    -f - < "${script_dir}/seed-admin.sql"
}
