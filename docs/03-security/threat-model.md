# Threat Model
**Last updated: 2026-07-07**

ZettaBridge threat model for beta. Reviewed against OWASP Top 10 and the specific attack surface of a financial execution bridge. Update before GA pen test.

---

## 1. Assets to Protect

| Asset | Sensitivity | Impact if compromised |
|-------|-------------|----------------------|
| Broker API credentials (`raw_creds`) | Critical | Attacker can place or cancel trades on user's broker account |
| `AES_KEY` | Critical | All stored broker credentials become decryptable |
| `JWT_SECRET` | High | All user sessions can be forged; attacker gains any user's access |
| Webhook URL tokens | High | Attacker can submit signals → place trades on behalf of user |
| User PII (email, password hash) | Medium | Account takeover, phishing |
| Trade audit log | Medium | Proof of trading activity; leak of user strategy |
| Stripe keys | Medium | Subscription manipulation; refund fraud |
| Platform admin credentials | Critical | Full platform access; can suspend users, set plans |

---

## 2. Threat Actors

| Actor | Capability | Motivation |
|-------|-----------|------------|
| **Unauthenticated web attacker** | HTTP access to public endpoints | Brute-force tokens, discover unprotected endpoints |
| **Authenticated user (self)** | Valid JWT; access to own resources | Escalate privileges, access other users' data |
| **Compromised TradingView account** | Knowledge of webhook URL | Trigger signals on victim's webhook |
| **Server-side attacker** | Code injection or SSRF | Extract secrets from memory or Secrets Manager |
| **Infrastructure attacker** | AWS console / ECR access | Extract secrets from task definitions, exfiltrate DB |
| **Broker API attacker** | Stolen access token from outside ZettaBridge | Use outside ZettaBridge if IP whitelist not enforced |

---

## 3. Trust Boundaries

```
[TradingView] ─── webhook URL token ────► [ALB] ──► [Backend API]
                                                          │
[Browser]     ─── JWT (Bearer) ──────────────────────────┤
                                                          │
[ECS Worker]  ─────────────────────────────── [Live Broker APIs]
                                                          │
[ECS Worker]  ─── Secrets Manager + KMS ──────────── [AWS]
                                                          │
[Platform Admin] ─ JWT (admin claim) ──────────────────►
```

Key trust decisions:
- The webhook URL token is the **only** trust anchor for ingest. No IP check, no signature, no additional secret.
- JWT is trusted after HS256 verification and revocation check. Once forged (if JWT_SECRET is compromised), it's trusted as any real token.
- ECS task IAM role is trusted by Secrets Manager — a compromised task can read all secrets the role has access to.

---

## 4. Threat Scenarios and Mitigations

### T1 — Webhook URL token brute-force

**Vector:** Attacker tries random UUID values at `POST /v1/webhook/<uuid>`.

**Likelihood:** Low — UUID v4 has 2^122 entropy. Brute-force is computationally infeasible.

**Mitigation:**
- UUID v4 token (not sequential)
- Per-webhook rate limiting rejects rapid ingest
- Server-wide rate limiting at ALB level (planned: AWS WAF)

**Residual risk:** Low

---

### T2 — Webhook URL token leakage

**Vector:** User accidentally pastes URL in public (screenshot, commit, Slack), or token is in browser history / server logs.

**Likelihood:** Medium — TradingView alert URLs are visible in the alert configuration. Users may share screenshots.

**Mitigation:**
- Token rotation: `POST /v1/webhooks/:id/rotate-token` — immediate; old token rejected
- Documented in broker-guide with explicit warning
- Rate limits bound damage even if attacker has the URL

**Residual risk:** Medium (user behaviour; mitigated by rotation)

---

### T3 — JWT forgery (JWT_SECRET compromise)

**Vector:** Attacker obtains `JWT_SECRET`, forges tokens for any user including admin.

**Likelihood:** Low — secret is in Secrets Manager; not in code, images, or logs.

**Mitigation:**
- `JWT_SECRET` only in Secrets Manager; injected via ECS task definition secret reference
- Rotation procedure: update Secrets Manager → redeploy ECS → all tokens invalidated
- KMS CMK protects Secrets Manager at rest

**Residual risk:** Low (would require AWS console access or a Secrets Manager vulnerability)

---

### T4 — AES_KEY compromise → credential decryption

**Vector:** Attacker obtains `AES_KEY`, decrypts all `encrypted_creds` from the RDS database.

**Likelihood:** Low — same path as T3 (Secrets Manager breach required).

**Mitigation:**
- `AES_KEY` in Secrets Manager with KMS CMK encryption
- Zero-key rejected at startup in production
- Planned: KMS envelope encryption per credential (S6) — key breach exposes only the DEK, not all credentials

**Residual risk:** Medium until S6 (single key covers all credentials in v1)

---

### T5 — IDOR (Insecure Direct Object Reference)

**Vector:** Authenticated user A sends `GET /v1/webhooks/<id>` with the ID of user B's webhook.

**Likelihood:** Low — all queries are scoped by JWT user ID.

**Mitigation:**
- All data access uses `WHERE user_id = $userID`
- 404 returned (not 403) to avoid confirming resource existence
- Covered in pen test scope (T5, S5)

**Residual risk:** Low until pen test confirms coverage

---

### T6 — Broker credential theft via log leakage

**Vector:** `raw_creds` or decrypted credential material appears in CloudWatch logs.

**Likelihood:** Low — `json:"-"` tag + broker HTTP header redaction.

