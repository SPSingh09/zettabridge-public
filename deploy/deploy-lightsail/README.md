# Lightsail + private GHCR deploy

Run ZettaBridge on **AWS Lightsail** (or any Ubuntu VM) by **pulling a private Docker image** from **GitHub Container Registry**. No public repo, no source code or `go build` on the server.

**Cost:** Lightsail plan only (~$24/mo for 4 GB Mumbai). GHCR + GitHub Actions stay on the **GitHub Free** tier for private repos.

---

## Architecture

```text
Laptop                    GitHub                         Lightsail VM
  │                         │                                  │
  │  git push main          │                                  │
  ├────────────────────────►│  Actions: docker build           │
  │                         │  push ghcr.io/.../zettabridge    │
  │                         │                                  │
  │  (one-time VM setup)    │                                  │  docker pull
  │                         │◄─────────────────────────────────┤  compose up
  │                         │                                  │  runtime.env (secrets)
```

| On GitHub | On VM only |
|-----------|------------|
| Source code | `runtime.env` (JWT, AES, DB password) |
| Private GHCR image | `ghcr.env` (read-only PAT) |
| GitHub Actions build | Postgres data, Redis |
| | `migrations/` + `migrate-rds.sh` (small ops files) |

---

## Part 1 — GitHub (one-time)

### 1.1 Push repo to GitHub (private)

Repo must be on GitHub for Actions + GHCR. Keep it **private**.

### 1.2 Set the dashboard API URL (repo variable)

The dashboard's `NEXT_PUBLIC_API_URL` is baked at build time — runtime env has no effect.

GitHub → **Settings** → **Variables** → **Actions** → **New repository variable**:

| Name | Value |
|------|-------|
| `NEXT_PUBLIC_API_URL` | `http://VM_IP:8080` (IP-only) or `https://api.yourdomain.com` (Caddy) |

Set this **before** the first CI push; rebuild the dashboard image after changing it.

### 1.3 Enable workflow

File: [`.github/workflows/docker-publish.yml`](../../.github/workflows/docker-publish.yml)

The workflow builds **two images** in parallel:

| Image | Tag (main) |
|-------|-----------|
| `ghcr.io/spsingh09/zettabridge` | `:staging` |
| `ghcr.io/spsingh09/zettabridge-dashboard` | `:staging` |

After push to `main`:

1. GitHub → **Actions** → **Publish Docker image** → confirm both jobs green
2. **Packages** (profile menu) → confirm both packages exist

Use **lowercase** username in all image URLs.

### 1.4 Link packages to repo (if pull fails later)

Package page → **Package settings** → **Manage Actions access** → grant this repo access.
Do this for **both** `zettabridge` and `zettabridge-dashboard`.

---

## Part 2 — GitHub PAT for VM pull (one-time)

VM needs a **read-only** token to pull the private image.

1. GitHub → **Settings** → **Developer settings** → **Fine-grained personal access tokens**
2. **Generate new token**
3. **Repository access:** Only `zettabridge`
4. **Permissions:** **Packages → Read-only**
5. Copy token (shown once)

---

## Part 3 — Lightsail VM (one-time)

Assumes Ubuntu 24.04, static IP, firewall **22 / 80 / 443**.

### 3.1 Bootstrap Docker and system packages

**Run this first** — installs Docker, Compose, `psql`, UFW, and `/opt/zettabridge/` layout.

From **WSL**:

```bash
STATIC_IP=YOUR_STATIC_IP
scp deploy/vm/bootstrap.sh ubuntu@$STATIC_IP:/tmp/
ssh ubuntu@$STATIC_IP 'sudo bash /tmp/bootstrap.sh'
```

Then **log out and SSH back in** (so `docker` works without sudo):

```bash
exit
ssh ubuntu@YOUR_STATIC_IP
docker --version
docker compose version
```

Script reference: [`deploy/vm/bootstrap.sh`](../vm/bootstrap.sh) (same as `requirements.txt` for the server).

### 3.2 Copy ops files from laptop (no full repo on VM)

On **WSL**:

