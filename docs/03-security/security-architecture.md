# Security Architecture
**Last updated: 2026-07-07**

---

## 1. Security Principles

1. **Credentials never leave the vault.** `raw_creds` are AES-256-GCM encrypted at rest and decrypted only in queue worker goroutine memory at execution time. They are never returned by any GET endpoint, never logged, and never stored on trade rows.

2. **Fail closed.** Missing Algo-ID, suspended accounts, paused webhooks, and revoked tokens all result in explicit rejection — not silent fallback.

3. **Least privilege.** ECS task IAM role grants only `secretsmanager:GetSecretValue` + `kms:Decrypt`. No IAM user credentials in code or images.

4. **Static egress.** All broker API calls exit through a single NAT Gateway Elastic IP. Brokers can whitelist this IP; no dynamic egress.

5. **No secrets in code or images.** Dockerfiles have no `ENV` with production values. All production secrets are in AWS Secrets Manager.

---

## 2. Credential Encryption

### Scheme

- **Algorithm:** AES-256-GCM (authenticated encryption; provides both confidentiality and integrity)
- **Key size:** 32 bytes (256-bit), stored as hex in AWS Secrets Manager
- **Key source:** `AES_KEY` env var, injected from Secrets Manager at ECS task startup
- **Package:** `internal/credenc`

### Encrypt / Decrypt flow

```
PUT /v1/credentials (raw_creds in request body)
    │
    ├── credenc.Encrypt(rawCreds, aesKey)
    │       └── AES-256-GCM nonce + ciphertext → base64
    │
    └── INSERT broker_credentials (encrypted_creds = base64 ciphertext)

Queue Worker (PlaceOrder)
    │
    ├── SELECT broker_credentials (encrypted_creds)
    ├── credenc.Decrypt(encryptedCreds, aesKey)
    │       └── base64 decode → AES-256-GCM decrypt → raw bytes in memory
    ├── broker.PlaceOrder(ctx, order, decryptedCred)
    └── [decryptedCred goes out of scope; GC'd]
```

### Key guards

- Server startup with `APP_ENV=production` and `AES_KEY = 000...0` (all-zero dev default) is rejected by `config.Validate()`. Process exits before accepting requests.
- Key is in AWS Secrets Manager, encrypted with KMS CMK (`alias/zettabridge-staging`).

### Known limitation

v1 uses a single global `AES_KEY` for all credentials. Rotating the key requires all users to re-submit their credentials (no in-app re-encryption). KMS envelope encryption per credential (planned as GA gate S6) will fix this.

---

## 3. Authentication and Session Management

### JWT design

| Property | Value |
|----------|-------|
| Algorithm | HS256 |
| Secret source | `JWT_SECRET` from Secrets Manager |
| Access token TTL | 5 hours |
| Claims | `sub` (user ID), `role` (user/admin), `exp`, `iat` |
| Storage (dashboard) | `localStorage` (key: `zb_token`) |

### Refresh tokens

- One refresh token per session, stored hashed (SHA-256) in `refresh_tokens` table.
- Refresh endpoint issues a new access token; refresh token is single-use (rotated on each refresh).
- Logout: access token added to Redis revocation set with TTL = remaining expiry; refresh token `revoked_at` set in DB.

### Revocation

On every authenticated request, `CheckRevoked` middleware checks Redis for the raw JWT. If found → 401 immediately. This covers the gap between logout and natural token expiry.

### Admin claim

- `role = 'admin'` claim in JWT is required for all `/v1/admin/*` routes.
- The `RequirePlatformAdmin()` Fiber middleware enforces this.
- Admin claim is set at JWT issuance time, based on `users.role` in the DB.
- `users.role` is set only by the `BOOTSTRAP_ADMIN_EMAIL` startup promotion or by an existing admin — never self-promoted via API.

### WebSocket authentication

Browser WebSocket cannot send custom headers during the upgrade handshake. Authentication uses a JWT query parameter: `GET /v1/ws/trades?token=<jwt>`. The handler validates the JWT before completing the upgrade.

---

## 4. Transport Security

| Layer | Control |
|-------|---------|
| ALB → Internet | HTTPS only; HTTP redirected to HTTPS by ALB listener rule |
| TLS certificates | AWS ACM (managed issuance and renewal) |
| TLS version | TLS 1.2+ (ACM + ALB default policy) |
| DNS | Cloudflare (DNS-only, no proxy) |
| ECS to RDS | PostgreSQL over private subnet (no TLS required on private network; can be enabled) |
| ECS to Redis | ElastiCache on private subnet |
| ECS to Broker APIs | HTTPS (broker requires TLS) via NAT Gateway |

---

## 5. Secrets Management

All production secrets are stored in AWS Secrets Manager:

| Secret | Type | Used by |
|--------|------|---------|
| `JWT_SECRET` | String (min 32 chars) | JWT signing / verification |
| `AES_KEY` | 32-byte hex string | Credential encryption/decryption |
| `DATABASE_URL` | PostgreSQL DSN | `store.NewPostgres()` |
| `REDIS_URL` | Redis URL | `store.NewRedis()` |
| `STRIPE_SECRET_KEY` | Stripe restricted key | Billing endpoints (disabled) |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook secret | Stripe event verification |
| `EMAIL_*` | SMTP credentials | Email verification + invites |

