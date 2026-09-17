// Ramping webhook ingest: ramp to peakVUs over 5 minutes (5B.1 pilot).
//
// Env: LOADTEST_BASE_URL, LOADTEST_WEBHOOK_TOKEN
//      LOADTEST_VUS      — peak VUs (default 50; use 200 when ecs_cpu >= 1024)
//      LOADTEST_SLEEP    — sleep between iterations in seconds (default 0.05)

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';

const baseURL = (__ENV.LOADTEST_BASE_URL || 'http://localhost:8080').replace(/\/$/, '');
const token = __ENV.LOADTEST_WEBHOOK_TOKEN || '';
const symbol = __ENV.LOADTEST_SYMBOL || 'RELIANCE';
const peakVUs = parseInt(__ENV.LOADTEST_VUS || '50', 10);
const iterSleep = parseFloat(__ENV.LOADTEST_SLEEP || '0.05');

if (!token) {
  throw new Error('LOADTEST_WEBHOOK_TOKEN is required');
}

const status202 = new Counter('zb_status_202');
const status429 = new Counter('zb_status_429');
const status503 = new Counter('zb_status_503');
const statusOther = new Counter('zb_status_other');

export const options = {
  scenarios: {
    webhook_ramp: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: Math.round(peakVUs * 0.25) },
        { duration: '2m', target: peakVUs },
        { duration: '1m', target: peakVUs },
        { duration: '1m', target: 0 },
      ],
      gracefulRampDown: '30s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(99)<2000'],
    checks: ['rate>0.95'],
    // zb_status_503 is tracked for observability but not gated here — a ramp
    // test intentionally probes the server's breaking point, so 503s at peak
    // are expected. http_req_failed rate<0.05 catches true saturation.
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

function recordStatus(status) {
  switch (status) {
    case 202: status202.add(1); break;
    case 429: status429.add(1); break;
    case 503: status503.add(1); break;
    default:  statusOther.add(1); break;
  }
}

export default function () {
  const url = `${baseURL}/v1/webhook/${token}`;
  const payload = JSON.stringify({
    action: 'BUY',
    symbol,
    lot: 1,
    comment: `ramp-${__VU}-${__ITER}-${Date.now()}`,
  });

  const res = http.post(url, payload, {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'webhook_ramp' },
  });

  recordStatus(res.status);

  check(res, {
    'status is 202': (r) => r.status === 202,
  });

  sleep(iterSleep);
}

function metricVal(metrics, name, key) {
  const m = metrics[name];
  if (!m || !m.values) return null;
  return m.values[key];
}

export function handleSummary(data) {
  const m = data.metrics;
  const lines = [
    '',
    '=== ZettaBridge webhook ramp summary ===',
    `peak_vus=${peakVUs} sleep=${iterSleep}s`,
    `http_req_failed: ${((metricVal(m, 'http_req_failed', 'rate') || 0) * 100).toFixed(2)}%`,
    `checks passed:   ${((metricVal(m, 'checks', 'rate') || 0) * 100).toFixed(2)}%`,
    `p99 latency:     ${(metricVal(m, 'http_req_duration', 'p(99)') || 0).toFixed(1)} ms`,
    'status breakdown:',
    `  202 accepted:  ${metricVal(m, 'zb_status_202', 'count') || 0}`,
    `  429 rate limit: ${metricVal(m, 'zb_status_429', 'count') || 0}`,
    `  503 queue full: ${metricVal(m, 'zb_status_503', 'count') || 0}`,
    `  other:          ${metricVal(m, 'zb_status_other', 'count') || 0}`,
  ];

  const s503 = metricVal(m, 'zb_status_503', 'count') || 0;
  const s429 = metricVal(m, 'zb_status_429', 'count') || 0;
  if (s503 > 0) {
    lines.push(
      '',
      'Hint: 503 = queue saturated. Lower LOADTEST_VUS or raise WORKER_COUNT/QUEUE_BUFFER.',
      `      Current peak: ${peakVUs} VUs. Try LOADTEST_VUS=${Math.round(peakVUs * 0.5)}.`,
    );
  }
  if (s429 > 0) {
    lines.push(
      '',
      'Hint: 429 = webhook rate_limit_per_sec/min is set. Re-run setup-loadtest-webhook.sh.',
    );
  }

  const text = lines.join('\n') + '\n';
  const out = JSON.stringify(data, null, 2);
  return {
    stdout: text + out,
    [`${__ENV.LOADTEST_RESULTS_DIR || 'results'}/webhook-ramp-summary.json`]: out,
  };
}