**Mitigation:**
- Go model `json:"-"` on `encrypted_creds` field
- `internal/integrations/brokers/.../httpclient` redacts `Authorization`, `auth-token`, `access-token`, `x-api-key`, and headers containing `token` or `secret`
- Log scrubbing audit (S2) before GA

**Residual risk:** Low — but not yet formally audited (S2 pending)

---

### T7 — SQL injection

**Vector:** Attacker injects SQL through webhook signal payload, credential label, or other user-controlled fields.

**Likelihood:** Low — all queries use parameterised SQL (`database/sql` with `$1`, `$2` placeholders).

**Mitigation:**
- No ORM; all queries are explicitly parameterised
- No dynamic SQL string construction in application code
- Covered in pen test scope (S5)

**Residual risk:** Low

---

### T8 — Signal injection / order manipulation by TradingView impersonation

**Vector:** Attacker sends a crafted `POST /v1/webhook/<token>` payload with manipulated `symbol`, `action`, or `quantity` to place unintended trades.

**Likelihood:** Medium — the ingest endpoint is intentionally public; the token is the only guard.

**Mitigation:**
- Webhook guards: `allowed_symbols`, `allowed_actions` allowlists
- Per-webhook and per-plan rate limits
- Dedup prevents replay attacks within window
- User is responsible for keeping their webhook URL secret

**Residual risk:** Medium (by design — webhook URL is a capability token; mitigation is guard + rate limit + rotation)

---

### T9 — Account takeover via credential stuffing / brute-force

**Vector:** Attacker tries known email/password combinations from breach databases against `POST /v1/auth/login`.

**Likelihood:** Medium — platform uses email/password auth; no MFA.

**Mitigation:**
- bcrypt password hashing (slow by design)
- No brute-force lockout in v1 (planned: AWS WAF rate rule on login endpoint at GA)
- Email verification required (when enabled) adds a step

**Residual risk:** Medium — no lockout or MFA in v1. AWS WAF (G2) will add rate limiting at ALB level.

---

### T10 — Infrastructure compromise via ECR image or CI pipeline

**Vector:** Malicious code injected into the Docker image or build pipeline; container extracts Secrets Manager secrets at runtime.

**Likelihood:** Low — single-developer project; no external CI currently.

**Mitigation:**
- ECR scan-on-push enabled
- CI/CD pipeline (planned: GA gate G3) will include image scanning
- Task role permissions are least-privilege; cannot access other AWS services beyond secrets and logs

**Residual risk:** Low; elevated after team grows (future: supply chain controls)

---

### T11 — Duplicate live orders on TradingView retry

**Vector:** TradingView retries the webhook on timeout; two identical orders placed within seconds.

**Likelihood:** High (without mitigation) — TradingView's default retry behaviour is documented.

**Mitigation:**
- Signal dedup: Redis SETNX on signal hash; ingest returns **HTTP 409** when duplicate
- Configurable `dedup_window_sec` (recommended: 300s for live webhooks)
- Advanced guards require paid plan for **live** webhooks; paper webhooks exempt
- Documented in getting-started and runbook

**Residual risk:** Medium for users who do not enable dedup on live webhooks

---

## 5. OWASP Top 10 Coverage (2021)

| OWASP | Category | ZettaBridge status |
|-------|----------|--------------------|
| A01 | Broken Access Control | Mitigated — IDOR by user_id scope; admin claim gate; four-plan limit enforcement |
| A02 | Cryptographic Failures | Mitigated — AES-256-GCM at rest; TLS in transit; bcrypt passwords |
| A03 | Injection | Mitigated — parameterised SQL; no dynamic SQL |
| A04 | Insecure Design | In progress — fail-close, conservative defaults, execution-only scope |
| A05 | Security Misconfiguration | Partial — AWS Secrets Manager; no hardcoded secrets; security headers pending (S1) |
| A06 | Vulnerable Components | Low risk — vendored deps; ECR scan-on-push |
| A07 | Auth and Session Failures | Mitigated — JWT HS256; refresh rotation; server-side revocation; no session fixation |
| A08 | Software Integrity Failures | Partial — vendored Go deps; ECR scan; CI pipeline pending (G3) |
| A09 | Logging and Monitoring Failures | Partial — CloudWatch logs; Prometheus + Grafana; Telegram alerts; log scrub audit pending (S2) |
| A10 | SSRF | Low exposure — broker URLs are configured constants; no user-supplied URL fetch |

---

## 6. Residual Risk Summary

| Risk | Severity | Owner | Mitigation path |
|------|----------|-------|----------------|
| Single AES_KEY covers all credentials | High | Surya | KMS envelope encryption (S6) |
| No MFA on user login | Medium | Surya | Post-GA; AWS WAF rate limit (G2) short-term |
| No brute-force lockout on login | Medium | Surya | AWS WAF (G2) |
| Webhook URL leaked by user | Medium | User | Token rotation; rate limiting |
| T11 duplicate orders (dedup not enabled) | Medium | User | Documentation + onboarding warning |
| Security headers not yet added | Medium | Surya | S1 (pre-pen-test) |
| Pen test not yet completed | High | Surya | S5 (GA gate) |
| Log scrub not yet audited | Medium | Surya | S2 (pre-pen-test) |

---

## 7. Out of Scope for Beta

- Denial-of-service attacks beyond webhook rate limiting
- Physical infrastructure attacks
- Supply chain attacks on AWS managed services (RDS, ElastiCache, ECS)
- Broker-side security (Kite Connect, SmartAPI — not ZettaBridge's trust boundary)
- Financial regulations beyond SEBI Algo-ID compliance
