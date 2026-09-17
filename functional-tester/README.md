# Functional Tester

Live HTTP functional suite for ZettaBridge. Uses unique emails/org names per run.

## Run

```bash
bash functional-tester/run-functional-tests.sh
```

Or manually (use `-count=1` so Go does not replay a cached PASS without HTTP):

```bash
FUNCTIONAL_TEST_BASE_URL=http://localhost:8080 \
FUNCTIONAL_TEST_ADMIN_EMAIL=admin@example.com \
FUNCTIONAL_TEST_ADMIN_PASSWORD=admin123 \
go test -tags functional -count=1 ./functional-tester -v
```

The helper script applies migrations (001–014), bootstraps the admin user, rebuilds the Docker server, and runs tests.

### Billing (PR 4B.1)

Default `make functional-test` includes `TestBillingDisabled` (no Stripe keys).

Signed webhook tests (Phase C/D without Stripe CLI):

```bash
make functional-test-billing
```

Real Stripe Checkout URL (optional, manual browser payment):

```bash
export STRIPE_SECRET_KEY=sk_test_...
export STRIPE_PRICE_PRO=price_...
# ... other STRIPE_* vars — see docker-compose.billing.yml
export FUNCTIONAL_TEST_BILLING=1
export FUNCTIONAL_TEST_BILLING_CHECKOUT=1
bash functional-tester/run-functional-billing-tests.sh
```

