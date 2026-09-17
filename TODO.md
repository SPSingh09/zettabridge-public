# Post-Demo TODO

Work deliberately deferred while implementing the Paper Trading Engine phases
(see `DEMO_EXECUTION_PLAN.md`), to keep each phase scoped and avoid touching
live broker trading. Pick these up once the demo/paper trading phases are
stable.

## From Phase 2 (Paper Trading Engine MVP)

- **Wrap live broker execution behind `domain.ExecutionDestination`. — DONE (2026-07-05)**
  Live orders now dispatch through `liveBrokerDestination.Execute`
  (`internal/platform/queue/live_destination.go`), the counterpart to
  `paperengine.Engine`, so both paper and live webhooks go through one
  `domain.ExecutionDestination.Execute(ctx, req)` call from `queue.process()`.
  Implementation notes:
  - `liveBrokerDestination` is constructed fresh inside `process()` on every
    call (not cached on `Queue`) because `q.newBroker` can be reassigned
    after `queue.New()` via `Queue.SetBrokerFactory` (`app.go` does this to
    enable Zerodha Publisher mode) — caching it would have silently pinned a
    stale factory.
  - `ExecutionRequest`/`ExecutionResult` grew purely-additive live-only
    fields (`UserID`, `OrgID`, `Comment`, `SLPoints`/`TPPoints`,
    `FixedLotSize`/`MaxRiskPct`/`MaxLotSize` on the request;
    `Quantity`/`OrderType`/`SLPrice`/`TPPrice`/`AlgoID`/`BrokerType`/`Handoff`
    on the result) — `paperengine.Engine` is untouched by fields it doesn't use.
  - `maporder.Build`'s only live-webhook dependency (`wh.DefaultOrderType`)
    became a plain `defaultOrderType string` parameter so it no longer needs
    a `*store.Webhook`.
  - Metrics fidelity: a rejection's `brokerType` used to come from a local
    variable in `process()`; now `executionError` (wraps the failure +
    `brokerType`) carries it back through the `error` return so
    `zettabridge_trades_total{broker=...}` keeps the same tag on every
    existing rejection path, including `""` for "credential not found".
    **Caught and fixed a real bug here**: `brokererr.TradeFields` does a
    direct `err.(*brokererr.Error)` type assertion, not `errors.As` — passing
    it the `*executionError` wrapper directly silently produced
    `error_code="internal"` for every live rejection. Fixed by unwrapping to
    `ee.err` before calling `TradeFields` in `process()`.
  - No feature flag / dual-path fallback was added (deliberate — see the
    approved plan); safety net was preserving the exact existing test
    injection seams instead.
  - Verified: `go build ./... && go vet ./... && go test ./...` clean; the
    ~25 tests in `queue_test.go` + `paper_test.go` + the `httptest`-backed
    integration tests in `internal/integration/live_queue_test.go` /
    `live_queue_auth_test.go` all pass **unchanged** except the mechanical
    `maporder_test.go` call-site update for the new signature. Also
    live-stack-verified against the real docker-compose stack in
    `BROKER_MODE=mock`: sent a real signal through a live-destination
    webhook (success → `submitted` status, correct mock broker order ID,
    correct `zettabridge_trades_total{broker_type="zerodha",...}` tag; then
    a bracket+CNC rejection → `error_code="bracket_not_supported"` with the
    broker tag still correctly attached, confirming the `TradeFields` fix
    end-to-end on the running server, not just in the test suite).

## From Phase 0 (Product Cleanup) — done

Implemented in `dashboard/` (the real dashboard UI — not `zettabridge-web`,
which is marketing/docs only): dedicated `/paper-accounts` section
(`PaperAccountForm.tsx`, list + new pages, nav link), backend
`internal/modules/paperaccounts` API, and `CredentialForm.tsx` create-mode no
longer offers "Demo" (existing demo credentials untouched in edit mode).

**Paper account editing added 2026-07-04** (was deferred here): `PATCH
/v1/paper-accounts/:id` (`UpdatePaperAccount`, `store.UpdatePaperAccount`) —
`label` and `default_product` are always editable; `starting_balance` only
while the account is still flat (`cash_balance == starting_balance`, i.e. no
fill has moved cash yet), otherwise 409 — editing it after fills would
silently invalidate the account's own Equity/P&L math. `market_profile` and
`exchange` remain intentionally immutable (asset-class identity referenced by
every instrument lookup — changing them mid-life doesn't corrupt past data
but silently reinterprets what the account trades going forward; open a new
account instead). Edit-in-place UX on `paper-accounts/page.tsx` list, same
pencil/Save/Cancel pattern as `admin/instruments`.

**Pause/resume added 2026-07-04**: `PUT /v1/paper-accounts/:id/pause|resume`
(`SetPaperAccountStatus`, `store.UpdatePaperAccountStatus`) toggles the
pre-existing but previously-unenforced `status` column. Now actually
enforced at both paper signal entry points — `internal/platform/queue/paper.go`
(`processPaperOrder`) rejects ORDER_SIGNAL with "paper account is paused" and
`internal/modules/signals/paper_ingest.go` (`ingestPaperPriceUpdate`) 403s
PRICE_UPDATE the same way. Pausing does *not* stop the Phase 6 mark-to-market
sweep from marking existing open positions — it only blocks new trading
activity, not the account's displayed P&L.

