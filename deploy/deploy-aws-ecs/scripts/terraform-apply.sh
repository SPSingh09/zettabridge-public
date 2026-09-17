#!/usr/bin/env bash
# Apply (or plan) staging Terraform — Phase 4 adapter split included.
#
# Usage (from repo root):
#   ./deploy/deploy-aws-ecs/scripts/terraform-apply.sh --plan-only
#   ./deploy/deploy-aws-ecs/scripts/terraform-apply.sh --auto-approve
#   ./deploy/deploy-aws-ecs/scripts/terraform-apply.sh --auto-approve --with-deploy
#
# Requires: terraform, aws CLI

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
# shellcheck disable=SC1091
source "${ROOT}/deploy/deploy-aws-ecs/scripts/common.sh"

zb_load_staging_env

TF_DIR="$(zb_tf_dir)"
PLAN_ONLY=0
AUTO_APPROVE=0
WITH_DEPLOY=0

for arg in "$@"; do
  case "${arg}" in
    --plan-only)     PLAN_ONLY=1 ;;
    --auto-approve)  AUTO_APPROVE=1 ;;
    --with-deploy)   WITH_DEPLOY=1 ;;
    -h|--help)
      sed -n '2,10p' "$0"
      exit 0
      ;;
    *)
      echo "unknown option: ${arg}" >&2
      exit 1
      ;;
  esac
done

zb_require_cmd terraform
zb_require_cmd aws

zb_step "Terraform staging (${TF_DIR})"
aws sts get-caller-identity --output text >/dev/null

cd "${TF_DIR}"
terraform init -input=false
terraform validate

PLAN_OUT="staging.tfplan"
terraform plan -out="${PLAN_OUT}"

if [[ "${PLAN_ONLY}" == "1" ]]; then
  echo
  echo "Plan saved to ${TF_DIR}/${PLAN_OUT} (--plan-only, no apply)."
  exit 0
fi

if [[ "${AUTO_APPROVE}" == "1" ]]; then
  terraform apply -auto-approve "${PLAN_OUT}"
else
  echo
  echo "Review the plan above. To apply:"
  echo "  cd ${TF_DIR} && terraform apply ${PLAN_OUT}"
  echo "Or re-run with --auto-approve"
  exit 0
fi

echo
echo "Terraform apply complete."

if [[ "${WITH_DEPLOY}" == "1" ]]; then
  echo
  echo "Running app deploy (build/push ECR + ECS rollout)..."
  bash "${ROOT}/deploy/deploy-aws-ecs/scripts/aws-deploy.sh"
elif [[ "${PLAN_ONLY}" == "0" ]]; then
  echo
  echo "IMPORTANT: Terraform updates task definitions and env vars but does NOT rebuild the app image."
  echo "Run one of:"
  echo "  ./deploy/deploy-aws-ecs/scripts/aws-deploy.sh"
  echo "  ./deploy/deploy-aws-ecs/scripts/terraform-apply.sh --auto-approve --with-deploy"
  echo "  ./deploy/deploy-aws-ecs/scripts/deploy-staging-full.sh --skip-infra"
fi
