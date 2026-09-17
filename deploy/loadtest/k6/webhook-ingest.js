// Constant-VU webhook ingest load (5B.1).
//
// Env: LOADTEST_BASE_URL, LOADTEST_WEBHOOK_TOKEN, LOADTEST_VUS, LOADTEST_DURATION
//      LOADTEST_SLEEP (seconds between iterations, default 0.05)
//      LOADTEST_PROFILE (smoke | load — smoke uses gentler rate + thresholds)

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';

const baseURL = (__ENV.LOADTEST_BASE_URL || 'http://localhost:8080').replace(/\/$/, '');
const token = __ENV.LOADTEST_WEBHOOK_TOKEN || '';
const symbol = __ENV.LOADTEST_SYMBOL || 'RELIANCE';
const vus = parseInt(__ENV.LOADTEST_VUS || '10', 10);
const duration = __ENV.LOADTEST_DURATION || '1m';
const iterSleep = parseFloat(__ENV.LOADTEST_SLEEP || '0.05');
const profile = __ENV.LOADTEST_PROFILE || 'load';

const status202 = new Counter('zb_status_202');
const status429 = new Counter('zb_status_429');
const status503 = new Counter('zb_status_503');
const statusOther = new Counter('zb_status_other');

if (!token) {
  throw new Error('LOADTEST_WEBHOOK_TOKEN is required');
}

const smokeThresholds = {
  http_req_failed: ['rate<0.02'],
  http_req_duration: ['p(99)<1000'],
  checks: ['rate>0.98'],
  zb_status_503: ['count<100'],
};

// Thresholds for cloud HTTPS (WSL → ALB → ECS Fargate, 0.5 vCPU).
// At ~270 req/s on 0.5 vCPU, tail latency regularly hits 500–1500ms — p99<2000
// is the realistic SLO for this staging size. Tighten to p99<500 when ecs_cpu ≥ 1024.
// Smoke uses p99<1000 — even at low VU count, WSL→ElastiCache dedup writes and
// WSL→Fargate TLS variance push occasional spikes past 500ms.
const loadThresholds = {
  http_req_failed: ['rate<0.05'],
  http_req_duration: ['p(99)<2000'],
  checks: ['rate>0.95'],
};

export const options = {
  scenarios: {
    webhook_ingest: {
      executor: 'constant-vus',
      vus,
      duration,
      gracefulStop: '10s',
    },
  },
  thresholds: profile === 'smoke' ? smokeThresholds : loadThresholds,
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

function recordStatus(status) {
  switch (status) {
    case 202:
      status202.add(1);
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
}

export default function () {
  const url = `${baseURL}/v1/webhook/${token}`;
  const payload = JSON.stringify({
    action: 'BUY',
    symbol,
    lot: 1,
    comment: `load-${__VU}-${__ITER}-${Date.now()}`,
  });

  const res = http.post(url, payload, {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'webhook_ingest' },
  });

  recordStatus(res.status);

  check(res, {
    'status is 202': (r) => r.status === 202,
    'queued true': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.queued === true;
      } catch (_) {
        return false;
      }
    },
  });

  sleep(iterSleep);
}

function metricVal(metrics, name, key) {
  const m = metrics[name];
  if (!m || !m.values) {
    return null;
  }
  return m.values[key];
}

export function handleSummary(data) {
  const m = data.metrics;
  const lines = [
    '',
    '=== ZettaBridge webhook ingest summary ===',
    `profile=${profile} vus=${vus} sleep=${iterSleep}s`,
    `http_req_failed: ${((metricVal(m, 'http_req_failed', 'rate') || 0) * 100).toFixed(2)}%`,
    `checks passed: ${((metricVal(m, 'checks', 'rate') || 0) * 100).toFixed(2)}%`,
    `p99 latency: ${(metricVal(m, 'http_req_duration', 'p(99)') || 0).toFixed(1)} ms`,
    'status breakdown:',
    `  202 accepted: ${metricVal(m, 'zb_status_202', 'count') || 0}`,
    `  429 rate limit: ${metricVal(m, 'zb_status_429', 'count') || 0}`,
    `  503 queue full: ${metricVal(m, 'zb_status_503', 'count') || 0}`,
    `  other: ${metricVal(m, 'zb_status_other', 'count') || 0}`,
  ];

  const s503 = metricVal(m, 'zb_status_503', 'count') || 0;
  const s429 = metricVal(m, 'zb_status_429', 'count') || 0;
  if (s503 > 0) {
    lines.push(
      '',
      'Hint: 503 = ingest queue saturated (QUEUE_BUFFER). Lower VUs/sleep for smoke,',
      '      or raise WORKER_COUNT/QUEUE_BUFFER on the server for target runs.',
    );
  }
  if (s429 > 0) {
    lines.push(
      '',
      'Hint: 429 = webhook rate_limit_per_sec/min or free daily cap. Re-run setup-loadtest-webhook.sh',
      '      (pro_plus user, rate_limit_per_sec: 0 on ingest webhook).',
    );
  }

  const text = lines.join('\n') + '\n';
  const out = JSON.stringify(data, null, 2);
  return {
    stdout: text + out,
    [`${__ENV.LOADTEST_RESULTS_DIR || 'results'}/webhook-ingest-summary.json`]: out,
  };
}
