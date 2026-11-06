// Load test of the main DCS read and write endpoints (DCS-NFR-PER-01,
// DCS-NFR-PER-02). tests/load/run_load.py sets the environment and runs this
// script in the grafana/k6 container against the kind BDD stack.
//
// The SRS gives no load level and no response-time limit. The values below
// are the limits of this test, sized for one GitHub-hosted runner (4 vCPU,
// 16 GB) that also runs kind, Postgres, IPFS, Keycloak, Hydra, ORCE, DSS and
// pdf-core: a constant 20 reads/s and 2 contract creations/s for DURATION.
// The test fails when one limit is exceeded:
//   - read  p95 < 500 ms, p99 < 1500 ms
//   - write p95 < 2000 ms
//   - less than 1 % failed requests
//   - no dropped iterations (the arrival rate was held)
import http from 'k6/http';
import { check } from 'k6';
import exec from 'k6/execution';

const base = __ENV.DCS_BASE_URL;
const params = {
  headers: { Authorization: `Bearer ${__ENV.DCS_TOKEN}`, 'Content-Type': 'application/json' },
};
const readRate = Number(__ENV.READ_RATE || 20);
const writeRate = Number(__ENV.WRITE_RATE || 2);
const duration = __ENV.DURATION || '60s';

const scenarios = {
  read: {
    executor: 'constant-arrival-rate',
    exec: 'read',
    rate: readRate,
    timeUnit: '1s',
    duration,
    preAllocatedVUs: 20,
    maxVUs: Number(__ENV.MAX_VUS || 100),
  },
};
// WRITE_RATE=0 (the capacity steps of run_load.py) runs reads only.
if (writeRate > 0) {
  scenarios.write = {
    executor: 'constant-arrival-rate',
    exec: 'write',
    rate: writeRate,
    timeUnit: '1s',
    duration,
    preAllocatedVUs: 10,
    maxVUs: 50,
  };
}

export const options = {
  scenarios,
  thresholds: {
    'http_req_duration{kind:read}': ['p(95)<500', 'p(99)<1500'],
    ...(writeRate > 0 ? { 'http_req_duration{kind:write}': ['p(95)<2000'] } : {}),
    http_req_failed: ['rate<0.01'],
    dropped_iterations: ['count<1'],
  },
  summaryTrendStats: ['avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
};

export function read() {
  const tagged = { ...params, tags: { kind: 'read' } };
  const r =
    exec.scenario.iterationInTest % 2 === 0
      ? http.get(`${base}/contract/retrieve/${__ENV.CONTRACT_DID}`, tagged)
      : http.get(`${base}/contract/search?state=DRAFT`, tagged);
  check(r, { 'read 200': (res) => res.status === 200 });
}

export function write() {
  const body = JSON.stringify({ template_did: __ENV.TEMPLATE_DID, parties: JSON.parse(__ENV.PARTIES) });
  const r = http.post(`${base}/contract/create`, body, { ...params, tags: { kind: 'write' } });
  check(r, { 'write 200': (res) => res.status === 200 });
}
