# orchestrator

A production-grade Go job orchestrator inspired by [Trigger.dev](https://trigger.dev). Accepts job definitions via REST API, queues runs in Postgres using `SELECT FOR UPDATE SKIP LOCKED` (no external broker), dispatches them via HTTP to user endpoints, and handles retries with smart retry strategies (exponential, linear, fixed, custom). Supports workflow DAGs with fan-in/fan-out, cron scheduling, webhooks with retry and dead letter queue, real-time SSE streaming, per-project API key auth, job versioning, cost budgets, health scoring, environment endpoint overrides, execution tracing, run continuation, and optional Sequin CDC integration. Ships as a single Go binary.

## Features

### Core Job Engine

- Job management with CRUD, cron scheduling, and JSON payload schemas
- 13-state finite state machine with validated transitions: `delayed`, `queued`, `dequeued`, `executing`, `waiting`, `completed`, `failed`, `timed_out`, `crashed`, `system_failed`, `canceled`, `expired`, `dead_letter`
- Postgres `SKIP LOCKED` queue — no Kafka, RabbitMQ, or SQS required
- HTTP dispatch with configurable timeout per job
- Smart retry with 4 strategies: exponential backoff (default), linear, fixed delay, custom per-attempt delays
- Idempotency keys for deduplication
- Batch dequeue for throughput
- Priority queues (higher priority runs dequeued first)
- Fan-out child jobs via SDK spawn endpoint
- Per-job run TTL (`run_ttl_secs`) for auto-expiring stale runs
- Job versioning with automatic snapshot on update
- Job groups for logical organization
- Job dependencies with configurable conditions

### Smart Retry Strategies

Four retry strategies are available per job, configured via the `retry_strategy` field:

| Strategy | Formula | Use Case |
|----------|---------|----------|
| `exponential` (default) | `base × 2^(attempt-1)` ± 20% jitter | Transient failures, rate limits |
| `linear` | `base × attempt` ± 20% jitter | Predictable backoff ramp |
| `fixed` | `base` ± 20% jitter | Constant polling interval |
| `custom` | Per-attempt delays via `retry_delays_secs` array | Full control over delay sequence |

All strategies apply ±20% jitter to prevent thundering herd. Delays are capped at 1 hour. A minimum floor of 1 second is enforced.

### Adaptive Timeout

When `FF_ADAPTIVE_TIMEOUT` is enabled, the executor dynamically adjusts timeouts based on historical execution data. Jobs that consistently complete quickly get shorter timeouts, while jobs with variable execution times get longer ones. This prevents premature timeouts on slow jobs while improving resource recovery on fast ones.

### Run Dead Letter Queue

When `FF_RUN_DLQ` is enabled, runs that permanently fail after exhausting all retry attempts are moved to a `dead_letter` state instead of being marked as `failed`. Dead-lettered runs can be inspected and replayed via the API, providing a safety net for permanent failures.

### Execution Replay & Debug Bundles

When `FF_EXECUTION_TRACING` and `FF_DEBUG_BUNDLE` are enabled, the executor captures detailed execution traces including timing breakdowns (queue wait, dequeue, connect, TTFB, transfer, total) and stores them as JSONB on the run. Debug mode can be enabled per-run to capture additional diagnostic data. Debug bundles aggregate run data, events, checkpoints, and usage into a single downloadable payload for troubleshooting.

### Run Continuation & Lineage

When `FF_RUN_CONTINUATION` is enabled, runs can spawn continuation runs that form a parent-child lineage chain. The SDK `continue` endpoint creates a new run linked to the original via `continuation_of`, with `lineage_depth` tracking the chain depth. Parent runs wait until all descendants reach a terminal state. The lineage tree is queryable via the API.

### Job Health Score

When `FF_JOB_HEALTH_SCORING` is enabled, the `GET /v1/jobs/{jobID}/health` endpoint returns aggregated health metrics over a configurable time window (1h, 1d, 7d, 30d). Metrics include success rate, average and p95 duration, and a composite health score. The health score factors in success rate, timeout rate, crash rate, and latency stability.

### Environment Endpoint Overrides

When `FF_ENVIRONMENTS` is enabled, environments can be created per project with key-value variables. Jobs can be linked to an environment via `environment_id`. When a job is dispatched, the executor resolves environment variables and checks for an `ENDPOINT_URL` override. This allows routing the same job to different endpoints (e.g., staging vs. production) without modifying the job definition. All overridden URLs pass through SSRF validation.

### Cost Budgets

When `FF_COST_BUDGETS` is enabled, the system enforces per-run and daily project cost limits. AI model usage (tokens, cost in micro-USD) is tracked via the SDK `usage` endpoint. Project quotas define `max_cost_per_run_microusd` and `max_daily_cost_microusd`. Budget checks occur at trigger time (daily limit) and on each usage report (per-run limit). When a budget is exceeded, the run is failed gracefully with a budget error.

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
- Execution trace capture with timing breakdowns per run

### Security and Authentication

- Dual auth: internal secret OR per-project API keys (auto-detected from `Authorization` header)
- API keys: SHA-256 hashed at rest, scoped to project, revocable, tracks last used timestamp
- JWT run tokens for SDK endpoint authentication
- SSRF protection on job endpoint URLs and environment URL overrides (blocks private/loopback addresses)
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
- Feature flags for progressive rollout of new capabilities

### CDC (Change Data Capture)

- Optional [Sequin Stream](https://sequinstream.com) integration for real-time change capture
- Consumes CDC events from Postgres WAL via Sequin's HTTP pull API
- Table handlers for `job_runs`, `workflow_runs`, `workflow_step_runs`
- Publishes change events to Redis pub/sub channels for downstream consumers
- Batch processing with ack/nack lifecycle and long-poll support
- Graceful shutdown and automatic error recovery with backoff

### CLI

- Full-featured command-line interface with 48+ commands
- Multi-context configuration for managing multiple environments
- Declarative resource management with YAML manifests (validate, apply, diff, export)
- Interactive terminal dashboard (TUI) with queue metrics and run explorer
- System keychain credential storage (macOS Keychain, Windows Credential Manager)
- Seven output formats: table, JSON, YAML, CSV, wide, Go template, JSONPath
- Shell completion for Bash, Zsh, Fish, and PowerShell
- Extension system for custom plugins (`orchestrator-<name>` executables in PATH)
- Database backup/restore, migration management, and pprof profiling
- CI/CD-ready with `--ci` mode, environment variable configuration, and `wait`/`drain` commands

See [CLI.md](CLI.md) for the complete CLI reference.

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

### Core Settings

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
| `SECRET_ENCRYPTION_KEY` | Encryption key for job secrets (required when `FF_SECRET_INJECTION` is enabled) | — | No\* |
| `WORKER_CONCURRENCY` | Max concurrent job executions | `10` | No |
| `LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` | `info` | No |
| `HEARTBEAT_INTERVAL` | Worker heartbeat check interval | `10s` | No |
| `POLLER_INTERVAL` | Delayed job polling interval | `5s` | No |
| `REAPER_INTERVAL` | Stale run reaper interval | `30s` | No |
| `STALE_THRESHOLD` | Time before a run is considered stale | `60s` | No |
| `RUN_RETENTION_SHORT` | Retention for completed/failed/canceled/expired runs | `30d` | No |
| `RUN_RETENTION_LONG` | Retention for timed_out/crashed/system_failed runs | `90d` | No |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP endpoint for tracing | — | No |

### Database Connection Pool

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `DB_MAX_CONNS` | Max database connections | `25` | No |
| `DB_MIN_CONNS` | Min database connections | `5` | No |
| `DB_MAX_CONN_LIFETIME` | Max connection lifetime | `30m` | No |
| `DB_MAX_CONN_IDLE_TIME` | Max connection idle time | `5m` | No |

### Rate Limiting

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `RATE_LIMIT_REQUESTS` | Global rate limit (requests per window) | `100` | No |
| `RATE_LIMIT_WINDOW` | Rate limit window duration | `1m` | No |
| `TRIGGER_RATE_LIMIT_REQUESTS` | Trigger endpoint rate limit | `10` | No |
| `TRIGGER_RATE_LIMIT_WINDOW` | Trigger rate limit window | `1m` | No |

### CORS

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CORS_ALLOWED_ORIGINS` | Allowed CORS origins (comma-separated) | `*` | No |
| `CORS_ALLOW_CREDENTIALS` | Allow CORS credentials | `false` | No |

### Sequin CDC

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `SEQUIN_BASE_URL` | Sequin API base URL (enables CDC consumer) | — | No |
| `SEQUIN_CONSUMER_NAME` | Sequin Stream consumer name | — | No\* |
| `SEQUIN_API_TOKEN` | Sequin API authentication token | — | No\* |
| `SEQUIN_BATCH_SIZE` | CDC messages per poll batch | `10` | No |
| `SEQUIN_WAIT_TIME_MS` | CDC long-poll wait time in milliseconds | `5000` | No |

\*Required when `SEQUIN_BASE_URL` is set.

### Feature Flags

All feature flags default to `false`. Enable them by setting the environment variable to `true`.

| Variable | Description |
|----------|-------------|
| `FF_ADAPTIVE_TIMEOUT` | Dynamic timeout adjustment based on historical execution data |
| `FF_RUN_DLQ` | Dead letter queue for permanently failed runs |
| `FF_EXECUTION_TRACING` | Capture execution timing traces on each run |
| `FF_DEBUG_BUNDLE` | Enable debug bundles with execution diagnostics |
| `FF_RUN_CONTINUATION` | Lineage-based run continuation with parent-child tracking |
| `FF_JOB_HEALTH_SCORING` | Aggregated health metrics endpoint per job |
| `FF_SMART_RETRY` | Smart retry strategies (exponential, linear, fixed, custom) |
| `FF_ENVIRONMENTS` | Environment endpoint overrides with SSRF validation |
| `FF_COST_BUDGETS` | Per-run and daily project cost budget enforcement |
| `FF_USAGE_TRACKING` | AI model usage tracking (tokens, cost) |
| `FF_CONCURRENCY_LIMITS` | Per-job concurrency caps |
| `FF_PROJECT_QUOTAS` | Per-project quota enforcement (max jobs, runs, cost) |
| `FF_EXECUTION_WINDOWS` | Cron-based execution window scheduling |
| `FF_QUEUE_PARTITIONING` | Partition-based queue isolation |
| `FF_PROGRESS_STREAMING` | SDK progress reporting with SSE streaming |
| `FF_CHECKPOINTS` | SDK-driven run checkpointing |
| `FF_ERROR_CLASSIFICATION` | Automatic error classification (transient, client, etc.) |
| `FF_CIRCUIT_BREAKER` | Endpoint circuit breaker protection |
| `FF_BULKHEADS` | Bulkhead isolation for job categories |
| `FF_PAYLOAD_VALIDATION` | Trigger payload schema validation |
| `FF_JOB_TAGS` | String map tags on jobs |
| `FF_RUN_ANNOTATIONS` | Key-value annotations on runs |
| `FF_SECRET_INJECTION` | Encrypted secret injection into job payloads |
| `FF_RUN_REPLAY` | Replay failed runs |
| `FF_DRY_RUN` | Dry-run trigger validation without execution |
| `FF_RUN_RETENTION` | Automatic cleanup of terminal runs past retention |
| `FF_BATCH_JOB_OPS` | Batch job CRUD operations |
| `FF_JOB_GROUPS` | Logical job grouping |
| `FF_JOB_DEPENDENCIES` | Inter-job dependency tracking |

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
| `POST` | `/v1/jobs/{jobID}/clone` | Clone a job |
| `GET` | `/v1/jobs/{jobID}/health?window=7d` | Get job health score (requires `FF_JOB_HEALTH_SCORING`) |
| `POST` | `/v1/jobs/{jobID}/dependencies` | Create a job dependency |
| `GET` | `/v1/jobs/{jobID}/dependencies` | List job dependencies |
| `DELETE` | `/v1/jobs/{jobID}/dependencies/{depID}` | Delete a job dependency |
| `POST` | `/v1/jobs/batch` | Batch create jobs |
| `POST` | `/v1/jobs/batch-enable` | Batch enable jobs |
| `POST` | `/v1/jobs/batch-disable` | Batch disable jobs |

```bash
# Create a job with smart retry and environment
curl -X POST http://localhost:8080/v1/jobs \
  -H "Authorization: Bearer $INTERNAL_SECRET" \
  -H "Content-Type: application/json" \
  -d '{
    "project_id": "proj_1",
    "name": "Send Email",
    "slug": "send-email",
    "endpoint_url": "https://your-app.com/jobs/send-email",
    "max_attempts": 5,
    "timeout_secs": 60,
    "retry_strategy": "custom",
    "retry_delays_secs": [1, 5, 30, 120, 600],
    "environment_id": "env_staging"
  }'

# Trigger a job run
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
# Note: Dry-run mode requires FF_DRY_RUN feature flag to be enabled.
```

```bash
# Get job health score (7-day window)
curl -H "Authorization: Bearer $INTERNAL_SECRET" \
  "http://localhost:8080/v1/jobs/{jobID}/health?window=7d"
# Returns: total_runs, completed_runs, failed_runs, timed_out_runs, crashed_runs,
#   canceled_runs, expired_runs, success_rate, avg_duration_secs, p95_duration_secs, health_score
```

### Job Groups

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/job-groups` | Create a job group |
| `GET` | `/v1/job-groups?project_id=X` | List job groups |
| `GET` | `/v1/job-groups/{groupID}` | Get a job group |
| `PATCH` | `/v1/job-groups/{groupID}` | Update a job group |
| `DELETE` | `/v1/job-groups/{groupID}` | Delete a job group |
| `GET` | `/v1/job-groups/{groupID}/jobs` | List jobs in a group |

### Environments

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/environments` | Create an environment |
| `GET` | `/v1/environments?project_id=X` | List environments |
| `GET` | `/v1/environments/{envID}` | Get an environment |
| `PATCH` | `/v1/environments/{envID}` | Update an environment |
| `DELETE` | `/v1/environments/{envID}` | Delete an environment |
| `GET` | `/v1/environments/{envID}/variables` | Get resolved variables (inherits from parent) |

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
| `GET` | `/v1/runs/{runID}/checkpoints` | List run checkpoints |
| `GET` | `/v1/runs/{runID}/usage` | List run AI model usage records |
| `GET` | `/v1/runs/{runID}/tool-calls` | List run tool calls |
| `GET` | `/v1/runs/{runID}/outputs` | List run structured outputs |
| `GET` | `/v1/runs/{runID}/debug-bundle` | Get debug bundle (requires `FF_DEBUG_BUNDLE`) |
| `POST` | `/v1/runs/{runID}/debug` | Enable/disable debug mode for a run |
| `GET` | `/v1/runs/{runID}/lineage` | List run continuation lineage chain |
| `GET` | `/v1/runs/dlq` | List dead-lettered runs |
| `POST` | `/v1/runs/{runID}/dlq-replay` | Replay a dead-lettered run |
| `POST` | `/v1/runs/bulk-cancel` | Cancel multiple runs by ID |

```bash
# List runs with status filter
curl -H "Authorization: Bearer $INTERNAL_SECRET" \
  "http://localhost:8080/v1/runs?project_id=proj_1&status=executing&limit=20"

# Cancel a run
curl -X DELETE http://localhost:8080/v1/runs/{runID} \
  -H "Authorization: Bearer $INTERNAL_SECRET"

# Get a debug bundle
curl -H "Authorization: Bearer $INTERNAL_SECRET" \
  "http://localhost:8080/v1/runs/{runID}/debug-bundle"

# List dead-lettered runs
curl -H "Authorization: Bearer $INTERNAL_SECRET" \
  "http://localhost:8080/v1/runs/dlq?project_id=proj_1"

# Replay a dead-lettered run
curl -X POST http://localhost:8080/v1/runs/{runID}/dlq-replay \
  -H "Authorization: Bearer $INTERNAL_SECRET"

# View run lineage tree
curl -H "Authorization: Bearer $INTERNAL_SECRET" \
  "http://localhost:8080/v1/runs/{runID}/lineage"
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
| `POST` | `/sdk/v1/runs/{runID}/progress` | Report progress (percent, message, step, ETA) |
| `POST` | `/sdk/v1/runs/{runID}/heartbeat` | Send heartbeat (resets stale timer) |
| `POST` | `/sdk/v1/runs/{runID}/annotate` | Attach key-value annotations to run metadata |
| `POST` | `/sdk/v1/runs/{runID}/checkpoint` | Save a run checkpoint |
| `POST` | `/sdk/v1/runs/{runID}/usage` | Report AI model usage (tokens, cost). Enforces per-run budget when `FF_COST_BUDGETS` is enabled |
| `POST` | `/sdk/v1/runs/{runID}/tool-call` | Record a tool call with input/output |
| `POST` | `/sdk/v1/runs/{runID}/output` | Upsert a structured output with optional schema validation |
| `POST` | `/sdk/v1/runs/{runID}/complete` | Mark run completed with result |
| `POST` | `/sdk/v1/runs/{runID}/fail` | Mark run failed with error |
| `POST` | `/sdk/v1/runs/{runID}/spawn` | Spawn a child job run |
| `POST` | `/sdk/v1/runs/{runID}/continue` | Create a continuation run (requires `FF_RUN_CONTINUATION`). Links to parent via lineage |

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

# Report AI model usage
curl -X POST http://localhost:8080/sdk/v1/runs/{runID}/usage \
  -H "Authorization: Bearer $RUN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "openai",
    "model": "gpt-4o",
    "prompt_tokens": 1500,
    "completion_tokens": 500,
    "total_tokens": 2000,
    "cost_microusd": 3500
  }'

