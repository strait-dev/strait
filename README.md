# orchestrator

A Go job orchestrator inspired by Trigger.dev. Accepts job definitions via REST API, queues runs in Postgres using SELECT FOR UPDATE SKIP LOCKED, dispatches them via HTTP to user endpoints, handles retries with exponential backoff, and reports results back. Supports priority queues, idempotency, cron scheduling, fan-out child jobs, webhooks, real-time SSE streaming, and Prometheus metrics.

## Features

- Job management (CRUD with cron scheduling, payload schemas)
- Priority queue with Postgres SKIP LOCKED (no external broker)
- HTTP dispatch with configurable timeout and retry
- 12-state FSM with validated transitions
- Idempotency keys for deduplication
- Batch dequeue for throughput
- Fan-out child jobs (spawn from SDK)
- Webhook notifications with HMAC-SHA256
- Real-time SSE log streaming via Redis pubsub
- Cron scheduler, delayed job poller, stale run reaper
- OpenTelemetry tracing (OTLP) and Prometheus metrics
- Rate limiting (global + per-endpoint)
- Graceful shutdown with in-flight job completion
- Multi-mode deployment (api, worker, or all-in-one)

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
export JWT_SIGNING_KEY=your-jwt-key-must-be-at-least-32-chars

# Run (api + worker in one process)
go run ./cmd/orchestrator --mode all
```

## Configuration

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| DATABASE_URL | PostgreSQL connection string | — | Yes |
| REDIS_URL | Redis connection string (for SSE streaming) | — | No |
| MODE | Run mode: api, worker, or all | all | No |
| PORT | HTTP server port | 8080 | No |
| INTERNAL_SECRET | API authentication secret | — | Yes |
| JWT_SIGNING_KEY | JWT signing key (min 32 chars) | — | Yes |
| WORKER_CONCURRENCY | Max concurrent job executions | 10 | No |
| POLLER_INTERVAL | Delayed job polling interval | 5s | No |
| HEARTBEAT_INTERVAL | Heartbeat check interval | 10s | No |
| REAPER_INTERVAL | Stale run reaper interval | 30s | No |
| STALE_THRESHOLD | Time before a run is considered stale | 60s | No |
| LOG_LEVEL | Log level: debug, info, warn, error | info | No |
| OTEL_EXPORTER_OTLP_ENDPOINT | OTLP endpoint for tracing | — | No |
| DB_MAX_CONNS | Max database connections | 25 | No |
| DB_MIN_CONNS | Min database connections | 5 | No |
| DB_MAX_CONN_LIFETIME | Max connection lifetime | 30m | No |
| DB_MAX_CONN_IDLE_TIME | Max connection idle time | 5m | No |
| RATE_LIMIT_REQUESTS | Global rate limit (requests per window) | 100 | No |
| RATE_LIMIT_WINDOW | Rate limit window duration | 1m | No |

## API Reference

**Auth**: All `/v1/*` endpoints require `Authorization: Bearer <INTERNAL_SECRET>` header.
SDK `/sdk/v1/*` endpoints require `Authorization: Bearer <run_token>` (JWT from trigger response).

### Health
- `GET /health` — Liveness check, always returns `{"status": "ok"}`
- `GET /health/ready` — Readiness check, verifies database connectivity
- `GET /metrics` — Prometheus metrics (OpenTelemetry)

### Jobs
- `POST /v1/jobs` — Create a job
- `GET /v1/jobs?project_id=X` — List jobs for a project
- `GET /v1/jobs/{jobID}` — Get a job
- `PATCH /v1/jobs/{jobID}` — Update a job
- `DELETE /v1/jobs/{jobID}` — Soft-delete (disable) a job
- `POST /v1/jobs/{jobID}/trigger` — Trigger a job run (rate limited: 10/min)

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
curl -X POST http://localhost:8080/v1/jobs/{jobID}/trigger \
  -H "Authorization: Bearer $INTERNAL_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"payload": {"to": "user@example.com", "subject": "Hello"}}'
```

### Runs
- `GET /v1/runs?project_id=X` — List runs (supports `status`, `limit`, `cursor` params)
- `GET /v1/runs/{runID}` — Get a run
- `DELETE /v1/runs/{runID}` — Cancel a run (propagates to children)
- `GET /v1/runs/{runID}/stream` — SSE event stream
- `GET /v1/runs/{runID}/children` — List child runs

```bash
# List runs
curl -H "Authorization: Bearer $INTERNAL_SECRET" \
  "http://localhost:8080/v1/runs?project_id=proj_1&status=executing"
```

### Stats
- `GET /v1/stats` — Queue statistics (queued, executing, delayed counts)

```bash
# Get stats
curl -H "Authorization: Bearer $INTERNAL_SECRET" http://localhost:8080/v1/stats
```

### SDK (Run Token Auth)
- `POST /sdk/v1/runs/{runID}/log` — Log an event
- `POST /sdk/v1/runs/{runID}/heartbeat` — Send heartbeat
- `POST /sdk/v1/runs/{runID}/complete` — Mark run completed
- `POST /sdk/v1/runs/{runID}/fail` — Mark run failed
- `POST /sdk/v1/runs/{runID}/spawn` — Spawn a child job run

```bash
# Log an event from the job endpoint
curl -X POST http://localhost:8080/sdk/v1/runs/{runID}/log \
  -H "Authorization: Bearer $RUN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message": "Processing started", "level": "info"}'
```

The trigger response includes a `run_token` JWT that the job endpoint uses to call SDK APIs.

## Job Endpoint Contract

The user's HTTP endpoint receives job execution requests and interacts with the orchestrator:
- Receives POST with JSON payload
- Headers: `X-Run-ID`, `X-Job-ID`, `X-Attempt`, `Content-Type: application/json`
- Should return 2xx for success (response body stored as `result`)
- Non-2xx triggers retry (if attempts remaining) or failure
- Can use SDK endpoints with the run token to: log events, send heartbeats, report completion/failure, spawn child jobs

## Webhooks

- When a run reaches a terminal state, a webhook is sent to the job's `webhook_url` (if configured)
- Signed with HMAC-SHA256 using `webhook_secret`
- Header: `X-Webhook-Signature: sha256=<hex>`
- Payload shape: `{"run_id", "job_id", "project_id", "status", "attempt", "result", "error", "timestamp"}`

## Running Tests

```bash
# Unit tests
go test ./...

# Unit tests with race detector
go test -race ./...

# Integration tests (requires Docker)
docker compose up -d
go test -tags integration -race ./internal/store/... ./internal/queue/...

# Lint
golangci-lint run ./...
```

## Deployment

- Docker: `docker build -t orchestrator .` then run with env vars
- Fly.io: `fly deploy` with secrets set via `fly secrets set`
- Modes: run API and worker separately for scaling, or `--mode all` for single process

## Project Structure

```
orchestrator/
├── cmd/orchestrator/       # Entrypoint, flag parsing, graceful shutdown
├── internal/
│   ├── api/                # HTTP handlers, middleware, routes
│   ├── config/             # Environment configuration
│   ├── dbscan/             # Shared database row scanning
│   ├── domain/             # Types, FSM, error types
│   ├── pubsub/             # Publisher interface + Redis implementation
│   ├── queue/              # Queue interface + Postgres SKIP LOCKED
│   ├── scheduler/          # Cron, delayed poller, reaper
│   ├── store/              # Database queries (jobs, runs, events)
│   ├── telemetry/          # OpenTelemetry tracing + Prometheus metrics
│   ├── testutil/           # Integration test helpers
│   └── worker/             # Executor, pool, backoff, heartbeat, webhook
├── migrations/             # SQL migrations (embed via go:embed)
├── docker-compose.yml      # Postgres + Redis for development
├── Dockerfile              # Multi-stage Go 1.26 build
├── fly.toml                # Fly.io deployment config
└── .github/workflows/      # CI: lint + test
```

## Tech Stack

- Go 1.26
- PostgreSQL 17 (primary store + job queue)
- Redis 7 (pub/sub for SSE)
- pgx/v5 (raw SQL, no ORM)
- chi/v5 (HTTP router)
- golang-migrate (schema migrations)
- robfig/cron/v3 (cron scheduling)
- golang-jwt/v5 (JWT auth)
- OpenTelemetry + Prometheus (observability)
- testcontainers-go (integration tests)

For detailed technical architecture, see [ARCHITECTURE.md](ARCHITECTURE.md).
