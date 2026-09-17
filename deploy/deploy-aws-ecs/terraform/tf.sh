#!/usr/bin/env bash
# Run Terraform in deploy/deploy-aws-ecs/terraform/staging from any working directory.
#
#   ./deploy/deploy-aws-ecs/terraform/tf.sh init
#   ./deploy/deploy-aws-ecs/terraform/tf.sh plan
#   ./deploy/deploy-aws-ecs/terraform/tf.sh apply

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/staging" && pwd)"
exec terraform -chdir="${ROOT}" "$@"
