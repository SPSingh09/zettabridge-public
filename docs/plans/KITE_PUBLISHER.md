# Kite Publisher — Implementation Plan

> **Historical implementation plan.** Current product behavior is defined in [STATUS.md](../STATUS.md) and [scope.md](../01-product/scope.md). Last reviewed: 2026-07-07.

**Feature:** Add `execution_mode` (publisher | user_api_oauth) to Zerodha credentials.
**Goal:** Support Zerodha Kite Publisher (browser-redirect order confirmation) alongside the existing Kite Connect API OAuth path, without creating a separate broker type or touching Angel / Dhan / MT5.

**Last updated:** 2026-07-01
**Status:** Planning

---

## Codebase grounding

| Area | File | Current state |
|---|---|---|
| Broker interface | `internal/domain/broker.go` | `PlaceOrder` / `CancelOrder` / `GetAccountEquity` |
| Live broker factory | `internal/integrations/brokers/livebrokers/factory.go` | `switch cred.BrokerType` → returns adapter |
| Outer factory | `internal/integrations/brokers/brokerfactory/factory.go` | picks mock vs live |
| Worker | `internal/platform/queue/queue.go` | calls `b.PlaceOrder(ctx, placeReq)` at line 310 |
| Credential struct | `internal/store/models.go` | no `ExecutionMode` field yet |
| Cred format parser | `internal/brokercreds/creds.go` | colon-delimited per broker type |
| Zerodha OAuth | `internal/integrations/brokers/zerodha/auth/zerodhaauth.go` | HMAC state signing already done — reuse pattern |
| Config | `internal/config/config.go` | `BillingEnabled bool` pattern for feature flags |
| Schema | `migrations/000_schema.sql` | next migration = `033_*.sql` |

---

## Mental model

```
Signal arrives
  ↓
Validate / guard / dedup / rate-limit   (unchanged)
  ↓
Resolve credential
  ↓
Resolve broker + execution_mode
  ↓
  ├── zerodha + user_api_oauth  →  ZerodhaBroker   →  Kite API → order_id
  ├── zerodha + publisher       →  PublisherExecutor → pending_confirmation + handoff fields
  ├── angel   + direct_api      →  AngelBroker
  ├── dhan    + direct_api      →  DhanBroker
  └── mt5_cloud + direct_api   →  MT5Broker
```

The webhook / risk / compliance pipeline never changes. Only the final executor differs.

---

## What NOT to do

- Do not create `broker_type = zerodha_publisher` — causes reporting, routing, and admin confusion
- Do not modify `ZerodhaBroker` or its OAuth path
- Do not mark Publisher callback as `filled` — user confirmed flow on Kite, ZettaBridge doesn't have fill data
- Do not let Publisher use the OAuth access token accidentally
- Do not let API OAuth use the Publisher api_key accidentally
- Do not allow silent mode switching while active webhooks exist
- Do not add Publisher logic inside the generic webhook ingest handler

---

## Phase 1 — DB migration `033_zerodha_publisher.sql`

**New file:** `migrations/033_zerodha_publisher.sql`

