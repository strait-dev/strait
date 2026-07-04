# E2E Tests

Playwright tests for the Strait dashboard. Run these to verify end-to-end user flows before submitting changes to the frontend.

## Source Of Truth

| Area | Source |
|---|---|
| Playwright config | `playwright.config.ts` |
| Authenticated fixtures | `fixtures/` |
| Test data helpers | `fixtures/api.ts` |

## Prerequisites

- Docker Compose running (`cd apps/strait && docker compose up -d`)
- Go backend running with the same `INTERNAL_SECRET` as the app
  (`cd apps/strait && go run ./cmd/strait --mode all`)
- Root `.env` and `apps/app/.env` populated from the example files
- Playwright browsers installed (`cd apps/app && bunx playwright install chromium`)

When Playwright starts its own dev server locally (i.e. not in CI), the
`webServer` command in `playwright.config.ts` runs `bun run db:migrate:bun`
before starting `bun run dev`. Better Auth migration failures surface here,
before any test runs.

## Running locally

```bash
# Run all tests (starts dev server automatically)
bun run e2e

# Run the backend-backed core dashboard suite
bun run e2e:core

# Focused suites
bun run e2e:smoke
bun run e2e:regression
bun run e2e:settings
bun run e2e:visual

# Run with browser visible
bun run e2e:headed

# Run with Playwright UI
bun run e2e:ui

# Run a specific test file
bun run e2e -- tests/auth/login.spec.ts

# Run tests matching a pattern
bun run e2e -- --grep "dashboard"
```

For backend-backed dashboard work, start the Go backend explicitly before
running Playwright. Keep the app and backend pointed at the same Postgres,
Redis, and `INTERNAL_SECRET` values:

```bash
# Terminal 1: dependencies and Go API
cd apps/strait
docker compose up -d
export DATABASE_URL=postgres://strait:strait@localhost:15432/strait?sslmode=disable
export REDIS_URL=redis://localhost:16379
export INTERNAL_SECRET=<32-plus-char-local-secret>
export JWT_SIGNING_KEY=<32-plus-char-local-jwt-key>
export ENCRYPTION_KEY=<32-byte-local-encryption-key>
export ALLOW_PRIVATE_ENDPOINTS=true
export WEBHOOK_REQUIRE_TLS=false
go run ./cmd/strait --mode all
```

```bash
# Terminal 2: app e2e
cd apps/app
export AUTH_DATABASE_URL=postgres://strait:strait@localhost:15432/strait?sslmode=disable
export DATABASE_URL=postgres://strait:strait@localhost:15432/strait?sslmode=disable
export REDIS_URL=redis://localhost:16379
export STRAIT_API_URL=http://localhost:8080
export INTERNAL_SECRET=<same-local-secret-as-go-api>
export BETTER_AUTH_URL=http://localhost:5173
export BETTER_AUTH_SECRET=<32-plus-char-local-auth-secret>
export E2E_USER_EMAIL=e2e-owner@example.com
export E2E_USER_PASSWORD=dogfood-local-password
bun run e2e:core

# Run a focused subset
bun run e2e -- tests/core-dashboard/webhook-deliveries.spec.ts
```

The focused suites split coverage by stability and blast radius:

- `e2e:smoke`: harness, dashboard smoke, and navigation checks.
- `e2e:regression`: real-backend jobs, runs, workflows, webhooks, DLQ, schedules, events, and logs.
- `e2e:settings`: destructive or settings-heavy account, org, project, and security flows.
- `e2e:visual`: chart rendering, responsive layout, and theme checks.