# Create a continuation run
curl -X POST http://localhost:8080/sdk/v1/runs/{runID}/continue \
  -H "Authorization: Bearer $RUN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"payload": {"step": "next-batch", "offset": 10000}}'
```

## Job Endpoint Contract

When a job run is dispatched, the orchestrator sends a POST request to the job's `endpoint_url` (or the environment-overridden URL if `FF_ENVIRONMENTS` is enabled):

- **Method**: `POST`
- **Headers**: `X-Run-ID`, `X-Job-ID`, `X-Attempt`, `Content-Type: application/json`
- **Body**: The run's JSON payload
- **2xx response**: Run succeeds. Response body is stored as `result`.
- **Non-2xx response**: Triggers retry (if attempts remaining) or marks as failed (or `dead_letter` if `FF_RUN_DLQ` is enabled).
- **Timeout**: Configurable per job via `timeout_secs`. Adaptive adjustment available via `FF_ADAPTIVE_TIMEOUT`.

The trigger response includes a `run_token` JWT. Your endpoint can use this token to call SDK endpoints for logging, heartbeats, completion, failure, spawning child jobs, reporting usage, and creating continuation runs.

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
# Unit tests
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
│   ├── worker/             # Executor, pool, backoff strategies, heartbeat, webhook dispatch, SSRF validation
│   └── workflow/           # DAG validation, engine, step callback, conditions
├── migrations/             # 39 SQL migrations (embedded via go:embed)
├── docker-compose.yml      # Postgres 17 + Redis 7 for development
├── Dockerfile              # Multi-stage Go 1.26 build
├── fly.toml                # Fly.io deployment config
├── .golangci.yml           # golangci-lint v2 config (18 linters)
├── .github/workflows/      # CI: lint + test
└── CLI.md                  # CLI reference documentation
```