```sql
-- 1. execution_mode column on broker_credentials
ALTER TABLE broker_credentials
  ADD COLUMN IF NOT EXISTS execution_mode TEXT NOT NULL DEFAULT 'direct_api';

ALTER TABLE broker_credentials
  ADD CONSTRAINT broker_credentials_execution_mode_check
  CHECK (execution_mode IN ('direct_api', 'user_api_oauth', 'publisher'));

-- 2. Backfill: existing Zerodha rows → user_api_oauth (preserves current behavior exactly)
UPDATE broker_credentials
  SET execution_mode = 'user_api_oauth'
  WHERE broker_type = 'zerodha';

-- 3. Allow pending_confirmation trade status (Publisher trades sit here until user confirms)
ALTER TABLE trades DROP CONSTRAINT IF EXISTS trades_status_check;
ALTER TABLE trades ADD CONSTRAINT trades_status_check
  CHECK (status IN ('queued','submitted','filled','rejected','cancelled','pending_confirmation'));

-- 4. publisher_orders — one row per handoff attempt
CREATE TABLE IF NOT EXISTS publisher_orders (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id              UUID NOT NULL REFERENCES users(id),
  credential_id        UUID NOT NULL REFERENCES broker_credentials(id),
  webhook_id           UUID NULL REFERENCES webhooks(id),
  trade_id             UUID NULL,          -- links back to trades; NULL for dashboard-initiated orders
  broker               TEXT NOT NULL DEFAULT 'zerodha',
  execution_mode       TEXT NOT NULL DEFAULT 'publisher',
  basket_payload_json  JSONB NOT NULL,
  basket_payload_hash  TEXT NOT NULL,
  status               TEXT NOT NULL DEFAULT 'created',
  -- status values: created | awaiting_confirmation | user_returned | user_cancelled | expired
  callback_status      TEXT NULL,          -- raw status param from Kite callback
  kite_request_token   TEXT NULL,          -- from Kite callback on success
  signed_state         TEXT NOT NULL,      -- HMAC state for callback verification
  redirected_at        TIMESTAMPTZ NULL,
  callback_at          TIMESTAMPTZ NULL,
  expires_at           TIMESTAMPTZ NOT NULL,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_publisher_orders_user  ON publisher_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_publisher_orders_trade ON publisher_orders(trade_id)
  WHERE trade_id IS NOT NULL;

-- 5. publisher_order_events — audit trail per publisher order
CREATE TABLE IF NOT EXISTS publisher_order_events (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  publisher_order_id  UUID NOT NULL REFERENCES publisher_orders(id),
  event_type          TEXT NOT NULL,
  -- event_type values: SIGNAL_RECEIVED | BASKET_CREATED | AWAITING_USER_CONFIRMATION |
  --                    USER_RETURNED_FROM_KITE | USER_CANCELLED | EXPIRED | FAILED_VALIDATION
  event_payload       JSONB NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pub_events_order ON publisher_order_events(publisher_order_id);
```

**Deploy note:** Step 1 is additive only. The backfill UPDATE keeps all existing Zerodha credentials working. No behavior changes until Phase 5 is deployed with `ZERODHA_PUBLISHER_ENABLED=true`.

---

## Phase 2 — Domain types + feature flag

### `internal/domain/broker.go` — add execution mode constants and extend `OrderResult`

```go
type ExecutionMode string

const (
    ExecutionModeDirectAPI    ExecutionMode = "direct_api"
    ExecutionModeUserAPIOAuth ExecutionMode = "user_api_oauth"
    ExecutionModePublisher    ExecutionMode = "publisher"
)

// HandoffResult carries the fields needed to POST-submit a Kite basket from the browser.
// Only set when execution_mode = publisher.
type HandoffResult struct {
    Provider         string            // "zerodha"
    Method           string            // "POST_FORM"
    Action           string            // "https://kite.zerodha.com/connect/basket"
    Fields           map[string]string // api_key + data (basket JSON)
    PublisherOrderID string
}

// OrderResult — add Handoff field (nil for all non-publisher modes)
type OrderResult struct {
    Status    string
    OrderID   string
    FillPrice float64
    SLPrice   float64
    TPPrice   float64
    Handoff   *HandoffResult // non-nil only for publisher mode
}
```

### `internal/store/models.go` — add field to `BrokerCredential`

```go
ExecutionMode string `db:"execution_mode" json:"execution_mode"`
// direct_api | user_api_oauth | publisher
```

### `internal/config/config.go` — feature flag

```go
ZerodhaPublisherEnabled bool // ZERODHA_PUBLISHER_ENABLED
```

In `Load()`:
```go
ZerodhaPublisherEnabled: getEnvBool("ZERODHA_PUBLISHER_ENABLED", false),
```

---

## Phase 3 — Credential format for Publisher mode

**Edit:** `internal/brokercreds/creds.go`

Publisher only needs `api_key` (no `api_secret`, no `access_token` — Publisher does not use OAuth). Add a single-part parse branch under the zerodha case:

