#!/usr/bin/env bash
# Alias for aws-deploy.sh — build, push to ECR, deploy to ECS, smoke test.
#
# Usage:
#   ./deploy/deploy-aws-ecs/scripts/deploy-staging.sh
#   ./deploy/deploy-aws-ecs/scripts/deploy-staging.sh --backend-only
#   ./deploy/deploy-aws-ecs/scripts/deploy-staging-full.sh   # Terraform + migrate + deploy
#
exec "$(dirname "$0")/aws-deploy.sh" "$@"
