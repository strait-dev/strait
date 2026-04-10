# Strait — Go Service

The core backend for Strait: a production-grade background job and workflow platform. This service exposes the REST API, executes job runs, orchestrates durable workflows, manages code deployments, and ships all observability.

## Table of contents

- [Architecture](#architecture)
- [Editions](#editions)
- [Getting started](#getting-started)
- [Internal packages](#internal-packages)
- [Configuration](#configuration)
- [Database and migrations](#database-and-migrations)
- [Testing](#testing)
- [Observability](#observability)
- [Contributing](#contributing)

---

## Architecture

The binary (`strait`) runs in one of three modes, set via `--mode`:

| Mode | What runs |
|---|---|
| `api` | HTTP server only |
| `worker` | Job execution worker only |
| `all` | API + worker in one process (default for dev and single-node deploys) |

**Request path:**

```
Client → chi router → Huma middleware (auth, rate limit, idempotency)
    → handler (internal/api/) → store (internal/store/) → PostgreSQL
                              → pub/sub (internal/pubsub/) → subscribers
```

**Job execution path:**

```
POST /v1/jobs/{id}/runs → enqueue (store) → worker/executor
    → compute runtime (Docker / Kubernetes / HTTP)
    → run result → store → webhook delivery
```

**Workflow path:**

```
POST /v1/workflows/{id}/trigger → WorkflowEngine
    → step execution (worker) → StepCallback → next step
    → saga compensation on failure
```

**Code deployment path:**

```
POST /v1/code-deployments → tarball upload (objectstore)
    → build.Orchestrator → BuildKit → push to registry
    → K8s job pods pick up new image
```

All service wiring happens in `cmd/strait/services.go`. Start there to understand how everything connects.

---

## Editions

Two editions compile from the same codebase, controlled at compile time by a build tag. The edition is immutable once built.

| Edition | Build command | Description |
|---|---|---|
| Community | `go build ./...` | Self-hosted, open-source. Docker and K8s compute, no billing. |
| Cloud | `go build -tags cloud ./...` | SaaS at strait.dev. Managed execution, multi-region, billing, advanced analytics. |

`internal/domain/edition_community.go` and `internal/domain/edition_cloud.go` each implement `ParseEdition()` and feature gate constants. Only one compiles per build.

When adding a cloud-only feature: gate it with `domain.CurrentEdition().IsCloud()` and put the implementation in a `_cloud.go` file with `//go:build cloud`.

---

## Getting started

**Prerequisites:** Go 1.26, Docker (testcontainers + local infra), `golangci-lint`.

```bash
# Start local infrastructure (Postgres, Redis, Sequin)
docker compose up -d

# Build — community edition
cd apps/strait
go build ./...

# Build — cloud edition
go build -tags cloud ./...

# Run unit tests
go test ./...

# Run integration tests (spins up real Postgres via testcontainers)
go test -tags=integration ./...

# Lint
golangci-lint run --timeout=5m ./...
```

Secrets and environment variables are managed in Doppler:

```bash
doppler secrets --project strait --config dev
```

See `internal/config/config.go` for every supported env var and its default value.

---

## Internal packages

### `api/`

HTTP API layer. Built on [Huma v2](https://github.com/danielgtaylor/huma) and [chi](https://github.com/go-chi/chi). Every resource has its own file: `jobs.go`, `runs.go`, `workflows.go`, `code_deployments.go`, etc.

Handlers are thin: validate input, call the store or a domain service, return. Business logic belongs in domain packages, not in handlers.

Cross-cutting concerns wired in `server.go`:

- **Auth**: JWT + API key verification, RBAC enforcement, tenant isolation
- **Rate limiting**: per-IP and per-project limits (chi httprate)
- **Idempotency**: deduplication via Redis for mutating endpoints
- **Instrumentation**: OTEL span + Prometheus counter on every handler

When adding a handler: register it in `server.go`, write adversarial tests (`*_adversarial_test.go`) covering auth bypass and injection, and keep the OpenAPI schema in `schemas/strait.json` in sync.

### `store/`

All database access. Uses `pgx/v5` directly — no ORM. Every table has a corresponding file with typed methods. The `DBTX` interface allows passing either a `*pgxpool.Pool` or a `pgx.Tx`, enabling transactions across multiple store calls.

Custom error types (`ErrJobNotFound`, `ErrRunNotFound`, etc.) live here. Handlers catch these and translate them to HTTP 404/409.

The `analytics/` subpackage queries ClickHouse (with Postgres fallback) for the 32 dashboard endpoints under `/v1/analytics/`.

Do not put business logic in the store. If a method needs to enforce a rule (e.g. max runs per project), that belongs in a domain service or the worker, not here.

### `domain/`

Core types and enums shared across all packages. `Job`, `JobRun`, `Workflow`, `WorkflowStep`, `CodeDeployment`, `Project`, `Organization`, `Plan`, etc.

Status transitions are typed constants defined here. If you add a new status, add it to the domain enum and write a migration to add it to the Postgres enum type.

Edition gating is here. `domain` has zero non-stdlib dependencies — keep it that way.

### `worker/`

Dequeues `JobRun` records and executes them. The `Executor` struct controls concurrency via a per-project bulkhead, dispatches to a compute runtime, tracks in-flight runs, and handles graceful drain on shutdown.

Retry logic is here: exponential backoff, adaptive backoff by error type, max attempt limits.

After each run completes, the executor calls `workflow.StepCallback` to advance workflow state.

### `workflow/`

Durable multi-step workflow engine. `WorkflowEngine` dequeues pending workflow runs, resolves the next step, dispatches a job run for it, and waits for `StepCallback`.

Supports: sequential steps, conditional branching, loops, event wait steps, approval steps, and compensation (saga rollback via `compensation_job_id` on steps).

Max chain depth is enforced here (see `domain.MaxWorkflowDepth`). Never bypass this limit — deeply nested workflows are a DoS vector.

### `compute/`

Container execution abstraction. Three backends:

| Backend | Selected when |
|---|---|
| `docker` | Local dev, self-hosted community |
| `k8s` | Production; `COMPUTE_RUNTIME=k8s` |
| `http` | Serverless/managed execution; cloud only |

The `k8s` backend creates Kubernetes `Job` resources and polls pod phase until terminal. Injects env vars, secrets, and the run payload as `STRAIT_PAYLOAD`. Image pull policy is configurable via `IMAGE_PULL_POLICY` (default `IfNotPresent`).

Security: image URIs are validated against `ALLOWED_REGISTRIES` before any execution. No shell injection, no path traversal. See `validate.go`.

### `build/`

Code deployment pipeline: source tarball → Docker image → container registry.

`Builder` coordinates the full pipeline:
1. Downloads the source tarball from object storage
2. Generates a runtime-specific Dockerfile (templates in `build/runtimes/`)
3. Submits to BuildKit via gRPC
4. Pushes the resulting image to the configured registry
5. Returns `BuildResult{ImageURI, Digest, FinishedAt}`

`Orchestrator` is the background loop that claims pending `CodeDeployment` records and drives `Builder`. Wire Prometheus metrics in with `WithOrchestratorMetrics(m)`.

`DeploymentGC` periodically purges expired pending/failed deployments. Wire metrics with `WithGCMetrics(m)`.

### `registry/`

Container registry abstraction. Two implementations:

- **ECR** (`ecr.go`): AWS Elastic Container Registry. Uses the standard AWS credential chain (`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / role ARN). Auto-creates repositories with immutable tags and scan-on-push enabled.
- **Generic** (`registry.go`): Docker Registry v2 API for any self-hosted registry.

Selected at startup based on `CONTAINER_REGISTRY_TYPE`.

### `pubsub/`

In-process pub/sub for event broadcasting. Topics include run status changes, workflow step transitions, and build log streaming (used for SSE endpoints). The `Publisher` interface has a Redis-backed production implementation and an in-memory implementation for tests.

### `webhook/`

Durable webhook delivery. Persists `WebhookDelivery` records to Postgres, then a background worker dequeues and delivers them. Retries with exponential backoff. Dead-letter queue after exhausted attempts.

Every delivery is HMAC-SHA256 signed. Circuit breaker per destination URL. Metrics for delivery success and failure rates.

### `billing/`

Usage-based quota enforcement (cloud edition only). Plans map to resource limits: concurrent runs, team members, webhook endpoints, log retention. Quotas are checked synchronously in handlers before run enqueue.

Stripe integration handles subscription lifecycle. Stripe webhooks sync plan state via CDC. In community edition, all quota checks are no-ops.

### `clickhouse/`

Optional analytics backend. Exports structured event records (run completions, step transitions, approvals) to ClickHouse in batches. The `Exporter` flushes on size or interval.

Never required for correctness — if ClickHouse is unavailable, analytics endpoints fall back to Postgres aggregate queries. Do not add correctness logic here.

### `telemetry/`

Initializes all observability:

- **Traces**: OTLP export to the OTel Collector (`OTEL_EXPORTER_OTLP_ENDPOINT`)
- **Metrics**: Prometheus exporter, scraped at `/metrics`
- **Profiling**: Pyroscope continuous profiling
- **Errors**: Sentry with automatic secret scrubbing

The `Metrics` struct in `metrics.go` holds every counter, histogram, and gauge used across the service. Add new metrics in `InitMetrics()` and add the field to `Metrics`. Use dot-notation names (`strait.subsystem.metric_name`) — the Prometheus SDK converts them to underscores.

Note: `Int64Counter` names get a `_total` suffix appended by the Prometheus SDK (e.g. `strait.code_deploy.total` → `strait_code_deploy_total_total`). Account for this in PromQL queries and Grafana panels.

### `config/`

Single `Config` struct loaded from environment variables via `aconfig`. All defaults are declared here. Sensitive fields are redacted in log output.

When adding a config field: set a sane default, document it inline with a comment. Do not add required fields without defaults unless the service genuinely cannot start without the value.

### `objectstore/`

S3-compatible object storage (AWS S3 or Cloudflare R2). Used for source tarballs uploaded before code deployment builds, and for job run log archives.

Selected via `OBJECT_STORE_TYPE`. R2 requires `OBJECT_STORE_FORCE_PATH_STYLE=true`.

### `scheduler/`

Cron-style job triggering. Manages scheduled job definitions with cron expressions, persisting next-run times. Uses `robfig/cron/v3` internally. Handles timezone offsets.

### `cdc/`

Change Data Capture via [Sequin](https://sequinstream.com). Listens for Postgres row changes and feeds them to internal pub/sub topics. Used to propagate database state changes to the workflow and event trigger systems without polling.

### `cache/`

In-memory cache backed by [Otter](https://github.com/maypok86/otter). Used for hot-path reads: project quota snapshots, plan limits, allowlist lookups. TTL-based invalidation. Never used for correctness — cached data may be stale by up to one TTL window.

### `testutil/`

Test helpers shared across all packages:

- `TestDB`: spins up a real PostgreSQL container (testcontainers), runs all migrations, returns a pool. Call `CleanTables(t, db)` between tests.
- `TestRedis`: miniredis in-memory Redis instance.
- `Factory`: generates domain objects with defaults for testing.

Only used in test files. The `integration` build tag gates `TestDB` so unit tests don't pull in testcontainers.

---

## Configuration

All configuration is via environment variables. See `internal/config/config.go` for the full list. Key groups:

| Group | Key variables |
|---|---|
| Database | `DATABASE_URL` |
| Redis | `REDIS_URL` |
| Mode | `SERVER_MODE` (api / worker / all) |
| Compute | `COMPUTE_RUNTIME`, `K8S_NAMESPACE`, `IMAGE_PULL_POLICY` |
| Registry | `CONTAINER_REGISTRY_TYPE`, `ECR_REGION`, `ECR_REGISTRY_ID`, `ECR_ROLE_ARN` |
| Object store | `OBJECT_STORE_TYPE`, `OBJECT_STORE_BUCKET`, `OBJECT_STORE_ENDPOINT`, `OBJECT_STORE_ACCESS_KEY`, `OBJECT_STORE_SECRET_KEY` |
| BuildKit | `BUILDKIT_ADDRESS`, `BUILDKIT_ADDRESSES`, `BUILDKIT_NAMESPACE` |
| Observability | `OTEL_EXPORTER_OTLP_ENDPOINT`, `SENTRY_DSN`, `PYROSCOPE_URL` |
| Billing | `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` |
| Security | `JWT_SECRET`, `ENCRYPTION_KEY`, `ALLOWED_REGISTRIES` |

In production, all secrets are managed in Doppler and synced to the `strait-env` Kubernetes secret. Never commit real credentials.

---

## Database and migrations

Migrations live in `migrations/` as numbered `.up.sql` / `.down.sql` pairs, embedded in the binary via `//go:embed`.

```
migrations/000001_create_jobs.up.sql
migrations/000001_create_jobs.down.sql
...
```

Migrations run automatically on startup with a Postgres advisory lock to prevent races during rolling deploys.

**Rules:**

1. Create the next numbered pair: `000NNN_your_change.{up,down}.sql`
2. Write idempotent SQL — use `IF NOT EXISTS`, `IF EXISTS`
3. Always write the down migration and verify it cleanly reverts the up
4. Never modify an existing migration — always add a new one

---

## Testing

### Unit tests

```bash
go test ./...
go test -race ./...   # run with race detector before every PR
```

### Integration tests

```bash
go test -tags=integration ./...
```

Spin up real Docker containers via testcontainers. Required for `store/`, `queue/`, and `pubsub/` packages. Run in CI.

### Test file conventions

| File pattern | Purpose |
|---|---|
| `*_test.go` | Standard unit and integration tests |
| `*_adversarial_test.go` | Security and abuse-case tests: auth bypass, injection, oversized input |
| `*_fuzz_test.go` | Go fuzz tests for input parsing |

Every handler in `api/` must have a happy-path test and an adversarial test covering auth, authorization, and all user-controlled input fields.

### Mocks

Generated via `moq` (`//go:generate` directives in test files). Regenerate after interface changes:

```bash
go generate ./...
```

Do not hand-write mocks.

---

## Observability

### Metrics

All Prometheus metrics are on the `Metrics` struct in `internal/telemetry/metrics.go`. Scraped at `/metrics`.

Naming convention: `strait.<subsystem>.<metric>` in the meter call. The SDK converts dots to underscores and appends `_total` to counters. Always verify the rendered name in Prometheus before writing PromQL.

Grafana dashboards are in `k8s/`:
- `grafana-dashboard.json` — platform-wide overview
- `grafana-dashboard-code-deployments.json` — build pipeline deep-dive

### Traces

OTLP traces export to the in-cluster OTel Collector (`otel-collector.strait.svc:4317`), forwarded to Grafana Tempo. HTTP handler spans follow `METHOD /path`. Internal spans follow `package.FunctionName`.

### Logs

Structured JSON via `slog`. In development, `tint` adds color. Level controlled by `LOG_LEVEL` (default `info`). Never log tokens, credentials, or payload content.

### Errors

Sentry is initialized in `cmd/strait/main.go`. Use `sentry.CaptureException(err)` only for unexpected errors needing human attention — not for validation errors or expected 4xx responses.

---

## Contributing

### Code conventions

- Raw SQL with `pgx/v5` — no ORM, no query builders
- Structured concurrency with `sourcegraph/conc` and `alitto/pond`
- Wrap errors with `%w` and enough context to trace the call site
- No global state — everything wired through constructors and functional options (`WithLogger`, `WithMetrics`, `WithStore`, etc.)
- No emojis in code, comments, logs, or commit messages

### Adding a new resource

1. Add domain types in `internal/domain/`
2. Write the migration
3. Add store methods in `internal/store/`
4. Add API handlers in `internal/api/`
5. Register routes and wire the handler in `internal/api/server.go`
6. Add an OTEL span and a Prometheus counter in the handler
7. Write unit tests, adversarial tests, and at least one integration test

### Adding a new metric

1. Add the field to `Metrics` in `internal/telemetry/metrics.go`
2. Initialize it in `InitMetrics()` following the existing pattern
3. Inject `metrics *telemetry.Metrics` via a functional option into the package that emits it
4. Guard every metric call with `if m.metrics != nil`
5. Add or update the relevant Grafana dashboard panel in `k8s/`

### Commit style

Conventional commits are required: `type(scope): summary`

```
feat(api): add webhook retry endpoint
fix(worker): drain in-flight runs on SIGTERM
test(build): cover orchestrator dispatch paths
refactor(store): extract run query helpers
```

Never use `--no-verify`. If a lefthook check fails, fix the underlying issue.

### Pull requests

- One logical change per PR
- All CI checks must pass (lint, unit tests, security scan) before merge
- Integration tests run in CI — do not merge a failing integration suite
- PR descriptions should explain what changed and why
