// Webhook per-second rate-limit enforcement load test.
//
// Fires signals faster than the webhook rate_limit_per_sec and measures 429s.
//
// rate-limit-free  → webhook with rate_limit_per_sec=1  (LOADTEST_RL_FREE_TOKEN)
// rate-limit-paid  → webhook with rate_limit_per_sec=10 (LOADTEST_RL_PAID_TOKEN)
//
// Env: LOADTEST_BASE_URL
//      LOADTEST_RL_FREE_TOKEN / LOADTEST_RL_PAID_TOKEN (from setup-loadtest-webhook.sh)
//      LOADTEST_PLAN        (free | paid — selects token + thresholds)
//      LOADTEST_VUS, LOADTEST_DURATION, LOADTEST_SYMBOL

import http from 'k6/http';
import { check } from 'k6';
import { Counter, Rate } from 'k6/metrics';

const baseURL = (__ENV.LOADTEST_BASE_URL || 'http://localhost:8080').replace(/\/$/, '');
const plan = __ENV.LOADTEST_PLAN || 'free';
const symbol = __ENV.LOADTEST_SYMBOL || 'RELIANCE';
const duration = __ENV.LOADTEST_DURATION || '30s';

const token = plan === 'paid'
  ? (__ENV.LOADTEST_RL_PAID_TOKEN || __ENV.LOADTEST_WEBHOOK_TOKEN || '')
  : (__ENV.LOADTEST_RL_FREE_TOKEN || __ENV.LOADTEST_WEBHOOK_TOKEN || '');

if (!token) {
  throw new Error('LOADTEST_RL_*_TOKEN or LOADTEST_WEBHOOK_TOKEN is required');
}

const defaultVUs = plan === 'paid' ? 20 : 5;
const vus = parseInt(__ENV.LOADTEST_VUS || String(defaultVUs), 10);

const status202 = new Counter('zb_rl_status_202');
const status429 = new Counter('zb_rl_status_429');
const statusOther = new Counter('zb_rl_status_other');
const rateLimitedRate = new Rate('zb_rl_rate_limited');

// With rate_limit_per_sec=1 and 5 VUs bursting, most requests should be 429.
// With rate_limit_per_sec=10 and 20 VUs bursting, expect a meaningful 429 fraction.
const minExpected429Rate = plan === 'paid' ? 0.30 : 0.50;

export const options = {
  scenarios: {
    rate_limit_burst: {
      executor: 'constant-vus',
      vus,
      duration,
      gracefulStop: '5s',
    },
  },
  thresholds: {
    zb_rl_rate_limited: [`rate>${minExpected429Rate}`],
    zb_rl_status_other: ['count<10'],
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

let iter = 0;

export default function () {
  iter++;
  const url = `${baseURL}/v1/webhook/${token}`;
  const payload = JSON.stringify({
    action: iter % 2 === 0 ? 'BUY' : 'SELL',
    symbol,
    lot: 1,
    comment: `rl-test-${__VU}-${__ITER}`,
  });

  const res = http.post(url, payload, {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'rate_limit_enforce' },
  });

  const is429 = res.status === 429;
  rateLimitedRate.add(is429 ? 1 : 0);

  switch (res.status) {
    case 202:
      status202.add(1);
      break;
    case 429:
      status429.add(1);
      break;
    default:
      statusOther.add(1);
      break;
  }

  check(res, {
    'status is 202 or 429': (r) => r.status === 202 || r.status === 429,
  });
}

function metricVal(metrics, name, key) {
  const m = metrics[name];
  if (!m || !m.values) return null;
  return m.values[key];
}

export function handleSummary(data) {
  const m = data.metrics;
  const total202 = metricVal(m, 'zb_rl_status_202', 'count') || 0;
  const total429 = metricVal(m, 'zb_rl_status_429', 'count') || 0;
  const totalOther = metricVal(m, 'zb_rl_status_other', 'count') || 0;
  const total = total202 + total429 + totalOther;
  const ratePct = total > 0 ? ((total429 / total) * 100).toFixed(1) : '0.0';

  const lines = [
    '',
    `=== ZettaBridge rate-limit enforcement (webhook rate_limit_per_sec, plan=${plan}) ===`,
    `vus=${vus} duration=${duration}`,
    `total requests : ${total}`,
    `202 accepted   : ${total202}`,
    `429 rate-limited: ${total429}  (${ratePct}%)`,
    `other (errors) : ${totalOther}`,
    '',
    total429 > 0
      ? `✓ Webhook rate limiter is active — ${ratePct}% of requests were throttled.`
      : '✗ No 429s observed — re-run setup-loadtest-webhook.sh (needs pro_plus for dedicated RL webhooks).',
  ];

  if (plan === 'free') {
    lines.push('', 'Expected: >50% throttled with rate_limit_per_sec=1 and 5 VUs bursting.');
  } else {
    lines.push('', 'Expected: >30% throttled with rate_limit_per_sec=10 and 20 VUs bursting.');
  }

  const text = lines.join('\n') + '\n';
  const out = JSON.stringify(data, null, 2);
  return {
    stdout: text + out,
    [`${__ENV.LOADTEST_RESULTS_DIR || 'results'}/rate-limit-enforce-summary.json`]: out,
  };
}