```go
case "zerodha":
    if len(parts) == 1 {
        // publisher mode: api_key only
        return Parsed{BrokerType: brokerType, APIKey: parts[0]}, nil
    }
    if len(parts) == 2 {
        return Parsed{BrokerType: brokerType, APIKey: parts[0], APISecret: parts[1]}, nil
    }
    if len(parts) == 3 {
        return Parsed{BrokerType: brokerType, APIKey: parts[0], APISecret: parts[1], AccessToken: parts[2]}, nil
    }
    return Parsed{}, fmt.Errorf("%w: zerodha publisher requires api_key; api oauth requires api_key:api_secret or api_key:api_secret:access_token", ErrInvalidFormat)
```

Update `FormatHint("zerodha")` to return mode-aware text:
- Publisher: `"api_key"`
- API OAuth: `"api_key:api_secret"`

---

## Phase 4 — Publisher executor package

**New package:** `internal/integrations/brokers/zerodha/publisher/`

```
internal/integrations/brokers/zerodha/publisher/
├── basket.go         — build Kite basket payload: []BasketItem → JSON string
├── mapper.go         — domain.PlaceRequest → BasketItem
├── validator.go      — publisher-specific validation (v1 restrictions)
├── state.go          — HMAC-signed callback state (reuse zerodhaauth pattern)
├── executor.go       — implements domain.Broker
└── executor_test.go
```

### `basket.go`

```go
type BasketItem struct {
    Exchange        string  `json:"exchange"`
    TradingSymbol   string  `json:"tradingsymbol"`
    TransactionType string  `json:"transaction_type"` // BUY | SELL
    Quantity        int     `json:"quantity"`
    OrderType       string  `json:"order_type"`       // MARKET | LIMIT
    Product         string  `json:"product"`          // MIS | CNC | NRML
    Price           float64 `json:"price,omitempty"`
    Variety         string  `json:"variety"`          // regular
}

func BuildBasket(items []BasketItem) (string, error)
// → JSON array string, e.g. `[{"exchange":"NSE",...}]`
```

### `mapper.go`

```go
func MapOrder(req *domain.PlaceRequest) (BasketItem, error)
```

v1 restrictions enforced in the mapper:
- Exchange must be NSE or BSE
- Variety must be `regular`
- No F&O, no SL/TP, no bracket order type

### `validator.go`

```go
func Validate(parsed brokercreds.Parsed, req *domain.PlaceRequest) error
```

Checks:
- `parsed.APIKey` is non-empty
- Exchange is NSE or BSE
- Quantity > 0
- OrderType is MARKET or LIMIT
- LIMIT orders have Price > 0
- Product is MIS, CNC, or NRML

### `state.go`

Reuse the HMAC pattern from `zerodhaauth.go`:

```go
// State format: publisherOrderID|userID|credentialID|expiresUnix|hmac
// Signed with HMAC-SHA256 using JWT_SECRET

func GenerateState(publisherOrderID, userID, credentialID string, secret string) string
func ParseState(state, secret string) (publisherOrderID, userID, credentialID string, err error)
// ParseState returns error if signature invalid or token expired
```

Expiry: 30 minutes (sufficient for user to review and submit on Kite).

### `executor.go`

