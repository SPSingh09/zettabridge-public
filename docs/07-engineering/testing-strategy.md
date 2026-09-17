# Testing Strategy
**Last updated: 2026-07-07**

ZettaBridge uses a layered test pyramid. All CI-safe tests run without real broker API access.

---

## 1. Test Pyramid

| Level | Command | Requires | What it covers |
|-------|---------|----------|----------------|
| **Unit** | `make test` | Nothing | `brokercreds`, `maporder`, `brokererr`, `guard`, `plan`, `risk`, config validation |
| **Live adapter (httptest)** | `make test` | Nothing | `internal/livebrokers/*_test.go` against golden fixtures in `testdata/` |
| **Queue + factory** | `make test` | Nothing | `internal/queue`, `internal/brokerfactory`, `internal/integration` |
| **Functional (E2E)** | `make functional-test` | docker-compose | Full HTTP API with `BROKER_MODE=mock`; auth, paper accounts, credentials, webhooks, trades, P&L, WebSocket (no org tests) |
| **Functional (billing)** | `make functional-test-billing` | docker-compose + billing overlay | Signed Stripe webhooks, idempotency, admin billing override |
| **Functional (staging)** | `make functional-test-staging` | Staging URL | Same suite against deployed staging; no local Docker |
| **Live routing (E2E)** | `make functional-test-live-routing` | docker-compose | Free-tier mock routing when `BROKER_MODE=live` |
| **Live smoke (manual)** | Bruno **Live Smoke** folder | Live broker creds | Real brokers; not in CI |
| **Load test** | `make loadtest-smoke` / `loadtest-staging` | k6 + staging URL | Ingest throughput; setup upgrades user to **pro_plus** for dedicated rate-limit webhooks |
| **Staging smoke** | `make staging-smoke` | Staging URL | Ephemeral user → paper account → paper webhook → ingest → poll FILLED order |
| **Security test** | `make sectest-staging` | Staging URL | IDOR, JWT tamper, injection, brute-force probes |

---

## 2. Unit and Integration Tests

```bash
# All unit + integration tests (no Docker, no broker APIs)
make test

# Identical — CI alias
make test-unit

# Run directly
go test ./...
```

### Golden fixtures

Recorded broker response shapes for httptest servers:

```
internal/livebrokers/testdata/
  zerodha_*.json
  mt5_*.json
  angel_*.json
  dhan_*.json
  dhan_instrument.csv
```

`fixtures_test.go` loads these in httptest servers so adapter tests stay stable when broker APIs change.

### Integration path

`internal/integration/` tests the full path end-to-end with an httptest broker:

```
queue → brokerfactory (live) → live adapter → httptest broker
```

Covers happy-path (Zerodha, MT5) and auth-failure rejection (`error_code=auth_failed`).

---

## 3. Functional Tests (E2E)

Functional tests use `//go:build functional` and test the full HTTP API against a running docker-compose stack.

### Local (BROKER_MODE=mock)

```bash
# Start docker-compose stack first
docker compose up -d

# Run full functional suite
make functional-test

# Billing flow (requires billing overlay)
make functional-test-billing

# Free-tier live routing (BROKER_MODE=live, no real creds)
make functional-test-live-routing
```

The default `docker-compose.yml` sets `BROKER_MODE=mock` and `BILLING_ENABLED=false`. No Stripe keys required.

### Staging

```bash
export STAGING_BASE_URL=https://api.staging.zettabridge.net
export STAGING_ADMIN_EMAIL=...
export STAGING_ADMIN_PASSWORD=...

# Run functional suite against staging
make functional-test-staging

# Optionally include live broker routing test
STAGING_RUN_LIVE_ROUTING=1 ./functional-tester/run-functional-staging.sh
```

Uses `-count=1` so Go test cache does not skip HTTP round-trips.

### Coverage

Functional tests cover: auth (register, login, refresh, logout, email verify), **paper accounts**, credentials (live broker types, verify), webhooks (CRUD, pause, rotate-token, paper + live guards), trade audit (`comment`, `signal_key`, `error_code`), P&L aggregation, WebSocket push, metrics (`/metrics` Prometheus counters), dedup **409** responses, and billing when overlay is active. **Org lifecycle tests removed** — `/v1/orgs/*` no longer mounted.

