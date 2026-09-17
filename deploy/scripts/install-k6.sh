#!/usr/bin/env bash
# Install k6 from the official Grafana deb repo (Ubuntu 22.04/24.04, amd64 + arm64).
#
# Usage:
#   sudo bash deploy/scripts/install-k6.sh
#
# Called by deploy/vm/bootstrap.sh; safe to run alone on WSL for load tests.

set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive

echo "==> Installing k6"
install -m 0755 -d /etc/apt/keyrings
if [[ ! -f /etc/apt/keyrings/k6-archive-keyring.gpg ]]; then
  curl -fsSL https://dl.k6.io/key.gpg | gpg --dearmor -o /etc/apt/keyrings/k6-archive-keyring.gpg
  chmod a+r /etc/apt/keyrings/k6-archive-keyring.gpg
fi
if [[ ! -f /etc/apt/sources.list.d/k6.list ]]; then
  echo "deb [signed-by=/etc/apt/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
    > /etc/apt/sources.list.d/k6.list
fi
apt-get update -qq
apt-get install -y -qq k6

k6 version
echo "k6 installed."
