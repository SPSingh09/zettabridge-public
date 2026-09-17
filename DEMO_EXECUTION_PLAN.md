# ZettaBridge Paper Trading Engine — Implementation Plan

Broker-free demo/paper trading mode, starting with Indian equities, later
extensible to crypto and forex. This plan is a living document — phases and
scope may be tweaked as work progresses.

## Target Product Concept

Demo mode becomes a first-class product: **ZettaBridge Paper Trading Engine**,
not "Zerodha Demo" / "Angel Demo" / "Dhan Demo" / "MT5 Demo".

Flow:

```
Create paper trading account
  -> Create webhook
  -> Send TradingView / bot / Python signal
  -> ZettaBridge validates signal
  -> Paper engine simulates order
  -> Position opens/closes internally
  -> Dashboard shows trades, positions, P&L, win rate, notional, fill rate
  -> User upgrades to live broker later
```

Final architecture:

```
Webhook Receiver
      |
Signal Validator
      |
Risk Guards
      |
Execution Router
      |
---------------------------------
| Paper Trading Engine          |
| Zerodha Publisher Engine      |
| Zerodha OAuth Engine          |
| Angel Engine                  |
| Dhan Engine                   |
| MT5 Engine                    |
---------------------------------
      |
Orders / Positions / Trades / P&L
      |
Dashboard
```

For demo mode, the route is always `Webhook -> Validator -> Guards -> Paper
Trading Engine -> Paper Account`. Never `Webhook -> Zerodha / Angel / Dhan /
MT5`.

---

## Phase 0 — Product Cleanup Before Coding

**Goal:** separate demo execution from broker execution conceptually and in
the UI, before any schema/engine work starts.

- Replace the current "Broker: Zerodha, Mode: Demo" framing with:
  - **Execution destination**: `Paper Trading` | `Live Broker`
  - Paper Trading -> `Indian Equity` (now), `Indian F&O`, `Forex`, `Crypto` (later)
  - Live Broker -> `Zerodha`, `Angel`, `Dhan`, `MT5`
- New "Add Paper Trading Account" flow (replaces demo credential form) with fields:
  - Label, Market profile, Starting balance, Base currency, Exchange, Default product, Allowed order types
  - Indian MVP defaults: `My Indian paper account`, Indian Equity, ₹100,000, INR, NSE, MIS, MARKET/LIMIT
- Hide broker-specific fields entirely in paper mode: Broker, Zerodha, Publisher, OAuth, API key/secret, SEBI Algo ID, Redirect URL.
- UI copy: "Paper Trading mode is simulated inside ZettaBridge. It does not connect to Zerodha, Angel, Dhan, or MT5 and does not place real orders."

