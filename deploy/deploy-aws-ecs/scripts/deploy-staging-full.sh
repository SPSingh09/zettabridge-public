#!/usr/bin/env bash
# Full staging pipeline: Terraform → migrations → ECR push → ECS deploy → smoke.
#
# Usage (from repo root):
#   ./deploy/deploy-aws-ecs/scripts/deploy-staging-full.sh
#   ./deploy/deploy-aws-ecs/scripts/deploy-staging-full.sh --skip-infra
#   ./deploy/deploy-aws-ecs/scripts/deploy-staging-full.sh --skip-migrate
#   ./deploy/deploy-aws-ecs/scripts/deploy-staging-full.sh --full          # + functional tests
#   ./deploy/deploy-aws-ecs/scripts/deploy-staging-full.sh --deploy-only    # skip terraform + migrate
#
# One-time: cp deploy/deploy-aws-ecs/staging-aws.env.example staging-aws.env and fill in.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SCRIPTS="${ROOT}/deploy/deploy-aws-ecs/scripts"

SKIP_INFRA=0
SKIP_MIGRATE=0
DEPLOY_ONLY=0
FULL=0
EXTRA=()

for arg in "$@"; do
  case "${arg}" in
    --skip-infra)    SKIP_INFRA=1 ;;
    --skip-migrate)  SKIP_MIGRATE=1 ;;
    --deploy-only)   DEPLOY_ONLY=1; SKIP_INFRA=1; SKIP_MIGRATE=1 ;;
    --full)          FULL=1 ;;
    -h|--help)
      sed -n '2,14p' "$0"
      exit 0
      ;;
    *)
      EXTRA+=("${arg}")
      ;;
  esac
done

if [[ "${DEPLOY_ONLY}" == "0" && "${SKIP_INFRA}" == "0" ]]; then
  bash "${SCRIPTS}/terraform-apply.sh" --auto-approve
fi

if [[ "${DEPLOY_ONLY}" == "0" && "${SKIP_MIGRATE}" == "0" ]]; then
  bash "${SCRIPTS}/run-migrations.sh"
fi

DEPLOY_ARGS=()
[[ "${FULL}" == "1" ]] && DEPLOY_ARGS+=(--full)
DEPLOY_ARGS+=("${EXTRA[@]}")

bash "${SCRIPTS}/aws-deploy.sh" "${DEPLOY_ARGS[@]}"
