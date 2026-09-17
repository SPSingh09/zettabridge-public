# Bruno collections

Open this folder in Bruno:

```
bruno/ZettaBridge
```

Do **not** open the parent `bruno/` directory — `bruno.json` lives inside `ZettaBridge/`.

## First run

1. Open collection **ZettaBridge API**
2. Select environment **local** (top-right dropdown)
3. Confirm `baseUrl` is `http://localhost:8080`
4. Run **Health → GET Healthz**

## Plan tiers (testing)

| Plan | Paper accts | Paper WH | Live WH | Live brokers | Paper trades/mo | Multi-product creds |
|------|-------------|----------|---------|--------------|-----------------|---------------------|
| free | 1 | 1 | 0 | 0 | 10 | no |
| paper | 3 | 5 | 0 | 0 | ∞ | no |
| pro | 5 | 8 | 1 | 1 | ∞ | no |
| pro_plus | 10 | 15 | 5 | 3 | ∞ | **yes** |

Use **Admin → PUT Set User Plan** (or **set-user-plan-pro-plus**) then **Plans → GET Me — *-limits** to verify caps.

## Recommended flows

**Free — paper only:** Auth → Plans/free-tier-demo-routing → Paper Trading folder (no credential needed).

**Paper plan:** Admin set `paper` → Paper Trading (multiple accounts/webhooks).

**Pro — live Indian:** Admin set `pro` → Credentials/create-zerodha (`account_mode: live`) → Webhooks/create-indian → Signals/ingest-indian-buy.

**Pro Plus — multi-product:** Admin set-user-plan-pro-plus → Credentials/create-multi-product-pro-plus → Webhooks/create-indian → Signals/ingest-indian-cnc (includes `"product": "CNC"`).

**Market data (paper marks):** Admin → settings-market-data-provider-put (`fyers`) + fyers-connect after credentials configured.

**Billing catalog:** Billing/plans (4 tiers, `multi_product_credentials` on pro_plus).

## Credentials

- `raw_creds` on POST/PUT only; never returned in responses.
- Indian brokers: `exchange` (NSE|BSE) required.
- Single product: `"product": "MIS"` (default).
- Pro Plus multi-product: `"products": ["MIS","CNC","NRML"]`.
- Live signals with multiple products must include `"product"` in the JSON payload.

Formats: `zerodha` = `api_key:access_token`, `mt5_cloud` = `auth_token:account_id`, `dhan` = `client_id:access_token`, `angel` = `api_key:client_code:jwt`.

See **Credentials/folder.bru** and `docs/broker-guide.md`.

## Paper trading

Typed payload (`ORDER_SIGNAL` / `PRICE_UPDATE`) — see **Paper Trading/folder.bru**.

MARKET fills can use live quotes when FYERS market data is connected; otherwise include `price` in ORDER_SIGNAL.

## Live mode

Set `BROKER_MODE=live` on the server for real broker APIs. Requires **pro** or **pro_plus** plan and `account_mode: live`.
