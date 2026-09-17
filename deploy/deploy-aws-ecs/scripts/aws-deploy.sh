#!/usr/bin/env bash
# ZettaBridge AWS ECS deployment script (Core + dashboard + execution adapter sidecars).
#
# Builds and pushes Docker images to ECR, force-deploys ECS Fargate services,
# waits for stability, runs smoke tests (Core + optional Zerodha adapter health).
#
# One-time setup:
#   cp deploy/deploy-aws-ecs/staging-aws.env.example deploy/deploy-aws-ecs/staging-aws.env
#   # edit STAGING_ADMIN_EMAIL, STAGING_ADMIN_PASSWORD
#
# Usage (from repo root):
#   ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh
#   ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --deploy-only
#   ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --test-only
#   ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --backend-only
#   ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --adapters-only   # sidecars only (same image)
#   ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --skip-adapters
#   ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --dashboard-only
#   ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --full            # + functional tests
#   ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --reset           # stop all ECS, then deploy fresh
#
# Full pipeline (Terraform + migrate + deploy):
#   ./deploy/deploy-aws-ecs/scripts/deploy-staging-full.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${ROOT}"

# shellcheck disable=SC1091
source "${ROOT}/deploy/deploy-aws-ecs/scripts/common.sh"
zb_load_staging_env

# ── Config (override via env or staging-aws.env) ──────────────────────────────
AWS_REGION="${AWS_REGION:-ap-south-1}"
IMAGE_TAG="${IMAGE_TAG:-staging}"
ECS_CLUSTER="${ECS_CLUSTER:-zettabridge-staging}"
ECS_SERVICE_BACKEND="${ECS_SERVICE_BACKEND:-zettabridge-staging}"
ECS_SERVICE_DASHBOARD="${ECS_SERVICE_DASHBOARD:-zettabridge-staging-dashboard}"
TF_DIR="$(zb_tf_dir)"

ACCOUNT_ID="$(aws sts get-caller-identity --query Account --output text)"
ECR_BACKEND="${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/zettabridge-staging"
ECR_DASHBOARD="${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/zettabridge-staging-dashboard"

if [[ -z "${STAGING_BASE_URL:-}" ]]; then
  STAGING_BASE_URL="$(zb_tf_output api_url)"
fi
STAGING_BASE_URL="${STAGING_BASE_URL:-}"

# ── Flags ─────────────────────────────────────────────────────────────────────
DEPLOY=1
TEST=1
FULL=0
BACKEND=1
DASHBOARD=1
ADAPTERS=1
ADAPTERS_ONLY=0
RESET=0

for arg in "$@"; do
  case "${arg}" in
    --deploy-only)    TEST=0 ;;
    --test-only)      DEPLOY=0 ;;
    --backend-only)   DASHBOARD=0; ADAPTERS=0 ;;
    --dashboard-only) BACKEND=0; ADAPTERS=0 ;;
    --adapters-only)  BACKEND=0; DASHBOARD=0; ADAPTERS=1; ADAPTERS_ONLY=1 ;;
    --skip-adapters)  ADAPTERS=0 ;;
    --full)           FULL=1 ;;
    --reset)          RESET=1 ;;
    -h|--help)
      sed -n '2,26p' "$0"
      exit 0
      ;;
    *)
      echo "unknown option: ${arg}" >&2
      exit 1
      ;;
  esac
done

zb_require_cmd aws
zb_require_cmd docker

ADAPTER_SERVICES=()
zb_discover_adapter_services

