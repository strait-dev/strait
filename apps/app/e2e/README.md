# E2E Tests

Playwright tests for the Strait dashboard. Run these to verify end-to-end user flows before submitting changes to the frontend.

## Prerequisites

- Docker Compose running (`cd apps/strait && docker compose up -d`)
- Go backend running with the same `INTERNAL_SECRET` as the app
  (`cd apps/strait && go run ./cmd/strait --mode all`)
- Infisical configured (`infisical init` in the repo root)
- Playwright browsers installed (`cd apps/app && bunx playwright install chromium`)

## Running locally

```bash
# Run all tests (starts dev server automatically via Infisical)
bun run e2e

# Run the backend-backed core dashboard suite
bun run e2e:core

# Run with browser visible
bun run e2e:headed

# Run with Playwright UI
bun run e2e:ui

# Run a specific test file
bun run e2e -- tests/auth/login.spec.ts

# Run tests matching a pattern
bun run e2e -- --grep "dashboard"
```

For local backend-backed dashboard work, it is often useful to run Strait on a
non-default port so the app and Go service use the exact same Infisical-exported
secret:

```bash
# Terminal 1: dependencies
cd apps/strait && docker compose up -d

# Terminal 2: Go API + worker. Requires apps/app/.dev.vars from Infisical export.
cd apps/strait
set -a; source ../app/.dev.vars; set +a
PORT=18081 \
GRPC_PORT=15052 \
DATABASE_URL='postgres://strait:strait@localhost:15432/strait?sslmode=disable' \
REDIS_URL='redis://localhost:16379' \
CLICKHOUSE_EXPORT_ENABLED=false \
ALLOW_PRIVATE_ENDPOINTS=true \
go run ./cmd/strait --mode all

# Terminal 3: core dashboard e2e
cd apps/app
STRAIT_API_URL=http://localhost:18081 bun run e2e:core
```

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `E2E_USER_EMAIL` | Yes | Email for the test user |
| `E2E_USER_PASSWORD` | Yes | Password for the test user |
| `AUTH_DATABASE_URL` | Yes | PostgreSQL connection for auth DB |
| `INTERNAL_SECRET` | Yes | Go API internal secret |
| `STRAIT_API_URL` | No | Go API URL (default: `http://localhost:8080`) |
| `E2E_FAKE_ENDPOINT_URL` | No | External fake job endpoint. When omitted, global setup starts a local endpoint automatically. |
| `E2E_FAKE_ENDPOINT_PUBLIC_HOST` | No | Hostname published for the managed fake endpoint (default: `localtest.me`, which resolves to loopback). |
| `BETTER_AUTH_SECRET` | Yes | Better Auth secret |
| `BETTER_AUTH_URL` | Yes | Better Auth URL |

These are injected automatically by Infisical in local development.

## Adding new tests

1. Create a spec file in the appropriate `e2e/tests/<category>/` directory
2. Import the test fixture: `import { test, expect } from "../../fixtures";`
   The fixture re-exports Playwright's `test` and `expect` plus custom helpers: an authenticated `page` (already logged in) and an `api` helper for creating test data.
3. Tests using the `chromium` project get an authenticated `page` with `storageState`. The `chromium` project uses stored authentication so tests start already logged in -- no manual login step needed.
4. Auth tests should use `test.use({ storageState: { cookies: [], origins: [] } })` to clear session
5. Use the `api` fixture to seed test data via the Go API
6. Prefer `TestDataFactory` for backend resources so cleanup is registered with the test
7. Clean up seeded data in `test.afterAll`
8. Add `data-testid` attributes to components as needed
## Test structure

```
e2e/
  playwright.config.ts    # Playwright config
  global-setup.ts         # Signup test user, save storageState
  global-teardown.ts      # Delete test user
  fixtures/
    api.ts                # API helper for data seeding
    auth.ts               # Authenticated test fixture
    index.ts              # Re-exports
  tests/
    auth/                 # Login, signup, session
    dashboard/            # Metrics, charts, activity
    core-dashboard/       # Backend-backed dashboard, jobs, runs, and ops surfaces
    harness/              # Local service and setup smoke tests
    jobs/                 # Jobs list, job detail
    runs/                 # Runs list, run detail
    workflows/            # Workflows list
    schedules/            # Schedules list
    webhooks/             # Webhooks list
    billing/              # Billing tabs
    settings/             # Account, project settings
    onboarding/           # New user flow
    dlq/                  # Failed run review
    events-logs/          # Events, logs
    navigation/           # Sidebar, routing
    error-states/         # 404, auth errors
```

## CI

Tests run in CI with 4 parallel shards. Each shard gets its own Postgres database
(`strait_e2e_{run_id}_{shard}`) and Redis DB index to prevent conflicts.

## Debugging

```bash
# View the HTML report from the last run
bunx playwright show-report

# Run with trace recording
bun run e2e -- --trace on

# Debug a specific test
bun run e2e -- --debug tests/auth/login.spec.ts
```
