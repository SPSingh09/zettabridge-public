# Action Items
**Last updated: 2026-07-07**

Current open action items. For longer-horizon pending work see [pending-work.md](pending-work.md); for go/no-go gates see [STATUS.md](../STATUS.md).

---

## Immediate (before beta open to traders)

- [ ] Whitelist NAT Elastic IP with Angel One SmartAPI app (IP Whitelist in app settings)
- [ ] Whitelist NAT Elastic IP with Dhan developer portal
- [ ] Update ECS task definition: `BROKER_MODE=live`, Angel client IP env vars set to NAT Elastic IP
- [ ] Place one real test order end-to-end (**Pro** plan, live Zerodha/Angel/Dhan credential); confirm `status=filled` and real `broker_order` ID in trades
- [ ] Confirm SEBI Algo-ID appears in broker order book for at least one Indian broker
- [ ] Run `make functional-test-staging` against AWS staging URL; all tests green
- [ ] Add Stripe webhook endpoint to Stripe Dashboard for staging URL *(low priority — billing disabled)*
- [ ] Regenerate swagger to remove stale `/v1/orgs/*` paths

## Short-term

- [ ] Add Zerodha per-user API key explainer to dashboard credential create form
- [ ] Operations drill: rotate webhook token + suspend/unsuspend a test user
- [ ] Verify JWT login/refresh flow works on `api.staging.zettabridge.net` domain
- [ ] CORS: confirm no browser errors from `app.staging.zettabridge.net` → `api.staging.zettabridge.net`
- [ ] Validate paper-account → paper webhook flow on staging (Free and Paper tiers)

## Execution adapters

- [ ] Confirm zerodha-adapter sidecar E2E on staging with `EXECUTION_ADAPTER_MODE=http`
- [ ] Document enabling Angel/Dhan/MT5 in `enabled_adapters` when ready for E2E

## Security hardening (GA gates)

- [ ] Add `Strict-Transport-Security`, `X-Content-Type-Options`, `X-Frame-Options`, `Content-Security-Policy` response headers (S1)
- [ ] Log scrub audit: confirm no credential material in CloudWatch logs (S2)
- [ ] Load test: 500 VU ingest, p99 < 100 ms (S4)
- [ ] Pen test: IDOR, JWT tamper, injection, webhook flood, brute-force (S5)
- [ ] KMS envelope encryption per credential (S6)
