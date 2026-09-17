#!/usr/bin/env bash
# Stop (or restart) all staging ECS services — clears stuck deployments.
#
# Usage (from repo root):
#   ./deploy/deploy-aws-ecs/scripts/staging-reset.sh              # scale all to 0
#   ./deploy/deploy-aws-ecs/scripts/staging-reset.sh --up         # scale back up (adapters → Core)
#   ./deploy/deploy-aws-ecs/scripts/staging-reset.sh --backend-only
#
# Typical full clean redeploy:
#   ./deploy/deploy-aws-ecs/scripts/staging-reset.sh
#   ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --reset

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
# shellcheck disable=SC1091
source "${ROOT}/deploy/deploy-aws-ecs/scripts/common.sh"
zb_load_staging_env

AWS_REGION="${AWS_REGION:-ap-south-1}"
ECS_CLUSTER="${ECS_CLUSTER:-zettabridge-staging}"
ECS_SERVICE_BACKEND="${ECS_SERVICE_BACKEND:-zettabridge-staging}"
ECS_SERVICE_DASHBOARD="${ECS_SERVICE_DASHBOARD:-zettabridge-staging-dashboard}"

MODE="down"
BACKEND=1
DASHBOARD=1
ADAPTERS=1

for arg in "$@"; do
  case "${arg}" in
    --up)           MODE="up" ;;
    --down)         MODE="down" ;;
    --backend-only) DASHBOARD=0; ADAPTERS=0 ;;
    --adapters-only) BACKEND=0; DASHBOARD=0 ;;
    -h|--help)
      sed -n '2,14p' "$0"
      exit 0
      ;;
    *)
      echo "unknown option: ${arg}" >&2
      exit 1
      ;;
  esac
done

zb_require_cmd aws
zb_require_cmd jq

zb_step "Staging ECS reset (${MODE}) → ${ECS_CLUSTER} (${AWS_REGION})"

if [[ "${MODE}" == "down" ]]; then
  zb_ecs_reset_staging "${BACKEND}" "${DASHBOARD}" "${ADAPTERS}"
  echo
  echo "All selected services scaled to 0."
  echo "Redeploy with: ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh --reset"
  exit 0
fi

# --up: start without building a new image (uses current task definition + ECR tag).
zb_ecs_start_staging "${BACKEND}" "${DASHBOARD}" "${ADAPTERS}"
echo
echo "Services started."
