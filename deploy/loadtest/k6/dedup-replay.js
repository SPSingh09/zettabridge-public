// Identical payload replay — first request queues (202), replays deduplicate (409).

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';

const baseURL = (__ENV.LOADTEST_BASE_URL || 'http://localhost:8080').replace(/\/$/, '');
const token = __ENV.LOADTEST_DEDUP_WEBHOOK_TOKEN || __ENV.LOADTEST_WEBHOOK_TOKEN || '';
const symbol = __ENV.LOADTEST_SYMBOL || 'RELIANCE';
const vus = parseInt(__ENV.LOADTEST_VUS || '50', 10);
const duration = __ENV.LOADTEST_DURATION || '2m';

const status202 = new Counter('zb_dedup_status_202');
const status409 = new Counter('zb_dedup_status_409');
const status429 = new Counter('zb_dedup_status_429');
const status503 = new Counter('zb_dedup_status_503');
const statusOther = new Counter('zb_dedup_status_other');

if (!token) {
  throw new Error('LOADTEST_DEDUP_WEBHOOK_TOKEN or LOADTEST_WEBHOOK_TOKEN is required');
}

// Same body for every request (dedup key depends on webhook + payload hash).
const fixedPayload = JSON.stringify({
  action: 'BUY',
  symbol,
  lot: 1,
  comment: 'dedup-loadtest-fixed-payload',
});

export const options = {
  scenarios: {
    dedup_replay: {
      executor: 'constant-vus',
      vus,
      duration,
      gracefulStop: '5s',
    },
  },
  // 409 dedup responses count as http_req_failed in k6 — gate on checks only.
  thresholds: {
    checks: ['rate>0.95'],
    zb_dedup_status_409: ['count>100'],
  },
};

export default function () {
  const url = `${baseURL}/v1/webhook/${token}`;
  const res = http.post(url, fixedPayload, {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'dedup_replay' },
  });

  switch (res.status) {
    case 202:
      status202.add(1);
      break;
    case 409:
      status409.add(1);
      break;
    case 429:
      status429.add(1);
      break;
    case 503:
      status503.add(1);
      break;
    default:
      statusOther.add(1);
      break;
  }

  check(res, {
    'status is 202 or 409': (r) => r.status === 202 || r.status === 409,
    'accepted (queued or deduplicated)': (r) => {
      try {
        const body = JSON.parse(r.body);
        if (r.status === 409) {
          return body.deduplicated === true;
        }
        return body.queued === true;
      } catch (_) {
        return false;
      }
    },
  });

  sleep(0.02);
}

export function handleSummary(data) {
  const c202 = data.metrics.zb_dedup_status_202?.values?.count ?? 0;
  const c409 = data.metrics.zb_dedup_status_409?.values?.count ?? 0;
  const c429 = data.metrics.zb_dedup_status_429?.values?.count ?? 0;
  const c503 = data.metrics.zb_dedup_status_503?.values?.count ?? 0;
  const cOther = data.metrics.zb_dedup_status_other?.values?.count ?? 0;
  const checkRate = data.metrics.checks?.values?.rate ?? 0;

  const hint =
    c429 > 0
      ? 'Many 429s — dedup webhook still has rate_limit_per_min/sec; re-run run-loadtest.sh (patches limits) or setup-loadtest-webhook.sh'
      : c503 > 0
        ? 'Many 503s — queue full; dedup_window_sec may be 0 so every request enqueues'
        : c409 === 0
          ? 'No 409s — dedup not active on this webhook token'
          : 'OK';

  const summary = {
    ...data,
    dedup_replay_hint: hint,
    dedup_status_counts: { '202': c202, '409': c409, '429': c429, '503': c503, other: cOther },
    checks_pass_rate: checkRate,
  };

  const out = JSON.stringify(summary, null, 2);
  return {
    stdout: out,
    [`${__ENV.LOADTEST_RESULTS_DIR || 'results'}/dedup-replay-summary.json`]: out,
  };
}
