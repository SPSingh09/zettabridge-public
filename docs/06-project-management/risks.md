# Risk Register
**Last updated: 2026-07-07**

All known risks to ZettaBridge's beta and GA timeline. Review and update at each milestone. Add new risks as they are identified.

**Probability:** Low · Medium · High
**Impact:** Low · Medium · High · Critical
**Status:** Open · Mitigated · Accepted · Closed

---

## Regulatory Risks

| ID | Risk | Probability | Impact | Mitigation | Status | Owner |
|----|------|-------------|--------|------------|--------|-------|
| REG.1 | SEBI Algo-ID sandbox validation not completed before beta open — live orders may be non-compliant | High | Critical | Algo-ID code is shipped; sandbox validation (P3.2.S) is a beta gate; `SEBI_ALGO_ID_REQUIRED=true` auto-enforced in live mode | Open | Surya |
| REG.2 | ZettaBridge is perceived as providing investment advice — regulatory risk without appropriate disclaimers | Medium | High | Platform is an execution bridge only; Terms of Service and Risk Disclosure must clearly state no investment advice is given; disclaimer required before beta opens | Open | Surya |
| REG.3 | SEBI algo-trading regulations evolve (e.g. registration thresholds change) after beta open | Low | High | Monitor NSE/SEBI circulars; per-credential `algo_id` field allows fast re-registration without code changes | Open | Surya |
| REG.4 | No registered business entity — cannot enter into contracts, process payments, or operate commercially at scale | High | Critical | Beta operates without commercial payment processing (admin billing only); business registration is a prerequisite for payment gateways and GA | Open | Surya |

---

## Broker Risks

| ID | Risk | Probability | Impact | Mitigation | Status | Owner |
|----|------|-------------|--------|------------|--------|-------|
| BKR.1 | Angel One NAT EIP whitelist not approved before beta — live Angel credentials cannot be used | High | Critical | Whitelist request is purely a manual portal action; it is a beta gate (B4.1); Dhan is a fallback for initial live test | Open | Surya |
| BKR.2 | Dhan NAT EIP whitelist not approved before beta | High | Critical | Same as BKR.1; both are beta gates (B4.2) | Open | Surya |
| BKR.3 | Zerodha access tokens expire daily — users forget to refresh, causing `auth_failed` | High | Medium | broker-guide.md documents the manual PUT flow; dashboard shows credential verify status; automatic refresh deferred to post-GA | Open | Surya |
| BKR.4 | Angel One session token expires within hours — higher frequency of credential failures | High | Medium | Same mitigation as BKR.3; credential verify endpoint surfaces failure immediately | Open | Surya |
| BKR.5 | Broker API changes (endpoint deprecation, payload schema change) break live adapters | Low | High | Adapters are isolated per-broker in `internal/livebrokers/`; mock mode allows rapid local testing; monitor broker changelogs | Open | Surya |
| BKR.6 | Broker outage during live trading — orders cannot be placed | Medium | High | `BROKER_MODE=mock` rollback procedure documented; workers log broker errors as `error_code=broker_error`; no retry on order placement (idempotency) | Open | Surya |
| BKR.7 | Zerodha per-user API key constraint permanently prevents multi-user Zerodha accounts on a shared platform key | Certain (confirmed) | Medium | Accepted: each user provides own Kite Connect API key; documented in scope.md and broker-guide.md; no workaround exists | Accepted | Surya |
| BKR.8 | Dhan `correlationId` field interpretation may change — SEBI compliance gap for Dhan orders | Low | High | Confirmed with Dhan support that `correlationId` is the correct field for Algo-ID; keep correspondence as evidence; re-verify post-broker-API-update | Mitigated | Surya |

---

## Security Risks

| ID | Risk | Probability | Impact | Mitigation | Status | Owner |
|----|------|-------------|--------|------------|--------|-------|
| SEC.1 | Raw `AES_KEY` in environment / Secrets Manager compromised — all stored credentials decryptable | Low | Critical | Key is in Secrets Manager (not code/image); KMS CMK encryption of Secrets Manager in place; KMS envelope encryption (S6) will further reduce blast radius; key rotation procedure documented in operations-security.md | Open | Surya |
| SEC.2 | Penetration test not completed before GA — unknown vulnerabilities in production | High (not done yet) | High | Pre-pen-test hardening (security headers, log scrubbing, rate limit verification) is a planned gate before pen test; pen test is a GA gate | Open | Surya |
| SEC.3 | Webhook URL token leaked — attacker places orders on behalf of user | Medium | High | Token rotation via `POST /v1/webhooks/:id/rotate-token`; old token immediately rejected; user suspension blocks further abuse; rate limits reduce blast radius | Mitigated | Surya |
| SEC.4 | JWT secret compromised — all sessions invalidated, accounts potentially accessible | Low | High | `JWT_SECRET` in Secrets Manager; rotation requires ECS redeploy (all tokens invalidated, users re-login); no token-level revocation in v1 | Open | Surya |
| SEC.5 | Security response headers missing — XSS and clickjacking vectors open | High (not yet implemented) | Medium | S1 (security headers) is a planned pre-pen-test item; HSTS, X-Content-Type-Options, X-Frame-Options to be added via Fiber middleware | Open | Surya |
| SEC.6 | IDOR vulnerabilities allow user A to access user B's webhooks, credentials, or trades | Low | High | All routes use JWT user ID for ownership check; to be validated in pen test (S5) | Open | Surya |
| SEC.7 | Broker credential raw_creds accidentally logged or returned in API response | Low | Critical | `raw_creds` has `json:"-"` tag; broker HTTP client redacts auth headers; log hygiene checklist in operations-security.md; to be validated in log scrubbing audit (S2) | Mitigated | Surya |
| SEC.8 | No credential versioning — AES_KEY rotation requires all users to re-enter credentials | Certain (v1 limitation) | Medium | Documented in scope.md as a known constraint; KMS envelope encryption (S6) + credential versioning (S7) will fix this post-beta | Accepted (v1) | Surya |