```go
type Executor struct {
    cred        *store.BrokerCredential
    parsed      brokercreds.Parsed
    pg          PublisherStore   // interface: InsertPublisherOrder, InsertPublisherOrderEvent
    jwtSecret   string           // for HMAC state signing
    callbackURL string           // https://api.zettabridge.net/v1/publisher/callback
}

// PlaceOrder:
// 1. validator.Validate(parsed, req) — return ErrInvalidRequest if fails
// 2. mapper.MapOrder(req) → BasketItem
// 3. basket.BuildBasket([]BasketItem{item}) → basket JSON
// 4. state.GenerateState(newUUID, cred.UserID, cred.ID, jwtSecret) → signedState
// 5. pg.InsertPublisherOrder(ctx, &PublisherOrder{...}) → publisher_order row
// 6. pg.InsertPublisherOrderEvent(ctx, publisherOrderID, "BASKET_CREATED", nil)
// 7. return &domain.OrderResult{
//      Status:  "pending_confirmation",
//      OrderID: publisherOrderID,        // stored in trades.order_id
//      Handoff: &domain.HandoffResult{
//          Provider: "zerodha",
//          Method:   "POST_FORM",
//          Action:   "https://kite.zerodha.com/connect/basket",
//          Fields:   map[string]string{"api_key": parsed.APIKey, "data": basketJSON},
//          PublisherOrderID: publisherOrderID,
//      },
// }

func (e *Executor) PlaceOrder(ctx context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error)

// CancelOrder and GetAccountEquity are not supported in Publisher mode
func (e *Executor) CancelOrder(ctx context.Context, req *domain.CancelRequest) error {
    return brokererr.New(brokererr.CodeUnsupportedOperation,
        "Zerodha Publisher mode does not support cancel — cancel directly on Kite")
}

func (e *Executor) GetAccountEquity(ctx context.Context) (float64, error) {
    return 0, brokererr.New(brokererr.CodeUnsupportedOperation,
        "Zerodha Publisher mode does not support account equity fetch")
}
```

---

## Phase 5 — Live broker factory routing

**Edit:** `internal/integrations/brokers/livebrokers/factory.go`

Change the zerodha case to branch on `execution_mode`:

```go
case "zerodha":
    switch domain.ExecutionMode(cred.ExecutionMode) {
    case domain.ExecutionModePublisher:
        if !cfg.ZerodhaPublisherEnabled {
            return nil, fmt.Errorf("Zerodha Publisher mode is not enabled on this server")
        }
        return publisher.NewExecutor(cred, parsed, publisherStore, cfg.JWTSecret, publisherCallbackURL), nil

    case domain.ExecutionModeUserAPIOAuth, domain.ExecutionModeDirectAPI, "":
        // empty string → backfill safety: treat as user_api_oauth
        return &ZerodhaBroker{adapterBase: base, parsed: parsed}, nil

    default:
        return nil, fmt.Errorf("unsupported execution_mode %q for zerodha", cred.ExecutionMode)
    }
```

**Edit:** `internal/integrations/brokers/brokerfactory/factory.go` — pass `*config.Config` through to `livebrokers.NewWithParsed` so the publisher enabled flag and JWT secret reach the factory.

---

## Phase 6 — Worker: store `pending_confirmation` result

**Edit:** `internal/platform/queue/queue.go`

After `b.PlaceOrder(ctx, placeReq)` at line 310, the result for Publisher mode has:
- `result.Status = "pending_confirmation"`
- `result.OrderID = publisherOrderID` (UUID of the publisher_orders row)
- `result.Handoff != nil`

The worker stores the trade normally — the status `pending_confirmation` is now allowed by the trades constraint added in the migration. No structural change to the worker loop is needed; the trade is persisted and the dashboard polls it.

The handoff fields are already stored in the `publisher_orders` row created by the executor. The dashboard fetches them via `GET /v1/publisher/orders/:id`.

---

## Phase 7 — Publisher HTTP module

**New package:** `internal/modules/publisher/handler.go`

Register in `internal/handler/handler.go` alongside other module routes.

### Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/publisher/orders/:id` | JWT | Poll status + get handoff form fields |
| `POST` | `/v1/publisher/orders` | JWT | Create publisher order from dashboard (no webhook) |
| `GET` | `/v1/publisher/callback` | Public | Kite redirects here after user action |

### `GET /v1/publisher/orders/:id`

Response:
```json
{
  "id": "uuid",
  "status": "awaiting_confirmation",
  "expires_at": "2026-07-01T12:30:00Z",
  "handoff": {
    "provider": "zerodha",
    "method": "POST_FORM",
    "action": "https://kite.zerodha.com/connect/basket",
    "fields": {
      "api_key": "xxx",
      "data": "[{...basket items...}]"
    }
  }
}
```

Ownership enforced: user JWT must match `publisher_orders.user_id`.

### `GET /v1/publisher/callback` (public)

