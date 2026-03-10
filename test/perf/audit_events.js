import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 10,
  duration: '90s',
  thresholds: {
    http_req_duration: ['p(95)<400'],
    http_req_failed: ['rate<0.01'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const TOKEN = __ENV.API_TOKEN || '';
const PROJECT_ID = __ENV.PROJECT_ID || 'perf-project';

export default function () {
  const headers = {
    Authorization: `Bearer ${TOKEN}`,
    'Content-Type': 'application/json',
  };

  const res = http.get(`${BASE_URL}/v1/audit-events?project_id=${PROJECT_ID}&limit=100`, { headers });
  check(res, {
    'status is 200': (r) => r.status === 200,
  });

  sleep(0.25);
}