---

## 4. Billing Tests

Billing test layers (all under `make functional-test-billing`):

| Layer | Command | What it proves |
|-------|---------|----------------|
| **Unit** | `make test` | `internal/billing/*` — plan mapping, checkout guards, webhook idempotency, metrics |
| **Functional (default)** | `make functional-test` | `TestBillingDisabled` — plans endpoint, 503 on checkout/portal/stripe-webhook |
| **Functional (billing overlay)** | `make functional-test-billing` | Signed `checkout.session.completed` / `subscription.deleted`, idempotency, admin override |
| **Manual (real Stripe)** | `FUNCTIONAL_TEST_BILLING_CHECKOUT=1` + real `sk_test_*` | Creates Checkout URL for browser payment flow |

### Billing overlay setup

```bash
docker compose -f docker-compose.yml -f docker-compose.billing.yml up -d server
export FUNCTIONAL_TEST_BILLING=1
export FUNCTIONAL_TEST_STRIPE_WEBHOOK_SECRET=whsec_functional_test
go test -tags functional -count=1 ./functional-tester -run TestBilling -v
```

### Manual Stripe E2E

1. Export `STRIPE_SECRET_KEY=sk_test_...`, `STRIPE_WEBHOOK_SECRET`, and price IDs
2. Start billing overlay stack
3. `stripe listen --forward-to localhost:8080/v1/webhooks/stripe`
4. Bruno: **Auth → login** → **Billing → POST Checkout Pro** → pay with test card `4242...` → confirm `plan: pro`, `billing_source: stripe` in admin

---

## 5. Staging Deploy and Release

```bash
# HTTP smoke against staging (paper ingest path)
make staging-smoke

# Full functional suite against staging
make functional-test-staging

# Load test (setup-loadtest-webhook.sh upgrades user to pro_plus)
make loadtest-staging
```

Preflight before any test session:

```bash
make preflight-staging          # smoke + functional prereqs
make preflight-loadtest         # verifies load-test webhook ingest (202/409)
```

---

## 6. Load Tests

Requires k6 and a running staging server. Scripts in `deploy/loadtest/k6/`.

```bash
make loadtest-smoke             # 10 VU quick check
make loadtest-staging           # 500 VU target run
```

Setup: `./deploy/loadtest/setup-loadtest-webhook.sh` creates test webhooks and upgrades the load-test user to **pro_plus** (falls back to **pro** if migration 046 not applied). Writes tokens to `deploy/loadtest/env.local`.

### Results log

| Date | Environment | Scenario | VUs | p99 (ms) | 202 rate | Pass | Commit | Notes |
|------|-------------|----------|-----|----------|----------|------|--------|-------|
| — | staging | smoke | 10 | — | — | — | — | — |
| — | staging | target | 500 | — | — | — | — | sign-off |

---

## 7. Security Tests

Scripts in `deploy/sectest/scripts/`. Config in `deploy/sectest/env.local`.

```bash
make sectest-staging
```

See `deploy/sectest/checklist.md` for the full pen test checklist (IDOR, JWT tamper, injection, webhook flood, brute-force).

---

## 8. Bruno API Collections

Bruno collections in `bruno/ZettaBridge/`.

| Goal | Folder sequence |
|------|-----------------|
| Mock Zerodha + equity | Credentials → create-zerodha → verify → Webhooks → create-indian → Signals → ingest-indian-buy |
| Mock MT5 + forex | Credentials → create-mt5 → verify-mt5 → Webhooks → create → Signals → ingest-buy |
| Live smoke | **Live Smoke** → health → verify-credential → ingest-buy → trades |
| Billing (billing overlay) | Billing → GET plans → POST Checkout → Admin → GET user |

---

## 9. Adding Tests

- **New broker API shape:** add JSON under `internal/livebrokers/testdata/`, extend `fixtures_test.go`
- **New queue reject path:** add case to `internal/queue/queue_test.go`
- **New HTTP endpoint:** add Bruno request + functional helper in `functional-tester/`
- **New billing path:** add case to `internal/billing/*_test.go` and `functional-tester/billing_test.go`
