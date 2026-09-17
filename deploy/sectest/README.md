# Pen test tooling (5B.2)

Scripted abuse checks for auth, webhooks, tenant isolation, and metrics exposure.

## Prerequisites

- `curl`, `jq`, `bash`
- Staging URL reachable from your machine (WSL → Lightsail)

## Quick start

```bash
export STAGING_BASE_URL=http://52.66.136.127
./deploy/sectest/run-sectest.sh
```

Or:

```bash
cp deploy/sectest/env.example deploy/sectest/env.local
# edit SECTEST_BASE_URL
make sectest-staging
```

## Scripts

| Script | What it does |
|--------|----------------|
| `scripts/jwt-tamper.sh` | Missing Bearer, garbage JWT, alg=none, tampered signature |
| `scripts/admin-isolation.sh` | Non-admin blocked from `/v1/admin/*` |
| `scripts/webhook-token-probe.sh` | Invalid token, rotate-token invalidates old URL, paused webhook |
| `scripts/idor-probe.sh` | Two users — B cannot access A's webhooks, creds, trades |
| `scripts/webhook-flood.sh` | 25 parallel POSTs against `rate_limit_per_sec=5` webhook |
| `scripts/injection-probe.sh` | Invalid action, SQL-ish comment, oversized body |
| `scripts/metrics-auth.sh` | `/metrics` with optional `SECTEST_METRICS_TOKEN` |

Each script creates ephemeral `sectest-*@sectest.local` users; safe to re-run.

## Checklist

See [checklist.md](checklist.md) for manual items and sign-off table.

## Workflow

1. Run `run-sectest.sh` on Lightsail (Track A).
2. File failures as `security/5b.2` issues; fix and add regression tests where cheap.
3. Re-run until green.
4. After AWS 5A + HTTPS, re-run on production-like URL (Track B).

Plan: [docs/plans/phase-5b-security-hardening.md](../../docs/plans/phase-5b-security-hardening.md)
