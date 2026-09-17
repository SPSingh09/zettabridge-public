# shellcheck shell=bash
# Shared helpers for AWS ECS staging scripts. Source from other scripts — do not execute directly.

_zb_script_dir() {
  cd "$(dirname "${BASH_SOURCE[1]:-${BASH_SOURCE[0]}}")" && pwd
}

zb_repo_root() {
  cd "$(_zb_script_dir)/../../.." && pwd
}

zb_tf_dir() {
  echo "$(zb_repo_root)/deploy/deploy-aws-ecs/terraform/staging"
}

zb_load_staging_env() {
  local root
  root="$(zb_repo_root)"
  if [[ -f "${root}/deploy/deploy-aws-ecs/staging-aws.env" ]]; then
    set -a
    # shellcheck disable=SC1091
    source "${root}/deploy/deploy-aws-ecs/staging-aws.env"
    set +a
  fi
}

zb_step() {
  echo
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  $*"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

zb_require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

# Read a Terraform output (-raw). Returns empty string on failure.
zb_tf_output() {
  local name="$1"
  local tf_dir
  tf_dir="$(zb_tf_dir)"
  if [[ ! -d "${tf_dir}" ]]; then
    return 1
  fi
  (cd "${tf_dir}" && terraform output -raw "${name}" 2>/dev/null) || true
}

# Populate ADAPTER_SERVICES array from terraform or ECS_ADAPTER_SERVICES env.
zb_discover_adapter_services() {
  ADAPTER_SERVICES=()
  if [[ -n "${ECS_ADAPTER_SERVICES:-}" ]]; then
    local IFS=','
    read -ra ADAPTER_SERVICES <<< "${ECS_ADAPTER_SERVICES}"
    return 0
  fi

  local json adapters
  json="$(cd "$(zb_tf_dir)" && terraform output -json deployed_sidecar_adapters 2>/dev/null)" || return 0
  if [[ -z "${json}" || "${json}" == "null" || "${json}" == "[]" ]]; then
    return 0
  fi

  adapters="$(echo "${json}" | jq -r '.[]? // empty')"
  while IFS= read -r adapter; do
    [[ -z "${adapter}" ]] && continue
    ADAPTER_SERVICES+=("${ECS_CLUSTER:-zettabridge-staging}-${adapter}-adapter")
  done <<< "${adapters}"
}

zb_ecs_force_deploy() {
  local svc="$1"
  local count="${2:-}"
  local -a args=(
    --cluster "${ECS_CLUSTER}"
    --service "${svc}"
    --force-new-deployment
    --region "${AWS_REGION}"
    --no-cli-pager
  )
  if [[ -n "${count}" ]]; then
    args+=(--desired-count "${count}")
  fi
  aws ecs update-service "${args[@]}" \
    --query "service.serviceName" \
    --output text >/dev/null
  if [[ -n "${count}" ]]; then
    echo "  ✓ ${svc} — desired=${count}, deployment triggered"
  else
    echo "  ✓ ${svc} — deployment triggered"
  fi
}

# Build deduplicated list: adapters, backend, dashboard (non-empty).
zb_ecs_staging_service_list() {
  local backend="${1:-1}" dashboard="${2:-1}" adapters="${3:-1}"
  local -a list=() svc

  if [[ "${adapters}" == "1" ]]; then
    zb_discover_adapter_services
    list+=("${ADAPTER_SERVICES[@]}")
  fi
  if [[ "${backend}" == "1" && -n "${ECS_SERVICE_BACKEND:-}" ]]; then
    list+=("${ECS_SERVICE_BACKEND}")
  fi
  if [[ "${dashboard}" == "1" && -n "${ECS_SERVICE_DASHBOARD:-}" ]]; then
    list+=("${ECS_SERVICE_DASHBOARD}")
  fi

  local -a unique=()
  for svc in "${list[@]}"; do
    [[ -z "${svc}" ]] && continue
    local seen=0 u
    for u in "${unique[@]:-}"; do
      [[ "${u}" == "${svc}" ]] && seen=1 && break
    done
    [[ "${seen}" == "0" ]] && unique+=("${svc}")
  done

  printf '%s\n' "${unique[@]}"
}

# Wait until every listed service has no running or pending tasks.
zb_ecs_wait_stopped() {
  local -a services=("$@")
  [[ ${#services[@]} -eq 0 ]] && return 0

  local timeout_sec="${ECS_STOP_TIMEOUT_SEC:-600}"
  local poll_sec=10
  local started_at now elapsed
  started_at="$(date +%s)"

  echo "  (waiting for all tasks to stop — timeout ${timeout_sec}s)"
  while true; do
    local all_stopped=1
    local statuses
    statuses="$(aws ecs describe-services \
      --cluster "${ECS_CLUSTER}" \
      --services "${services[@]}" \
      --region "${AWS_REGION}" \
      --query "services[*].{name:serviceName,running:runningCount,pending:pendingCount,desired:desiredCount}" \
      --output json)"

    while IFS= read -r line; do
      local name running pending desired
      name=$(echo "$line" | jq -r '.name')
      running=$(echo "$line" | jq -r '.running')
      pending=$(echo "$line" | jq -r '.pending')
      desired=$(echo "$line" | jq -r '.desired')
      echo "  $(date -u +%H:%M:%S)  ${name}  desired=${desired} running=${running} pending=${pending}"
      [[ "${running}" == "0" && "${pending}" == "0" ]] || all_stopped=0
    done < <(echo "$statuses" | jq -c '.[]')

    if [[ "${all_stopped}" == "1" ]]; then
      echo "  ✓ All tasks stopped."
      return 0
    fi

    now="$(date +%s)"
    elapsed=$((now - started_at))
    if (( elapsed >= timeout_sec )); then
      echo "  ✗ Timed out waiting for tasks to stop." >&2
      return 1
    fi
    sleep "${poll_sec}"
  done
}

# Scale listed services to desired-count 0 (clears stuck ACTIVE/PRIMARY deployments).
zb_ecs_scale_to_zero() {
  local -a services=("$@")
  [[ ${#services[@]} -eq 0 ]] && return 0

  for svc in "${services[@]}"; do
    aws ecs update-service \
      --cluster "${ECS_CLUSTER}" \
      --service "${svc}" \
      --desired-count 0 \
      --region "${AWS_REGION}" \
      --no-cli-pager \
      --query "service.serviceName" \
      --output text >/dev/null
    echo "  ✓ ${svc} → desired=0"
  done
}

# Stop all staging ECS tasks. Saves prior desired counts in ZB_ECS_SAVED_DESIRED (assoc array).
zb_ecs_reset_staging() {
  local backend="${1:-1}" dashboard="${2:-1}" adapters="${3:-1}"
  local -a services=()
  mapfile -t services < <(zb_ecs_staging_service_list "${backend}" "${dashboard}" "${adapters}")
  [[ ${#services[@]} -eq 0 ]] && {
    echo "  (no ECS services to stop)"
    return 0
  }

  declare -gA ZB_ECS_SAVED_DESIRED=()
  local json
  json="$(aws ecs describe-services \
    --cluster "${ECS_CLUSTER}" \
    --services "${services[@]}" \
    --region "${AWS_REGION}" \
    --query "services[*].{name:serviceName,desired:desiredCount}" \
    --output json)"
  while IFS= read -r line; do
    local name desired
    name=$(echo "$line" | jq -r '.name')
    desired=$(echo "$line" | jq -r '.desired')
    ZB_ECS_SAVED_DESIRED["${name}"]="${desired}"
  done < <(echo "${json}" | jq -c '.[]')

  zb_step "Stop staging ECS services (scale to 0)"
  echo "  services: ${services[*]}"
  zb_ecs_scale_to_zero "${services[@]}"
  zb_ecs_wait_stopped "${services[@]}"
}

# Bring services back after reset: adapters first, then Core + dashboard.
zb_ecs_start_staging() {
  local backend="${1:-1}" dashboard="${2:-1}" adapters="${3:-1}"
  local -a adapter_svcs=() core_svcs=() svc target

  if [[ "${adapters}" == "1" ]]; then
    zb_discover_adapter_services
    adapter_svcs=("${ADAPTER_SERVICES[@]}")
  fi
  [[ "${backend}" == "1" && -n "${ECS_SERVICE_BACKEND:-}" ]] && core_svcs+=("${ECS_SERVICE_BACKEND}")
  [[ "${dashboard}" == "1" && -n "${ECS_SERVICE_DASHBOARD:-}" ]] && core_svcs+=("${ECS_SERVICE_DASHBOARD}")

  _zb_ecs_target_count() {
    local name="$1"
    if [[ -n "${ZB_ECS_SAVED_DESIRED[$name]+x}" && "${ZB_ECS_SAVED_DESIRED[$name]}" -gt 0 ]]; then
      echo "${ZB_ECS_SAVED_DESIRED[$name]}"
    else
      echo "1"
    fi
  }

  if [[ ${#adapter_svcs[@]} -gt 0 ]]; then
    zb_step "Start adapter sidecars"
    for svc in "${adapter_svcs[@]}"; do
      [[ -z "${svc}" ]] && continue
      target="$(_zb_ecs_target_count "${svc}")"
      zb_ecs_force_deploy "${svc}" "${target}"
    done
    zb_ecs_wait_stable "${adapter_svcs[@]}"
  fi

  if [[ ${#core_svcs[@]} -gt 0 ]]; then
    zb_step "Start Core + dashboard"
    for svc in "${core_svcs[@]}"; do
      target="$(_zb_ecs_target_count "${svc}")"
      zb_ecs_force_deploy "${svc}" "${target}"
    done
    zb_ecs_wait_stable "${core_svcs[@]}"
  fi
}

# Print ECS events + recent stopped tasks when a rollout is stuck.
zb_ecs_diagnose_service() {
  local svc="$1"
  echo
  echo "  ── Diagnostics: ${svc} ──"

  echo "  Recent service events:"
  aws ecs describe-services \
    --cluster "${ECS_CLUSTER}" \
    --services "${svc}" \
    --region "${AWS_REGION}" \
    --query 'services[0].events[0:8].[createdAt,message]' \
    --output text 2>/dev/null | sed 's/^/    /' || true

  local task_arns
  task_arns="$(aws ecs list-tasks \
    --cluster "${ECS_CLUSTER}" \
    --service-name "${svc}" \
    --desired-status STOPPED \
    --region "${AWS_REGION}" \
    --query 'taskArns[0:3]' \
    --output text 2>/dev/null || true)"
  if [[ -n "${task_arns}" && "${task_arns}" != "None" ]]; then
    echo "  Recent stopped tasks:"
    aws ecs describe-tasks \
      --cluster "${ECS_CLUSTER}" \
      --tasks ${task_arns} \
      --region "${AWS_REGION}" \
      --query 'tasks[*].{task:taskArn,stopCode:stopCode,reason:stoppedReason,exit:containers[0].exitCode,health:healthStatus}' \
      --output table 2>/dev/null || true
  fi

  echo "  Logs: aws logs tail /ecs/${ECS_CLUSTER} --since 30m --region ${AWS_REGION}"
}

zb_ecs_wait_stable() {
  local -a services=("$@")
  [[ ${#services[@]} -eq 0 ]] && return 0

  local timeout_sec="${ECS_WAIT_TIMEOUT_SEC:-1200}"
  local poll_sec=15
  local stuck_sec=300
  local started_at now elapsed stuck_since=-1

  started_at="$(date +%s)"
  echo "  (polls every ${poll_sec}s, timeout ${timeout_sec}s — Ctrl+C is safe, deploy continues in ECS)"

  while true; do
    local statuses all_stable=1 any_stuck=0
    statuses="$(aws ecs describe-services \
      --cluster "${ECS_CLUSTER}" \
      --services "${services[@]}" \
      --region "${AWS_REGION}" \
      --query "services[*].{name:serviceName,desired:desiredCount,running:runningCount,pending:pendingCount,deployments:deployments[*].{status:status,rollout:rolloutState,running:runningCount,desired:desiredCount}}" \
      --output json)"

    while IFS= read -r line; do
      local name desired running pending primary_rollout primary_running
      name=$(echo "$line" | jq -r '.name')
      desired=$(echo "$line" | jq -r '.desired')
      running=$(echo "$line" | jq -r '.running')
      pending=$(echo "$line" | jq -r '.pending')
      primary_rollout=$(echo "$line" | jq -r '.deployments[] | select(.status=="PRIMARY") | .rollout // empty' | head -n 1)
      primary_running=$(echo "$line" | jq -r '.deployments[] | select(.status=="PRIMARY") | .running // 0' | head -n 1)
      echo "  $(date -u +%H:%M:%S)  ${name}  desired=${desired} running=${running} pending=${pending} primary_rollout=${primary_rollout:-?} primary_running=${primary_running:-0}"
      [[ "$running" == "$desired" && "$pending" == "0" ]] || all_stable=0
      if [[ -n "${primary_rollout}" && "${primary_rollout}" != "COMPLETED" ]]; then
        all_stable=0
      fi
      if [[ "${primary_running:-0}" != "${desired}" ]]; then
        all_stable=0
        # Old task still serving while new PRIMARY tasks never become healthy.
        if [[ "${running}" == "${desired}" && "${desired}" -gt 0 ]]; then
          any_stuck=1
        fi
      fi
    done < <(echo "$statuses" | jq -c '.[]')

    if [[ "$all_stable" == "1" ]]; then
      echo "  ✓ All services stable (PRIMARY rollout complete)."
      return 0
    fi

    now="$(date +%s)"
    elapsed=$((now - started_at))

    if [[ "${any_stuck}" == "1" ]]; then
      if [[ "${stuck_since}" -lt 0 ]]; then
        stuck_since="${now}"
      elif (( now - stuck_since >= stuck_sec )); then
        echo
        echo "  ✗ Rollout stuck for ${stuck_sec}s — new tasks are not passing health checks." >&2
        for svc in "${services[@]}"; do
          zb_ecs_diagnose_service "${svc}"
        done
        echo
        echo "  Common fixes:" >&2
        echo "    1. Check CloudWatch logs for 'startup:' or 'ResourceInitializationError'" >&2
        echo "    2. terraform apply (health_check_grace_period) then aws-deploy.sh" >&2
        echo "    3. aws ecs update-service --cluster ${ECS_CLUSTER} --service <name> --force-new-deployment" >&2
        return 1
      fi
    else
      stuck_since=-1
    fi

    if (( elapsed >= timeout_sec )); then
      echo
      echo "  ✗ Timed out after ${timeout_sec}s waiting for ECS rollout." >&2
      for svc in "${services[@]}"; do
        zb_ecs_diagnose_service "${svc}"
      done
      return 1
    fi

    sleep "${poll_sec}"
  done
}

# Obtain a bearer token for smoke checks (admin login).
zb_staging_admin_token() {
  local base="${1:-${STAGING_BASE_URL:-}}"
  [[ -z "${base}" ]] && base="$(zb_tf_output api_url)"
  [[ -z "${base}" || -z "${STAGING_ADMIN_EMAIL:-}" || -z "${STAGING_ADMIN_PASSWORD:-}" ]] && return 1

  zb_require_cmd curl
  zb_require_cmd jq

  local login_body token
  login_body="$(curl -4 -sS -m 30 -X POST "${base%/}/v1/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"${STAGING_ADMIN_EMAIL}\",\"password\":\"${STAGING_ADMIN_PASSWORD}\"}")"
  token="$(echo "${login_body}" | jq -r '.data.token // empty')"
  [[ -n "${token}" ]] || return 1
  echo "${token}"
}

# When execution_adapter_mode=http, connect-info must advertise adapter callback URLs.
zb_smoke_zerodha_connect_info() {
  local mode expected_adapter base token body mode_out callback pub
  mode="$(zb_tf_output execution_adapter_mode)"
  [[ "${mode}" != "http" ]] && return 0

  expected_adapter="$(zb_tf_output zerodha_adapter_url)"
  [[ -z "${expected_adapter}" ]] && {
    echo "  ⚠ execution_adapter_mode=http but zerodha_adapter_url terraform output is empty" >&2
    return 1
  }

  base="${STAGING_BASE_URL:-}"
  [[ -z "${base}" ]] && base="$(zb_tf_output api_url)"
  [[ -z "${base}" ]] && {
    echo "  ⚠ connect-info smoke skipped (STAGING_BASE_URL not set)" >&2
    return 1
  }

  token="$(zb_staging_admin_token "${base}")" || {
    echo "  ⚠ connect-info smoke skipped (admin login failed — set STAGING_ADMIN_* in staging-aws.env)" >&2
    return 1
  }

  zb_step "Zerodha connect-info (http adapter mode)"
  body="$(curl -4 -sS -m 30 "${base%/}/v1/credentials/zerodha/connect-info" \
    -H "Authorization: Bearer ${token}")"

  mode_out="$(echo "${body}" | jq -r '.data.execution_adapter_mode // empty')"
  callback="$(echo "${body}" | jq -r '.data.callback_url // empty')"
  pub="$(echo "${body}" | jq -r '.data.publisher_callback_url // empty')"

  local ok=1
  if [[ "${mode_out}" != "http" ]]; then
    echo "  ✗ execution_adapter_mode=${mode_out:-<missing>} (want http) — Core image likely stale; run aws-deploy.sh" >&2
    ok=0
  fi
  if [[ "${callback}" != "${expected_adapter%/}/v1/credentials/zerodha/callback" ]]; then
    echo "  ✗ callback_url=${callback:-<missing>} (want ${expected_adapter%/}/v1/credentials/zerodha/callback)" >&2
    ok=0
  fi
  if [[ -n "${pub}" && "${pub}" != "${expected_adapter%/}/v1/publisher/callback" ]]; then
    echo "  ✗ publisher_callback_url=${pub} (want ${expected_adapter%/}/v1/publisher/callback)" >&2
    ok=0
  fi

  if [[ "${ok}" == "1" ]]; then
    echo "  ✓ connect-info advertises adapter URLs"
    return 0
  fi
  echo "  Hint: ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh  (Terraform alone does not rebuild the app image)" >&2
  return 1
}

zb_smoke_adapter_health() {
  local url="${1:-}"
  [[ -z "${url}" ]] && url="${STAGING_ZERODHA_ADAPTER_URL:-}"
  if [[ -z "${url}" ]]; then
    url="$(zb_tf_output zerodha_adapter_url)"
  fi
  [[ -z "${url}" ]] && return 0

  zb_require_cmd curl
  zb_require_cmd jq

  local health="${url%/}/v1/healthz"
  zb_step "Adapter health → ${health}"
  local body code status
  body="$(curl -4 -sS -m 30 -w '\n%{http_code}' "${health}" || echo -e "\n000")"
  code="$(echo "${body}" | tail -n 1)"
  body="$(echo "${body}" | head -n -1)"
  status="$(echo "${body}" | jq -r '.status // empty' 2>/dev/null || true)"
  if [[ "${code}" == "200" && "${status}" == "ok" ]]; then
    echo "  ✓ zerodha adapter healthz OK"
    return 0
  fi
  echo "  ⚠ adapter healthz failed (HTTP ${code}) — DNS/ACM may be pending" >&2
  echo "    body=${body}" >&2
  return 1
}

# Kite OAuth redirects are browser GETs without X-ZB-Service-Token. A 401 here
# means the zerodha-adapter image is stale (pre-fix group middleware on /v1).
zb_smoke_zerodha_oauth_public() {
  local url="${1:-}"
  [[ -z "${url}" ]] && url="${STAGING_ZERODHA_ADAPTER_URL:-}"
  if [[ -z "${url}" ]]; then
    url="$(zb_tf_output zerodha_adapter_url)"
  fi
  [[ -z "${url}" ]] && return 0

  zb_require_cmd curl

  local cb="${url%/}/v1/credentials/zerodha/callback?status=cancelled"
  zb_step "Zerodha OAuth callback public → ${cb}"
  local body code
  body="$(curl -4 -sS -m 30 -w '\n%{http_code}' "${cb}" || echo -e "\n000")"
  code="$(echo "${body}" | tail -n 1)"
  body="$(echo "${body}" | head -n -1)"
  if [[ "${code}" == "401" ]] && echo "${body}" | grep -q 'invalid service token'; then
    echo "  ✗ OAuth callback is service-token protected (adapter image stale)" >&2
    echo "    Run: ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --adapters-only" >&2
    return 1
  fi
  echo "  ✓ OAuth callback is not service-token gated (HTTP ${code})"
  return 0
}
