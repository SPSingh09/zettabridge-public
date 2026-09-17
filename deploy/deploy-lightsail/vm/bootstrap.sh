#!/usr/bin/env bash
# ZettaBridge VM bootstrap — installs everything the server needs (like requirements.txt).
#
# Installs:
#   - git, curl, ca-certificates, gnupg, ufw, postgresql-client, openssl
#   - Docker Engine + Compose plugin
#   - k6 (load test CLI — also useful on Ubuntu/WSL dev machines)
#   - /opt/zettabridge/ directory layout
#   - UFW: SSH (22), HTTP (80), HTTPS (443)
#
# Run on a fresh Ubuntu 22.04 / 24.04 VM as root:
#   sudo bash bootstrap.sh
#
# Or from laptop:
#   scp deploy/vm/bootstrap.sh ubuntu@YOUR_IP:/tmp/
#   ssh ubuntu@YOUR_IP 'sudo bash /tmp/bootstrap.sh'
#
# Tested: OCI Ampere (arm64), AWS Lightsail (amd64), DigitalOcean, Hetzner.

set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

DEPLOY_USER="${DEPLOY_USER:-ubuntu}"
export DEBIAN_FRONTEND=noninteractive

echo "==> ZettaBridge VM bootstrap (user: ${DEPLOY_USER})"

echo "==> Installing base packages"
apt-get update -qq
apt-get install -y -qq \
  ca-certificates \
  curl \
  gnupg \
  git \
  ufw \
  openssl \
  postgresql-client

echo "==> Installing Docker"
install -m 0755 -d /etc/apt/keyrings
if [[ ! -f /etc/apt/keyrings/docker.gpg ]]; then
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
fi

# shellcheck disable=SC1091
source /etc/os-release
arch="$(dpkg --print-architecture)"
echo \
  "deb [arch=${arch} signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
  > /etc/apt/sources.list.d/docker.list

apt-get update -qq
apt-get install -y -qq \
  docker-ce \
  docker-ce-cli \
  containerd.io \
  docker-buildx-plugin \
  docker-compose-plugin

systemctl enable --now docker

if id "${DEPLOY_USER}" &>/dev/null; then
  usermod -aG docker "${DEPLOY_USER}"
  echo "    Added ${DEPLOY_USER} to group docker (re-login SSH to use without sudo)"
fi

echo "==> Installing k6 (load test CLI)"
bash "$(dirname "$0")/../scripts/install-k6.sh"

echo "==> Configuring UFW (SSH + HTTP/S)"
ufw --force reset
ufw default deny incoming
ufw default allow outgoing
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable

echo "==> Creating /opt/zettabridge layout"
install -d -m 0750 /opt/zettabridge/{env,caddy,compose,scripts,migrations}
install -d -m 0755 /opt/zettabridge/data
install -d -m 0700 -o 999 -g 999 /opt/zettabridge/data/postgres

if id "${DEPLOY_USER}" &>/dev/null; then
  chown -R "${DEPLOY_USER}:${DEPLOY_USER}" /opt/zettabridge
  # Postgres container runs as UID 999 — data dir must not stay owned by ubuntu.
  chown -R 999:999 /opt/zettabridge/data/postgres
fi

if [[ ! -f /opt/zettabridge/env/runtime.env ]]; then
  cat > /opt/zettabridge/env/runtime.env <<'EOF'
# Fill secrets — see deploy/env/oci.staging.example
POSTGRES_PASSWORD=
APP_ENV=staging
JWT_SECRET=
AES_KEY=
APP_PUBLIC_URL=
CORS_ORIGINS=
BROKER_MODE=live
EARLY_BIRD_LIVE_ENABLED=false
SEBI_ALGO_ID_REQUIRED=false
EMAIL_VERIFICATION_REQUIRED=false
EMAIL_PROVIDER=log
METRICS_ENABLED=true
DATABASE_URL=postgres://zettabridge:PASS@127.0.0.1:5432/zettabridge?sslmode=disable
REDIS_URL=redis://redis:6379
EOF
  chmod 600 /opt/zettabridge/env/runtime.env
  [[ -n "${DEPLOY_USER}" ]] && chown "${DEPLOY_USER}:${DEPLOY_USER}" /opt/zettabridge/env/runtime.env 2>/dev/null || true
  echo "    Created /opt/zettabridge/env/runtime.env (edit before deploy)"
fi

if [[ ! -f /opt/zettabridge/caddy/Caddyfile ]]; then
  cat > /opt/zettabridge/caddy/Caddyfile <<'EOF'
# Replace with your domains (DNS A records → VM public IP).
# See deploy/oci/Caddyfile.example for the full template.
api.yourdomain.com {
	encode gzip
	reverse_proxy server:8080
}

app.yourdomain.com {
	encode gzip
	reverse_proxy dashboard:3000
}
EOF
  chmod 644 /opt/zettabridge/caddy/Caddyfile
fi

echo "==> Verifying Docker"
docker --version
docker compose version

echo
echo "==> Bootstrap complete"
echo
echo "Next steps (as ${DEPLOY_USER} — log out and SSH back in first if docker was just installed):"
echo "  1. scp deploy files to /opt/zettabridge/ (see deploy/lightsail/README.md)"
echo "  2. Edit /opt/zettabridge/env/runtime.env — set POSTGRES_PASSWORD, JWT_SECRET, AES_KEY,"
echo "       APP_PUBLIC_URL, CORS_ORIGINS"
echo "  3. Edit /opt/zettabridge/env/ghcr.env — set GHCR_USER, GHCR_TOKEN,"
echo "       ZETTABRIDGE_IMAGE, ZETTABRIDGE_DASHBOARD_IMAGE"
echo "  4. openssl rand -base64 48   # JWT_SECRET"
echo "     openssl rand -hex 32      # AES_KEY"
echo "  5. /opt/zettabridge/scripts/check-env.sh /opt/zettabridge/env/runtime.env"
echo "  6. cd /opt/zettabridge && ./deploy.sh login-ghcr && ./deploy.sh pull && ./deploy.sh up && ./deploy.sh migrate"
echo "  7. curl -sS http://127.0.0.1:8080/healthz"
echo ""
echo "  IP-only (no domain, no TLS):"
echo "    ufw allow 8080/tcp && ufw allow 3000/tcp"
echo "    In runtime.env: SERVER_BIND=0.0.0.0  DASHBOARD_BIND=0.0.0.0"
echo "    GitHub repo var NEXT_PUBLIC_API_URL=http://VM_IP:8080 (set before CI push)"
echo ""
echo "  With domain + TLS (Caddy):"
echo "    Edit /opt/zettabridge/caddy/Caddyfile — set api.yourdomain.com + app.yourdomain.com"
echo "    ./deploy.sh up --profile https"
