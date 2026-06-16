# Strait Go Service

The core backend for Strait. This service handles the REST API, job dispatch, workflow orchestration, the gRPC worker plane, and monitoring. If you are contributing to the Go backend, this is where you work.

## Table of contents

- [Architecture](#architecture)
- [Editions](#editions)
- [Getting started](#getting-started)
- [Packages](#packages)
- [Configuration](#configuration)
- [Database and migrations](#database-and-migrations)
- [Testing](#testing)
- [Monitoring](#monitoring)
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

```text
Client -> chi router -> Huma middleware (auth, rate limit, idempotency)
    -> handler -> store -> PostgreSQL
                -> pub/sub -> subscribers
```
**Job execution path (HTTP mode, default):**
```text
POST /v1/jobs/{id}/trigger -> enqueue -> worker/executor
    -> HTTP POST to job's endpoint_url (customer infra)
    -> run result -> store -> webhook delivery
```
**Job execution path (worker mode, gRPC):**
```text
POST /v1/jobs/{id}/trigger -> enqueue -> worker/executor
    -> match a connected worker (registered slugs)
    -> stream the run over the bidirectional gRPC channel
    -> worker executes the handler, streams result back
    -> run result -> store -> webhook delivery
```
**Workflow path:**
```text
POST /v1/workflows/{id}/trigger -> WorkflowEngine
    -> step execution (worker) -> StepCallback -> next step
    -> rollback workflows on failure
```

Strait does not build, push, or run customer code itself. It dispatches to
infrastructure the customer already operates: an HTTP endpoint they expose,
or a long-lived worker process that connects to the API over gRPC.

All service wiring happens in `cmd/strait/services.go`. Start there to understand how everything connects.

---

## Editions

Two editions compile from the same codebase, controlled at compile time by a build tag. The edition is immutable once built.

| Edition | Build command | Description |
|---|---|---|
| Community | `go build ./...` | Self-hosted, open-source. No billing. |
| Cloud | `go build -tags cloud ./...` | Hosted orchestrator at strait.dev (API + Postgres + Redis + scheduler + gRPC worker plane). Multi-region, Stripe billing, hosted reporting. Customer code runs on customer infra in both editions. |

`internal/domain/edition_community.go` and `internal/domain/edition_cloud.go` each implement `ParseEdition()` and feature gate constants. Only one compiles per build.

When adding a cloud-only feature: gate it with `domain.CurrentEdition().IsCloud()` and put the implementation in a `_cloud.go` file with `//go:build cloud`.

---

## Getting started

**Prerequisites:** Go 1.26, Docker (testcontainers + local infra), `golangci-lint`.

```bash
# Start local infrastructure (Postgres, Redis, Sequin).
docker compose up -d

# Build (community edition).
cd apps/strait
go build ./...

# Build (cloud edition).
go build -tags cloud ./...

# Run unit tests
go test ./...

# Run local integration shards with shared testcontainers
make test-integration-fast

# Lint
golangci-lint run --timeout=10m ./...
```

See `internal/config/config.go` for every supported env var and its default value.

## Source Of Truth

| Area | Source |
|---|---|
| Runtime wiring | `cmd/strait/services.go` |
| HTTP routes and OpenAPI registration | `internal/api/routes.go`, `internal/api/huma_registry.go`, `internal/api/huma_operations.go` |
| Configuration | `internal/config/config.go` |
| Domain states and event names | `internal/domain/types.go` |
| Database schema changes | `migrations/` |
| Customer-facing docs | `../docs/` |

---

## Packages

| Package | Purpose |
|---|---|
| `api` | HTTP API layer (Huma v2 + chi). Handlers, middleware, routing. The `api/grpc/` subpackage hosts the gRPC worker-plane server (registration, dispatch, heartbeats). |
| `store` | All database access via `pgx/v5`. Typed methods per table, custom error types. |
| `domain` | Core types, enums, execution modes (`http`, `worker`), and status transitions shared across all packages. Zero external dependencies. |
| `worker` | Claims and dispatches job runs. HTTP-mode dispatch posts to the job's endpoint URL; worker-mode dispatch streams the run to a connected worker over gRPC. Manages concurrency, retries, and graceful drain. |
| `workflow` | Durable multi-step workflow engine. Sequential, conditional, loop, and compensation steps. |
| `pubsub` | In-process event broadcasting (run status, workflow steps for SSE). |
| `webhook` | Durable webhook delivery with retries. Failed deliveries are sent to a review queue for inspection. |
| `billing` | Usage-based quota enforcement and Stripe integration (cloud edition only). |
| `clickhouse` | Optional analytics backend. Batched event export with Postgres fallback. |
| `telemetry` | Initializes traces, metrics, profiling, and error reporting. |
| `config` | Single config struct loaded from environment variables via `aconfig`. |
| `scheduler` | Cron-style job triggering with timezone support. |
| `cdc` | Change data capture via Sequin. Propagates Postgres row changes to pub/sub topics. |
| `cache` | In-memory TTL cache (Otter) for hot-path reads like quota snapshots. |
| `testutil` | Test helpers: real Postgres containers, in-memory Redis, domain object factories. |

For detailed package notes, contribution rules, and architecture context, see [AGENTS.md](../../AGENTS.md) and the [architecture docs](../../apps/docs/architecture.mdx).

---

## Configuration

All configuration is via environment variables. `internal/config/config.go` is the single source of truth. Every supported variable, its default, and its inline documentation live there. For a customer-facing reference, see `../docs/configuration/environment-variables.mdx`; for local examples, see the root `.env.example`.

---

## Database and migrations

Migrations live in `migrations/` as numbered `.up.sql` / `.down.sql` pairs, embedded in the binary via `//go:embed`. Migrations run automatically on startup.

**Rules:**

1. Create the next numbered pair: `000NNN_your_change.{up,down}.sql`
2. Write idempotent SQL. Use `IF NOT EXISTS` and `IF EXISTS`.
3. Always write the down migration and verify it cleanly reverts the up.
4. Never modify an existing migration. Always add a new one.

---

## Testing

```bash
go test ./...                       # unit tests
go test -race ./...                 # race detector. Run before every PR.
go test -tags=integration ./...     # integration tests (real Postgres via testcontainers)
```

For repeated local integration runs, keep shared Postgres and Redis containers
alive across `go test` package processes during a controlled sharded run:

```bash
make test-integration-fast              # all local integration shards
make test-integration-clean             # remove strait-test-* containers
./scripts/test-integration-fast.sh smoke # one shard
./scripts/test-integration-fast.sh db api
```

The fast runner sets `STRAIT_TEST_PERSIST_CONTAINERS=1`, `GOMAXPROCS=2`, and
`go test -p 1`, then cleans the shared containers at exit. Persistent mode is
opt-in and continues to give each test an isolated Postgres database or Redis
logical DB. Use `--keep-containers` when intentionally iterating on a small
target and you want the next command to reuse already warm containers.

| File pattern | Purpose |
|---|---|
| `*_test.go` | Standard unit and integration tests |
| `*_adversarial_test.go` | Security and abuse-case tests: auth bypass, injection, oversized input |
| `*_fuzz_test.go` | Go fuzz tests for input parsing |

Mocks are generated via `moq`. Run `go generate ./...` after interface changes.

---

## Monitoring

- **Metrics:** Prometheus, scraped at `/metrics`
- **Traces:** OTLP export to the OpenTelemetry Collector, forwarded to Grafana Tempo
- **Logs:** Structured JSON via `slog` (colorized in dev via `tint`). Level controlled by `LOG_LEVEL`.
- **Errors:** Sentry for unexpected errors needing human attention. Not for validation or expected 4xx.

---

## Contributing

### Code conventions

- Raw SQL with `pgx/v5`. No ORM, no query builders.
- Structured concurrency with `sourcegraph/conc` (safe goroutine lifecycle) and `alitto/pond` (bounded worker pools).
- Wrap errors with `%w` and enough context to trace the call site.
- No global state. Wire dependencies through constructors and functional options (`WithLogger`, `WithMetrics`, `WithStore`, ...).
- No emojis in code, comments, logs, or commit messages.

### Commit style

Conventional commits are required: `type(scope): summary`

```text
feat(api): add webhook retry endpoint
fix(worker): drain in-flight runs on SIGTERM
test(build): cover orchestrator dispatch paths
refactor(store): extract run query helpers
```

Never use `--no-verify`. If a lefthook check fails, fix the underlying issue.

## Docs Impact

When changing routes, environment variables, run states, webhook events, pricing gates, or user-visible behavior, update the docs in the same commit and run:

```bash
cd ../docs && bun run lint
```