Processing steps:
1. Parse query: `status`, `request_token` (success only), `state`
2. `state.ParseState(state, jwtSecret)` — reject 400 if invalid signature or expired
3. Load publisher_order by parsed `publisherOrderID` — reject 400 if not found or already terminal
4. Update `publisher_orders.status`:
   - `status=success` → `user_returned`
   - `status=cancelled` → `user_cancelled`
5. Set `kite_request_token`, `callback_at`, `callback_status`
6. Insert `publisher_order_events` row (`USER_RETURNED_FROM_KITE` or `USER_CANCELLED`)
7. Update linked `trades.status`:
   - `user_returned` → `submitted`
   - `user_cancelled` → `cancelled`
8. Redirect browser → `{FrontendURL}/trades/{tradeID}?publisher=1`

**Critical:** Do NOT set trade status to `filled`. Publisher confirms the user submitted the order on Kite; ZettaBridge does not have fill confirmation.

### `POST /v1/publisher/orders` (dashboard-initiated)

Allows a logged-in user to create a publisher order directly from the dashboard without a TradingView signal. Validates the credential is in publisher mode, builds the basket, inserts publisher_order, and returns the handoff fields.

---

## Phase 8 — Store layer additions

**Edit:** `internal/store/` — add `PublisherOrder` and `PublisherOrderEvent` structs and queries.

```go
type PublisherOrder struct {
    ID                string     `db:"id"`
    UserID            string     `db:"user_id"`
    CredentialID      string     `db:"credential_id"`
    WebhookID         *string    `db:"webhook_id"`
    TradeID           *string    `db:"trade_id"`
    Broker            string     `db:"broker"`
    ExecutionMode     string     `db:"execution_mode"`
    BasketPayloadJSON []byte     `db:"basket_payload_json"`
    BasketPayloadHash string     `db:"basket_payload_hash"`
    Status            string     `db:"status"`
    CallbackStatus    *string    `db:"callback_status"`
    KiteRequestToken  *string    `db:"kite_request_token"`
    SignedState       string     `db:"signed_state"`
    RedirectedAt      *time.Time `db:"redirected_at"`
    CallbackAt        *time.Time `db:"callback_at"`
    ExpiresAt         time.Time  `db:"expires_at"`
    CreatedAt         time.Time  `db:"created_at"`
    UpdatedAt         time.Time  `db:"updated_at"`
}

type PublisherOrderEvent struct {
    ID                string    `db:"id"`
    PublisherOrderID  string    `db:"publisher_order_id"`
    EventType         string    `db:"event_type"`
    EventPayload      []byte    `db:"event_payload"`
    CreatedAt         time.Time `db:"created_at"`
}
```

Required store methods on `PGStore`:
```go
InsertPublisherOrder(ctx context.Context, o *PublisherOrder) error
GetPublisherOrder(ctx context.Context, id string) (*PublisherOrder, error)
UpdatePublisherOrderCallback(ctx context.Context, id, status string, requestToken *string, callbackAt time.Time) error
InsertPublisherOrderEvent(ctx context.Context, e *PublisherOrderEvent) error
```

---

## Phase 9 — Credential handler updates

**Edit:** `internal/modules/credentials/handler.go`

### On create/update

- Validate `execution_mode` field:
  - `zerodha` → allow `user_api_oauth` or `publisher` only
  - `angel`, `dhan`, `mt5_cloud` → force `direct_api` (or reject other values with 400)
- For `publisher` mode, credential `encrypted_creds` must be single-part `api_key`
- For `user_api_oauth` mode, credential must be two-part or three-part

### Mode switch guard

If the credential already exists and `execution_mode` is changing, check for active webhooks:

```go
if existingCred.ExecutionMode != newMode {
    count, _ := pg.CountActiveWebhooksForCred(ctx, credID)
    if count > 0 && req.Query("force") != "true" {
        return 409, {
            "error": "credential has N active webhooks; pass ?force=true to switch mode",
            "active_webhooks": count,
        }
    }
}
```

### `CredentialResponse` update

Include `execution_mode` in the JSON response so the dashboard can render the correct UI.

---

## Phase 10 — Dashboard: credential form