## Database Schema

All primary keys are UUIDv7 stored as `TEXT`. Schema is managed by 39 migrations that run automatically on startup.

### Core Tables

| Table | Description |
|-------|-------------|
| `jobs` | Job definitions (name, slug, endpoint URL, retry config, cron, TTL, retry strategy, environment link) |
| `job_runs` | Execution instances with 13-state FSM, payload, result, error, execution trace, debug mode, continuation lineage |
| `run_events` | Structured log entries per run (type, level, message, data) |
| `job_versions` | Auto-snapshot of job config on every update |
| `api_keys` | Per-project API keys (SHA-256 hashed, revocable) |
| `webhook_deliveries` | Webhook delivery tracking and dead letter queue |
| `project_quotas` | Per-project quotas (max jobs, runs, concurrency, cost limits) |

### Workflow Tables

| Table | Description |
|-------|-------------|
| `workflows` | Workflow DAG definitions (name, slug, project, version) |
| `workflow_steps` | Step definitions (job reference, dependencies, conditions, failure policy) |
| `workflow_runs` | Workflow execution instances (status, payload, timestamps) |
| `workflow_step_runs` | Step execution tracking (status, deps counter, output, error) |

### Core Engine Tables

| Table | Description |
|-------|-------------|
| `run_usage` | AI model usage tracking per run (provider, model, tokens, cost in micro-USD) |
| `run_checkpoints` | SDK-driven run state checkpoints |
| `run_tool_calls` | Tool call recording with input/output and duration |
| `run_outputs` | Structured outputs with optional schema validation |
| `environments` | Environment definitions with key-value variables per project |
| `environment_variables` | Environment variable key-value pairs with inheritance |
| `endpoint_circuit_state` | Circuit breaker state per endpoint URL |
| `pricing_catalog` | Static pricing table for AI model cost calculation |
| `job_secrets` | Encrypted secrets scoped to job and environment |
| `job_groups` | Logical grouping of jobs |
| `job_dependencies` | Inter-job dependency definitions |

### Key New Columns (Core Engine)

| Table.Column | Type | Description |
|-------------|------|-------------|
| `jobs.retry_strategy` | TEXT | Retry strategy: `exponential`, `linear`, `fixed`, `custom` |
| `jobs.retry_delays_secs` | INT[] | Custom per-attempt delays in seconds (for `custom` strategy) |
| `jobs.environment_id` | TEXT | FK to environments table for endpoint override |
| `job_runs.execution_trace` | JSONB | Timing breakdown (queue_wait, dequeue, connect, ttfb, transfer, total) |
| `job_runs.debug_mode` | BOOLEAN | Whether debug diagnostics are enabled for this run |
| `job_runs.continuation_of` | TEXT | FK to parent run for continuation lineage |
| `job_runs.lineage_depth` | INT | Depth in continuation chain |
| `project_quotas.max_cost_per_run_microusd` | BIGINT | Per-run cost limit in micro-USD |
| `project_quotas.max_daily_cost_microusd` | BIGINT | Daily project cost limit in micro-USD |

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
