# orchestrator

A production-grade Go job orchestrator inspired by [Trigger.dev](https://trigger.dev). Accepts job definitions via REST API, queues runs in Postgres using `SELECT FOR UPDATE SKIP LOCKED` (no external broker), dispatches them via HTTP to user endpoints, and handles retries with smart strategies. Ships as a single Go binary.

## Features

- **Job engine** — 13-state FSM, priority queues, batch dequeue, idempotency keys, cron scheduling, job versioning
- **Smart retry** — exponential, linear, fixed, or custom per-attempt delays with jitter and 1-hour cap
- **Workflow DAGs** — fan-in/fan-out, step conditions, template variables, output transforms, sub-workflows, approval gates
- **SDK endpoints** — logging, heartbeats, progress, checkpoints, usage tracking, continuation, child job spawning
- **Webhooks** — HMAC-SHA256 signed, retry with backoff, dead letter queue
- **CDC** — real-time Postgres WAL change capture via [Sequin Stream](https://sequinstream.com)
- **Observability** — OpenTelemetry tracing, Prometheus metrics, structured JSON logging, SSE streaming
- **Security** — dual auth (internal secret + per-project API keys), JWT run tokens, SSRF protection, rate limiting
- **Cost budgets** — per-run and daily project cost limits with AI model usage tracking
- **Environments** — endpoint URL overrides per environment (staging/production routing)
- **Health scoring** — per-job success rate, latency, and composite health metrics
- **CLI** — 48+ commands, TUI dashboard, YAML manifests, 7 output formats, shell completion

## Quick Start

```bash
# Clone and start infrastructure
git clone https://github.com/leonardomso/orchestrator.git
cd orchestrator
docker compose up -d

# Set environment
export DATABASE_URL=postgres://orchestrator:orchestrator@localhost:5432/orchestrator?sslmode=disable
export REDIS_URL=redis://localhost:6379
export INTERNAL_SECRET=your-secret-here
export JWT_SIGNING_KEY=your-jwt-key-must-be-at-least-32-chars-long

# Run (api + worker in one process)
go run ./cmd/orchestrator --mode all
```

Migrations run automatically on startup. Use `--mode api` or `--mode worker` for separate scaling.

## Tech Stack

- **Go 1.26** — single binary, no runtime dependencies
- **PostgreSQL 18** — primary store, job queue (SKIP LOCKED), workflow state
- **Redis 8** — pub/sub for SSE streaming and CDC event publishing
- **Sequin** — CDC (Change Data Capture) from Postgres WAL
- **pgx/v5** — raw SQL with connection pooling (no ORM)
- **chi/v5** — lightweight HTTP router with middleware
- **golang-migrate/v4** — embedded SQL migrations
- **robfig/cron/v3** — cron expression scheduling
- **golang-jwt/v5** — JWT run token auth
- **OpenTelemetry** — distributed tracing (OTLP) + Prometheus metrics
- **viper** — environment variable configuration
- **testcontainers-go** — Postgres/Redis containers for integration tests

## Project Structure

```
orchestrator/
├── cmd/orchestrator/       # CLI commands (48+ commands, one file per group)
├── internal/
│   ├── api/                # HTTP handlers, middleware, auth, routes, SSRF validation
│   ├── cdc/                # Sequin CDC consumer, table handlers, HTTP client
│   ├── config/             # Environment configuration, feature flags (viper)
│   ├── dbscan/             # Shared database row scanning
│   ├── domain/             # Types, job run FSM, workflow FSM, error types
│   ├── e2e/                # End-to-end integration tests
│   ├── pubsub/             # Publisher interface + Redis implementation
│   ├── queue/              # Queue interface + Postgres SKIP LOCKED
│   ├── scheduler/          # Cron, delayed poller, stale run reaper, retention
│   ├── store/              # Database queries (pgx, raw SQL, no ORM)
│   ├── telemetry/          # OpenTelemetry tracing + Prometheus metrics
│   ├── testutil/           # Test factories, test DB/Redis helpers
│   ├── worker/             # Executor, pool, backoff strategies, heartbeat, webhook dispatch
│   └── workflow/           # DAG validation, engine, step callback, conditions, templates
├── migrations/             # 47 SQL migrations (embedded via go:embed)
├── docker-compose.yml      # Postgres 18 + Redis 8 + Sequin for development
├── Dockerfile              # Multi-stage Go 1.26 build
├── fly.toml                # Fly.io deployment config
├── .golangci.yml           # golangci-lint v2 config (18 linters)
└── .github/workflows/      # CI: lint + test
```

## Running Tests

```bash
go test ./...                                                        # Unit tests
go test -race ./...                                                  # Unit tests + race detector
go test -tags integration -race ./internal/store/... ./internal/queue/...  # Integration tests (Docker required)
go test -tags integration -race ./internal/e2e/...                   # E2E tests (Docker required)
golangci-lint run ./...                                              # Lint (18 linters)
go build ./...                                                       # Build
```

## Documentation

Full documentation is available in the `docs/` directory, powered by [Mintlify](https://mintlify.com).

| Section | Description |
|---------|-------------|
| [Introduction](docs/introduction.mdx) | Overview, key features, and getting started |
| [Quick Start](docs/quickstart.mdx) | Step-by-step setup guide |
| [Architecture](docs/architecture.mdx) | System design, component interactions, data flow |
| **Concepts** | |
| [Jobs](docs/concepts/jobs.mdx) | Job definitions, versioning, health scoring |
| [Runs](docs/concepts/runs.mdx) | 13-state FSM, lifecycle, events |
| [Workflows](docs/concepts/workflows.mdx) | DAG orchestration, step types, conditions |
| [Scheduling](docs/concepts/scheduling.mdx) | Cron, delayed execution, retention |
| [Retry Strategies](docs/concepts/retry-strategies.mdx) | Exponential, linear, fixed, custom |
| [Webhooks](docs/concepts/webhooks.mdx) | HMAC signing, delivery, dead letter queue |
| [Environments](docs/concepts/environments.mdx) | Endpoint routing per environment |
| [Cost Budgets](docs/concepts/cost-budgets.mdx) | Per-run and daily cost limits |
| [CDC](docs/concepts/cdc.mdx) | Sequin CDC, WAL streaming, table handlers |
| **Guides** | |
| [Authentication](docs/guides/authentication.mdx) | Internal secret, API keys, JWT run tokens |
| [Deployment](docs/guides/deployment.mdx) | Docker, Fly.io, scaling strategies |
| [Security](docs/guides/security.mdx) | SSRF protection, rate limiting, encryption |
| [Workflow Approvals](docs/guides/workflow-approvals.mdx) | Human-in-the-loop approval gates |
| [Job Dependencies](docs/guides/job-dependencies.mdx) | Inter-job dependency management |
| [Job Groups](docs/guides/job-groups.mdx) | Logical job grouping |
| [Idempotency](docs/guides/idempotency.mdx) | Deduplication and idempotency keys |
| [Debug Bundles](docs/guides/debug-bundles.mdx) | AI-powered run diagnostics |
| [SDK Integration](docs/guides/sdk-integration.mdx) | SDK endpoints for job executors |
| **Configuration** | |
| [Environment Variables](docs/configuration/environment-variables.mdx) | All config fields, defaults, feature flags |
| [Database](docs/configuration/database.mdx) | Connection pooling, migrations, indexes |
| **Development** | |
| [Contributing](docs/development/contributing.mdx) | Setup, code style, PR guidelines |
| [Testing](docs/development/testing.mdx) | Unit, integration, E2E, fuzz, benchmarks |
| [Database Schema](docs/development/database-schema.mdx) | Tables, columns, relationships |
| **API Reference** | |
| [API Overview](docs/api-reference/overview.mdx) | Authentication, pagination, rate limits |
| [OpenAPI Spec](docs/openapi.yaml) | Machine-readable API specification |
| **CLI Reference** | |
| [CLI Overview](docs/cli/overview.mdx) | Installation, configuration, output formats |
| [CLI Reference](CLI.md) | Complete CLI reference (48+ commands) |