Secrets Manager secrets are encrypted with KMS CMK (`alias/zettabridge-staging`). ECS task role (`ECSTaskRole-ZettaBridge`) has:
- `secretsmanager:GetSecretValue` on the specific secret ARNs
- `kms:Decrypt` on the CMK
- `logs:CreateLogStream`, `logs:PutLogEvents` for CloudWatch

No other AWS permissions.

---

## 6. Network Security

### VPC Architecture

```
VPC 10.0.0.0/16
├── Public subnets (2 AZs)
│   ├── ALB — internet-facing; only entry point for HTTPS
│   └── NAT Gateway + Elastic IP — sole outbound path for broker calls
│
└── Private subnets (2 AZs)
    ├── ECS Fargate tasks — no public IP; inbound only via ALB target group
    ├── RDS PostgreSQL — private; no public accessibility
    └── ElastiCache Redis — private; no public accessibility
```

ECS tasks have no public IPs. The only inbound path is the ALB. The only outbound path for broker calls is the NAT Gateway Elastic IP (static, whitelistable).

### Security Groups

| SG | Inbound | Outbound |
|----|---------|----------|
| ALB | 0.0.0.0/0 port 80 + 443 | ECS backend:8080, ECS dashboard:3000 |
| ECS backend | ALB SG port 8080 | RDS SG:5432, Redis SG:6379, 0.0.0.0/0:443 (broker APIs) |
| ECS dashboard | ALB SG port 3000 | ECS backend SG:8080 |
| RDS | ECS backend SG:5432 | None |
| Redis | ECS backend SG:6379 | None |

### Broker IP Whitelisting

Indian brokers (Angel One, Dhan, and optionally Zerodha) require their registered IP whitelist to include the NAT Gateway Elastic IP. This IP is static and owned by the AWS account — it does not change unless the NAT Gateway is recreated.

The static egress also means there is no way for a compromised broker token to be used from an attacker's IP (brokers reject unknown IPs).

---

## 7. Application-Layer Security

### Webhook URL (Bearer Capability Token)

- The ingest URL contains a UUID v4 token: `POST /v1/webhook/<token>`
- This token is the only auth on the ingest endpoint — no JWT required
- It functions as a bearer capability: anyone with the URL can submit signals
- Users must treat it as a secret (documented in broker-guide)
- Rotation via `POST /v1/webhooks/:id/rotate-token` is instant; old token rejected on next request

### Guard Validation

Before a signal is enqueued, the `guard` package checks:
1. Webhook is active (not paused)
2. User/org is not suspended (compliance gate — user-level only; org product removed)
3. Signal action is in `allowed_actions`
4. Signal symbol is in `allowed_symbols` (if non-empty)
5. Rate limit: tokens-per-second via Redis
6. Dedup: Redis SETNX on signal hash within `dedup_window_sec` window — duplicate ingest returns **HTTP 409**

### Log Hygiene

The following must **never** appear in logs:
- `raw_creds` or decrypted credential material
- `JWT_SECRET` or `AES_KEY`
- Broker HTTP `Authorization`, `auth-token`, `access-token`, or `x-api-key` headers

The `internal/livebrokers/httpclient` package redacts headers automatically. Header names containing `token` or `secret` are also redacted as a catch-all.

Fiber access logs emit: method, path, status, latency — no request bodies.

### IDOR Prevention

All data access is scoped by the authenticated JWT user ID:
- `SELECT ... WHERE user_id = $1` on webhooks, credentials, and paper accounts
- Admin routes use a separate admin JWT claim — not just a plan tier

### Rate Limiting

Three levels of rate limiting:
1. **Per-webhook** (`rate_limit_per_sec` field) — Redis token bucket in guard check
2. **Per-plan** — order throughput cap: free 1/s, paper 5/s, pro/pro_plus 10/s; free plan also has 100 ingests/day
3. **Broker** — per live credential cap 10 orders/sec; broker APIs have their own rate limits

---

## 8. Planned Security Hardening (GA gates)

| Item | Description | Gate |
|------|-------------|------|
| HTTP security headers | HSTS, X-Content-Type-Options, X-Frame-Options, Content-Security-Policy via Fiber middleware | S1 |
| Log scrubbing audit | Confirm no credential material in CloudWatch logs | S2 |
| Credential security review | Review credenc package, ensure no plaintext paths | S3 |
| Load test | Confirm rate limits hold under 500 VUs | S4 |
| Penetration test | IDOR, JWT tamper, injection, webhook flood, brute force | S5 |
| KMS envelope encryption | Per-credential DEK wrapped by KMS; eliminates single-key blast radius | S6 |
| AWS WAF | Managed rule sets (SQLi, XSS, common exploits) + rate rules on ALB | G2 |
| CI/CD pipeline | Remove manual deploy path; scan images in CI before ECS push | G3 |

---

## 9. Incident Procedures

See [runbook.md](../05-operations/runbook.md) for detailed playbooks covering:
- Duplicate order detection and webhook pause
- Credential compromise (broker token leaked, AES_KEY rotation)
- JWT_SECRET rotation
- User suspension
- Log hygiene verification
- NAT egress verification after infra changes
