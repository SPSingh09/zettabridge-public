# VM bootstrap — installs Docker, Compose, UFW, postgresql-client, and /opt/zettabridge layout.
#
# Run on a fresh Ubuntu VM (Lightsail, OCI, DO, Hetzner):
#   scp deploy/vm/bootstrap.sh ubuntu@YOUR_IP:/tmp/
#   ssh ubuntu@YOUR_IP 'sudo bash /tmp/bootstrap.sh'
#
# Then log out and SSH back in (docker group), and continue with deploy/lightsail/README.md

## Installs (like Python requirements.txt)

| Package | Purpose |
|---------|---------|
| docker-ce + compose plugin | Run app, Postgres, Redis |
| k6 | Load tests (`deploy/loadtest/`, phase 5B.1) |
| git, curl, ca-certificates | Ops / downloads |
| postgresql-client (`psql`) | `migrate-rds.sh` |
| openssl | Generate JWT_SECRET / AES_KEY |
| ufw | Host firewall (22, 80, 443) |

## After bootstrap

- **GHCR path:** [lightsail/README.md](../lightsail/README.md)
- **Build on VM:** [oci/README.md](../oci/README.md)