**Reset added 2026-07-04** (the "blew the account, want a fresh start or
top-up" escape hatch): `PUT /v1/paper-accounts/:id/reset`
(`ResetPaperAccount`, `store.ResetPaperAccount`) transactionally deletes
`paper_positions`/`paper_orders`/`paper_account_snapshots` for the account
(`paper_trades` cascades from `paper_orders`' `ON DELETE CASCADE` FK) and
resets `cash_balance` to `starting_balance` — or to a new value if
`starting_balance` is passed in the body, letting a blown account be topped
up in the same call instead of requiring a separate `PATCH` (which would
409 anyway, since the account isn't flat before the reset). Irreversible;
frontend confirms via `ConfirmDialog` (which gained an optional `children`
slot for this, to hold the new-balance input) before calling it, same as
account deletion.

**Closed (2026-07-04): paper accounts stay solo-only, permanently.** `paper_accounts`
has no `org_id` column and this is now a settled decision, not a deferred
item — household org members each get their own flat 1-account allowance;
there is no shared org paper account and none is planned.

## From Phase 3 (Webhook Integration) — done

Webhooks can now route to a paper account: `webhooks.paper_account_id`
(nullable `broker_cred_id`, exactly-one CHECK constraint — migration `036`),
`internal/platform/queue/paper.go` branches the queue worker to
`paperengine.Engine.Execute`, and `WebhookForm.tsx` has a unified "Execution
destination" select. The existing guard/rate-limit/dedup pipeline already
applied unchanged (it was broker-agnostic before this phase).

Deferred out of this pass:
- **Webhook detail page (`webhooks/[id]/page.tsx`) publisher-mode detection**
  still only checks `broker_cred_id` against `/v1/credentials` — it doesn't
  yet special-case paper destinations (e.g. showing paper-specific messaging
  instead of the live trades/P&L tabs). Not incorrect, just unpolished.
- **Switching a webhook's destination type via edit** (live → paper or back)
  is implemented in the backend (`UpdateWebhook`) but not exposed in
  `WebhookForm.tsx`'s edit mode UX beyond just re-selecting from the same
  dropdown — worth a UX pass (e.g. confirming the switch clears
  destination-specific settings).

## From Phase 4 (Paper Trading Dashboard) — done

`GET /v1/paper-accounts/:id`, `/positions`, `/orders`, `/pnl`
(`internal/store/paper_pnl.go`, `internal/modules/paperaccounts/handler.go`),
new detail page `dashboard/app/(app)/paper-accounts/[id]/page.tsx`.
`StatCard`/`ActionBadge`/`StatusBadge` extracted to shared components.

Deferred out of this pass:
- **No pagination on `/orders`.** `ListPaperOrdersWithPnLByAccount` fetches
  every order for the account with no `LIMIT`/offset. Fine for MVP volumes;
  will need pagination (or at least a sane default `LIMIT`, matching how live
  trades cap at 20/100 rows by plan — see `ListTrades` in
  `internal/modules/webhooks/handler.go`) once accounts accumulate real
  history.
- **No real-time updates on the paper account detail page.** Paper fills
  already flow through the same `insertTrade` → `TradeNotifier` →
  `tradepush.Hub` websocket path live trades use (confirmed — paper trades
  call `q.insertTrade(ctx, trade, "paper")`, the identical function), so the
  infrastructure is already there. `paper-accounts/[id]/page.tsx` just does
  static SWR fetches today, unlike `webhooks/[id]/page.tsx` which subscribes
  via `createTradeSocket`. Wiring the same subscription in would be
  straightforward — deferred since Phase 4's spec only asked for a detail
  page, not live updates.
- **No CSV export**, unlike live trades' `/trades/export`
  (`internal/modules/pnl/handler.go`'s `ExportTrades`).

## From Phase 6 (Price Feed v2: LTP Provider Interface) — done

`internal/marketdata` (`Provider` interface, `SignalPriceProvider` default,
`SnapshotJob` background sweep), new `GET /v1/paper-accounts/:id/snapshots`.
Also added on request: an admin-configurable runtime toggle
(`platform_settings` table, migration 037, `GET`/`PUT
/v1/admin/settings/market-data-provider`, dashboard `admin/settings` page)
— switching providers takes effect on the next background sweep, no server
restart. **Then, also on request**, real FYERS OAuth + quotes integration
(`internal/integrations/brokers/fyers/{auth,refresh}`, `fyers_connection`
table migration 038, `GET/PUT /v1/admin/fyers/status|connect|pin`) — see
DEMO_EXECUTION_PLAN.md Phase 6 status notes for full detail, what's
ground-truth-verified vs. best-effort, and live-verification notes.

**Confirmed done (2026-07-04): live FYERS login tested successfully** by the
user in a real browser — the "click Connect FYERS, log in, see connected:
true" step (previously untestable by me, since it needs real FYERS app
credentials) is now verified working end to end.

**Confirmed working as designed (2026-07-04): FYERS silent refresh.** Its
reconnect-notification fallback (email/Telegram to the connecting admin) is
sufficient when silent refresh fails — no polling/automatic-retry mechanism
wanted, admin manually reconnects via the dashboard when notified. No further
action planned here.

**Dashboard equity chart — done (2026-07-04)**, see Phase 7 section below (added
alongside the other paper-account dashboard improvements built the same day).

**Sweep frequency scope confirmed (2026-07-04): paper-trading-only, by
design, kept as one global interval.** `internal/marketdata.SnapshotJob` only
ever sweeps `ListPaperAccountsWithOpenPositions` — live account equity comes
directly from the broker via `domain.Broker.GetAccountEquity()` when needed
(e.g. risk-based lot sizing), never from this job, so the global (not
per-plan) interval has zero effect on live accounts. Per-plan sweep
frequency remains a possible future monetization lever for Paper Trading
specifically, not implemented now.

## From Phase 7 (Indian Market Enhancements) — done

`market_profiles` + `instruments` tables (migration 039, admin-configurable
via `/v1/admin/market-profiles`, `/v1/admin/instruments`, dashboard
`admin/instruments` page), order validation (unknown/inactive symbol,
tick/lot size, market hours) wired into `internal/platform/queue/paper.go` +
`internal/paperengine/fill.go`. Market-hours enforcement is an
admin-configurable runtime toggle (on by default), reusing Phase 6's
`platform_settings` pattern. See DEMO_EXECUTION_PLAN.md Phase 7 status note
for full detail, including a real `tzdata`-missing Docker image bug found
and fixed while verifying this live (affected any non-UTC webhook
`timezone`, live or paper, not just this phase's new code).

Deferred out of this pass:
- **Fees/slippage settings** (`slippage_bps`, `brokerage_model`, `tax_model`)
  — not requested this pass; the plan lists them as a later refinement.
- **F&O engine support.** `indian_fno` market profile + instrument rows can
  now be created (see the curation follow-up below), but the paper fill
  engine (`internal/paperengine`) has no segment-specific logic yet (margin,
  expiry, lot conventions beyond a flat `lot_size` multiple) — that's Phases
  9/10's actual scope. Forex/crypto market profiles don't exist yet either
  (no migration seeds them) — add via a new migration when those phases start.
- **Trading-hours *schedule* editing (not the enforcement toggle — that's
  already done).** Two different things were getting conflated here: (1)
  turning market-hours enforcement on/off is admin-configurable today (the
  `platform_settings` toggle on `admin/settings`, done since Phase 7) — that
  part is closed, no action needed. (2) Editing the actual `trading_days`
  *schedule shape itself* (which days/windows apply, e.g. changing NSE's
  09:15-15:30 Mon-Fri to something else) still has no UI or API path post-seed
  — market profiles are curated via migration with a fixed schedule
  (deliberately narrowed to base_currency/timezone/active as the only
  admin-editable fields). A proper day/window editor for (2) is a natural
  follow-up once more than one schedule shape is actually needed.
- **Docker image `tzdata` fix needs deploying to staging.** Fixed locally in
  `Dockerfile`; staging's running image predates this fix and needs a
  rebuild+redeploy (`./deploy/scripts/aws-deploy.sh --backend-only`) to pick
  it up — otherwise staging's non-UTC `timezone` webhooks (live or paper)
  keep silently evaluating against UTC.
- **No Bruno coverage for market-profile/instrument admin endpoints.** Unlike
  most of this codebase's admin surface, `/v1/admin/market-profiles` and
  `/v1/admin/instruments` (list/update/create/delete) were only verified by
  hand via curl, not added to the Bruno collection — worth backfilling.

**Data fix (2026-07-04): NIFTY/BANKNIFTY seed lot sizes were wrong.** Migration
039 seeded both as `instrument_type: INDEX` with `lot_size: 1`, same
placeholder as the individual cash-equity symbols in that seed — but unlike
single-share equities, NSE index derivatives only ever trade in the
exchange-specified lot. Migration `041_correct_index_lot_sizes.sql` updates
them to the real values per NSE's contract-value-band revision effective
January 2026: NIFTY 50 = 65, BANKNIFTY = 30 (confirmed via web search, not
guessed — these were 15/25/35-ish before the Jan 2026 revision). **These are
revised periodically by NSE/SEBI** — when the next circular lands, update via
the admin instruments UI/API (already fully editable), don't wait for
another migration.

**Follow-up (2026-07-04): market profiles are now curated, not admin-created.**
Per explicit request, removed `AdminCreateMarketProfile`/`POST
/v1/admin/market-profiles` entirely — profiles are pre-configured via
migration (039 seeded `indian_equity`, 040 added `indian_fno`, same NSE
09:15-15:30 Mon-Fri schedule) and admins only edit `base_currency`/
`timezone`/`active` on the existing ones (name/code are fixed identity,
rejected — actually just not accepted — by `AdminUpdateMarketProfile`'s
body struct). Added `DELETE /v1/admin/instruments/:id`
(`AdminDeleteInstrument`) and reworked `admin/instruments` page's tables to
an edit-in-place UX (values read-only until the pencil/Edit button is
clicked, then Save/Cancel) for both market profile currency/timezone and
instrument tick/lot size, per explicit request.

## From Phase 8 (Conversion to Live Broker) — done

Full detail in `DEMO_EXECUTION_PLAN.md`'s Phase 8 status note. Summary: the
actual paper→live conversion mechanism already existed from Phase 3
(`UpdateWebhook` + `WebhookForm`'s single destination dropdown), so this pass
was a real bug fix (`EnforceWebhookLimit` missing from `UpdateWebhook`'s
switch-to-live branch, letting the live `MaxWebhooks` cap be bypassed by
editing an existing webhook instead of creating one — fixed) plus additive
UI: `WebhookForm`'s new `cloneFrom` prop, `webhooks/new?cloneFrom=<id>`, and
"Ready to go live?" CTAs on both the paper account and paper-webhook detail
pages, all plan-aware (free-plan routes to `/billing`). Deliberately clones
into a *new* live webhook rather than converting the existing paper one in
place, so paper-testing keeps working after going live.

Deferred out of this pass: `returnTo`-threading through Zerodha's OAuth
round-trip (see DEMO_EXECUTION_PLAN.md for why); wrapping live execution
behind `domain.ExecutionDestination` (see the Phase 2 entry above — confirmed
still a real refactor, still not needed); auto-pausing the source paper
webhook after going live (not asked for, contradicts "never auto-convert").

**Follow-up fix (2026-07-04): missing warning when going live with zero
credentials.** The original "no execution destinations yet" message in
`WebhookForm.tsx` only fires when *both* `creds` and `paperAccounts` are
empty — but the `cloneFrom` (go-live) flow always has at least one paper
account/webhook already, so that combined check never fired even when the
user had zero live credentials, silently leaving the "Live Broker" optgroup
empty with no explanation. Added a second, `cloneFrom`-specific check:
`cloneFrom && creds.length === 0` now shows "You don't have a live broker
credential yet — Create one first" linking to `/credentials/new`.

## Webhook count policy change (2026-07-04) — paper + live now share one pool

**Reverses part of Phase 3's "Paper Trading is plan-independent" design, on
explicit user request.** Paper and live webhooks now count against the same
`MaxWebhooks` pool per plan (Free=1, Individual=5, Household=25) — any split
between paper and live is allowed within that total. Previously paper
webhooks had their own separate flat cap of 1-per-user regardless of plan,
completely uncounted against `MaxWebhooks`.

Removed entirely: `plan.MaxPaperWebhooksPerUser`, `plan.CanAddPaperWebhook`,
`plan.ErrPaperWebhookLimitReached`, `store.CountPaperWebhooksByUser`,
`shared.EnforcePaperWebhookLimit`. `CreateWebhook`'s paper branch
(`internal/modules/webhooks/handler.go`) now calls the same
`EnforceWebhookLimit` the live branch always used; that function's underlying
count query was repointed from the old `CountLiveWebhooksByUser`/
`CountLiveWebhooksByOrg` (which excluded paper) to the already-existing
`CountWebhooksByUser`/`CountWebhooksByOrg` (no destination filter — these
existed already for an unrelated admin-detail-view purpose).

**Correctness note for future readers**: `UpdateWebhook`'s destination-switch
branches (paper↔live) no longer do any count check at all, and this is
correct, not an oversight — every webhook always has exactly one of
`BrokerCredID`/`PaperAccountID` set, so switching an *existing* webhook's
type never changes the user's total webhook count; only genuine creation
needs the cap check. (Phase 8's own `EnforceWebhookLimit` addition to the
switch-to-live branch, from earlier the same day, had to be reverted here —
it was a correct fix for the *old* model, at the *time* it was written, and
became wrong once the model changed under it.) `EnforceWebhookResumeLimit`
needed a smaller fix: its resume-limit check was being skipped entirely for
paper webhooks (wrapped inside the live-credential-only validation block),
even though its underlying count query never excluded them — moved the check
out so it runs for both kinds now, matching what the count already measured.

**Not touched, on purpose**: `MaxPaperAccountsPerUser` (still flat-1,
unrelated to this — the user only asked about webhook counts, not account
counts); `PauseAllWebhooksByUser` (still only auto-pauses live webhooks on a
plan downgrade — deciding whether/how paper webhooks should also be
candidates for auto-pause when a downgrade shrinks the combined pool below
the user's current total is a separate design question nobody's asked yet,
left alone rather than guessed at); the guard-config/trade-history/export
plan exemptions for paper destinations (`EnforceAdvancedGuardsPlan`,
`ListTrades`' row cap, `ExportTrades`' audit-log gate) — those are unrelated
to webhook *count* and remain fully exempt).

Verified live with fresh test users across Free/Individual (Household's cap
is just a formula, spot-checked via `plan.MaxWebhooks`, not worth manually
creating 25 webhooks): Free — 1st paper webhook succeeds, 2nd 403s at cap 1.
Individual — 3 paper + 2 live = 5 succeeds, a 6th of *either* type 403s,
switching one webhook's destination while at the cap still succeeds (the
correctness fix above). No frontend changes were needed —
`webhooks/page.tsx`'s limit UI already summed all webhooks regardless of
destination.

## Marketing site copy (zettabridge-web) — not updated

`zettabridge-web` is the marketing/docs site only (no dashboard there). It
still describes the old "Demo/Live mode" framing instead of "Paper Trading /
Live Broker":

- `components/home/dashboard-preview.tsx` (screenshot caption + alt text,
  lines ~26-27)
- `app/docs/getting-started/page.tsx` (line ~28, "paper / demo account")

## Batch (2026-07-05): 6 of 7 outstanding items from a full TODO review

User asked for a full accounting of everything left in this file and gave
explicit per-item direction. Item 1 (live execution refactor) was
researched, found riskier than expected, and deferred per an explicit
decision — see the Phase 2 entry above. Items 7/8 (marketing site, Phases
9/10) stay deferred, unchanged. The other 6 shipped this pass:

- **Webhook paper-awareness + destination-switch confirmation.**
  `webhooks/[id]/page.tsx` gained a paper-specific empty-state message and a
  fix for the free-plan "Showing last 20 trades" footer incorrectly
  rendering for paper webhooks (which already get the full 100-row limit —
  the footer just didn't know that). `WebhookForm.tsx` now shows a
  `ConfirmDialog` (moved outside the `<form>` tag — its buttons default to
  `type="submit"` and would otherwise double-fire the outer form) before
  saving an edit that changes the execution destination, with copy that
  adapts to paper→live / live→paper / live→live-different-credential.
- **Pagination for `/paper-accounts/:id/orders`.** New `limit`/`offset`
  query params (default 50, clamped 1-200) plus a `CountPaperOrdersByAccount`
  total — response shape changed from a bare array to `{"orders": [...],
  "total": N}`. This broke the existing `orders.bru` Bruno test (fixed —
  see below) since it assumed an array. Frontend has a "Load more" button.
- **Real-time updates on the paper account detail page.** Reused the exact
  same `tradepush.Hub`/`createTradeSocket` infra `webhooks/[id]/page.tsx`
  already used — no backend changes needed, since paper fills already
  publish through the identical `insertTrade` path. On a matching trade
  event, revalidates positions/orders/pnl via SWR `mutate()` rather than
  reconstructing P&L math client-side.
- **CSV export: paper trades (new) + missing buttons (both).** The live
  export endpoint (`/v1/webhooks/:id/trades/export`) was fully built but had
  *zero* frontend callers — confirmed by grep, not assumed. Added the
  missing paper equivalent (`/v1/paper-accounts/:id/orders/export`,
  `ListPaperOrdersForExport`/`ExportPaperOrders`, no plan gating) and export
  buttons on both detail pages, via a new `downloadCsv()` helper
  (`dashboard/lib/api.ts` — fetches with the auth header and triggers a
  Blob download, since a plain `<a href>` can't carry a Bearer token).
- **Equity chart.** `recharts@3.8.1` was already an installed dependency,
  unused. Added a `PaperAccountSnapshot` type and an `AreaChart` on the
  paper account detail page consuming `/v1/paper-accounts/:id/snapshots`
  (reversed client-side — backend returns newest-first).
- **Fees/slippage settings**, deliberately simplified: one admin-editable
  `slippage_bps` (worsens fill price in the adverse direction) and one
  combined `fee_bps` (brokerage+taxes+charges estimate, applied to notional)
  per market profile — not a full accurate Indian tax stack (STT/GST/stamp
  duty/exchange charges differ by segment), stated explicitly rather than
  silently assumed. Migration `042_market_profile_fees.sql` (both columns
  default 0 — zero behavior change until an admin opts in, verified by the
  full existing paperengine test suite passing unchanged at 0 bps).
  `paper_trades.fees` turned out to already exist in the schema since
  migration 034 and was already wired through the store layer end to end —
  just never populated by `paperengine`, always silently `0.0`. Now
  populated; `GetPaperAccountPnL` sums it into a new `TotalFees` field and
  the `Equity`/`TotalPnL` formulas were corrected to net it out (previously
  would have overstated equity by the fee amount once fees went non-zero).
  3 new permanent `paperengine` tests with hand-computed expected fill
  price/fee/cash-delta. Admin `admin/instruments` page gained editable
  Slippage/Fee (bps) columns on the market profiles table.
- **Bruno coverage for market-profile/instrument admin endpoints** — 6 new
  `.bru` files (list/update profile, list/create/update/delete instrument),
  `folder.bru` docs updated to mention them. While re-running the full
  Paper Trading suite to confirm no regressions, found and fixed two real
  issues unrelated to the new coverage itself: (1) `orders.bru`'s existing
  test broke from the pagination response-shape change above — updated to
  read `data.orders` instead of `data`; (2) `ingest-sell-partial.bru` and
  `ingest-close.bru` were latently flaky (not a regression from today's
  code — the collection authenticates as the free-plan admin user,
  `orders_per_sec: 1`, and Bruno fires requests back-to-back with no delay,
  so a second request landing in the same wall-clock second as the one
  before it gets silently rejected with `rate limit exceeded (1 req/s)`;
  reproduced deterministically 3/3 runs before the fix). Added a ~1.1s
  busy-wait `script:pre-request` guard to both files — full suite now
  passes 18/18 requests, 27/27 tests, 3/3 consecutive runs.

Also answered directly (no code): paper accounts stay solo-only permanently
(see Phase 0 section); live FYERS OAuth login confirmed tested successfully
by the user; FYERS silent-refresh reconnect-notification fallback confirmed
sufficient as designed; sweep-frequency interval confirmed paper-trading-only
(live equity comes from the broker directly, never this job — see Phase 6
section); trading-hours *enforcement* toggle vs. *schedule* editing
distinguished, since only the former was actually done (see Phase 7 section).

## Symbol Request feature (2026-07-05) — DONE

User asked for a self-service loop off Paper Trading's "unknown or inactive
symbol" rejection: request a symbol be added (with a reason), track status,
admin reviews via a dashboard banner (Accept/Reject), and the request
auto-resolves the moment a matching instrument actually exists.

Backend: migration `043_symbol_requests.sql`
(`symbol_requests` table: pending → accepted → resolved, or → rejected;
partial unique index blocks duplicate active requests per user+profile+symbol).
`internal/store/symbol_requests.go` (CRUD + `ResolveSymbolRequestsByInstrument`,
a bulk UPDATE...RETURNING). New module `internal/modules/symbolrequests`
(user-facing create/list). `internal/modules/admin/symbol_requests.go`
(list/accept/reject). `AdminCreateInstrument` (`market_profiles.go`) gained
one addition: after creating an instrument, it auto-resolves any matching
pending/accepted requests and notifies affected users — this is the single
source of truth for "resolved," not a frontend action, so it fires
regardless of whether the admin got there via the banner's Accept button or
added the symbol independently.

Notifications: email (new `mailer.Sender` methods, same pattern as
`SendFyersReconnect`) + Telegram (existing `telegram.Sender.Send`, reused
inline) + in-app (`InsertNotification`, dormant — no bell UI exists yet in
the real dashboard, confirmed by grep, so the Symbol Requests page's own
`status` column is what actually satisfies "see status on my request page").
Admin/support side is two new optional env vars, `SUPPORT_NOTIFICATION_EMAIL`
/ `SUPPORT_TELEGRAM_CHAT_ID` (user's explicit choice over "email all
admin-role users" or "banner only") — unset in `docker-compose.yml` by
design; set them to actually enable outbound admin notifications.

**Real bug caught and fixed during live verification**: the migration's
`resolved_instrument_id TEXT REFERENCES instruments(id)` had no `ON DELETE`
clause, so once a request resolved, its linked instrument could never be
deleted (`AdminDeleteInstrument` 500'd) — a real regression on an existing
admin capability. Fixed with `ON DELETE SET NULL` (the resolved status
itself is what matters; the specific instrument link is just provenance).

Frontend: `/support` (list) + `/support/new` (form, supports
`?symbol=&profile=&exchange=` pre-fill) — renamed from `/symbol-requests`
2026-07-05, see follow-up below. `PendingSymbolRequestsBanner`
(admin-only, mirrors `ZerodhaReconnectBanner`, wired into `AuthGuard`) whose
Accept button navigates to `/admin/instruments?prefill_...` — that page now
reads those params to pre-seed its existing inline "add instrument" form.
Paper account detail page's trades table gets a "Request to add this
symbol →" deep link whenever a trade's rejection reason starts with
"unknown or inactive symbol".

**Follow-up (2026-07-05): nav rename to "Support" + two rejection messages made actionable.**
User asked to rename the "Symbol Requests" nav item to "Support" (framing
it as the general place for support requests, more types to be added
later) and to make two existing rejection messages tell the user what to
do next, not just what went wrong.
- **Frontend-only rename**: `dashboard/app/(app)/symbol-requests/` →
  `dashboard/app/(app)/support/`, `NavBar.tsx` label/href, every internal
  `router.push`/`Link href` updated. The backend API paths
  (`/v1/symbol-requests`, `/v1/admin/symbol-requests/...`) and the Go
  package/module names are **unchanged** — they still accurately describe
  what the feature does; only the user-facing route/label changed. No new
  "generic support request" data model was built — that's real future
  scope, not implied by a nav rename.
- **`internal/platform/queue/paper.go`**'s "unknown or inactive symbol"
  reject message now ends with "— request it be added via Support in your
  dashboard". The exact prefix is unchanged (`unknown or inactive symbol
  %q for market profile %q`), so the paper-accounts page's existing
  prefix-matched "Request to add this symbol →" link still fires; this
  fixes the message for every *other* place the raw reason surfaces (CSV
  export, direct API callers) that don't have that special-case link.
- **`internal/guard/guard.go`**'s `ErrSymbolNotAllowed` ("symbol not
  allowed for this webhook") is now built dynamically per-call via a new
  `symbolNotAllowedError(symbol)` helper, appending `— edit the webhook to
  add "SYMBOL" to its allowed symbols list` with the actual attempted
  symbol substituted in. Still wraps the original sentinel with `%w` so
  `errors.Is(err, ErrSymbolNotAllowed)` (used by `gatewayErrCode` in
  `modules/signals/handler.go`, and `guard_test.go`) keeps working
  unchanged — confirmed via the existing test suite passing without
  modification. This message is shared by both live and paper webhooks
  (one check in `guard.ValidateIngest` for the sync live-ingest path, one
  in `guard.ValidateSymbol` used by both `queue.go`'s shared front-half and
  `paper_ingest.go`) — both call sites now produce the improved message.
- Verified live against the docker-compose stack: a live webhook with
  default symbol RELIANCE rejected a TATAMOTORS signal with `symbol not
  allowed for this webhook — edit the webhook to add "TATAMOTORS" to its
  allowed symbols list`; a paper webhook whose own symbol was TATAMOTORS
  (passing the guard check) but not in the `indian_equity` instrument
  master was rejected with `unknown or inactive symbol "TATAMOTORS" for
  market profile "indian_equity" — request it be added via Support in your
  dashboard`. Re-ran `go build/vet/test` (clean) and the full Bruno `Paper
  Trading` suite (19/19, 30/30) to confirm zero regressions. Test
  webhooks/accounts cleaned up afterward.

Verified: `go build/vet/test` clean. `tsc --noEmit` + full `npm run build`
clean (both new routes compile). New Bruno coverage (`Symbol Requests`
folder + 6 new `Admin/symbol-requests-*.bru` files) exercising the full
lifecycle end-to-end against the real docker-compose stack — create two
requests, duplicate-blocked (409), admin list/accept/reject, create the
matching instrument, verify auto-resolve, verify the FK-delete fix — 12/12
requests, 21/21 tests passing; re-ran the full pre-existing Paper Trading
suite (19/19, 30/30) to confirm zero regressions. Manually verified the
admin/support "received" email fires correctly when
`SUPPORT_NOTIFICATION_EMAIL` is set (temporarily set, confirmed the log
line, reverted).

## Close a paper position from the Positions card (2026-07-05) — DONE

User asked to let a user close an open paper position directly from the
Positions card, instead of only via a `CLOSE` webhook signal.

**Key finding**: `paperengine.Engine.Execute` already computes CLOSE's
side/quantity itself from the existing position (`resolveCloseSide`,
`internal/paperengine/fill.go`) — the caller only needs to name the symbol.
But it still requires `Price > 0` for CLOSE exactly like BUY/SELL (no
fallback to the position's `last_price`), so a one-click button uses the
position's own `last_price` (always populated once a position exists — set
from the entry fill, kept current via `PRICE_UPDATE`/snapshots) as the
close price, making Close behave like a market order at the last known mark.

**Design choice: synchronous HTTP handler, not the webhook queue.**
`webhook_id` is nullable on `paper_orders` (`ON DELETE SET NULL`), so the
new endpoint calls `paperengine.Engine.Execute` directly — the same way
`queue/paper.go`'s `processPaperOrder` already does — with no `WebhookID`,
no fake webhook object, and no schema change. This is the same category as
the existing `ResetPaperAccount`/`SetPaperAccountStatus` endpoints (a
synchronous, authenticated, user-initiated account action), so it skips the
async webhook/queue/websocket/generic-trades-table machinery entirely —
the frontend just refreshes its own SWR caches (positions/orders/pnl)
directly after the call succeeds, exactly like those two existing endpoints
already do.

Backend: new `POST /v1/paper-accounts/:id/positions/close` (body
`{"symbol": "..."}`) on `paperaccounts.Handler` — looks up the position,
instrument, and market profile (mirroring `processPaperOrder`'s own
sequence), applies the same market-hours gate every other paper order
respects, then calls `PaperEngine.Execute`. `paperaccounts.Handler` gained
a `PaperEngine domain.ExecutionDestination` field, wired to a **second**
`paperengine.New(pg)` instance in `internal/handler/handler.go` (confirmed
stateless/cheap — just wraps the store reference — so a second instance
alongside the queue's own is safe, no changes to `queue.Queue`/`app.go`).

**Small refactor along the way**: extracted `queue/paper.go`'s
`checkPaperMarketHoursAt` body into a new exported `guard.CheckMarketHours`
(with a narrow `MarketHoursStore` interface), so the new HTTP endpoint can
apply the identical, already-tested market-hours check without duplicating
it. `checkPaperMarketHoursAt` is now a one-line delegator — same pattern as
the `resolveLot` extraction from the earlier live-execution-refactor item,
verified safe by the pre-existing tests passing unchanged.

Frontend: `paper-accounts/[id]/page.tsx`'s Positions table gained an
Actions column with a "Close" button per row, a `ConfirmDialog` naming the
exact quantity/symbol/last-price about to be closed, and on success reuses
the exact same refresh bundle the page's websocket handler already uses
(`mutatePositions(); setOffset(0); mutateOrders(); mutatePnl();`) plus a
toast — no new page-level state beyond what already existed.

Verified: `go build/vet/test` clean (guard extraction left
`checkPaperMarketHoursAt`'s own tests passing unchanged — the signal the
refactor was safe). `tsc --noEmit` + full `npm run build` clean. New Bruno
coverage in the `Paper Trading` folder (`close-position-ingest-buy.bru` →
`close-position-api.bru` → `close-position-no-open-position.bru`, using a
dedicated INFY position so it doesn't disturb the folder's existing
TCS-based lifecycle test) — full suite 22/22 requests, 34/34 tests, zero
regressions. Live end-to-end walkthrough against the docker-compose stack
with a real paper account/webhook/position: BUY → Close via the exact
endpoint the button calls → confirmed position quantity flips to 0,
avg_entry_price resets to 0, a new `CLOSE` order appears in `.../orders`
with **no `webhook_id`** (confirming the "no fake webhook" design), and
`.../pnl` reflects the trade — then confirmed the 409 paths (closing again
with nothing open, closing an unknown symbol). Cleaned up test data
afterward.

## Webhook Summary: live order fill tracking (2026-07-07) — not implemented

While building the webhook Summary tab's Broker Execution stats (Filled,
Fill Rate, Avg Latency, ZettaBridge-vs-broker rejection split), confirmed
that **no live broker adapter in this codebase ever reports a real "filled"
confirmation** — not just Zerodha OAuth, all of them:

- Zerodha OAuth (`internal/integrations/brokers/livebrokers/zerodha.go`) —
  `placeMarketOrder`/`placeCoverOrder`/`squareOff` all return
  `domain.StatusSubmitted` right after Kite's synchronous order-accept ACK.
- Zerodha Publisher (`internal/integrations/brokers/zerodha/publisher/executor.go`)
  — returns `domain.StatusPendingConfirmation`; Kite never calls back after
  the user acts on the basket page, so this is a structural, not just
  unimplemented, limitation (see the Publisher-specific audit below).
- Angel, Dhan, MT5 (`livebrokers/angel.go`, `dhan.go`, `mt5.go`) — all
  funnel through the same `submittedResult(...)` helper
  (`livebrokers/result.go`), which hardcodes `domain.StatusSubmitted`.
  Notably, Dhan's API response already includes an `OrderStatus` field
  (`dhan.go`, the `result` struct in `placeMarketOrderWithSecurity`) that is
  decoded but never read — an easy partial win if this gets picked up.

Only `internal/paperengine` (paper trading) ever sets a genuine `filled`
status, because it's a synchronous simulation, not a real broker.

**What would be needed**: a real fill-confirmation mechanism per broker —
either polling each broker's order-status API (e.g. Kite's `GET /orders`)
on a schedule for any credential with outstanding `submitted` trades, or
wiring a broker postback/webhook where available (Kite Connect supports
order update postbacks; worth checking Angel/Dhan for the same). This is a
genuinely separate feature (new background job or webhook receiver, a
`trades` status-transition write path for it, and probably a per-broker
adapter interface addition) — out of scope for the Summary-stats work.

**What shipped instead, so the Summary tab doesn't show fake data**:
`internal/modules/pnl/handler.go`'s `GetWebhookPnL` nils out `FillRate` for
any webhook with a live broker credential (it would otherwise compute
`0/(0+rejected)` and misleadingly imply 0% of orders filled); `WinRate`
and `Notional` were already correctly `nil`/`0` since they depend on
`fill_price`, which is never populated for live orders either. The frontend
(`dashboard/components/PnLSummary.tsx`) shows "—" with a "Not tracked for
live orders yet" sub-label for Filled/Fill Rate/Win Rate/Notional whenever
`broker_type` is set (i.e. any live broker, not just Publisher), instead of
displaying `0`/`0%`. Paper trading webhooks are unaffected — they get real
Filled/Fill Rate/Notional numbers, since the paper engine resolves
synchronously with genuine fill data.

Also fixed in passing: `Avg Latency` in `internal/store/webhook_summary.go`
was computed only from `status IN ('filled','rejected')`, which — combined
with `filled` never happening for live brokers — meant a completely healthy
webhook with only successful submissions and zero rejections showed no
latency at all. `submitted`/`pending_confirmation` carry a genuine
`created_at`→`updated_at` broker round-trip time (every live trade is
inserted with its terminal status already resolved, never actually held at
`queued` in the DB), so they're now included; `cancelled` stays excluded
since that reflects when a user chose to act, not broker speed.

## Paper account detail page — deferred from review (2026-07-07)

Shipped: Account Summary (health, buying power, margin, today's P&L), Trading
Performance (win/loss, profit factor, drawdown, expectancy), Risk (exposure,
open/today risk, positions), Positions & Trades (equity chart, trade
distribution), Diagnostics (webhook stats, market info, metadata), Live
Readiness score (`dashboard/lib/liveReadiness.ts`), backend
`GetPaperAccountWebhookDiagnostics` + extended `PaperPnLSummary`.

**Deferred:**

- **Average holding time** — needs entry/exit pairing per round-trip (not just
  per fill row in `paper_trades`).
- **Average R-multiple** — blocked until SL/TP is supported on paper signals.
- **Recent activity timeline** — unified feed of fills, marks, webhooks, and
  position updates (currently only `last_market_update_at` /
  `last_webhook_signal_at` proxies).
- **Export JSON button in UI** — backend `?format=json` on
  `/v1/paper-accounts/:id/orders/export` already works; dashboard still shows
  CSV only.
- **Download statement / Trade Report PDF** — not started.
- **JSON export of full P&L summary** — API returns JSON on GET `/pnl`; no
  dedicated download button yet.
