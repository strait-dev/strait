# Performance Scenarios

These k6 scripts provide baseline load tests for authorization and audit paths.

## Prerequisites

- k6 installed
- Running Strait API
- Valid API token with required permissions

## Run

```bash
BASE_URL=http://localhost:8080 API_TOKEN=strait_xxx PROJECT_ID=proj_123 k6 run test/perf/rbac_authz.js
BASE_URL=http://localhost:8080 API_TOKEN=strait_xxx PROJECT_ID=proj_123 k6 run test/perf/audit_events.js
```

## Suggested SLO Targets

- RBAC/authz hot paths: p95 < 300ms
- Audit list endpoint: p95 < 400ms
- Request failure rate < 1%