```bash
STATIC_IP=YOUR_STATIC_IP

ssh ubuntu@$STATIC_IP 'mkdir -p /opt/zettabridge/{env,scripts,migrations,compose}'

scp deploy/env/oci.staging.example \
    deploy/env/ghcr.env.example \
    ubuntu@$STATIC_IP:/opt/zettabridge/env/

scp deploy/scripts/migrate-rds.sh \
    deploy/scripts/check-env.sh \
    ubuntu@$STATIC_IP:/opt/zettabridge/scripts/

scp -r migrations ubuntu@$STATIC_IP:/opt/zettabridge/

scp deploy/compose/production.base.yml \
    deploy/compose/oci.staging.yml \
    ubuntu@$STATIC_IP:/opt/zettabridge/compose/

scp deploy/lightsail/deploy.sh ubuntu@$STATIC_IP:/opt/zettabridge/
scp deploy/oci/Caddyfile.example ubuntu@$STATIC_IP:/opt/zettabridge/caddy/Caddyfile

# Monitoring (4A.3/4A.4) — one command from repo root:
./deploy/scripts/sync-monitoring-to-vm.sh $STATIC_IP

ssh ubuntu@$STATIC_IP 'chmod +x /opt/zettabridge/deploy.sh /opt/zettabridge/scripts/*.sh'
```

When SQL migrations change, re-copy `migrations/` only.

### 3.3 Secrets on VM

```bash
ssh ubuntu@YOUR_STATIC_IP

cp /opt/zettabridge/env/oci.staging.example /opt/zettabridge/env/runtime.env
cp /opt/zettabridge/env/ghcr.env.example /opt/zettabridge/env/ghcr.env
chmod 600 /opt/zettabridge/env/runtime.env /opt/zettabridge/env/ghcr.env
nano /opt/zettabridge/env/runtime.env
nano /opt/zettabridge/env/ghcr.env
```

**runtime.env** — generate:

```bash
openssl rand -base64 48   # JWT_SECRET
openssl rand -hex 32      # AES_KEY
```

Set `POSTGRES_PASSWORD`, `DATABASE_URL`, `APP_PUBLIC_URL`, etc.

**ghcr.env** — example:

```bash
GHCR_USER=your-github-username
GHCR_TOKEN=ghp_xxxxxxxx
ZETTABRIDGE_IMAGE=ghcr.io/your-github-username/zettabridge:staging
ZETTABRIDGE_DASHBOARD_IMAGE=ghcr.io/your-github-username/zettabridge-dashboard:staging
```

Validate:

```bash
/opt/zettabridge/scripts/check-env.sh /opt/zettabridge/env/runtime.env
```

---

## Part 4 — First deploy on VM

```bash
cd /opt/zettabridge

./deploy.sh login-ghcr
./deploy.sh pull
./deploy.sh up
./deploy.sh migrate
curl -sS http://127.0.0.1:8080/healthz
```

---

## Part 5 — Every release (after `git push main`)

Wait for GitHub Actions to finish, then on VM:

```bash
cd /opt/zettabridge
./deploy.sh pull
./deploy.sh up
# If migrations/*.sql changed:
./deploy.sh migrate
```

Optional from laptop without SSH:

GitHub → Actions → **Publish Docker image** → **Run workflow** (manual dispatch).

---

## IP-only access (no domain, no TLS)

If you don't have a domain yet, expose both ports directly and skip Caddy.

On the **VM**:
```bash
sudo ufw allow 8080/tcp   # API
sudo ufw allow 3000/tcp   # Dashboard
```

In `/opt/zettabridge/env/runtime.env` add:
```bash
SERVER_BIND=0.0.0.0
DASHBOARD_BIND=0.0.0.0
CORS_ORIGINS=http://VM_IP:3000
APP_PUBLIC_URL=http://VM_IP:8080
```

**GitHub repo var** (set before pushing):
```
NEXT_PUBLIC_API_URL=http://VM_IP:8080
```

Then `./deploy.sh pull && ./deploy.sh up` (no `--profile https`).

Access: `http://VM_IP:3000` for the dashboard, `http://VM_IP:8080` for the API.

---

## HTTPS (optional)

```bash
sudo cp /opt/zettabridge/Caddyfile.example /opt/zettabridge/caddy/Caddyfile
sudo nano /opt/zettabridge/caddy/Caddyfile   # your domain
./deploy.sh up --profile https
```

Point DNS A record → Lightsail static IP.

---

## Beta vs full live env

**Beta (no Indian IP whitelist):**

```bash
BROKER_MODE=live
EARLY_BIRD_LIVE_ENABLED=false
SEBI_ALGO_ID_REQUIRED=false
BROKER_MT5_DEMO_BASE_URL=https://mt-client-api-v1.london.agiliumtrade.ai
```

**Full live (Indian + MT5):** set `BROKER_ANGEL_CLIENT_*` to static IP, whitelist at brokers, `SEBI_ALGO_ID_REQUIRED=true`.

---

## Staging smoke test

From **WSL** (requires `curl` and `jq`):

