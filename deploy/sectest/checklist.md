# Phase 5B.2 — Pen test checklist

Track A (Lightsail HTTP) and Track B (AWS HTTPS) sign-off. File findings as GitHub issues labeled `security/5b.2`.

**Run:** `./deploy/sectest/run-sectest.sh` (or `make sectest-staging`)

---

## Automated scripts

| Script | Area | Pass criteria |
|--------|------|---------------|
| `jwt-tamper.sh` | JWT auth | Missing/garbage/tampered JWT → 401 |
| `admin-isolation.sh` | Admin RBAC | Regular user → `/v1/admin/*` returns 403 |
| `webhook-token-probe.sh` | Webhook token | Unknown token 401; rotate invalidates old; paused → 403 |
| `idor-probe.sh` | Tenant isolation | User B denied on A's trades (404/403), PUT/DELETE webhook, PUT credential |
| `webhook-flood.sh` | Rate limits | Burst → ≥1 HTTP 429 (needs pro + `rate_limit_per_sec`; uses admin bump) |
| `injection-probe.sh` | Input validation | Invalid/oversized payloads → 4xx, not 500 |
| `metrics-auth.sh` | Metrics exposure | Public URL: 200/401/404 acceptable; auth gate when `SECTEST_METRICS_TOKEN` set |

---

## Manual checks (optional)

| # | Test | Expected | Track A | Track B |
|---|------|----------|---------|---------|
| M1 | Brute-force webhook URL (random tokens) | 401, no timing leak | [ ] | [ ] |
| M2 | Expired JWT (`exp` in past) | 401 | [ ] | [ ] |
| M3 | HTTPS redirect / HSTS (when TLS enabled) | Enforced | n/a | [ ] |
| M4 | OWASP ZAP baseline (staging HTTPS) | No high/critical | n/a | [ ] |

---

## Sign-off log

| Date | Environment | Commit | Runner | Scripts | Manual | Notes |
|------|-------------|--------|--------|---------|--------|-------|
| | Lightsail HTTP | | | | | Pilot |
| | AWS HTTPS | | | | | Beta gate |

**Exit:** All P0/P1 findings closed; sign-off row filled before 5C closed beta.
