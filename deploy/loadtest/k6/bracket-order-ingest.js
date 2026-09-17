// Bracket order (SL/TP) ingest load test — verifies the SL/TP code-path at scale.
//
// Uses an Indian broker (Zerodha/Angel/Dhan) or MT5 webhook pointed at an MIS credential.
// Under BROKER_MODE=mock the worker handles SL/TP without calling a real broker, so this
// test is safe to run in CI or staging without live credentials.
//
// Env: LOADTEST_BASE_URL, LOADTEST_WEBHOOK_TOKEN (MIS product, sl_pts/tp_pts accepted)
//      LOADTEST_VUS, LOADTEST_DURATION, LOADTEST_SLEEP, LOADTEST_PROFILE (smoke | load)

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';

const baseURL = (__ENV.LOADTEST_BASE_URL || 'http://localhost:8080').replace(/\/$/, '');
const token = __ENV.LOADTEST_BRACKET_WEBHOOK_TOKEN || __ENV.LOADTEST_WEBHOOK_TOKEN || '';
const symbol = __ENV.LOADTEST_SYMBOL || 'RELIANCE';
const vus = parseInt(__ENV.LOADTEST_VUS || '5', 10);
const duration = __ENV.LOADTEST_DURATION || '1m';
const iterSleep = parseFloat(__ENV.LOADTEST_SLEEP || '0.1');
const profile = __ENV.LOADTEST_PROFILE || 'smoke';

if (!token) {
  throw new Error('LOADTEST_WEBHOOK_TOKEN is required');
}

const status202 = new Counter('zb_bracket_202');
const status429 = new Counter('zb_bracket_429');
const status503 = new Counter('zb_bracket_503');
const statusOther = new Counter('zb_bracket_other');

const smokeThresholds = {
  http_req_failed: ['rate<0.05'],
  http_req_duration: ['p(99)<2000'],
  checks: ['rate>0.95'],
};

const loadThresholds = {
  http_req_failed: ['rate<0.05'],
  http_req_duration: ['p(99)<2000'],
  checks: ['rate>0.95'],
};

export const options = {
  scenarios: {
    bracket_ingest: {
      executor: 'constant-vus',
      vus,
      duration,
      gracefulStop: '10s',
    },
  },
  thresholds: profile === 'smoke' ? smokeThresholds : loadThresholds,
};

const actions = ['BUY', 'SELL'];

function recordStatus(status) {
  switch (status) {
    case 202: status202.add(1); break;
    case 429: status429.add(1); break;
    case 503: status503.add(1); break;
    default:  statusOther.add(1); break;
  }
}

export default function () {
  // Alternate BUY/SELL to avoid single-direction accumulation under mock broker.
  const action = actions[__ITER % 2];

  const payload = JSON.stringify({
    action,
    symbol,
    lot: 1,
    sl_pts: 20,
    tp_pts: 30,
    comment: `bracket-load-${__VU}-${__ITER}`,
  });

  const res = http.post(`${baseURL}/v1/webhook/${token}`, payload, {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'bracket_ingest' },
  });

  recordStatus(res.status);

  check(res, {
    'status is 202': (r) => r.status === 202,
    'queued true': (r) => {
      try {
        return JSON.parse(r.body).queued === true;
      } catch (_) {
        return false;
      }
    },
  });

  sleep(iterSleep);
}

function metricVal(metrics, name, key) {
  const m = metrics[name];
  return m && m.values ? m.values[key] : null;
}

export function handleSummary(data) {
  const m = data.metrics;
  const lines = [
    '',
    '=== ZettaBridge bracket order ingest summary ===',
    `profile=${profile} vus=${vus} sleep=${iterSleep}s`,
    `http_req_failed: ${((metricVal(m, 'http_req_failed', 'rate') || 0) * 100).toFixed(2)}%`,
    `checks passed: ${((metricVal(m, 'checks', 'rate') || 0) * 100).toFixed(2)}%`,
    `p99 latency: ${(metricVal(m, 'http_req_duration', 'p(99)') || 0).toFixed(1)} ms`,
    'status breakdown:',
    `  202 accepted: ${metricVal(m, 'zb_bracket_202', 'count') || 0}`,
    `  429 rate limit: ${metricVal(m, 'zb_bracket_429', 'count') || 0}`,
    `  503 queue full: ${metricVal(m, 'zb_bracket_503', 'count') || 0}`,
    `  other: ${metricVal(m, 'zb_bracket_other', 'count') || 0}`,
  ];

  const text = lines.join('\n') + '\n';
  const out = JSON.stringify(data, null, 2);
  return {
    stdout: text + out,
    [`${__ENV.LOADTEST_RESULTS_DIR || 'results'}/bracket-order-summary.json`]: out,
  };
}