**Status: done.** Implemented as a dedicated `/paper-accounts` section in
`dashboard/` (separate from `/credentials`, not a merged "Execution
destination" picker) — `POST/GET/DELETE /v1/paper-accounts` API,
`PaperAccountForm.tsx`, `paper-accounts/page.tsx` + `paper-accounts/new/page.tsx`.
New broker credential creation (`CredentialForm.tsx`, create mode only) no
longer offers "Demo" — only "Live"; free-plan/no-live-access users are
directed to create a Paper Trading Account instead. Existing demo broker
credentials are untouched (edit mode unchanged). "Allowed order types" moved
to the Phase 3 webhook-level guards rather than being a per-account field —
`Exchange`/`Default product` are per-account (persisted), `Allowed order
types`/actions/symbols will be per-webhook.

---

## Phase 1 — Database & Domain Model

**Goal:** first-class paper trading accounts, orders, positions, trades.

Tables:

- `paper_accounts` — id, user_id, label, market_profile, base_currency, starting_balance, cash_balance, status, timestamps. `market_profile` starts as `indian_equity`, later `indian_fno`, `forex`, `crypto`.
- `paper_orders` — id, user_id, paper_account_id, webhook_id, signal_id, symbol, exchange, side, order_type, product, quantity, requested_price, fill_price, status (`RECEIVED`/`VALIDATED`/`FILLED`/`REJECTED`/`CANCELLED`), reason, raw_signal (JSONB), timestamps. MVP: valid orders go straight to `FILLED`.
- `paper_positions` — id, user_id, paper_account_id, symbol, exchange, product, quantity (signed: + long / - short / 0 closed), avg_entry_price, last_price, unrealized_pnl, timestamps. Unique on (paper_account_id, exchange, symbol, product).
- `paper_trades` — id, user_id, paper_account_id, order_id, symbol, exchange, side, quantity, price, realized_pnl, fees, created_at. This is the executed trade history.
- `paper_account_snapshots` (optional, can slip to Phase 3) — id, user_id, paper_account_id, equity, cash_balance, unrealized_pnl, realized_pnl, created_at.

---

## Phase 2 — Paper Trading Engine MVP

**Goal:** a webhook signal creates a simulated order and updates position/P&L.

- Common execution interface:
  ```go
  type ExecutionDestination interface {
      Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error)
  }
  ```
  Implementations: `PaperExecutionEngine`, `ZerodhaPublisherEngine`, `ZerodhaOAuthEngine`, `AngelEngine`, `DhanEngine`, `MT5Engine`. Demo mode always routes to `PaperExecutionEngine` — never through broker packages.
- `ExecutionRequest` / `ExecutionResult` structs carry account/webhook/signal IDs, market profile, exchange, symbol, side (BUY/SELL/CLOSE), order type (MARKET/LIMIT), product, quantity, price, raw signal.
- **MVP fill rules:**
  - MARKET fills at `signal.price`; reject with "price is required for paper MARKET order until live data feed is enabled" if missing.
  - LIMIT: for v1, fill immediately at the provided price (simpler rule than real limit matching — add proper BUY `<=`/SELL `>=` matching later).
- **Position update rules:**
  - BUY: adds to long, or reduces/flips a short position (remaining qty opens long if buy_qty > abs(short_qty)).
  - SELL: adds to short, or reduces/flips a long position (remaining qty opens short if sell_qty > long_qty).
  - CLOSE: SELL full qty if long, BUY full qty if short, reject "No open position to close" if flat.

---

## Phase 3 — Webhook Integration

**Goal:** user can select a paper account as a webhook's execution destination.

- Webhook create/edit page: rename "Broker credential" field to "Execution destination", with paper accounts listed alongside live broker credentials (clearly labeled `— Live`).
- Copy for paper destinations: "Paper trading destination. Signals are simulated inside ZettaBridge and do not place real orders."
- Guards for paper mode: allowed actions (BUY/SELL/CLOSE), allowed order types (MARKET/LIMIT), allowed symbols (seed list e.g. RELIANCE, INFY, TCS, SBIN). Secret passphrase and deduplication still apply. Risk guards still apply: max quantity, max notional, max orders/minute, allowed symbols, allowed actions.

**Status: done.** `webhooks.broker_cred_id` is now nullable, `webhooks.paper_account_id` added (exactly one required, enforced by a CHECK constraint — migration `036_webhook_paper_destination.sql`). The queue worker (`internal/platform/queue/queue.go`/`paper.go`) branches to `paperengine.Engine.Execute` for paper webhooks; the entire existing guard/rate-limit/dedup pipeline (`internal/guard`) applies unchanged to both destinations since it was already broker-agnostic. `WebhookForm.tsx` now has a single "Execution destination" select combining paper accounts and live credentials, with the required paper-mode copy.

Paper-destination webhooks are capped at a flat 1 per user (`plan.MaxPaperWebhooksPerUser`), completely independent of the live-trading `MaxWebhooks` tier — confirmed and fixed two related bugs along the way: (1) the live `MaxWebhooks` count was including paper webhooks (now `CountLiveWebhooksByUser`/`CountLiveWebhooksByOrg` exclude them), and (2) a live-trading plan downgrade was pausing *all* solo webhooks including paper ones (`PauseAllWebhooksByUser` now excludes paper webhooks — paper trading is unaffected by live-trading plan changes).

**Correction (2026-07-03):** Three separate places were still gating paper webhooks behind the live-trading Individual+ plan, inconsistent with Paper Trading being a flat, plan-independent tier:
1. `EnforceAdvancedGuardsPlan` (`internal/modules/webhooks/handler.go`) gated `allowed_symbols`/`dedup_window_sec`/rate limits/trading hours. Now takes an `isPaperDestination` argument and exempts paper webhooks entirely; `WebhookForm.tsx`'s `disabled`/"Individual" badge on these fields is now gated by `isPaid || isPaperDestination`.
2. `ListTrades` capped free-plan trade history at 20 rows regardless of destination. Paper webhooks now always get the full 100-row window.
3. `ExportTrades` (`internal/modules/pnl/handler.go`) gated CSV/JSON export behind `CanViewAuditLogs` regardless of destination. Now skipped for paper webhooks.

All three verified against the real running docker stack: a free-plan user can now set `allowed_symbols`/`dedup_window_sec`, get full trade history, and export trades on a paper webhook (previously 403/truncated), while the same operations on a live-broker webhook are still correctly blocked/limited for free-plan users.

**Bug found and fixed while building the Bruno test collection (2026-07-03):** deleting a paper account that still had a webhook attached returned a bare `500` — the `paper_account_id` FK's `ON DELETE SET NULL` left the webhook with neither `broker_cred_id` nor `paper_account_id` set, tripping the `webhooks_one_destination_check` CHECK constraint. Fixed by adding `CountWebhooksByPaperAccount` and checking it in `DeletePaperAccount` before attempting the delete, returning a clean `409 "paper account is in use by one or more webhooks"` instead — mirrors `DeleteCredential`'s existing identical check for broker credentials. Verified against the real server: 409 while a webhook is attached, 204 once the webhook is removed first.

**Bruno collection:** added a full `Paper Trading` folder (create/list/get/delete account, create webhook, update guards as a free-plan user, ingest BUY/SELL signals, positions/orders/pnl) plus `Failure Modes/Paper Trading` (account limit, delete-while-in-use, both/neither webhook destination, missing-price async rejection). Every request was run against the real server via the Bruno CLI (`npx @usebruno/cli run`), not just written and assumed correct — full happy-path flow is 13/13 requests and 21/21 tests passing, failure-mode folder is 8/8 and 16/16.

---

## Phase 4 — Paper Trading Dashboard

**Goal:** make paper trading feel useful and valuable (unlike live Publisher mode, full metrics are available here).

- Paper account detail page: starting balance, cash balance, equity, realized P&L, unrealized P&L, total P&L, open positions, paper trades, win rate, fill rate, notional.
- Cards: Paper Equity, Total P&L, Realized P&L, Unrealized P&L, Win Rate, Fill Rate, Total Trades, Notional.
- Positions table: symbol, qty, avg price, LTP, unrealized P&L, product.
- Trades table: time, side, symbol, qty, fill price, status, realized P&L.
- Disclaimer: "Paper trading results are simulated and may differ from live execution due to slippage, liquidity, latency, rejection, and broker/exchange rules."

**Status: done.** New endpoints `GET /v1/paper-accounts/:id`, `/positions`,
`/orders`, `/pnl` (`internal/store/paper_pnl.go`, `internal/modules/paperaccounts/handler.go`).
Metrics reuse the live-trading `internal/store/pnl.go` conventions
(`fill_rate = filled/(filled+rejected)`, same `COALESCE`/`FILTER` SQL shape)
with one deliberate improvement: win rate uses paper trades' exact
`realized_pnl` field directly (`realized_pnl > 0`/`< 0`) rather than live
trading's FIFO fill-price approximation, since paper trades don't have the
missing-field limitation that approximation exists to work around. Equity is
computed on the fly as `StartingBalance + RealizedPnL + UnrealizedPnL` (no
snapshot job exists yet — that's Phase 6). The "Trades table" is sourced from
`paper_orders` left-joined to `paper_trades` (not `paper_trades` alone) so
rejected signals are visible too, satisfying the spec's "status" column,
which a fills-only table can't provide. `StatCard`/`ActionBadge`/`StatusBadge`
extracted into shared components (`dashboard/components/`) and reused by both
the webhook and paper account detail pages.

---

## Phase 5 — Price Feed v1: Signal-Based Pricing

**Goal:** launch without any external market data dependency.

- Rule: every signal must include `price` (TradingView users send `{{close}}`).
- Advantages: no data provider needed, no licensing blocker, immediate MVP, easy to explain.
- Limitation: P&L only updates when a new signal/price arrives. Copy: "In Paper Trading v1, prices are updated from incoming webhook signals. Real-time mark-to-market will be added later."

**Status: done.** The actual rule was already enforced since Phase 2 —
`paperengine/fill.go`'s `validateOrder` rejects any MARKET or LIMIT order
(including CLOSE, since it still carries an order type) with
`price <= 0` ("price is required for paper MARKET/LIMIT order..."). This
phase's real work was making that behavior visible to users: the paper
destination's info box in `WebhookForm.tsx` now explains the `price` field
requirement and the TradingView `{{close}}` convention, and the paper account
detail page's Positions card carries the required "prices are updated from
incoming webhook signals" copy. No functional/backend change.

---

## Phase 6 — Price Feed v2: LTP Provider Interface

**Goal:** add external price feeds without vendor lock-in.

- Interface:
  ```go
  type MarketDataProvider interface {
      GetLTP(ctx context.Context, symbol MarketSymbol) (*Quote, error)
      GetBatchLTP(ctx context.Context, symbols []MarketSymbol) ([]Quote, error)
  }
  ```
- Providers (implement `SignalPriceProvider` first; add Dhan/FYERS/TrueData/GlobalDatafeeds/Crypto/Forex providers later).
- Background job (every 30–60s on free plan): find open paper positions, fetch latest LTP, update unrealized P&L and account equity, write account snapshot.

**Status: done (interface + background job; real feed still a stub).**
`internal/marketdata` — `Provider` interface (`GetLTP`/`GetBatchLTP`, exactly as spec'd above, keyed by `Symbol{Exchange, Symbol}`), `SignalPriceProvider` (default — a deliberate permanent no-op returning `ErrNoQuote`, since paper positions still only reprice from incoming ORDER_SIGNAL/PRICE_UPDATE signals per Phase 5's rule), and `SnapshotJob` (ticker-based background sweep, `MARKET_DATA_SNAPSHOT_INTERVAL_SEC` — default 45s — over every account with an open position; refreshes `last_price`/`unrealized_pnl` from whatever the configured provider returns, then writes one `paper_account_snapshots` row per account via `GetPaperAccountPnL`). New read endpoint `GET /v1/paper-accounts/:id/snapshots`.

Vendor decision: target real feed is **FYERS**, but no API credentials exist yet — `internal/marketdata/fyers.Provider` is a structural stub (returns `ErrNoQuote` until `FYERS_API_KEY`/`FYERS_ACCESS_TOKEN` are set and `MARKET_DATA_PROVIDER=fyers`). The background job's logic doesn't change when the real FYERS HTTP calls are filled in later — it already treats "provider has nothing new to report" as the normal case, not a failure, so `SignalPriceProvider` and an unconfigured `fyers.Provider` behave identically today.

Deliberately did **not** fabricate simulated/random price movement as a placeholder — that would misrepresent real market conditions in a product whose whole pitch is realistic paper trading. An unconfigured provider means "no new price data," not "invent one."

Verified live: BUY 10@821.5 → snapshot job (5s test interval) wrote 3 identical snapshots (equity 100000, cash_balance 91785, unrealized_pnl 0) while flat-priced; a `PRICE_UPDATE` to 830 was picked up by the very next sweep (unrealized_pnl 85, equity 100085) — confirming the same background job transparently reflects both signal-driven and (once wired) provider-driven price changes. A permanent unit test suite (`internal/marketdata/snapshotjob_test.go`) covers: accounts with no open positions are skipped entirely, `SignalPriceProvider` still writes a snapshot from current state without updating any position, and a real quote both updates the position and the resulting snapshot.

Deferred (superseded below): real FYERS HTTP integration, dashboard equity chart UI (still not built — out of scope for this pass).

**Admin-configurable provider toggle (2026-07-04) — added on request, no restart needed.**
The provider (`signal` | `fyers`) is now switchable at runtime from the admin dashboard, not just via the `MARKET_DATA_PROVIDER` env var at deploy time. New generic `platform_settings` key-value table (migration `037`, reusable for future global admin settings beyond this one) + `store.GetPlatformSetting`/`SetPlatformSetting`. `marketdata.Registry` holds every compiled-in provider by name; `SnapshotJob.activeProvider` re-reads the `market_data_provider` platform_settings row on **every sweep** (falling back to the env default if unset/unreadable/unrecognized) — so an admin's change takes effect on the next tick, no server restart. Admin API: `GET`/`PUT /v1/admin/settings/market-data-provider` (validates against `marketdata.KnownProviders`, 400 on garbage input, audit-logged via `LogUserAudit`). Dashboard: `admin/settings` page (radio select + save, warns if `fyers` is selected without credentials configured). Verified live: GET → PUT `fyers` → GET reflects it immediately; PUT with an invalid provider name correctly 400s. A dedicated permanent test (`TestSweep_RuntimeProviderSwitch`) proves the same running `SnapshotJob` instance switches providers between two `Sweep()` calls when the underlying setting changes — no re-construction needed, matching production reality where the job is built once at startup and never restarted for this.

**Real FYERS OAuth + quotes integration (2026-07-04) — added on request, after user created a FYERS app and got App ID/Secret ID.**
User chose the full OAuth flow (not manual daily token pasting) but scoped it explicitly: market-data-only (never order placement), and admin-only via the admin dashboard (not per-user like Zerodha's broker OAuth) — this is a single, platform-wide connection.

Before writing any code, verified the actual FYERS API v3 wire format by downloading FYERS's own official `fyers-apiv3` Python SDK (`pip download`) and reading `fyersModel.py` directly — not guessing from third-party blog posts. Confirmed from source: login URL (`generate-authcode`), token exchange (`validate-authcode`, `appIdHash = sha256(appID:secretID)`), and the quotes endpoint (`/data/quotes`, `Authorization: appID:accessToken` header, response `{"s":"ok","d":[{"n":"NSE:SBIN-EQ","v":{"lp":825.5}}]}`) — all ground-truth verified, not assumed. **One real gap found**: FYERS's official SDK has *no* refresh-token support at all — silent daily refresh only exists as an undocumented, community-reported `validate-refresh-token` endpoint needing a PIN. Flagged this explicitly to the user rather than silently coding against an unverified endpoint; user chose to attempt it anyway *in addition to* a reconnect-notification fallback (mirroring the existing `zerodha/refresh` pattern in this codebase for the same "token expires daily, no supported refresh" situation).

Built: `internal/integrations/brokers/fyers/auth` (HMAC-signed OAuth state carrying the admin's userID, `LoginURL`, `ExchangeAuthCode`, best-effort `RefreshAccessToken`), `internal/marketdata/fyers.Provider` (now a real quotes client with an in-memory token — no DB round-trip per quote fetch), `internal/integrations/brokers/fyers/refresh.Job` (ticker job: loads the connection, refreshes ~2h before expiry, falls back to emailing/Telegramming the connecting admin on failure — mirrors `zerodharefresh`'s notify pattern), new `fyers_connection` table (migration `038`, single-row, tokens/PIN encrypted via the existing `credenc` AES-256-GCM mechanism, same as broker credentials), admin API (`GET/PUT /v1/admin/fyers/status|connect|pin`, plus a public `/v1/admin/fyers/callback` for FYERS's own redirect), and a `admin/settings` dashboard section (status badge, Connect button, PIN field).

Config reworked: `FYERS_API_KEY`/`FYERS_ACCESS_TOKEN` (Phase 6's static-stub design) replaced by `FYERS_APP_ID`/`FYERS_SECRET_ID`/`FYERS_CALLBACK_URL` (deploy-time app registration) + `FYERS_REFRESH_INTERVAL_MIN` (default 60) — the actual access/refresh tokens are no longer env vars at all, only DB-stored (encrypted) and admin-connected.

Verified live (without real FYERS credentials, which the user correctly did not paste into chat): admin login → FYERS status (`app_configured`/`connected`/`pin_set` all false) → connect returns 400 (not configured) → set PIN → status shows `pin_set: true`, encrypted in DB (`v1:...` prefix, not plaintext) → temporarily set fake `FYERS_APP_ID`/`FYERS_SECRET_ID`/`FYERS_CALLBACK_URL` → connect now returns a correctly-formed FYERS login URL with all 4 expected query params → callback with missing params redirects to `/admin/settings?fyers=error&reason=...` correctly → reverted the fake env vars afterward. Full unit test coverage with mock HTTP servers for everything that doesn't require a live FYERS session: state sign/verify/tamper/expiry, login URL construction, quotes response parsing (real verified shape), and all 5 refresh-job paths (no connection, valid+not-near-expiry, near-expiry-missing-data, successful silent refresh, failed refresh → notify). The actual browser-based FYERS login itself cannot be tested without the user's real app credentials and a live browser session — that step is on the user.

**Confirmed design decision (2026-07-05): `PRICE_UPDATE` intentionally ignores `MARKET_DATA_PROVIDER`.** User asked whether setting the provider to `fyers` should block/restrict manual `PRICE_UPDATE` signals — confirmed **no**, it stays a universal manual override regardless of provider (`internal/modules/signals/paper_ingest.go`'s `ingestPaperPriceUpdate` never reads the platform setting at all). Practical effect: with `fyers` configured and connected, a manual `PRICE_UPDATE` is transient — the next `SnapshotJob` sweep (`MARKET_DATA_SNAPSHOT_INTERVAL_SEC`) overwrites it with the real FYERS quote if one exists; if FYERS has no quote for that symbol (not connected, market closed), the manual value persists until one does. Documented in `ingest-price-update.bru`.

---

## Phase 7 — Indian Market Enhancements

**Goal:** make Indian paper trading realistic enough.

- `instruments` symbol master table (market_profile, exchange, symbol, name, instrument_type, tick_size, lot_size, active). Seed: RELIANCE, TCS, INFY, SBIN, HDFCBANK, ICICIBANK, NIFTY, BANKNIFTY.
- Market hours: NSE equity 09:15–15:30 IST, Mon–Fri. Initially allow orders anytime with a warning; later reject outside hours unless after-hours simulation is enabled.
- Fees/slippage settings (`slippage_bps`, `brokerage_model`, `tax_model`): start at zero, later add 1–5 bps slippage and basic brokerage/tax approximation.

**Status: done (instruments + market_profiles admin-configurable; tick/lot size reject immediately; market-hours enforcement admin-configurable, on by default, reject with a clear error — no warn-only stage).**
Scope expanded on request beyond the original spec: `market_profiles` is now a full admin-configurable table (migration `039`), not a fixed 4-value enum — admins can add/edit profiles (name, base_currency, timezone, trading_days) via `GET/POST/PATCH /v1/admin/market-profiles` and the `admin/instruments` dashboard page, seeded with one row (`indian_equity`, Mon-Fri 09:15-15:30 Asia/Kolkata). `instruments` (migration `039`) is the symbol master — seeded with the 8 listed symbols, admin CRUD via `/v1/admin/instruments`, editable tick_size/lot_size. `POST /v1/paper-accounts` now accepts `market_profile` (validated against active profiles, defaults to `indian_equity`).

Order validation (paper only, `internal/platform/queue/paper.go` + `internal/paperengine/fill.go`):
- Unknown/inactive symbol for the account's market profile → rejected before the fill engine is even called ("unknown or inactive symbol").
- Tick size / lot size → rejected immediately inside `paperengine.validateOrder` (`domain.ExecutionRequest.TickSize/LotSize`, tolerant of float rounding via `isMultiple`). Lot size is skipped for `CLOSE` — its quantity is engine-computed, not user-supplied.
- Market hours — **admin-configurable, on by default** (`guard.MarketHoursEnforcementSettingKey` in `platform_settings`, reusing Phase 6's runtime-toggle pattern), rejects with "outside trading hours for {profile name} ({timezone})" via `guard.WithinSchedule` (extracted from the existing per-webhook `WithinTradingHours` so both share one implementation). Deliberately **on by default** per explicit product decision: live FYERS data only flows during real market hours, so orders outside those hours would be trading on stale/absent data; disable it for signal-only testing.

**Real bug found and fixed while verifying this live:** the server's Docker image (`Dockerfile`) had no `tzdata` package installed, so `time.LoadLocation("Asia/Kolkata")` failed at runtime. `guard.WithinSchedule` silently caught that error and fell back to UTC — meaning **any webhook (live or paper) with a non-UTC `timezone` on its trading_hours has been silently evaluated against UTC this whole time**, not the configured zone. Fixed by adding `tzdata` to the runtime image's `apt-get install` line. This needs deploying to staging too (rebuild + redeploy), not just local — it's a pre-existing latent bug, not something this phase introduced, just uncovered by adding the first admin-facing IANA-timezone validation (`time.LoadLocation` check in `AdminCreateMarketProfile`).

Verified live: instrument CRUD (create/duplicate-409/update/deactivate), market profile CRUD (create/duplicate-409), tick-size rejection (820.53 vs 0.05 tick → rejected with exact reason, then 825.55 → filled), market-hours rejection (real outside-hours test correctly rejected, then admin-disabled to prove the bypass, then re-enabled), unknown-symbol rejection (blocked at the pre-existing webhook `allowed_symbols` guard layer, which runs before Phase 7's instrument check). Full permanent test coverage: `paperengine` (tick/lot compliant fill, tick/lot violations, CLOSE ignores lot size), `queue` (unknown symbol, inactive instrument, market-hours enforced/disabled/default-on, tick/lot wiring into the engine request) — all using injectable-clock helpers (`checkPaperMarketHoursAt`) rather than depending on wall-clock time, so the suite is deterministic regardless of when it runs.

Deferred: fees/slippage settings (not requested this pass), F&O/forex/crypto market profiles are structurally supported (admin can create the rows) but the paper engine itself has no segment-specific logic for them yet — that's Phases 9/10's job.

---

## Phase 8 — Conversion to Live Broker

**Goal:** use paper mode to drive upgrades to live trading.

- CTA on paper dashboard: "Ready to go live? Connect a live broker to execute real orders." -> Connect Zerodha / Angel / Dhan.
- Flow: paper webhook works -> user clicks "Connect Live Broker" -> creates live credential -> creates new live webhook or duplicates the paper one -> reviews risk guards -> confirms live activation.
- Never auto-convert a paper strategy to live execution without explicit user confirmation. Warning: "Live trading can result in financial loss. Review all symbols, quantities, order types, and risk limits before enabling live execution."

**Status (2026-07-04): done.** Research before implementing found the actual
conversion *mechanism* already existed from Phase 3 (`UpdateWebhook` already
allowed switching `broker_cred_id`/`paper_account_id`; `WebhookForm` already
rendered both Paper/Live optgroups in one dropdown in both create and edit
mode) — so this phase was almost entirely additive UI wiring plus one real
backend gap it surfaced, not a new mechanism:
- **Bug fixed**: `UpdateWebhook`'s switch-to-live branch checked
  `MayUseCredential` but never `EnforceWebhookLimit` — unlike the mirror-image
  switch-to-paper branch, which does call `EnforcePaperWebhookLimit`. This let
  a user bypass the live `MaxWebhooks` cap by editing an existing webhook
  instead of creating a new one. Fixed to match the existing paper-branch
  pattern exactly (`internal/modules/webhooks/handler.go`).
- **New "clone as live" flow** (deliberately *not* an in-place conversion —
  the source paper webhook/account are left completely untouched, so the
  user keeps paper-testing while also going live): `WebhookForm.tsx` gained
  an optional `cloneFrom` prop (create mode only) that pre-fills every field
  from an existing paper webhook via the already-existing `toForm()` helper,
  except `destination` (blanked — never silently default to a live
  credential) and `label` (suffixed `(Live)`). `webhooks/new/page.tsx` reads
  a `?cloneFrom=<id>` query param and fetches that webhook. A prominent
  `Alert variant="warning"` with the plan's exact required copy renders
  whenever `cloneFrom` is set.
- **CTA entry points**: a "Ready to go live?" card on
  `paper-accounts/[id]/page.tsx` (linking to `/webhooks/new?cloneFrom=...`,
  or to account creation/webhook creation first if either doesn't exist yet)
  and a matching banner on `webhooks/[id]/page.tsx` when the webhook is a
  paper destination. Both are plan-aware — mirroring `CredentialForm.tsx`'s
  existing `canLive = user.plan !== 'free' || user.early_bird_live_active`
  check — routing to `/billing` instead of the clone flow for free-plan
  users (still shown, for discoverability, just relabeled "Upgrade").
- **Deliberately out of scope**: threading a `returnTo` redirect through
  Zerodha's OAuth round-trip (existing inline "connect a broker" link in
  `WebhookForm` already covers the no-credential-yet case, just not
  seamlessly); wrapping live order execution behind
  `domain.ExecutionDestination` (confirmed via code trace this is a real
  behavior-preserving refactor of `queue.process()`'s live branch, not
  needed for this phase's actual goal — live orders keep flowing through the
  existing `brokerfactory`/`livebrokers` path, completely untouched);
  auto-pausing the source paper webhook after going live (not asked for, and
  against the "never auto-convert" spirit — the pause button built earlier
  this session is available as a manual next step).

Verified live: registered a fresh individual-plan test user, created a paper
account + paper webhook, created a live Angel credential, POSTed a live
webhook with the paper webhook's exact cloned config (confirmed both
webhooks then coexisted independently, original paper account/webhook
untouched) — then filled the plan's `MaxWebhooks=5` cap with live webhooks
and confirmed the fixed `UpdateWebhook` now correctly 403s an attempt to
switch the paper webhook to live ("webhook limit reached for your plan"),
where it would have silently succeeded before the fix. All test data cleaned
up afterward. `go build/vet/test`, dashboard `tsc --noEmit`, and full
`npm run build` all clean.

---

## Phase 9 — Crypto Demo Extension

**Goal:** add crypto paper trading without changing the core engine.

- Market profile `crypto_spot`: base currency USDT/USD/INR, symbols BTCUSDT/ETHUSDT, decimal quantities, 24/7 market hours, no MIS/CNC.
- Order types MARKET/LIMIT, actions BUY/SELL/CLOSE (same engine, different market profile rules).
- Market data later: Binance public API, CoinGecko, CoinMarketCap, CryptoCompare.

---

## Phase 10 — Forex Demo Extension

**Goal:** add forex paper trading once Indian equities are stable.

- Market profile `forex`: base currency USD, symbols EURUSD/GBPUSD/USDJPY/XAUUSD, lot/unit quantity model, leverage simulation, pip value calculation, spread/slippage.
- Order types MARKET/LIMIT, actions BUY/SELL/CLOSE.
- Market data later: Twelve Data, Finnhub, Alpha Vantage, OANDA practice API.
- Forex P&L logic is more involved than equities (pip value, base/quote currency) — do this last.

---

## Recommended Implementation Timeline

**Week 1 — Foundation**
Rename demo -> Paper Trading; add `paper_accounts`/`paper_orders`/`paper_positions`/`paper_trades` tables; `PaperExecutionEngine` skeleton; account creation UI.
Deliverable: user can create an Indian Equity Paper Trading Account.

**Week 2 — Webhook Execution**
Webhook can select a paper account; validate BUY/SELL/CLOSE and MARKET/LIMIT; require symbol/quantity/price; fill order immediately; update position; save trade.
Deliverable: a TradingView signal opens/closes a paper position.

**Week 3 — Dashboard & Metrics**
Paper account dashboard; positions table; trades table; realized/unrealized/total P&L; win rate; fill rate; notional.
Deliverable: user can see complete paper trading performance.

**Week 4 — Guards & Polish**
Allowed actions/symbols; max quantity/notional; deduplication; webhook secret; disclaimer; empty states; upgrade CTA.
Deliverable: free demo mode ready for closed beta users.

**Week 5+ — Market Data Provider**
`MarketDataProvider` interface; keep `SignalPriceProvider` as default; optional Indian LTP provider; background mark-to-market updater; account equity snapshots.
Deliverable: open positions update P&L without requiring new webhook signals.

---

## MVP Scope

Build only this first:

- Market profile: Indian Equity Paper Trading
- Supported actions: BUY, SELL, CLOSE
- Supported order types: MARKET, LIMIT
- Required signal fields: `secret`, `symbol`, `action`, `quantity`, `price`
- P&L: realized, unrealized (based on last known price), win rate, fill rate, notional
- No external market data in v1

### Suggested signal payloads

```json
{
  "secret": "my-secret",
  "symbol": "SBIN",
  "action": "BUY",
  "quantity": 10,
  "order_type": "MARKET",
  "price": 820.5
}
```

Close:

```json
{
  "secret": "my-secret",
  "symbol": "SBIN",
  "action": "CLOSE",
  "price": 825.0
}
```

Limit:

```json
{
  "secret": "my-secret",
  "symbol": "SBIN",
  "action": "BUY",
  "quantity": 10,
  "order_type": "LIMIT",
  "limit_price": 819.0,
  "price": 819.0
}
```

---

## Business Rule — Plan Limits

**Correction (2026-07-03):** Paper Trading is its own flat, plan-independent
tier, not scaled by the Free/Individual/Household ladder below. Every user —
regardless of live-trading plan — gets exactly **1 paper account**
(`plan.MaxPaperAccountsPerUser` in the Go backend). Individual and Household
are paid plans **for live broker trading only** (they raise `MaxWebhooks`/
`MaxBrokers` for live broker credentials); they do not grant additional
paper accounts. Conceptually, the "Free" plan effectively *is* the "Paper
Trading" plan today — it is not charged for yet, but is expected to be
monetized separately later (not implemented; no billing work has been done
for this).

Live trading plan limits (existing, unrelated to paper trading):
- **Free:** 1 webhook, 1 broker credential — no live trading access.
- **Individual (paid):** 5 webhooks, 3 broker credentials, live trading enabled.
- **Household (paid):** 25 webhooks, 15 broker credentials (5 seats), live trading enabled.

---

## Final Recommended Build Order

1. Remove broker dependency from demo mode
2. Create Paper Trading Account model
3. Implement signal-price-based paper execution
4. Add positions and P&L
5. Add webhook destination = paper account
6. Add dashboard analytics
7. Add upgrade path to live broker
8. Add real-time/delayed LTP provider later
9. Extend same engine to crypto
10. Extend same engine to forex
