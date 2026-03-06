# orchestrator

A production-grade Go job orchestrator inspired by [Trigger.dev](https://trigger.dev). Accepts job definitions via REST API, queues runs in Postgres using `SELECT FOR UPDATE SKIP LOCKED` (no external broker), dispatches them via HTTP to user endpoints, and handles retries with exponential backoff. Supports workflow DAGs with fan-in/fan-out, cron scheduling, webhooks with retry and dead letter queue, real-time SSE streaming, per-project API key auth, job versioning, and optional Sequin CDC integration. Ships as a single Go binary.

## Features

### Core Job Engine

- Job management with CRUD, cron scheduling, and JSON payload schemas
- 12-state finite state machine with validated transitions: `delayed`, `queued`, `dequeued`, `executing`, `waiting`, `completed`, `failed`, `timed_out`, `crashed`, `system_failed`, `canceled`, `expired`
- Postgres `SKIP LOCKED` queue — no Kafka, RabbitMQ, or SQS required
- HTTP dispatch with configurable timeout per job
- Retry with exponential backoff and jitter
- Idempotency keys for deduplication
- Batch dequeue for throughput
- Priority queues (higher priority runs dequeued first)
- Fan-out child jobs via SDK spawn endpoint
- Per-job run TTL (`run_ttl_secs`) for auto-expiring stale runs
- Job versioning with automatic snapshot on update

### Workflow DAGs

- Directed acyclic graph workflows with Kahn's algorithm validation
- Fan-in with atomic dependency counter (Postgres row-level lock serializes concurrent updates)
- Fan-out: multiple steps can depend on the same parent step
- Three failure policies per step: `fail_workflow`, `skip_dependents`, `continue`
- Step conditions: `step_status`, `all_of`, `any_of` (supports nesting)
- Payload merging: trigger payload + step-level payload + parent step outputs (keyed by `step_ref`)
- 6-state step FSM: `pending`, `waiting`, `running`, `completed`, `failed`, `skipped`, `canceled`
- Event-driven step progression — hooks into all 6 terminal code paths (executor, SDK, cancel, reaper)

### Observability

- OpenTelemetry distributed tracing (OTLP export)
- Prometheus metrics endpoint (`/metrics`)
- Structured JSON logging via `log/slog`
- Real-time SSE event streaming via Redis pub/sub
- Webhook notifications with HMAC-SHA256 signing
- Webhook retry with exponential backoff and dead letter queue table

### Security and Authentication

- Dual auth: internal secret OR per-project API keys (auto-detected from `Authorization` header)
- API keys: SHA-256 hashed at rest, scoped to project, revocable, tracks last used timestamp
- JWT run tokens for SDK endpoint authentication
- SSRF protection on job endpoint URLs (blocks private/loopback addresses)
- CORS middleware with configurable origins and credentials
- Rate limiting: global per-IP + per-endpoint trigger throttle (10/min)

### Operations

- Multi-mode deployment: `api`, `worker`, or `all` (single binary)
- Graceful shutdown with in-flight job completion
- Cron scheduler, delayed job poller, stale run reaper
- Redis Sentinel failover support
- Database connection pool tuning (max conns, lifetime, idle time)
- Automatic schema migrations on startup (embedded via `go:embed`)
- Bulk trigger and bulk cancel endpoints

### CDC (Change Data Capture)

- Optional [Sequin Stream](https://sequinstream.com) integration for real-time change capture
- Consumes CDC events from Postgres WAL via Sequin's HTTP pull API
- Table handlers for `job_runs`, `workflow_runs`, `workflow_step_runs`
- Publishes change events to Redis pub/sub channels for downstream consumers
- Batch processing with ack/nack lifecycle and long-poll support
- Graceful shutdown and automatic error recovery with backoff

## Quick Start

```bash
# Clone
git clone https://github.com/leonardomso/orchestrator.git
cd orchestrator

# Start Postgres and Redis
docker compose up -d

# Set environment
export DATABASE_URL=postgres://orchestrator:orchestrator@localhost:5432/orchestrator?sslmode=disable
export REDIS_URL=redis://localhost:6379
export INTERNAL_SECRET=your-secret-here
export JWT_SIGNING_KEY=your-jwt-key-must-be-at-least-32-chars-long

# Run (api + worker in one process)
go run ./cmd/orchestrator --mode all
```

The `--mode` flag overrides the `MODE` env var. Migrations run automatically on startup.

## Configuration

All configuration is via environment variables.

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `DATABASE_URL` | PostgreSQL connection string | — | Yes |
| `REDIS_URL` | Redis connection string (SSE streaming, CDC events) | — | No |
| `REDIS_SENTINEL_MASTER` | Redis Sentinel master name | — | No |
| `REDIS_SENTINEL_ADDRS` | Comma-separated Sentinel addresses | — | No |
| `MODE` | Run mode: `api`, `worker`, or `all` | `all` | No |
| `PORT` | HTTP server port | `8080` | No |
| `INTERNAL_SECRET` | API authentication secret | — | Yes |
| `JWT_SIGNING_KEY` | JWT signing key (min 32 chars) | — | Yes |
| `WORKER_CONCURRENCY` | Max concurrent job executions | `10` | No |
| `LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` | `info` | No |
| `HEARTBEAT_INTERVAL` | Worker heartbeat check interval | `10s` | No |
| `POLLER_INTERVAL` | Delayed job polling interval | `5s` | No |
| `REAPER_INTERVAL` | Stale run reaper interval | `30s` | No |
| `STALE_THRESHOLD` | Time before a run is considered stale | `60s` | No |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP endpoint for tracing | — | No |
| `DB_MAX_CONNS` | Max database connections | `25` | No |
| `DB_MIN_CONNS` | Min database connections | `5` | No |
| `DB_MAX_CONN_LIFETIME` | Max connection lifetime | `30m` | No |
| `DB_MAX_CONN_IDLE_TIME` | Max connection idle time | `5m` | No |
| `RATE_LIMIT_REQUESTS` | Global rate limit (requests per window) | `100` | No |
| `RATE_LIMIT_WINDOW` | Rate limit window duration | `1m` | No |
| `CORS_ALLOWED_ORIGINS` | Allowed CORS origins (comma-separated) | `*` | No |
| `CORS_ALLOW_CREDENTIALS` | Allow CORS credentials | `false` | No |
| `SEQUIN_BASE_URL` | Sequin API base URL (enables CDC consumer) | — | No |
| `SEQUIN_CONSUMER_NAME` | Sequin Stream consumer name | — | No\* |
| `SEQUIN_API_TOKEN` | Sequin API authentication token | — | No\* |
| `SEQUIN_BATCH_SIZE` | CDC messages per poll batch | `10` | No |
| `SEQUIN_WAIT_TIME_MS` | CDC long-poll wait time in milliseconds | `5000` | No |

\*Required when `SEQUIN_BASE_URL` is set.

## API Reference

All `/v1/*` endpoints require authentication via `Authorization: Bearer <token>` header (internal secret or API key).

SDK `/sdk/v1/*` endpoints require a run token JWT issued by the trigger response.

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Liveness check |
| `GET` | `/health/ready` | Readiness check (verifies Postgres + Redis) |
| `GET` | `/metrics` | Prometheus metrics |

### Jobs

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/jobs` | Create a job |
| `GET` | `/v1/jobs?project_id=X` | List jobs for a project (supports `tag_key` and `tag_value` filters) |
| `GET` | `/v1/jobs/{jobID}` | Get a job |
| `PATCH` | `/v1/jobs/{jobID}` | Update a job (auto-versions) |
| `DELETE` | `/v1/jobs/{jobID}` | Soft-delete (disable) a job |
| `POST` | `/v1/jobs/{jobID}/trigger` | Trigger a run (rate limited: 10/min) |
| `POST` | `/v1/jobs/{jobID}/trigger/bulk` | Trigger multiple runs |
| `GET` | `/v1/jobs/{jobID}/versions` | List version history |

```bash
# Create a job
curl -X POST http://localhost:8080/v1/jobs \
  -H "Authorization: Bearer $INTERNAL_SECRET" \
  -H "Content-Type: application/json" \
  -d '{
    "project_id": "proj_1",
    "name": "Send Email",
    "slug": "send-email",
    "endpoint_url": "https://your-app.com/jobs/send-email",
    "max_attempts": 3,
    "timeout_secs": 60
  }'

# Trigger a job run
```bash
curl -X POST http://localhost:8080/v1/jobs/{jobID}/trigger \
  -H "Authorization: Bearer $INTERNAL_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"payload": {"to": "user@example.com", "subject": "Hello"}}'
```

```bash
# Dry-run trigger (validates without executing)
curl -X POST http://localhost:8080/v1/jobs/{jobID}/trigger \
  -H "Authorization: Bearer $INTERNAL_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"dry_run": true, "payload": {"data": "test"}}'
```
```bash
# Note: Dry-run mode requires FFDryRun feature flag to be enabled. Returns DryRunValidationResult instead of creating a run.
### Runs

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/runs?project_id=X` | List runs (supports `status`, `metadata_key`, `metadata_value`, `limit`, `cursor`) |
| `GET` | `/v1/runs/{runID}` | Get a run |
| `POST` | `/v1/runs/{runID}/replay` | Replay a failed run |
| `DELETE` | `/v1/runs/{runID}` | Cancel a run (propagates to children) |
| `GET` | `/v1/runs/{runID}/stream` | SSE event stream |
| `GET` | `/v1/runs/{runID}/children` | List child runs |
| `GET` | `/v1/runs/{runID}/events` | List run events (supports `level`, `type` filters) |
| `POST` | `/v1/runs/bulk-cancel` | Cancel multiple runs by ID |

```bash
# List runs with status filter
curl -H "Authorization: Bearer $INTERNAL_SECRET" \
  "http://localhost:8080/v1/runs?project_id=proj_1&status=executing&limit=20"

# Cancel a run
curl -X DELETE http://localhost:8080/v1/runs/{runID} \
  -H "Authorization: Bearer $INTERNAL_SECRET"
```

### Workflows

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/workflows` | Create a workflow with steps |
| `GET` | `/v1/workflows?project_id=X` | List workflows |
| `GET` | `/v1/workflows/{workflowID}` | Get workflow with steps |
| `PATCH` | `/v1/workflows/{workflowID}` | Update workflow (can replace steps) |
| `DELETE` | `/v1/workflows/{workflowID}` | Delete workflow (cascades) |
| `POST` | `/v1/workflows/{workflowID}/trigger` | Trigger a workflow run |
| `GET` | `/v1/workflows/{workflowID}/runs` | List runs for a workflow |

```bash
# Create a workflow with two steps (B depends on A)
curl -X POST http://localhost:8080/v1/workflows \
  -H "Authorization: Bearer $INTERNAL_SECRET" \
  -H "Content-Type: application/json" \
  -d '{
    "project_id": "proj_1",
    "name": "Data Pipeline",
    "slug": "data-pipeline",
    "steps": [
      {
        "job_id": "'$EXTRACT_JOB_ID'",
        "step_ref": "extract",
        "on_failure": "fail_workflow"
      },
      {
        "job_id": "'$TRANSFORM_JOB_ID'",
        "step_ref": "transform",
        "depends_on": ["extract"],
        "on_failure": "fail_workflow"
      }
    ]
  }'