```bash
export STAGING_BASE_URL=http://52.66.136.127
export STAGING_ADMIN_EMAIL=admin@zettaflux.com
export STAGING_ADMIN_PASSWORD='your-password'
./deploy/scripts/staging-smoke.sh
```

| Variable | Purpose |
|----------|---------|
| `STAGING_BASE_URL` | `http://STATIC_IP` or `https://api.staging.example.com` |
| `STAGING_ADMIN_EMAIL` | Bootstrap admin (for role + plan bump to read trades) |
| `STAGING_ADMIN_PASSWORD` | Admin password |
| `STAGING_REGISTER_ADMIN=1` | Register admin if login fails (then restart VM with bootstrap) |
| `STAGING_SKIP_ADMIN=1` | Skip admin; ingest-only (trades step may fail on free plan) |

---

## Staging functional suite (Tier 2)

Full API regression from WSL (requires **Go** on laptop):

```bash
export STAGING_BASE_URL=http://52.66.136.127
export STAGING_ADMIN_EMAIL=admin@zettaflux.com
export STAGING_ADMIN_PASSWORD='your-password'
./functional-tester/run-functional-staging.sh
# or: make functional-test-staging
```

| Variable | Purpose |
|----------|---------|
| `STAGING_RUN_LIVE_ROUTING=1` | Also run live mock-routing test (`BROKER_MODE=live`) |
| `STAGING_LIVE_ROUTING_ONLY=1` | Live routing test only |
| `STAGING_FUNCTIONAL_TIMEOUT` | Default `15m` |

Creates many ephemeral users/orgs — **disposable staging only**. Run nightly or before releases.

---

## Monitoring (4A.3 + 4A.4)

Sidecar on the **same VM** — Prometheus scrapes `127.0.0.1:8080/metrics`, Grafana on localhost only, Telegram alerts to a **staging** group (`environment=staging`).

**First time:** copy monitoring files from your laptop (VM has no `deploy/` tree):

```bash
# On WSL (repo root)
./deploy/scripts/sync-monitoring-to-vm.sh YOUR_STATIC_IP
```

**On the VM:**

```bash
cp /opt/zettabridge/env/monitoring.env.example /opt/zettabridge/env/monitoring.env
chmod 600 /opt/zettabridge/env/monitoring.env
# Set TELEGRAM_BOT_TOKEN, TELEGRAM_STAGING_CHAT_ID, GRAFANA_ADMIN_PASSWORD

/opt/zettabridge/scripts/monitoring-up.sh

# From laptop: SSH tunnel to Grafana
ssh -L 3001:127.0.0.1:3001 ubuntu@YOUR_STATIC_IP

# Test Telegram routing
/opt/zettabridge/scripts/monitoring-test-alert.sh
```

See [docs/observability.md](../../docs/observability.md).

---

## Troubleshooting

| Error | Fix |
|-------|-----|
| `denied` on `docker pull` | Re-run `./deploy.sh login-ghcr`; check PAT has **Packages read** |
| `manifest unknown` | Actions run failed or wrong `ZETTABRIDGE_IMAGE` tag (use `:staging`) |
| `exec format error` | Image must be `linux/amd64` (workflow builds amd64 for Lightsail) |
| migrate fails | Ensure `/opt/zettabridge/migrations/` exists; `DATABASE_URL` uses `127.0.0.1` |
| Grafana empty / Scrape DOWN | API binds `127.0.0.1:8080` — re-run `./deploy/scripts/sync-monitoring-to-vm.sh` then `monitoring-down.sh && monitoring-up.sh` (Prometheus joins `zettabridge_internal` and scrapes `server:8080`) |

---

## Cost checklist

| Item | Cost |
|------|------|
| Private GitHub repo | $0 |
| GitHub Actions (private) | $0 within 2,000 min/mo |
| Private GHCR storage (small image) | $0 typical |
| Lightsail 4 GB + static IP | ~$24/mo |
| AWS ECR / ECS Terraform | **Not used** |

---

## Files

| Path | Purpose |
|------|---------|
| [`.github/workflows/docker-publish.yml`](../../.github/workflows/docker-publish.yml) | Build + push private image |
| [`../env/ghcr.env.example`](../env/ghcr.env.example) | VM pull credentials template |
| [`deploy.sh`](deploy.sh) | `pull`, `up`, `migrate` on VM |
| [`../compose/production.base.yml`](../compose/production.base.yml) | Postgres + Redis + app |
| [`../compose/oci.staging.yml`](../compose/oci.staging.yml) | VM volumes, ports, Caddy |
