# Load test tooling (5B.1)

[k6](https://grafana.com/docs/k6/latest/) scenarios for webhook ingest on staging.

## Prerequisites

- [k6](https://grafana.com/docs/k6/latest/set-up/install-k6/) on WSL/laptop: `sudo bash deploy/scripts/install-k6.sh`
- Dedicated load-test webhooks (do **not** reuse functional-test users)

## One-time setup

```bash
# staging-aws.env must have STAGING_ADMIN_EMAIL + STAGING_ADMIN_PASSWORD
./deploy/loadtest/setup-loadtest-webhook.sh
```

This writes `deploy/loadtest/env.local` with:

| Token | Webhook | Purpose |
|-------|---------|---------|
| `LOADTEST_WEBHOOK_TOKEN` | loadtest-ingest | smoke / ramp / target / soak / bracket |
| `LOADTEST_DEDUP_WEBHOOK_TOKEN` | loadtest-dedup | dedup replay (409 on identical payload) |
| `LOADTEST_RL_FREE_TOKEN` | loadtest-rl-1 | rate-limit-free (`rate_limit_per_sec=1`) |
| `LOADTEST_RL_PAID_TOKEN` | loadtest-rl-10 | rate-limit-paid (`rate_limit_per_sec=10`) |

Setup upgrades the load-test user to **pro_plus** (falls back to **pro** if migration 046 is not applied).  
With **pro** only, all tokens point at the single live webhook — dedup/rate-limit scenarios need migration 046 for dedicated webhooks.

## Run scenarios

```bash
./deploy/loadtest/run-loadtest.sh smoke              # 10 VU, 1 min  — quick sanity
./deploy/loadtest/run-loadtest.sh ramp               # ramp to 20 VU peak
./deploy/loadtest/run-loadtest.sh target             # 40 VU, 3 min  — staging gate (0.5 vCPU)
./deploy/loadtest/run-loadtest.sh dedup              # identical payload → 202 then 409
./deploy/loadtest/run-loadtest.sh soak               # 100 VU, 15 min
./deploy/loadtest/run-loadtest.sh bracket            # SL/TP ingest path
./deploy/loadtest/run-loadtest.sh rate-limit-free    # webhook 1 req/s → expect 429s
./deploy/loadtest/run-loadtest.sh rate-limit-paid    # webhook 10 req/s → expect 429s
```

Or via Makefile:

```bash
make loadtest-smoke
make loadtest-staging              # default: target scenario
make loadtest-staging SCENARIO=dedup
```

Results JSON: `deploy/loadtest/results/*-summary.json`

---

## Staging benchmark (0.5 vCPU Fargate)

| Parameter | Value |
|-----------|-------|
| VUs | **40** (default target) |
| Duration | 3 min |
| Sleep between requests | 0.1 s |
| ECS vCPU | 0.5 |

**Pass criteria (staging smoke / target)**

| Metric | Threshold |
|--------|-----------|
| `http_req_failed` | < 5% |
| `http_req_duration p99` | < 2000 ms |
| `checks` (202 + queued:true) | > 95% |

---

## During a run

**Grafana:** SSH tunnel → `http://localhost:3001` → **ZettaBridge Overview** → set time range **Last 15 min** → filter **Cluster** to `aws-ecs-staging` when load-testing ECS.

| Load-test scenario | Panels that should move |
|--------------------|-------------------------|
| smoke / target / ramp / soak / bracket | **Ingest accepted (in range)** and **Trades submitted (in range)** rise; **Trades rejected (in range)** stays flat on mock broker |
| dedup | **Ingest deduplicated (in range)** and **Dedup hits (in range)** rise; preflight must show 202→409 before k6 starts |
| rate-limit-free / rate-limit-paid | **Ingest rate limited /s** rises (not **Ingest rejected**) |
| guard failures | **Ingest rejected (in range)** and **Ingest rejected /s** rise |
| target / soak at saturation | **Ingest queue full /s** may spike; k6 also reports 503 |
| any ingest load test | **Trades persisted /s** and **Execution HTTP latency** rise as workers call adapters (`op=place_order`) |

**Note:** Redeploy Core after metric label changes so `/metrics` exposes `rate_limited` and `queue_full`. Re-sync the dashboard JSON to the monitoring VM (`./deploy/deploy-aws-ecs/scripts/monitoring-enable-aws-scrape.sh` or `sync-monitoring-to-vm.sh`).

**After run:** Re-run `make functional-test-staging` after a full target/soak run.

## Out of scope

k6 exercises `POST /v1/webhook/:token` ingest only. P&L and WebSocket are covered by functional tests.  
Stripe billing is not load-tested.
