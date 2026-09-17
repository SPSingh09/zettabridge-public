#!/usr/bin/env bash
# Run ZettaBridge SQL migrations against staging RDS.
#
# Launches a one-off ECS Fargate task (using the currently deployed image) with
# MIGRATE_ONLY=true. The task connects to RDS from inside the VPC, applies any
# pending migrations, then exits. No direct database access from your machine needed.
#
# Usage (from repo root):
#
#   # Apply pending migrations only (normal use):
#   ./deploy/deploy-aws-ecs/scripts/run-migrations.sh
#
#   # Wipe the DB and re-apply the full schema (staging reset):
#   ./deploy/deploy-aws-ecs/scripts/run-migrations.sh --reset
#
#   # Dry-run: shows config, does not launch task:
#   DRY_RUN=1 ./deploy/deploy-aws-ecs/scripts/run-migrations.sh [--reset]
#
# Prerequisites: aws CLI, jq

set -euo pipefail

AWS_REGION="${AWS_REGION:-ap-south-1}"
CLUSTER="${ECS_CLUSTER:-zettabridge-staging}"
SERVICE="${ECS_SERVICE:-zettabridge-staging}"
DRY_RUN="${DRY_RUN:-0}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-300}"
RESET_SCHEMA="false"

for arg in "$@"; do
  case "${arg}" in
    --reset) RESET_SCHEMA="true" ;;
    *) echo "Unknown argument: ${arg}" >&2; exit 1 ;;
  esac
done

if ! command -v aws >/dev/null 2>&1; then
  echo "error: aws CLI not found." >&2; exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq not found — install it (apt install jq)." >&2; exit 1
fi

echo "=== ZettaBridge migration runner ==="
echo "cluster      : ${CLUSTER}  region: ${AWS_REGION}"
echo "reset schema : ${RESET_SCHEMA}"
echo ""

if [[ "${RESET_SCHEMA}" == "true" ]]; then
  echo "WARNING: --reset will DROP all tables and data in the staging database."
  echo "         This is irreversible. You have 5 seconds to cancel (Ctrl-C)."
  sleep 5
fi

# ── 1. Resolve task definition and network config from the running service ──

echo "Fetching service config..."
SERVICE_DESC="$(aws ecs describe-services \
  --cluster "${CLUSTER}" --services "${SERVICE}" \
  --region "${AWS_REGION}" \
  --query 'services[0]' --output json)"

TASK_DEF="$(echo "${SERVICE_DESC}" | jq -r '.taskDefinition')"
SUBNETS="$(echo "${SERVICE_DESC}" | jq -c '.networkConfiguration.awsvpcConfiguration.subnets')"
SECURITY_GROUPS="$(echo "${SERVICE_DESC}" | jq -c '.networkConfiguration.awsvpcConfiguration.securityGroups')"

echo "task definition : ${TASK_DEF}"
echo "subnets         : ${SUBNETS}"
echo "security groups : ${SECURITY_GROUPS}"
echo ""

if [[ "${DRY_RUN}" == "1" ]]; then
  if [[ "${RESET_SCHEMA}" == "true" ]]; then
    echo "DRY_RUN=1 — would launch task with RESET_SCHEMA=true + MIGRATE_ONLY=true (full wipe + re-migrate)."
  else
    echo "DRY_RUN=1 — would launch task with MIGRATE_ONLY=true (apply pending migrations only)."
  fi
  echo "Re-run without DRY_RUN=1 to execute."
  exit 0
fi

# ── 2. Build container environment overrides ────────────────────────────────

if [[ "${RESET_SCHEMA}" == "true" ]]; then
  OVERRIDES='{
    "containerOverrides": [{
      "name": "zettabridge",
      "environment": [
        {"name": "MIGRATE_ONLY",   "value": "true"},
        {"name": "RESET_SCHEMA",   "value": "true"}
      ]
    }]
  }'
else
  OVERRIDES='{
    "containerOverrides": [{
      "name": "zettabridge",
      "environment": [{"name": "MIGRATE_ONLY", "value": "true"}]
    }]
  }'
fi

# ── 3. Launch the one-off migration task ────────────────────────────────────

echo "Launching one-off ECS migration task..."
RUN_RESULT="$(aws ecs run-task \
  --cluster "${CLUSTER}" \
  --task-definition "${TASK_DEF}" \
  --launch-type FARGATE \
  --region "${AWS_REGION}" \
  --network-configuration "awsvpcConfiguration={subnets=${SUBNETS},securityGroups=${SECURITY_GROUPS},assignPublicIp=DISABLED}" \
  --overrides "${OVERRIDES}" \
  --output json)"

TASK_ARN="$(echo "${RUN_RESULT}" | jq -r '.tasks[0].taskArn')"
if [[ -z "${TASK_ARN}" || "${TASK_ARN}" == "null" ]]; then
  echo "error: failed to start ECS task." >&2
  echo "${RUN_RESULT}" >&2
  exit 1
fi

TASK_ID="${TASK_ARN##*/}"
echo "Task launched: ${TASK_ID}"
echo ""

# ── 4. Wait for the task to complete ────────────────────────────────────────

echo "Waiting for task to complete (timeout: ${WAIT_TIMEOUT}s)..."
ELAPSED=0
POLL=5
while true; do
  TASK_DESC="$(aws ecs describe-tasks \
    --cluster "${CLUSTER}" \
    --tasks "${TASK_ARN}" \
    --region "${AWS_REGION}" \
    --output json)"

  LAST_STATUS="$(echo "${TASK_DESC}" | jq -r '.tasks[0].lastStatus')"
  printf "  [%3ds] status: %s\n" "${ELAPSED}" "${LAST_STATUS}"

  if [[ "${LAST_STATUS}" == "STOPPED" ]]; then
    break
  fi

  if [[ "${ELAPSED}" -ge "${WAIT_TIMEOUT}" ]]; then
    echo ""
    echo "error: timed out waiting for migration task after ${WAIT_TIMEOUT}s." >&2
    echo "Check: https://ap-south-1.console.aws.amazon.com/ecs/v2/clusters/${CLUSTER}/tasks/${TASK_ID}" >&2
    exit 1
  fi

  sleep "${POLL}"
  ELAPSED=$((ELAPSED + POLL))
done

# ── 5. Report result ─────────────────────────────────────────────────────────

EXIT_CODE="$(echo "${TASK_DESC}" | jq -r '.tasks[0].containers[0].exitCode')"
STOP_REASON="$(echo "${TASK_DESC}" | jq -r '.tasks[0].stoppedReason // "none"')"

echo ""
echo "Task stopped. exitCode=${EXIT_CODE}  reason=${STOP_REASON}"

if [[ "${EXIT_CODE}" == "0" ]]; then
  echo ""
  echo "Migrations applied successfully."
  echo "View logs: aws logs tail /ecs/zettabridge-staging --follow --region ${AWS_REGION}"
else
  echo ""
  echo "error: migration task exited with code ${EXIT_CODE}." >&2
  echo "View logs: aws logs tail /ecs/zettabridge-staging --follow --region ${AWS_REGION}" >&2
  exit 1
fi
