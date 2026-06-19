# Strait Local Dogfood

The dogfood suite is a local release-readiness workflow for the Strait
dashboard and service. It runs against real local infrastructure and exercises
the product through the browser first.

## Runtime

`bun run dogfood` starts or reuses:

- Postgres 18 on `localhost:15432`
- Redis 8 on `localhost:16379`
- the Go Strait backend in `all` mode on `localhost:18082`
- the gRPC listener on `localhost:15053`
- the Vite dashboard on `localhost:5173`
- the Playwright fake customer HTTP endpoint
- a real local gRPC worker process for the worker-mode dogfood journey
- an owner/admin browser session and a limited member browser session

The default mode reuses the local database and containers so batches can be run
repeatedly without paying full setup cost each time. Use `--reset` when local
state becomes confusing.

If `DATABASE_URL` or `REDIS_URL` point at custom local ports, the harness expects
those services to already be listening. If the default ports are not listening,
the harness starts or reuses Docker containers named
`strait-app-e2e-postgres` and `strait-app-e2e-redis`.

The limited member defaults to `e2e-limited@strait.local` and the same password
as `E2E_USER_PASSWORD`. Override with `E2E_LIMITED_USER_EMAIL` and
`E2E_LIMITED_USER_PASSWORD` when needed.

```bash
cd apps/app
bun run dogfood
bun run dogfood -- --list
bun run dogfood -- --doctor
bun run dogfood -- jobs
bun run dogfood -- grpc
bun run dogfood -- jobs runs workflows
bun run dogfood -- --reset smoke
```

Use `bun run dogfood -- --doctor` when a batch cannot start. It checks the
required local tools, default Postgres and Redis ports, Docker availability,
the Strait API health endpoint, and the dashboard URL without building or
starting the full stack.

## Batches

| Batch | Purpose |
|---|---|
| `smoke` | Proves the local API, dashboard, fake endpoint, auth, project context, stats, and analytics endpoints are usable. |
| `dashboard` | Dashboard health, metrics, activity, and core dashboard views. |
| `jobs` | Browser-first job create/edit/delete and HTTP job operation plus job list/detail coverage. |
| `grpc` | Browser-created worker-mode job dispatched through a real local gRPC worker process. |
| `runs` | Browser-first run list/detail, status visibility, retry, cancel, and run history. |
| `schedules` | Browser-first schedule create/edit/delete plus scheduled job list/detail operations. |
| `workflows` | Browser-first simple workflow create/delete plus workflow list/detail, DAG, trigger, and pause/resume. |
| `operations` | Browser-first webhook, DLQ, event, log, search, and status-filter operations. |
| `navigation` | Sidebar navigation plus search, cursor pagination, and status-filter checks across seeded core surfaces. |
| `webhooks` | Webhook subscription operations and delivery visibility. |
| `dlq` | Dead-letter inspection and retry operations. |
| `events` | Events and logs visibility for runs and workflows. |
| `settings` | Browser-first API key create/revoke, project switching/isolation, organization, project, and settings surfaces. |
| `permissions` | Owner versus limited-member behavior, direct denied actions, and project isolation. |
| `visual` | Responsive layout, charts, keyboard accessibility, and theme checks. |
| `all` | All release-readiness batches except visual-only checks. |

## Coverage Matrix

| Area | Dogfood contract | Status |
|---|---|---|
| Harness | One command starts or reuses the local stack and runs selected batches. | Implemented; pending live local stack pass |
| Dashboard health | Browser verifies dashboard health cards and API verifies stats and analytics response shapes. | Implemented; pending live local stack pass |
| HTTP jobs | Browser creates, edits core fields, triggers, pauses, resumes, deletes, inspects, and verifies backend state. | Implemented; pending live local stack pass |
| gRPC worker jobs | Harness starts a real local worker, browser triggers worker-mode job, UI shows result. | Implemented; pending live local stack pass |
| Runs | Browser verifies completed and failed runs, opens run details, retries failed rows, and cancels an active run. | Implemented; pending live local stack pass |
| Schedules | Browser creates, edits core fields, triggers, pauses, resumes, and deletes scheduled jobs. | Implemented; pending live local stack pass |
| Workflows | Browser creates and deletes a simple workflow. Trigger/pause/resume and DAG coverage remain in focused workflow specs. | Implemented; pending live local stack pass |
| Webhooks | Browser creates, searches, views, sends a test delivery to the fake endpoint, and deletes subscriptions; focused specs cover delivery attempts. | Implemented; pending live local stack pass |
| DLQ | Browser inspects dead-letter runs, retries rows, and discards supported rows. | Implemented; pending live local stack pass |
| Events/logs | Browser confirms event and log surfaces reflect real workflow event activity with search and status filters. | Implemented; pending live local stack pass |
| API keys | Browser creates and revokes project API keys, including one-time secret visibility and backend list verification. | Implemented; pending live local stack pass |
| Projects/settings | Browser switches between same-organization projects and confirms active-project data isolation. | Implemented; pending live local stack pass |
| Permissions | Owner can mutate; limited member can read core surfaces but cannot mutate jobs, runs, schedules, workflows, webhooks, DLQ, API keys, or settings and sees clean errors. | Implemented; pending live local stack pass |
| Command actions | Browser opens command-palette quick actions and verifies advertised job, schedule, and workflow create paths show working forms. | Implemented; pending live local stack pass |
| Navigation/filters/pagination | Browser navigates via the sidebar and verifies search/status-filter/cursor-pagination behavior against seeded jobs, runs, schedules, workflows, DLQ, events, logs, and webhooks. | Implemented; pending live local stack pass |

## Exclusions

This dogfood pass intentionally excludes Stripe checkout, hosted billing flows,
email magic links, SSO, passkeys, marketing/contact forms, and third-party
provider callback wiring. Those need separate staged or provider-contract
checks.

## Completion Standard

The dogfood run passes only when browser actions and backend state agree. API
helpers may seed prerequisites and assert final state, but they should not
replace the user action being tested.
