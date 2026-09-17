#!/usr/bin/env bash
# HTTP smoke tests against AWS ECS staging (no build/deploy).
#
# One-time setup:
#   cp deploy/deploy-aws-ecs/staging-aws.env.example deploy/deploy-aws-ecs/staging-aws.env
#   # edit STAGING_ADMIN_PASSWORD (and STAGING_BASE_URL if not using terraform output)
#
# Usage (from repo root, WSL or Linux):
#   ./deploy/deploy-aws-ecs/scripts/staging-smoke.sh
#   ./deploy/deploy-aws-ecs/scripts/staging-smoke.sh --skip-adapters
#   ./deploy/deploy-aws-ecs/scripts/staging-smoke.sh --full
#
# Requires: curl, jq, aws cli (for terraform output), admin creds in staging-aws.env
#
# What runs:
#   1. Core smoke (health, admin, paper account → webhook → ORDER_SIGNAL → FILLED)
#   2. Zerodha adapter healthz + OAuth callback public check (unless --skip-adapters)
#   3. Full functional suite with --full

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
exec bash "${ROOT}/deploy/deploy-aws-ecs/scripts/aws-deploy.sh" --test-only "$@"
