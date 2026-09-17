# Beta Phase 4 — Live Broker Flip and Beta Launch

> **Historical implementation plan.** Current product behavior is defined in [STATUS.md](../STATUS.md) and [scope.md](../01-product/scope.md). Last reviewed: 2026-07-07.

**Target: Days 7–10**
**Goal:** Real orders flow through AWS to live brokers. Ops drills done. Beta opens to selected traders.

Depends on: [beta-phase-3-app-wiring.md](beta-phase-3-app-wiring.md)

---

## Phase 9 — Broker live flip on AWS

### 9.1 Whitelist AWS NAT EIP with brokers

The whitelistable IP is the NAT Gateway Elastic IP from Phase 2 (not the ECS task private IP, not the ALB IP).

| Broker | Portal | Action |
|--------|--------|--------|
| Angel One | SmartAPI developer portal → App settings → Whitelist IP | Add `<NAT-EIP>` |
| Dhan | Dhan developer portal → App settings → IP Whitelist | Add `<NAT-EIP>` |
| Zerodha | No server-side IP whitelist needed | — |
| MT5 / MetaApi | No IP whitelist needed | — |

Verify from an ECS task that outbound IP is the NAT EIP:
```bash
# Run as a one-off Fargate task in private subnet
curl -s https://ifconfig.me
# Must return <NAT-EIP>
```

### 9.2 Flip BROKER_MODE to live

Update ECS backend task definition env:
```
BROKER_MODE=live
BROKER_ANGEL_CLIENT_LOCAL_IP=<NAT-EIP>
BROKER_ANGEL_CLIENT_PUBLIC_IP=<NAT-EIP>
BROKER_ANGEL_MAC_ADDRESS=<mac-from-host>
```

Note: `SEBI_ALGO_ID_REQUIRED` auto-becomes `true` when `BROKER_MODE=live` (set in `internal/config/config.go`). Do not override to `false`.

Deploy new task definition revision and force new deployment:
```bash
aws ecs update-service --cluster zettabridge-staging \
  --service zettabridge-backend --force-new-deployment
```

Verify in CloudWatch logs:
```
startup: broker_mode=live workers=20 queue_buffer=500
startup: angel egress local_ip=<NAT-EIP> public_ip=<NAT-EIP>
```

### 9.3 Create real Pro credential and verify

- [ ] Upgrade a test user to Pro plan via `/admin`
- [ ] Add live Angel or Dhan credential with real API keys and `account_mode=live`
- [ ] Hit `POST /v1/credentials/:id/verify` → must return `{"valid": true}`
- [ ] Check CloudWatch logs: no auth errors, no IP rejection from broker

---

## Phase 10 — Live smoke test

Place a minimal real order through the full stack. Use the smallest possible lot size.

| Step | Action | Evidence to save |
|------|--------|-----------------|
| 1 | POST signal to webhook URL | Request ID from response |
| 2 | `GET /v1/webhooks/:id/trades` | Trade row with `status=submitted`, real `broker_order` ID |
| 3 | Verify order in broker portal | Broker confirms pending order |
| 4 | Test CancelOrder ✓ | `DELETE /v1/webhooks/:id/trades/:tradeId` → `status=cancelled` |
| 5 | Test SL/TP bracket | POST bracket order → both SL and TP legs visible in broker |
| 6 | Confirm SEBI Algo-ID | Broker order details show Algo-ID value |
| 7 | WebSocket push | Trade status update appears in dashboard WebSocket feed |
| 8 | P&L tab | P&L stats reflect the completed trade |
| 9 | Audit log | Order event visible in org activity / admin view |

Save: CloudWatch log snippet, broker portal screenshot, dashboard trade row screenshot, request IDs.

---

## Phase 11 — Beta readiness and ops drills

### Ops drills

| Drill | Steps | Passed when |
|-------|-------|-------------|
| Rotate webhook token | Admin dashboard → webhook settings → rotate token | Old token rejected (403), new token accepted |
| Suspend user | Admin `/admin` → suspend a test user | User's next API call returns 403 |
| Unsuspend user | Admin → unsuspend | User can log in again |
| Broker credential re-verify | After env change, re-verify live credential | Returns valid |
| ECS service restart | `aws ecs update-service --force-new-deployment` | Service restarts, health check passes, no data loss |
| DB connection failover | RDS failover test (Multi-AZ) | App reconnects automatically within ~30s |

### Support docs to prepare

- [ ] Known limitations doc: SEBI Algo-ID requirement for live orders; no queued-trade cancellation; ECS cold start latency
- [ ] Broker setup guide: how users register API keys with Angel, Dhan, Zerodha, MT5 and add to ZettaBridge
- [ ] Incident rollback path: how to revert to `BROKER_MODE=mock` in under 5 minutes (update task def + force deploy)
- [ ] Support email / contact channel ready

---

## Beta Go / No-Go Checklist

All items must be `[x]` before opening beta to real traders.

**Infrastructure:**
- [ ] AWS NAT EIP confirmed and stable
- [ ] NAT EIP whitelisted with Angel One and Dhan portals
- [ ] RDS + Redis live and accessible only from private VPC
- [ ] Secrets Manager / KMS wired to ECS tasks (no plaintext secrets)
- [ ] Backend ECS service healthy (CloudWatch logs clean)
- [ ] Dashboard ECS service healthy
- [ ] ALB + HTTPS + DNS working for both api/app domains
- [ ] HTTP → HTTPS redirect working

**Application:**
- [ ] CORS verified from dashboard domain (no browser errors)
- [ ] Auth / JWT flow works on AWS domain
- [ ] Email verification links use AWS API domain
- [ ] Stripe checkout, portal, and webhook verified on AWS URLs
- [ ] `make staging-functional-test` passes on AWS staging

**Broker live path:**
- [ ] One real Pro credential created and verified (`valid: true`)
- [ ] Tiny live order placed end-to-end (`status=submitted`, real broker order ID)
- [ ] SL/TP bracket order verified
- [x] CancelOrder verified (`DELETE /v1/webhooks/:id/trades/:tradeId` — fully built)
- [ ] SEBI Algo-ID confirmed in broker order

**Operations:**
- [ ] Rotate token and suspend user drills complete
- [ ] Known limitations doc written
- [ ] Broker setup guide ready
- [ ] Rollback plan documented and tested
- [ ] Support channel ready

**Signed:** ___________________   Date: ___________

---

## After beta opens (Week 2–3)

- Monitor CloudWatch for 5xx rate, order_rejected count, queue depth
- Collect trader feedback on broker setup friction, dashboard UX, SL/TP behavior
- Fix P0/P1 issues before opening to more traders
- Track live order volume and broker quota headroom
