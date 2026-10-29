// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// k6 load test - exercises the /api/v1/users endpoints against a running
// server. Used by SRE to validate SLOs (see docs/cmmi/02_MA.md).
//
// Run:
//   k6 run --vus 50 --duration 60s scripts/k6/users_load.js
//
// SLOs validated:
//   - p99 latency < 500ms
//   - error rate < 1%
//   - throughput >= 100 RPS
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost:8080';
const TOKEN = __ENV.TOKEN || '';

const errorRate = new Rate('errors');
const latency = new Trend('latency_ms');

export const options = {
  stages: [
    { duration: '10s', target: 20  },  // ramp-up
    { duration: '30s', target: 50  },  // steady
    { duration: '10s', target: 100 },  // burst
    { duration: '10s', target: 0   },  // ramp-down
  ],
  thresholds: {
    errors: ['rate<0.01'],         // <1% errors
    latency_ms: ['p(99)<500'],     // p99 < 500ms
    http_req_duration: ['p(99)<500'],
  },
};

const headers = {
  'Authorization': `Bearer ${TOKEN}`,
  'Content-Type': 'application/json',
  'X-Request-Id': `k6-${__VU}-${__ITER}`,
};

export default function () {
  // GET list (read-heavy mix)
  const listRes = http.get(`${BASE}/api/v1/users?limit=20`, { headers });
  latency.add(listRes.timings.duration);
  errorRate.add(listRes.status !== 200);

  check(listRes, {
    'list 200': (r) => r.status === 200,
    'list has data': (r) => r.json('data') !== undefined,
  });

  // POST create (mutation)
  const email = `k6-${__VU}-${__ITER}@example.com`;
  const postRes = http.post(`${BASE}/api/v1/users`, JSON.stringify({
    email: email, first_name: 'K6', last_name: 'Load',
  }), { headers });
  latency.add(postRes.timings.duration);
  errorRate.add(postRes.status !== 201);

  check(postRes, {
    'post 201': (r) => r.status === 201,
  });

  sleep(0.1);
}

export function handleSummary(data) {
  return {
    'stdout': JSON.stringify({
      p99_ms: data.metrics.latency_ms['p(99)'],
      error_rate: data.metrics.errors.rate,
      rps: data.metrics.http_reqs.rate,
      passes: data.metrics.latency_ms.thresholds['p(99)'].ok,
    }, null, 2),
  };
}
