// k6 Load Testing Script for Distributed KV Store
// Usage: k6 run --vus 50 --duration 30s tests/k6_load_test.js
// Install: brew install k6  (or see https://k6.io/docs/get-started/installation/)

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// Custom metrics
const putLatency = new Trend('put_latency', true);
const getLatency = new Trend('get_latency', true);
const deleteLatency = new Trend('delete_latency', true);
const errorRate = new Rate('error_rate');
const opsCounter = new Counter('total_operations');

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

// Test scenarios
export const options = {
  scenarios: {
    // Smoke test - basic validation
    smoke: {
      executor: 'constant-vus',
      vus: 1,
      duration: '10s',
      startTime: '0s',
      tags: { test_type: 'smoke' },
    },
    // Load test - normal expected load
    load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '15s', target: 20 },   // Ramp up
        { duration: '30s', target: 20 },   // Steady state
        { duration: '10s', target: 0 },    // Ramp down
      ],
      startTime: '15s',
      tags: { test_type: 'load' },
    },
    // Stress test - beyond normal capacity
    stress: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '10s', target: 50 },
        { duration: '20s', target: 50 },
        { duration: '10s', target: 100 },
        { duration: '20s', target: 100 },
        { duration: '10s', target: 0 },
      ],
      startTime: '70s',
      tags: { test_type: 'stress' },
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
    put_latency: ['p(95)<200'],
    get_latency: ['p(95)<100'],
    error_rate: ['rate<0.05'],
  },
};

// Helper: generate random key
function randomKey() {
  return `test:k6:${__VU}:${Date.now()}:${Math.random().toString(36).slice(2, 8)}`;
}

// Helper: generate random value
function randomValue(size = 256) {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  let result = '';
  for (let i = 0; i < size; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}

export default function () {
  const key = randomKey();
  const value = randomValue();
  const headers = { 'Content-Type': 'application/json' };

  // PUT operation
  const putRes = http.put(`${BASE_URL}/api/v1/kv/${key}`, JSON.stringify({ value }), { headers });
  putLatency.add(putRes.timings.duration);
  opsCounter.add(1);

  const putOk = check(putRes, {
    'PUT status is 201': (r) => r.status === 201,
    'PUT response has success': (r) => {
      try { return JSON.parse(r.body).success === true; } catch { return false; }
    },
  });
  errorRate.add(!putOk);

  // GET operation
  const getRes = http.get(`${BASE_URL}/api/v1/kv/${key}`, { headers });
  getLatency.add(getRes.timings.duration);
  opsCounter.add(1);

  const getOk = check(getRes, {
    'GET status is 200': (r) => r.status === 200,
    'GET returns correct value': (r) => {
      try { return JSON.parse(r.body).value === value; } catch { return false; }
    },
  });
  errorRate.add(!getOk);

  // DELETE operation (50% of the time)
  if (Math.random() < 0.5) {
    const delRes = http.del(`${BASE_URL}/api/v1/kv/${key}`, null, { headers });
    deleteLatency.add(delRes.timings.duration);
    opsCounter.add(1);

    const delOk = check(delRes, {
      'DELETE status is 200': (r) => r.status === 200,
    });
    errorRate.add(!delOk);
  }

  // Health check (10% of iterations)
  if (Math.random() < 0.1) {
    const healthRes = http.get(`${BASE_URL}/api/v1/health`);
    check(healthRes, {
      'Health returns 200': (r) => r.status === 200,
      'Health reports healthy': (r) => {
        try { return JSON.parse(r.body).healthy === true; } catch { return false; }
      },
    });
  }

  sleep(0.1); // 100ms think time
}

// Batch operations test
export function batchTest() {
  const headers = { 'Content-Type': 'application/json' };
  const items = [];
  for (let i = 0; i < 100; i++) {
    items.push({ key: `batch:${__VU}:${i}`, value: randomValue(128) });
  }

  // Batch PUT
  const batchPutRes = http.post(`${BASE_URL}/api/v1/kv/batch/put`, JSON.stringify({ items }), { headers });
  check(batchPutRes, {
    'Batch PUT succeeds': (r) => r.status === 200,
    'All items written': (r) => {
      try { return JSON.parse(r.body).success_count === 100; } catch { return false; }
    },
  });

  // Batch GET
  const keys = items.map(i => i.key);
  const batchGetRes = http.post(`${BASE_URL}/api/v1/kv/batch/get`, JSON.stringify({ keys }), { headers });
  check(batchGetRes, {
    'Batch GET succeeds': (r) => r.status === 200,
  });

  sleep(1);
}