# Trigger the workflow
curl -X POST http://localhost:8080/v1/workflows/{workflowID}/trigger \
  -H "Authorization: Bearer $INTERNAL_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"payload": {"source": "s3://bucket/data.csv"}}'
```

### Workflow Runs

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/workflow-runs?project_id=X` | List workflow runs (supports `status`, `limit`) |
| `GET` | `/v1/workflow-runs/{workflowRunID}` | Get a workflow run |
| `DELETE` | `/v1/workflow-runs/{workflowRunID}` | Cancel workflow run + all steps + job runs |
| `GET` | `/v1/workflow-runs/{workflowRunID}/steps` | List step runs |

### API Keys

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/api-keys` | Create an API key for a project |
| `GET` | `/v1/api-keys?project_id=X` | List API keys |
| `DELETE` | `/v1/api-keys/{keyID}` | Revoke an API key |

```bash
# Create an API key
curl -X POST http://localhost:8080/v1/api-keys \
  -H "Authorization: Bearer $INTERNAL_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"project_id": "proj_1", "name": "production"}'
# Response includes the raw key (only shown once). Use it in Authorization header.
```

### Other

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/stats` | Queue statistics (queued, executing, delayed counts) |
| `GET` | `/v1/webhook-deliveries` | List webhook deliveries (supports `status`, `limit`) |

