# E2E Tests

Playwright tests for the Strait dashboard. Run these to verify end-to-end user flows before submitting changes to the frontend.

## Prerequisites

- Docker Compose running (`cd apps/strait && docker compose up -d`)
- Go backend running (`cd apps/strait && go run ./cmd/strait --mode all`)

## Running locally

```bash
# Run all tests (starts local-first dev server automatically)
bun run e2e

# Run with browser visible
bun run e2e:headed

# Run with Playwright UI
bun run e2e:ui

# Run a specific test file
bun run e2e -- tests/auth/login.spec.ts

# Run tests matching a pattern
bun run e2e -- --grep "dashboard"
```

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `E2E_USER_EMAIL` | No | Email for the test user (defaults to `e2e@local.strait`) |
| `E2E_USER_PASSWORD` | No | Password for the test user (defaults to `e2epassword123`) |
| `AUTH_DATABASE_URL` | No | Defaults to local Docker PostgreSQL auth DB |
| `INTERNAL_SECRET` | No | Defaults to `strait-local-internal-secret-32chars` |
| `STRAIT_API_URL` | No | Go API URL (default: `http://localhost:8080`) |
| `BETTER_AUTH_SECRET` | No | Defaults to local dev Better Auth secret |
| `BETTER_AUTH_URL` | No | Defaults to `http://localhost:5173` |

These default automatically in local development through `bun run dev`.

## Adding new tests

1. Create a spec file in the appropriate `e2e/tests/<category>/` directory
2. Import the test fixture: `import { test, expect } from "../../fixtures";`
   The fixture re-exports Playwright's `test` and `expect` plus custom helpers: an authenticated `page` (already logged in) and an `api` helper for creating test data.
3. Tests using the `chromium` project get an authenticated `page` with `storageState`. The `chromium` project uses stored authentication so tests start already logged in -- no manual login step needed.
4. Auth tests should use `test.use({ storageState: { cookies: [], origins: [] } })` to clear session
5. Use the `api` fixture to seed test data via the Go API
6. Clean up seeded data in `test.afterAll`
7. Add `data-testid` attributes to components as needed
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
    jobs/                 # Jobs list, job detail
    runs/                 # Runs list, run detail
    workflows/            # Workflows list
    schedules/            # Schedules list
    webhooks/             # Webhooks list
    billing/              # Billing tabs
    settings/             # Account, project settings
    onboarding/           # New user flow
    dlq/                  # Dead letter queue
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