See [docs/testing.md](../docs/testing.md#stripe-billing-pr-4b1).

## Staging (Tier 2)

Against Lightsail or any deployed staging URL — **no Docker on your laptop**:

```bash
export STAGING_BASE_URL=http://52.66.136.127
export STAGING_ADMIN_EMAIL=admin@zettaflux.com
export STAGING_ADMIN_PASSWORD='your-admin-password'
bash functional-tester/run-functional-staging.sh
```

Or: `make functional-test-staging`

**Note:** The staging runner passes `-count=1` to disable Go test caching. Without it, a second run can finish in ~1s with `(cached)` and **no webhook traffic** on the VM.

| Variable | Purpose |
|----------|---------|
| `STAGING_BASE_URL` | Staging API base (`http://IP` or `https://domain`) |
| `STAGING_ADMIN_EMAIL` / `STAGING_ADMIN_PASSWORD` | Existing admin (required) |
| `STAGING_METRICS_TOKEN` | Optional — when staging sets `METRICS_TOKEN`, pass the same value to assert `/metrics` |
| `STAGING_ENABLED_ADAPTERS` | Optional — defaults to `paper,zerodha` (matches ECS staging) |
| `STAGING_RUN_LIVE_ROUTING=1` | After full suite, run `TestFreeUserMockRoutingWhenBrokerLive` |
| `STAGING_LIVE_ROUTING_ONLY=1` | Live routing test only |
| `STAGING_FUNCTIONAL_TIMEOUT` | Go test timeout (default `15m`) |

**Prerequisites:** admin registered and promoted once via `BOOTSTRAP_ADMIN_EMAIL`. Staging should have `EMAIL_VERIFICATION_REQUIRED=false` (default in `runtime.env.example`).

**Caveats (`FUNCTIONAL_TEST_STAGING=1`, set automatically by the runner):**

- Keeps test users on **free billing** during personal-resource tests so `BROKER_MODE=live` routes demo orders to mock.
- Temporarily bumps to **pro_plus** for multi-broker credential setup and dedup guard tests, then restores **free** before webhook ingests.
- Uses **zerodha** (not dhan/angel/mt5) when `FUNCTIONAL_TEST_ENABLED_ADAPTERS` is `paper,zerodha` (default for staging and docker-compose).
- Upgrades to **enterprise billing** before org lifecycle tests (org create requires billing enterprise).
- Skips **Indian broker submitted trade** assert on staging (live routing requires `algo_id` + real creds).
- Skips live broker **verify** probes when fake creds would fail against real APIs.
- Skips **webhook trade audit**, **PnL**, and **WebSocket push** asserts for personal MT5 and org webhooks on staging (demo/live routing without real creds).
- Skips **dedup trade audit** after ingest queue checks on personal MT5 webhook.
- Temporarily bumps to **pro** only when reading trade audit logs for solo free users.
- **1.1s pause** between webhook ingests to respect free-tier rate limits (1 req/s).

**Note:** Each run registers many unique users/orgs — use disposable staging DB only.

## Live routing test (optional)

When the server runs with `BROKER_MODE=live`, free users without active early bird must still route demo orders to mock adapters:

```bash
make functional-test-live-routing
```

This runs only `TestFreeUserMockRoutingWhenBrokerLive` using `docker-compose.live.yml` — no real broker credentials required.

## API coverage (47 endpoints)

| Method | Path | Covered in |
|--------|------|------------|
| GET | `/healthz` | health |
| GET | `/v1/ws/trades` | assertWebSocketTradePush (local mock only) |
| POST | `/v1/auth/register` | registerOwner, registerTempUser |
| POST | `/v1/auth/login` | loginOwner, loginAdmin |
| POST | `/v1/auth/logout` | assertSessionLogout |
| GET | `/v1/me` | assertMe, assertAdminMe, assertMemberCanSeeOrg |
| POST | `/v1/webhook/:token` | ingest BUY/SELL/CLOSE |
| GET | `/v1/credentials` | listCredentialsHasAtLeast |
| POST | `/v1/credentials` | createPersonalCredential, createOrgCredential |
| PUT | `/v1/credentials/:id` | updatePersonalCredential, updateOrgCredential |
| DELETE | `/v1/credentials/:id` | deleteCredential |
| GET | `/v1/webhooks` | listWebhooksHasAtLeast |
| POST | `/v1/webhooks` | createPersonalWebhook, createOrgWebhook, member webhook |
| PUT | `/v1/webhooks/:id/pause` | pauseWebhook |
| PUT | `/v1/webhooks/:id/resume` | resumeWebhook |
| DELETE | `/v1/webhooks/:id` | deleteWebhookByID |
| GET | `/v1/webhooks/:id/trades` | ingestSignalAndAssertTrade |
| GET | `/v1/webhooks/:id/pnl` | assertWebhookPnL; free-tier 403 on staging |
| GET | `/v1/admin/users` | assertAdminUsersListPlain, assertAdminUserListing |
| GET | `/v1/admin/users/:id` | assertAdminUserDetailAndPatch (temp + owner) |
| PATCH | `/v1/admin/users/:id` | suspend/unsuspend temp, enable/disable orgs |
| PUT | `/v1/admin/users/:id/plan` | assertAdminUserDetailAndPatch |
| GET | `/v1/admin/orgs` | assertAdminOrgListingAndPatch |
| GET | `/v1/admin/orgs/:id` | assertAdminOrgListingAndPatch |
| PATCH | `/v1/admin/orgs/:id` | seat_limit, suspend/unsuspend |
| GET | `/v1/admin/orgs/:id/members` | assertAdminOrgListingAndPatch |
| POST | `/v1/orgs` | createOrg |
| GET | `/v1/orgs` | assertOrgViews, assertMemberCanSeeOrg |
| GET | `/v1/orgs/:id` | assertOrgViews, assertMemberIsOwner |
| PATCH | `/v1/orgs/:id` | assertOrgViews |
| DELETE | `/v1/orgs/:id` | deleteOrgAsCurrentOwner |
| GET | `/v1/orgs/:id/members` | listOrgMembersHasAtLeast |
| GET | `/v1/orgs/:id/members/:userId` | getOrgMember |
| PATCH | `/v1/orgs/:id/members/:userId` | patchOrgMemberRole/Status |
| DELETE | `/v1/orgs/:id/members/:userId` | removeOrgMember |
| POST | `/v1/orgs/:id/invites` | createInvite |
| GET | `/v1/orgs/:id/invites` | listOrgInvitesHasAtLeast |
| DELETE | `/v1/orgs/:id/invites/:inviteId` | revokeInvite |
| GET | `/v1/invites/:token` | previewInvite |
| POST | `/v1/invites/accept` | acceptInvite |
| GET | `/v1/orgs/:id/webhooks` | assertOrgScopedListEndpoints |
| GET | `/v1/orgs/:id/trades` | assertOrgScopedListEndpoints, ingest |
| GET | `/v1/orgs/:id/pnl` | assertOrgScopedListEndpoints, assertOrgPnL |
| POST | `/v1/orgs/:id/transfer-ownership` | transferOwnership |
| POST | `/v1/orgs/:id/leave` | leaveOrg |
| GET | `/v1/orgs/:id/usage` | assertOrgUsage |
| GET | `/v1/orgs/:id/activity` | assertOrgActivity |

## Notes

- Point at a disposable local stack (Postgres + Redis + server).
- `BOOTSTRAP_ADMIN_EMAIL` must match `FUNCTIONAL_TEST_ADMIN_EMAIL` unless the run script seeds admin directly.
- Set `FUNCTIONAL_TEST_SKIP_SERVER_REBUILD=1` to skip Docker rebuild before tests.
