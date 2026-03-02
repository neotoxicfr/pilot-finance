import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');
const BASE_URL = __ENV.BASE_URL || 'http://localhost:3000';

export const options = {
  scenarios: {
    // Ramp up to 50 VUs, hold, ramp down
    stress: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 20 },   // Ramp up
        { duration: '1m', target: 50 },     // Peak
        { duration: '30s', target: 0 },     // Ramp down
      ],
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<1000', 'p(99)<2000'],
    http_req_failed: ['rate<0.05'],
    errors: ['rate<0.05'],
  },
};

export default function () {
  // Mix of public + authenticated requests
  const pages = ['/login', '/privacy', '/legal', '/api/health'];
  const page = pages[Math.floor(Math.random() * pages.length)];

  const res = http.get(`${BASE_URL}${page}`);
  check(res, {
    [`${page} returns 2xx/3xx`]: (r) => r.status < 400,
  }) || errorRate.add(1);

  sleep(Math.random() * 2 + 0.5);
}