### SDK Endpoints (Run Token Auth)

These endpoints are called by your job endpoint using the JWT run token from the trigger response.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/sdk/v1/runs/{runID}/log` | Log an event |
| `POST` | `/sdk/v1/runs/{runID}/heartbeat` | Send heartbeat (resets stale timer) |
| `POST` | `/sdk/v1/runs/{runID}/annotate` | Attach key-value annotations to run metadata |
| `POST` | `/sdk/v1/runs/{runID}/complete` | Mark run completed with result |
| `POST` | `/sdk/v1/runs/{runID}/fail` | Mark run failed with error |
| `POST` | `/sdk/v1/runs/{runID}/spawn` | Spawn a child job run |

```bash
# Log an event from your job endpoint
curl -X POST http://localhost:8080/sdk/v1/runs/{runID}/log \
  -H "Authorization: Bearer $RUN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message": "Processing row 1500/10000", "level": "info"}'

# Complete the run with a result
curl -X POST http://localhost:8080/sdk/v1/runs/{runID}/complete \
  -H "Authorization: Bearer $RUN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"result": {"rows_processed": 10000}}'
```

## Job Endpoint Contract

When a job run is dispatched, the orchestrator sends a POST request to the job's `endpoint_url`:

- **Method**: `POST`
- **Headers**: `X-Run-ID`, `X-Job-ID`, `X-Attempt`, `Content-Type: application/json`
- **Body**: The run's JSON payload
- **2xx response**: Run succeeds. Response body is stored as `result`.
- **Non-2xx response**: Triggers retry (if attempts remaining) or marks as failed.
- **Timeout**: Configurable per job via `timeout_secs`.

The trigger response includes a `run_token` JWT. Your endpoint can use this token to call SDK endpoints for logging, heartbeats, completion, failure, and spawning child jobs.

## Workflows

Workflows define a DAG (directed acyclic graph) of steps, where each step executes a job. Steps can depend on other steps, enabling fan-out and fan-in patterns.

### Concepts

- **Workflow**: A named DAG definition with one or more steps.
- **Step**: A node in the DAG. References a job by `job_id` and is identified by a unique `step_ref`.
- **Dependencies**: A step lists its parent steps in `depends_on`. Root steps (no dependencies) start immediately.
- **Workflow Run**: An execution instance of a workflow. Contains step runs for each step.
- **Step Run**: Tracks the execution of a single step within a workflow run.

### Step Dependencies and Fan-In

When a step has multiple dependencies, it uses an atomic counter to track completions. Each parent step that completes atomically increments the counter via a single `UPDATE ... RETURNING` query. The step only starts when all dependencies are met. Postgres row-level locks serialize concurrent updates, preventing race conditions.

### Failure Policies

Each step can specify an `on_failure` policy:

| Policy | Behavior |
|--------|----------|
| `fail_workflow` | Fail the entire workflow and cancel remaining steps (default) |
| `skip_dependents` | Skip all downstream steps, but let other branches continue |
| `continue` | Treat the failure as a success for dependency purposes |

### Step Conditions

Steps can have conditions that control whether they run when dependencies are met:

```json
{"type": "step_status", "step_ref": "extract", "status": "completed"}
```

```json
{"type": "all_of", "conditions": [
  {"type": "step_status", "step_ref": "a", "status": "completed"},
  {"type": "step_status", "step_ref": "b", "status": "completed"}
]}
```

```json
{"type": "any_of", "conditions": [
  {"type": "step_status", "step_ref": "primary", "status": "completed"},
  {"type": "step_status", "step_ref": "fallback", "status": "completed"}
]}
```

If a condition evaluates to false, the step is skipped.

### Payload Flow

When a step starts, its payload is constructed by merging three sources (later sources override earlier):

1. **Trigger payload**: The payload provided when triggering the workflow
2. **Step payload**: Static payload defined on the step
3. **Parent outputs**: A `parent_outputs` key containing the result of each parent step, keyed by `step_ref`

## Webhooks

When a job run reaches a terminal state (`completed`, `failed`, `timed_out`, etc.), a webhook is sent to the job's `webhook_url` if configured.

- **Signing**: HMAC-SHA256 using the job's `webhook_secret`. Header: `X-Webhook-Signature: sha256=<hex>`.
- **Retry**: Failed deliveries are retried with exponential backoff.
- **Dead Letter Queue**: Permanently failed deliveries are stored in the `webhook_deliveries` table for inspection.
- **Payload**:

```json
{
  "run_id": "...",
  "job_id": "...",
  "project_id": "...",
  "status": "completed",
  "attempt": 1,
  "result": {"rows_processed": 10000},
  "error": null,
  "timestamp": "2025-01-15T10:30:00Z"
}
```

## CDC (Change Data Capture)

The orchestrator includes an optional CDC consumer that integrates with [Sequin Stream](https://sequinstream.com) to capture real-time database changes from Postgres WAL.

### How It Works

1. Sequin reads your Postgres WAL (logical replication) and buffers changes
2. The orchestrator's CDC consumer polls Sequin's HTTP API for batches of change events
3. Events are routed to table-specific handlers based on the table name
4. Handlers publish structured change events to Redis pub/sub channels
5. Processed events are acknowledged; failed events are nacked for redelivery

### Monitored Tables

| Table | Pub/Sub Channel |
|-------|----------------|
| `job_runs` | `cdc:project:{project_id}:job_runs` |
| `workflow_runs` | `cdc:project:{project_id}:workflow_runs` |
| `workflow_step_runs` | `cdc:workflow_run:{workflow_run_id}:steps` |

### Setup

1. Deploy Sequin and connect it to your Postgres database (see [Sequin docs](https://sequinstream.com/docs))
2. Create a Sequin Stream sink for the tables you want to monitor
3. Set the `SEQUIN_BASE_URL`, `SEQUIN_CONSUMER_NAME`, and `SEQUIN_API_TOKEN` environment variables
4. The CDC consumer starts automatically alongside the API/worker

CDC is disabled when `SEQUIN_BASE_URL` is not set.

## Authentication

The orchestrator supports two authentication mechanisms for API endpoints:

### Internal Secret

Set `INTERNAL_SECRET` and pass it as a bearer token:

```
Authorization: Bearer your-secret-here
```

### Per-Project API Keys

Create scoped API keys via the `/v1/api-keys` endpoint. The raw key is returned once on creation. Use it the same way:

```
Authorization: Bearer sk_live_abc123...
```

API keys are SHA-256 hashed at rest, scoped to a single project, and track last used timestamps. Revoke keys via `DELETE /v1/api-keys/{keyID}`.

The server auto-detects which auth method is being used by checking the token against the internal secret first, then looking up API keys by hash.

### Run Tokens (SDK)

When a job run is triggered, the response includes a `run_token` — a short-lived JWT scoped to that specific run. Your job endpoint uses this token to authenticate SDK calls:

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

## Running Tests

```bash
# Unit tests (367 tests, ~14k lines)
go test ./...

