# Strait — Go Service

The core backend for Strait. This service handles the REST API, job execution, workflow orchestration, code deployments, and observability. If you're contributing to the Go backend, this is where you work.

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
Client -> chi router -> Huma middleware (auth, rate limit, idempotency)
    -> handler -> store -> PostgreSQL
                -> pub/sub -> subscribers
```
**Job execution path:**
```
POST /v1/jobs/{id}/runs -> enqueue -> worker/executor
    -> compute runtime (Docker / Kubernetes / HTTP)
    -> run result -> store -> webhook delivery
```
**Workflow path:**
```
POST /v1/workflows/{id}/trigger -> WorkflowEngine
    -> step execution (worker) -> StepCallback -> next step
    -> rollback workflows on failure
```
**Code deployment path:**
```
POST /v1/code-deployments -> tarball upload (object store)
    -> build orchestrator -> BuildKit -> push to registry
    -> K8s job pods pick up new image
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
golangci-lint run --timeout=10m ./...
```

See `internal/config/config.go` for every supported env var and its default value.

---

## Internal packages

| Package | Purpose |
|---|---|
| `api` | HTTP API layer (Huma v2 + chi). Handlers, middleware, routing. |
| `store` | All database access via `pgx/v5`. Typed methods per table, custom error types. |
| `domain` | Core types, enums, and status transitions shared across all packages. Zero external dependencies. |
| `worker` | Claims and executes job runs. Manages concurrency, retries, and graceful drain. |
| `workflow` | Durable multi-step workflow engine. Sequential, conditional, loop, and compensation steps. |
| `compute` | Container execution abstraction. Docker, Kubernetes, and HTTP backends. |
| `build` | Code deployment pipeline: source tarball to Docker image to registry. |
| `registry` | Container registry abstraction. ECR and generic Docker Registry v2. |
| `pubsub` | In-process event broadcasting (run status, workflow steps, build logs for SSE). |
| `webhook` | Durable webhook delivery with retries. Failed deliveries are sent to a review queue for inspection. |
| `billing` | Usage-based quota enforcement and Stripe integration (cloud edition only). |
| `clickhouse` | Optional analytics backend. Batched event export with Postgres fallback. |
| `telemetry` | Initializes traces, metrics, profiling, and error reporting. |
| `config` | Single config struct loaded from environment variables via `aconfig`. |
| `objectstore` | S3-compatible storage (AWS S3 / Cloudflare R2) for tarballs and log archives. |
| `scheduler` | Cron-style job triggering with timezone support. |
| `cdc` | Change data capture via Sequin. Propagates Postgres row changes to pub/sub topics. |
| `cache` | In-memory TTL cache (Otter) for hot-path reads like quota snapshots. |
| `testutil` | Test helpers: real Postgres containers, in-memory Redis, domain object factories. |

For detailed package documentation, implementation patterns, and contribution recipes, see [AGENTS.md](../../AGENTS.md) and the [architecture docs](../../apps/docs/architecture.mdx).

---

## Configuration

All configuration is via environment variables. `internal/config/config.go` is the single source of truth — every supported variable, its default, and inline documentation live there. For a quick reference of available variables, see the root `.env.example`.

---

## Database and migrations

Migrations live in `migrations/` as numbered `.up.sql` / `.down.sql` pairs, embedded in the binary via `//go:embed`. Migrations run automatically on startup.

**Rules:**

1. Create the next numbered pair: `000NNN_your_change.{up,down}.sql`
2. Write idempotent SQL — use `IF NOT EXISTS`, `IF EXISTS`
3. Always write the down migration and verify it cleanly reverts the up
4. Never modify an existing migration — always add a new one

---

## Testing

```bash
go test ./...                       # unit tests
go test -race ./...                 # race detector — run before every PR
go test -tags=integration ./...     # integration tests (real Postgres via testcontainers)
```

| File pattern | Purpose |
|---|---|
| `*_test.go` | Standard unit and integration tests |
| `*_adversarial_test.go` | Security and abuse-case tests: auth bypass, injection, oversized input |
| `*_fuzz_test.go` | Go fuzz tests for input parsing |

Mocks are generated via `moq` — run `go generate ./...` after interface changes.

---

## Observability

- **Metrics:** Prometheus, scraped at `/metrics`
- **Traces:** OTLP export to the OpenTelemetry Collector, forwarded to Grafana Tempo
- **Logs:** Structured JSON via `slog` (colorized in dev via `tint`). Level controlled by `LOG_LEVEL`.
- **Errors:** Sentry for unexpected errors needing human attention. Not for validation or expected 4xx.

---

## Contributing

### Code conventions

- Raw SQL with `pgx/v5` — no ORM, no query builders
- Structured concurrency with `sourcegraph/conc` (safe goroutine lifecycle) and `alitto/pond` (bounded worker pools)
- Wrap errors with `%w` and enough context to trace the call site
- No global state — everything wired through constructors and functional options (`WithLogger`, `WithMetrics`, `WithStore`, etc.)
- No emojis in code, comments, logs, or commit messages

### Commit style

Conventional commits are required: `type(scope): summary`

```
feat(api): add webhook retry endpoint
fix(worker): drain in-flight runs on SIGTERM
test(build): cover orchestrator dispatch paths
refactor(store): extract run query helpers
```

Never use `--no-verify`. If a lefthook check fails, fix the underlying issue.