---

## Infrastructure Risks

| ID | Risk | Probability | Impact | Mitigation | Status | Owner |
|----|------|-------------|--------|------------|--------|-------|
| INF.1 | ECS service runs single task — no redundancy; task failure = downtime | Medium | High | ECS desired count = 1; ECS auto-restarts failed tasks; single-task is acceptable for beta; scale to 2+ for GA | Open | Surya |
| INF.2 | Redis is single ElastiCache node — outage resets dedup windows and rate limits | Low | Medium | Redis is cache-only (no durable state); on Redis loss, dedup and rate limits reset gracefully; documented in DR plan | Open | Surya |
| INF.3 | RDS single-AZ on staging — AZ failure causes database outage | Low | High | Single-AZ is acceptable for staging; GA uses Multi-AZ; RDS PITR enabled with 7-day retention | Open (staging only) | Surya |
| INF.4 | No CI/CD pipeline — manual deploys are error-prone and slow | High | Medium | G3 (CI/CD pipeline) is a GA gate; staging deploys are manual currently; checklist in beta-phase plans | Open | Surya |
| INF.5 | No load test completed — system behaviour under 500 concurrent webhooks is unknown | High | High | S4 (load test) is a GA gate; k6 scripts exist at `deploy/loadtest/k6/`; run against AWS staging | Open | Surya |
| INF.6 | No disaster recovery drill — RTO target of < 1 hour unverified | High | High | G5 (DR drill) is a GA gate; RDS PITR and Redis recovery procedures are documented | Open | Surya |
| INF.7 | AWS costs exceed budget before revenue — staging runs continuously | Medium | Medium | Budget alert at $100 set in CloudWatch; ECS minimum task count; use Fargate Spot for non-prod tasks to reduce cost | Open | Surya |
| INF.8 | No AWS WAF — ALB directly exposed to public internet without L7 protection | High | Medium | G2 (AWS WAF) is a GA gate; app-level rate limiting and auth guards provide partial defence | Open | Surya |
| INF.9 | Stale OpenAPI/swagger lists removed org endpoints — integrators may assume org API exists | Medium | Low | Router is authoritative; regenerate swagger (AD.2); STATUS.md documents removal | Open | Surya |

---

## Business Risks

| ID | Risk | Probability | Impact | Mitigation | Status | Owner |
|----|------|-------------|--------|------------|--------|-------|
| BUS.1 | Payment gateway blocked indefinitely — no revenue path for beta users | High | High | Admin billing override covers closed beta; business registration is the only unblock; no technical mitigation | Open | Surya |
| BUS.2 | Stripe invite-only for India — Stripe approval may take months after registration | Medium | High | Apply for Stripe India as soon as business registration completes; Razorpay is a backup (same business registration requirement) | Open | Surya |
| BUS.3 | Beta traders lose real money due to a platform bug — reputational and potential legal exposure | Low | Critical | Paper plan for simulation; live requires Pro+ and explicit live credential; sandbox Algo-ID validation before live; incident rollback to `BROKER_MODE=mock` in < 5 minutes | Open | Surya |
| BUS.4 | SEBI/broker compliance change invalidates Algo-ID implementation after launch | Low | High | Implementation follows latest NSE circulars and broker confirmations; modular adapter design allows fast field changes; per-credential `algo_id` requires no platform code changes for ID rotation | Open | Surya |
| BUS.5 | Users confuse `BROKER_MODE=mock` (server staging) with paper trading (user plan feature) | Medium | Medium | Glossary and scope.md distinguish server mock vs paper engine; dashboard has dedicated Paper Accounts nav | Mitigated | Surya |

---

## Operational Risks

| ID | Risk | Probability | Impact | Mitigation | Status | Owner |
|----|------|-------------|--------|------------|--------|-------|
| OPS.1 | Duplicate live orders placed on TradingView retry before dedup window set | High | High | Dedup available on paid plans for **live** webhooks (paper exempt); onboarding must highlight `dedup_window_sec`; default 2s, recommend 300s for live | Open | Surya |
| OPS.2 | Broker token expiry not noticed by user — trades silently rejected | High | Medium | `auth_failed` error_code visible in dashboard trades tab and Prometheus metrics; Telegram alert on `auth_failed` spike; post-GA: proactive token expiry notifications | Open | Surya |
| OPS.3 | No oncall runbooks for common incidents | High | Medium | G6 (oncall runbooks) is a GA gate; operations-security.md has initial incident flows | Open | Surya |
| OPS.4 | Platform admin panel accidentally used to grant elevated access | Low | Medium | Admin endpoints require a separate admin JWT claim; admin account is a fixed bootstrap user; no self-serve admin promotion | Mitigated | Surya |
| OPS.5 | Webhook URL shared publicly — anyone can place orders on that webhook | Medium | High | Webhook URL is a bearer capability token; rotation is instant; users must treat it as a secret; documented in broker-guide; rate limits reduce blast radius | Mitigated | Surya |