**Edit:** credential create/edit form in `dashboard/`

For `broker_type = zerodha`, show an **Execution Mode** selector after the broker type dropdown:

```
Execution Mode *
  ○ Kite Publisher        — user reviews and places order on Kite
  ○ User API OAuth        — direct API via Kite Connect (requires OAuth)
```

**Kite Publisher selected:**
- Show: "Publisher App API Key" text field (single value, no secret)
- Show banner: "Orders are prepared by ZettaBridge and must be confirmed by the user on Kite. No SEBI Algo-ID required."

**User API OAuth selected:**
- Show existing: "API Key" + "API Secret" + OAuth connect button
- Show banner: "Orders may be placed directly using the connected Kite OAuth session."

**Edit:** `dashboard/lib/types.ts` — add `execution_mode: string` to `BrokerCredential` type.

---

## Phase 11 — Dashboard: handoff review page

**New component/page** in `dashboard/`

Triggered when user opens a trade with `status = pending_confirmation`.

```
Review & Place on Zerodha

  Symbol:   RELIANCE
  Action:   BUY
  Quantity: 10 shares
  Type:     MARKET
  Exchange: NSE

  ⏱ Expires in 28 minutes

  [Place Order on Zerodha]    [Cancel]
```

**"Place Order on Zerodha"** button:

```tsx
// Fetches handoff fields from GET /v1/publisher/orders/:id
// Then submits via POST form to Kite basket URL
const form = document.createElement('form')
form.method = 'POST'
form.action = handoff.action
Object.entries(handoff.fields).forEach(([k, v]) => {
  const input = document.createElement('input')
  input.type = 'hidden'
  input.name = k
  input.value = v
  form.appendChild(input)
})
document.body.appendChild(form)
form.submit()
```

After Kite redirects back to callback → backend updates trade → user lands on `/trades/{id}?publisher=1` showing the updated status.

---

## Phase 12 — Tests

### Factory tests (`internal/integrations/brokers/brokerfactory/factory_test.go`)

- `zerodha + user_api_oauth` → `*ZerodhaBroker` (existing behavior, must not break)
- `zerodha + publisher + ZERODHA_PUBLISHER_ENABLED=true` → `*publisher.Executor`
- `zerodha + publisher + ZERODHA_PUBLISHER_ENABLED=false` → error
- `zerodha + ""` (empty, backfill safety) → `*ZerodhaBroker`
- `zerodha + invalid_mode` → error
- `angel + direct_api` → `*AngelBroker` (unchanged)

### Publisher executor tests (`internal/integrations/brokers/zerodha/publisher/executor_test.go`)

- MARKET BUY NSE → basket payload correct + `pending_confirmation` status
- LIMIT BUY with price → `price` field in basket item
- Missing api_key → validation error
- NFO exchange → rejected with clear error
- SL/TP bracket → rejected (unsupported in v1)
- `CancelOrder` → returns `ErrUnsupportedOperation`
- `GetAccountEquity` → returns `ErrUnsupportedOperation`

### Publisher mapper tests (`internal/integrations/brokers/zerodha/publisher/mapper_test.go`)

- MARKET BUY → correct variety, transaction_type, quantity
- MARKET SELL → `transaction_type: SELL`
- LIMIT BUY with price → `order_type: LIMIT`, `price` set
- LIMIT without price → error
- Invalid exchange (FOREX) → error
- Zero quantity → error
- Invalid product → error

### Callback handler tests (`internal/modules/publisher/handler_test.go`)

- Valid state + `status=success` → trade `submitted`, publisher order `user_returned`, 302 redirect
- Valid state + `status=cancelled` → trade `cancelled`, publisher order `user_cancelled`, 302 redirect
- Invalid HMAC → 400
- Expired state → 400
- Unknown publisher_order_id → 400
- Already terminal publisher order → 400 (idempotency guard)

---

## Phase 13 — Bruno tests

**New files** in `bruno/ZettaBridge/Zerodha Publisher/`:

```
create-publisher-credential.bru
get-publisher-handoff.bru
publisher-callback-success.bru
publisher-callback-cancel.bru
switch-to-publisher-mode.bru
switch-to-api-oauth-mode.bru
```