All suites run under a single Playwright project, `chromium` (Desktop
Chrome), configured in `playwright.config.ts`. "Focused suites" narrows which
spec directories run, not which browsers -- there is no cross-browser
coverage today.

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `E2E_USER_EMAIL` | Yes | Email for the test user |
| `E2E_USER_PASSWORD` | Yes | Password for the test user |
| `AUTH_DATABASE_URL` | Yes | PostgreSQL connection for auth DB |
| `INTERNAL_SECRET` | Yes | Go API internal secret |
| `STRAIT_API_URL` | No | Go API URL (default: `http://localhost:8080`) |
| `E2E_FAKE_ENDPOINT_URL` | No | External fake job endpoint. When omitted, global setup starts a local endpoint automatically. |
| `E2E_FAKE_ENDPOINT_PUBLIC_HOST` | No | Hostname published for the managed fake endpoint (default: `127.0.0.1`). |
| `BETTER_AUTH_SECRET` | Yes | Better Auth secret |
| `BETTER_AUTH_URL` | Yes | Better Auth URL |
| `EXPECT_BASE_URL` | No | Base URL global setup/teardown and Playwright's `use.baseURL` target (default: `http://localhost:5173`) |
| `E2E_LIMITED_USER_EMAIL` | No | Email for the limited-permission test member (default: `e2e-limited@example.com`) |
| `E2E_LIMITED_USER_PASSWORD` | No | Password for the limited-permission test member (default: `E2E_USER_PASSWORD`, then a hardcoded fallback) |

Set these in `.env`, `apps/app/.env`, `apps/app/.dev.vars`, or the shell that
runs Playwright.

When running the dashboard e2e tests against a local Strait backend with the
managed fake endpoint, start the backend with `ALLOW_PRIVATE_ENDPOINTS=true` so
job dispatch and webhook delivery can call the loopback test server.

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
  global-setup.ts         # Signup test users, save storageState, start the fake endpoint
  global-teardown.ts      # Delete test users, stop the fake endpoint
  setup/
    auth.ts               # Sign in and persist storageState
    db.ts                 # Ensure test users, org, project, RBAC rows exist
  support/
    auth-db.ts            # Grant the unlimited e2e plan directly against the auth DB
    env.ts                 # Load .env / .dev.vars for the Playwright process
    fake-endpoint-server.mjs # Managed local fake job endpoint
    navigation.ts          # Shared navigation helpers
    route-crashes.ts       # Route-crash assertions
    run-context.ts         # Shared run context (project/org/user ids) across tests
    test-data.ts           # TestDataFactory for seeding and cleanup
  fixtures/
    api.ts                # API helper for data seeding
    auth.ts               # Authenticated test fixture
    index.ts              # Re-exports
  tests/
    auth/                 # Login, signup, session
    dashboard/            # Metrics, charts, activity
    core-dashboard/       # Backend-backed dashboard, jobs, runs, and ops surfaces
    crud/                 # End-to-end job lifecycle
    data/                 # Cross-page state, filter combinations, webhook delivery
    dogfood/              # Local release-readiness journeys (see dogfood.md)
    harness/              # Local service and setup smoke tests
    interaction/          # Accessibility, clipboard, command palette, responsive, theme
    invitation/            # Org invitation accept flow
    jobs/                 # Jobs list, job detail
    runs/                 # Runs list, run detail
    workflows/            # Workflows list
    schedules/            # Schedules list
    webhooks/             # Webhooks list
    billing/              # Billing tabs
    settings/             # Account, project settings
    org/                   # Org settings and operations
    projects/              # Project settings and switching
    onboarding/           # New user flow
    dlq/                  # Failed run review
    events-logs/          # Events, logs
    navigation/           # Sidebar, routing
    error-states/         # 404, auth errors
    security/              # Contact form and security-sensitive flows
```

See [`dogfood.md`](./dogfood.md) for the local release-readiness workflow. Run
it with `bun run dogfood` (`scripts/dogfood-local.mjs`); it starts or reuses
Postgres, Redis, the Go backend, the dashboard, the fake customer endpoint, a
real gRPC worker, and both an owner and a limited-member browser session.

## CI

The `E2E` workflow (`.github/workflows/e2e.yml`) already exists and runs the
full suite: it provisions Postgres and Redis, builds and starts the Go
backend and the app, installs Playwright, and runs the entire test suite
sharded across 4 runners (`playwright test --shard=N/4`). It is
`workflow_dispatch`-only, so it does not run automatically on pull requests.
The open item is wiring it into required PR checks.

## Validation

Run one focused local suite while iterating, then the core suite before merging dashboard changes:

```bash
cd apps/app
bun run e2e:smoke
bun run e2e:core
```

## Debugging

```bash
# View the HTML report from the last run
bunx playwright show-report

# Run with trace recording
bun run e2e -- --trace on

# Debug a specific test
bun run e2e -- --debug tests/auth/login.spec.ts
```