# Unit tests with race detector
go test -race ./...

# Integration tests (requires Docker for testcontainers)
go test -tags integration -race ./internal/store/... ./internal/queue/...

# E2E tests (requires Docker)
go test -tags integration -race ./internal/e2e/...

# Lint (golangci-lint v2, 18 linters)
golangci-lint run ./...

# Build
go build ./...
```

The test suite maintains a test-to-production code ratio of approximately 1.5:1.

## Deployment

### Docker

```bash
docker build -t orchestrator .
docker run -e DATABASE_URL=... -e REDIS_URL=... -e INTERNAL_SECRET=... -e JWT_SIGNING_KEY=... -p 8080:8080 orchestrator
```

### Fly.io

```bash
fly secrets set DATABASE_URL=... REDIS_URL=... INTERNAL_SECRET=... JWT_SIGNING_KEY=...
fly deploy
```

### Scaling

Run the API and worker in separate processes for independent scaling:

```bash
# API instances (stateless, scale horizontally)
orchestrator --mode api

# Worker instances (scale based on queue depth)
orchestrator --mode worker
```

Workers use Postgres `SKIP LOCKED` for dequeuing, so multiple worker instances can run safely without double-processing. Each worker dequeues and locks its own batch of runs.

## Project Structure

```
orchestrator/
├── cmd/orchestrator/       # Entrypoint, flag parsing, graceful shutdown
├── internal/
│   ├── api/                # HTTP handlers, middleware, auth, routes
│   ├── cdc/                # Sequin CDC consumer, table handlers, HTTP client
│   ├── config/             # Environment configuration (viper)
│   ├── dbscan/             # Shared database row scanning
│   ├── domain/             # Types, job run FSM, workflow FSM, error types
│   ├── e2e/                # End-to-end integration tests
│   ├── pubsub/             # Publisher interface + Redis implementation
│   ├── queue/              # Queue interface + Postgres SKIP LOCKED
│   ├── scheduler/          # Cron, delayed poller, stale run reaper
│   ├── store/              # Database queries (pgx, raw SQL, no ORM)
│   ├── telemetry/          # OpenTelemetry tracing + Prometheus metrics
│   ├── testutil/           # Test factories, test DB/Redis helpers
│   ├── worker/             # Executor, pool, backoff, heartbeat, webhook dispatch
│   └── workflow/           # DAG validation, engine, step callback, conditions
├── migrations/             # 13 SQL migrations (embedded via go:embed)
├── docker-compose.yml      # Postgres 17 + Redis 7 for development
├── Dockerfile              # Multi-stage Go 1.26 build
├── fly.toml                # Fly.io deployment config
├── .golangci.yml           # golangci-lint v2 config (18 linters)
└── .github/workflows/      # CI: lint + test
```

## Database Schema

All primary keys are UUIDv7 stored as `TEXT`. Schema is managed by 13 migrations that run automatically on startup.

| Table | Description |
|-------|-------------|
| `jobs` | Job definitions (name, slug, endpoint URL, retry config, cron, TTL) |
| `job_runs` | Execution instances with 12-state FSM, payload, result, error |
| `run_events` | Structured log entries per run (type, level, message, data) |
| `job_versions` | Auto-snapshot of job config on every update |
| `api_keys` | Per-project API keys (SHA-256 hashed, revocable) |
| `webhook_deliveries` | Webhook delivery tracking and dead letter queue |
| `workflows` | Workflow DAG definitions (name, slug, project, version) |
| `workflow_steps` | Step definitions (job reference, dependencies, conditions, failure policy) |
| `workflow_runs` | Workflow execution instances (status, payload, timestamps) |
| `workflow_step_runs` | Step execution tracking (status, deps counter, output, error) |

## Tech Stack

- **Go 1.26** — single binary, no runtime dependencies
- **PostgreSQL 17** — primary store, job queue (SKIP LOCKED), workflow state
- **Redis 7** — pub/sub for SSE streaming and CDC event publishing
- **pgx/v5** — raw SQL with connection pooling (no ORM)
- **chi/v5** — lightweight HTTP router with middleware
- **golang-migrate/v4** — embedded SQL migrations
- **robfig/cron/v3** — cron expression scheduling
- **golang-jwt/v5** — JWT run token auth
- **OpenTelemetry** — distributed tracing (OTLP) + Prometheus metrics
- **viper** — environment variable configuration
- **testcontainers-go** — Postgres/Redis containers for integration tests