**New sub-folder** in `bruno/ZettaBridge/Failure Modes/Zerodha Publisher/`:

```
publisher-disabled.bru               — ZERODHA_PUBLISHER_ENABLED=false
missing-publisher-api-key.bru        — empty api_key
invalid-basket-exchange.bru          — NFO exchange rejected
invalid-state-signature.bru          — tampered HMAC
expired-state.bru                    — state older than 30 min
switch-while-active-webhooks.bru     — mode switch without ?force=true
```

---

## Implementation sequence

Execute in this exact order. Each step is independently deployable.

| Step | What | Key files touched |
|---|---|---|
| **1** | Migration 033 — column + tables | `migrations/033_zerodha_publisher.sql` |
| **2** | Domain types + config flag | `internal/domain/broker.go`, `internal/store/models.go`, `internal/config/config.go` |
| **3** | Feature flag wired at false | `ZERODHA_PUBLISHER_ENABLED=false` in docker-compose + ECS env — safe deploy, no behavior change |
| **4** | Credential format parser | `internal/brokercreds/creds.go` |
| **5** | Publisher executor package | `internal/integrations/brokers/zerodha/publisher/` |
| **6** | Factory routing by execution_mode | `internal/integrations/brokers/livebrokers/factory.go`, `brokerfactory/factory.go` |
| **7** | Store layer queries | `internal/store/` |
| **8** | Publisher HTTP module | `internal/modules/publisher/handler.go`, wired in `internal/handler/handler.go` |
| **9** | Credential handler validates execution_mode + mode-switch guard | `internal/modules/credentials/handler.go` |
| **10** | Unit + callback tests | executor_test.go, handler_test.go, factory_test.go |
| **11** | Dashboard: credential form execution mode selector | `dashboard/` |
| **12** | Dashboard: handoff review page | `dashboard/` |
| **13** | Bruno tests | `bruno/ZettaBridge/Zerodha Publisher/` |
| **14** | Enable on staging | `ZERODHA_PUBLISHER_ENABLED=true` in ECS staging env |
| **15** | End-to-end staging smoke test | signal → pending_confirmation → handoff page → Kite → callback → submitted |

---

## Deployment plan

### Step 1: Migration only (no app change)

Apply `033_zerodha_publisher.sql`. The backfill UPDATE ensures all existing Zerodha credentials are on `user_api_oauth`. Zero downtime, zero behavior change.

### Step 2: App deploy with feature flag off

Deploy the new code with `ZERODHA_PUBLISHER_ENABLED=false`. Publisher credentials would error, but none exist yet. Existing `user_api_oauth` credentials unaffected.

### Step 3: Enable on staging

```
ZERODHA_PUBLISHER_ENABLED=true
```

Create a test Publisher credential. Do not modify existing Zerodha OAuth credentials.

### Step 4: Staging smoke test

```
signal → worker → publisher.Executor.PlaceOrder
       → trade status = pending_confirmation
       → GET /v1/publisher/orders/:id returns handoff fields
       → POST form to Kite basket
       → Kite callback GET /v1/publisher/callback?state=...&status=success
       → trade status = submitted
       → redirect to /trades/:id
```

### Step 5: Enable in production (closed beta only)

Start with admin/test user only.

---

## Rollback plan

**If Publisher has issues:**
```
ZERODHA_PUBLISHER_ENABLED=false
```
- All existing `user_api_oauth` credentials continue working
- Publisher credentials return "feature disabled" error
- `publisher_orders` and `publisher_order_events` tables remain with no new entries
- No DB rollback needed

**If mode switch guard has issues:**
- Credential handler reverts to not checking `execution_mode` change — existing mode remains in use

---

## Naming conventions (use these everywhere)

| Display name | Internal value |
|---|---|
| Kite Publisher | `publisher` |
| User API OAuth | `user_api_oauth` |
| Direct API | `direct_api` |

Avoid: `zerodha_v1`, `zerodha_v2`, `publisher_broker`, `manual_mode`.