zb_step "AWS ECS deploy → ${ECS_CLUSTER} (${AWS_REGION})"
echo "  backend:   ${ECR_BACKEND}:${IMAGE_TAG}"
echo "  dashboard: ${ECR_DASHBOARD}:${IMAGE_TAG}"
echo "  base url:  ${STAGING_BASE_URL:-<not set>}"
if [[ ${#ADAPTER_SERVICES[@]} -gt 0 ]]; then
  echo "  adapters:  ${ADAPTER_SERVICES[*]}"
else
  echo "  adapters:  (none — local mode or not in terraform output)"
fi

# ── Deploy ────────────────────────────────────────────────────────────────────
if [[ "${DEPLOY}" == "1" ]]; then
  if [[ "${RESET}" == "1" && "${ADAPTERS_ONLY}" == "0" ]]; then
    zb_ecs_reset_staging "${BACKEND}" "${DASHBOARD}" "${ADAPTERS}"
  fi

  if [[ "${ADAPTERS_ONLY}" == "0" ]]; then
    zb_step "Authenticate Docker to ECR"
    aws ecr get-login-password --region "${AWS_REGION}" \
      | docker login --username AWS --password-stdin \
          "${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"
  fi

  if [[ "${BACKEND}" == "1" || "${ADAPTERS}" == "1" ]]; then
    zb_step "Build backend (Core + adapter binaries in one image)"
    docker build -t "${ECR_BACKEND}:${IMAGE_TAG}" "${ROOT}"

    zb_step "Push backend"
    aws ecr get-login-password --region "${AWS_REGION}" \
      | docker login --username AWS --password-stdin \
          "${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"
    docker push "${ECR_BACKEND}:${IMAGE_TAG}"
  fi

  if [[ "${DASHBOARD}" == "1" ]]; then
    zb_step "Build dashboard"
    docker build \
      --build-arg NEXT_PUBLIC_API_URL="${STAGING_BASE_URL}" \
      -t "${ECR_DASHBOARD}:${IMAGE_TAG}" "${ROOT}/dashboard"

    zb_step "Push dashboard"
    docker push "${ECR_DASHBOARD}:${IMAGE_TAG}"
  fi

  deploy_services=()
  [[ "${BACKEND}" == "1" ]] && deploy_services+=("${ECS_SERVICE_BACKEND}")
  [[ "${DASHBOARD}" == "1" ]] && deploy_services+=("${ECS_SERVICE_DASHBOARD}")
  if [[ "${ADAPTERS}" == "1" ]]; then
    deploy_services+=("${ADAPTER_SERVICES[@]}")
  fi

  if [[ ${#deploy_services[@]} -gt 0 ]]; then
    if [[ "${RESET}" == "1" ]]; then
      zb_ecs_start_staging "${BACKEND}" "${DASHBOARD}" "${ADAPTERS}"
    else
      zb_step "Force new ECS deployment"
      for svc in "${deploy_services[@]}"; do
        [[ -z "${svc}" ]] && continue
        zb_ecs_force_deploy "${svc}"
      done

      zb_step "Waiting for ECS services to stabilise"
      zb_ecs_wait_stable "${deploy_services[@]}"
    fi
  fi

  if [[ "${BACKEND}" == "1" && "${ADAPTERS_ONLY}" == "0" ]]; then
    zb_smoke_zerodha_connect_info || exit 1
  fi

  if [[ "${ADAPTERS}" == "0" && ${#ADAPTER_SERVICES[@]} -gt 0 ]]; then
    mode="$(zb_tf_output execution_adapter_mode 2>/dev/null || true)"
    if [[ "${mode}" == "http" ]]; then
      echo
      echo "  NOTE: Adapter sidecars were NOT redeployed (e.g. --backend-only)."
      echo "        Kite OAuth uses exec-zerodha — roll out adapters too:"
      echo "        ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --adapters-only"
      zb_smoke_zerodha_oauth_public || true
    fi
  fi
else
  zb_step "[skip] Deploy (--test-only)"
fi

# ── Smoke test ────────────────────────────────────────────────────────────────
if [[ "${TEST}" == "0" ]]; then
  echo
  echo "Deploy complete (--deploy-only)."
  exit 0
fi

zb_require_cmd curl
zb_require_cmd jq

if [[ -z "${STAGING_BASE_URL}" ]]; then
  echo "STAGING_BASE_URL is not set and could not be read from Terraform output." >&2
  echo "Set it in deploy/deploy-aws-ecs/staging-aws.env" >&2
  exit 1
fi

if [[ -z "${STAGING_ADMIN_EMAIL:-}" || -z "${STAGING_ADMIN_PASSWORD:-}" ]]; then
  echo "Set STAGING_ADMIN_EMAIL and STAGING_ADMIN_PASSWORD in deploy/deploy-aws-ecs/staging-aws.env" >&2
  exit 1
fi

if [[ "${BACKEND}" == "1" ]]; then
  zb_step "Core smoke test → ${STAGING_BASE_URL}"
  export STAGING_BASE_URL
  bash "${ROOT}/deploy/scripts/staging-smoke.sh"
fi

if [[ "${ADAPTERS}" == "1" && ${#ADAPTER_SERVICES[@]} -gt 0 ]]; then
  zb_smoke_adapter_health || true
  zb_smoke_zerodha_oauth_public || exit 1
fi

if [[ "${FULL}" == "1" ]]; then
  zb_step "Functional tests"
  bash "${ROOT}/functional-tester/run-functional-staging.sh"
fi

zb_step "Done"
echo "  Deploy and smoke checks finished."
echo
echo "Logs: aws logs tail /ecs/zettabridge-staging --follow --region ${AWS_REGION}"
