import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const loginDuration = new Trend('login_duration', true);

// Configuration
const BASE_URL = __ENV.BASE_URL || 'http://localhost:3000';

export const options = {
  scenarios: {
    // Smoke test: 5 VUs for 30s
    smoke: {
      executor: 'constant-vus',
      vus: 5,
      duration: '30s',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],  // 95th < 500ms, 99th < 1s
    http_req_failed: ['rate<0.01'],                    // <1% errors
    errors: ['rate<0.01'],
  },
};

export default function () {
  // GET /login (public page)
  const loginPage = http.get(`${BASE_URL}/login`);
  check(loginPage, {
    'login page 200': (r) => r.status === 200,
    'login has form': (r) => r.body.includes('input'),
  }) || errorRate.add(1);

  sleep(0.5);

  // POST /login (auth flow)
  const start = Date.now();
  const loginRes = http.post(`${BASE_URL}/login`, {
    email: __ENV.TEST_EMAIL || 'loadtest@test.local',
    password: __ENV.TEST_PASSWORD || 'LoadTest1!secure',
  }, {
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    redirects: 0,
  });
  loginDuration.add(Date.now() - start);

  const loggedIn = loginRes.status === 200 || loginRes.status === 302 || loginRes.status === 303;
  check(loginRes, {
    'login succeeds': () => loggedIn,
  }) || errorRate.add(1);

  // If logged in, hit authenticated pages
  if (loggedIn) {
    const dashboard = http.get(`${BASE_URL}/`);
    check(dashboard, {
      'dashboard 200': (r) => r.status === 200,
    }) || errorRate.add(1);

    sleep(0.3);

    const accounts = http.get(`${BASE_URL}/accounts`);
    check(accounts, {
      'accounts 200': (r) => r.status === 200,
    }) || errorRate.add(1);

    sleep(0.3);

    const settings = http.get(`${BASE_URL}/settings`);
    check(settings, {
      'settings 200': (r) => r.status === 200,
    }) || errorRate.add(1);
  }

  // GET /api/health (always public)
  const health = http.get(`${BASE_URL}/api/health`);
  check(health, {
    'health 200': (r) => r.status === 200,
  }) || errorRate.add(1);

  sleep(1);
}

export function handleSummary(data) {
  return {
    stdout: textSummary(data, { indent: '  ', enableColors: true }),
  };
}

// k6 built-in text summary
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.1.0/index.js';
