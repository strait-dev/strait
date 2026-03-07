# Architecture

This document provides a deep technical overview of the job orchestrator's internals, design decisions, and component interactions.

### 1. System Overview

The system is designed as a distributed job orchestrator that uses PostgreSQL as its primary state store and message queue.

```
                    ┌──────────────────────────────────┐
                    │           API Server              │
                    │  (Chi router + middleware)         │
                    │                                    │
                    │  /v1/jobs/* ── Job CRUD + Health   │
                    │  /v1/jobs/{id}/trigger ── Enqueue  │
    HTTP ──────────>│  /v1/runs/* ── Run mgmt + DLQ     │
    clients         │  /sdk/v1/* ── SDK (JWT auth)      │
                    │  /metrics ── Prometheus            │
                    └──────────┬───────────────────────┘
                               │ Enqueue (budget check)
                               v
                    ┌──────────────────────────────────┐
                    │         PostgreSQL                 │
                    │                                    │
                    │  jobs ── job definitions           │
                    │  job_runs ── run state + queue     │
                    │  run_events ── log entries         │
                    │  run_usage ── AI cost tracking     │
                    │  environments ── endpoint config   │
                    │  project_quotas ── budget limits   │
                    │                                    │
                    │  Queue: SELECT FOR UPDATE          │
                    │         SKIP LOCKED                │
                    └──────────┬───────────────────────┘
                               │ Dequeue
                               v
                    ┌──────────────────────────────────┐
                    │         Worker Executor            │
                    │                                    │
                    │  Poll ─> DequeueN(available)       │
                    │  Resolve ─> Env override + SSRF   │
                    │  Execute ─> HTTP POST to endpoint  │
                    │  Retry ─> Smart strategy select    │
                    │  Trace ─> Execution timing capture │
                    │  DLQ ─> Dead letter on exhaustion  │
                    └──────────┬───────────────────────┘
                               │ Webhook / PubSub
                               v
                    ┌──────────────────────────────────┐
                    │  Scheduler         │  Redis       │
                    │  - Cron ticker     │  - PubSub    │
                    │  - Delayed poller  │  - SSE       │
                    │  - Stale reaper    │  streaming   │
                    │  - Retention       │              │
                    └──────────────────────────────────┘
```

The system is distributed as a single Go binary that can run in three modes:
- **api**: Handles HTTP requests, job management, and triggering.
- **worker**: Runs the executor, scheduler, and background maintenance tasks.
- **all**: Runs both API and worker components in a single process.

Graceful shutdown is implemented using `errgroup` and signal handling to ensure in-flight jobs are completed and resources are released cleanly.

### 2. Component Architecture

The core logic resides in the `internal/` directory, organized into the following packages:

**api**
Implements the Chi HTTP router and middleware chain. The middleware includes RequestID, RealIP, OTel tracing, RequestLogger, Recoverer, and Rate Limiting. It supports two authentication schemes:
- `internalSecretAuth`: Bearer token matching `INTERNAL_SECRET` for management endpoints.
- `runTokenAuth`: JWT HS256 (subject=runID) for SDK endpoints.
URL validation includes SSRF protection to block private and loopback IP addresses. The API layer also handles health score computation, debug bundle assembly, DLQ management, environment CRUD, cost budget checks at trigger time, and run continuation lineage queries.

**config**
Handles environment variable loading using Viper. It manages 50+ configuration fields including 25+ feature flags with sensible defaults and performs validation (e.g., `DATABASE_URL` and `INTERNAL_SECRET` are required, `JWT_SIGNING_KEY` must be at least 32 characters, `SECRET_ENCRYPTION_KEY` is required when `FF_SECRET_INJECTION` is enabled).

**domain**
Defines core types such as `Job` (30 fields including `retry_strategy`, `retry_delays_secs`, `environment_id`), `JobRun` (24 fields including `execution_trace`, `debug_mode`, `continuation_of`, `lineage_depth`), `RunUsage`, `RunCheckpoint`, `RunToolCall`, `RunOutput`, `Environment`, `JobGroup`, and `JobDependency`. It includes a `RunStatus` enum with 13 states (including `dead_letter`) and structured error types like `TransitionError`, `UnknownStatusError`, `EndpointError`, `FieldError`, and `ConfigError`, along with sentinel errors like `ErrJobNotFound`.

**store**
Provides raw SQL access via `pgx/v5`. It uses interface segregation with `JobStore`, `RunStore`, `EventStore`, `EnvironmentStore`, `JobGroupStore`, `JobSecretStore`, and others. The `Queries` struct accepts a `DBTX` interface, allowing it to work with both connection pools and transactions via the `WithTx` helper. `UpdateRunStatus` implements optimistic locking using `WHERE id = $2 AND status = $3`. New query capabilities include `GetJobHealthStats` (aggregated health metrics with configurable time windows), `SumRunCostMicrousd` and `SumProjectDailyCostMicrousd` (cost aggregation), `GetDebugBundle` (diagnostic data assembly), `ListRunLineage` (continuation chain traversal), `ListDeadLetterRuns` and `ReplayDeadLetterRun` (DLQ operations), and `GetResolvedEnvironmentVariables` (inherited variable resolution). All methods are instrumented with OTel spans.

**queue**
The `PostgresQueue` implements the `Queue` interface. It uses `SELECT FOR UPDATE SKIP LOCKED` for atomic job claiming, eliminating the need for an external message broker. `DequeueN` uses a CTE pattern for batch claims. Jobs are ordered by `priority DESC, created_at ASC`. Filters ensure that delayed or retrying jobs are only dequeued after their scheduled time.

**worker**
The executor polls the queue at configurable intervals. It uses a semaphore-based worker pool (buffered channel + WaitGroup). For each execution, it:
1. Fetches job configuration.
2. Resolves environment endpoint overrides (if `FF_ENVIRONMENTS` is enabled) with SSRF validation.
3. Transitions the state from `dequeued` to `executing`.
4. Dispatches an HTTP POST to the job endpoint (or overridden URL).
5. Captures execution traces (queue wait, dequeue, connect, TTFB, transfer, total) when `FF_EXECUTION_TRACING` is enabled.
6. Handles the result using the configured retry strategy (exponential, linear, fixed, or custom).
7. Moves permanently failed runs to `dead_letter` state when `FF_RUN_DLQ` is enabled.
8. Applies adaptive timeout adjustment when `FF_ADAPTIVE_TIMEOUT` is enabled.

In-flight jobs use `context.WithoutCancel` to survive process shutdown. A background goroutine sends heartbeats for each active run. The `backoff` module implements all four retry strategies with jitter and delay floor enforcement.

**scheduler**
Consists of background components:
- **CronScheduler**: Uses `robfig/cron/v3` to tick matching jobs and enqueue new runs.
- **DelayedPoller**: Finds runs with `status=delayed` that are ready to be queued.
- **Reaper**: Identifies stale `executing` runs (based on heartbeat), stale `dequeued` runs, and expired runs.
- **RetentionWorker**: Cleans up terminal runs past configurable retention periods (30d for completed/failed/canceled/expired, 90d for timed_out/crashed/system_failed).

**pubsub**
Defines a `Publisher` interface for real-time updates. The `RedisPublisher` implementation uses `go-redis/v9` to support SSE streaming via channels named `run:{runID}`.

**dbscan**
Provides shared row scanning logic. `ScanRun` handles all run columns in a fixed order, including new fields like `execution_trace`, `debug_mode`, `continuation_of`, and `lineage_depth`, using helpers like `NilIfEmptyString` to preserve SQL NULL semantics.

**telemetry**
Sets up the OpenTelemetry `TracerProvider` (OTLP HTTP) and `MeterProvider` (Prometheus). It tracks metrics such as `RunTransitions` (counter), `DispatchDuration` (histogram), and `DequeueDuration` (histogram).

### 3. Data Model

The system relies on the following primary tables in PostgreSQL:

**jobs**
```sql
id                  TEXT PRIMARY KEY              -- UUIDv7
project_id          TEXT NOT NULL
group_id            TEXT                          -- FK to job_groups
name                TEXT NOT NULL
slug                TEXT NOT NULL
description         TEXT
cron                TEXT                          -- 5-field cron expression
payload_schema      JSONB
tags                JSONB                         -- string map tags
endpoint_url        TEXT NOT NULL
fallback_endpoint_url TEXT
max_attempts        INT NOT NULL DEFAULT 3
timeout_secs        INT NOT NULL DEFAULT 300
max_concurrency     INT                           -- per-job concurrency cap
execution_window_cron TEXT                        -- when job can execute
timezone            TEXT                          -- project-level timezone override
rate_limit_max      INT
rate_limit_window_secs INT
dedup_window_secs   INT
enabled             BOOLEAN NOT NULL DEFAULT TRUE
webhook_url         TEXT
webhook_secret      TEXT
run_ttl_secs        INT
retry_strategy      TEXT                          -- exponential|linear|fixed|custom
retry_delays_secs   INT[]                         -- custom per-attempt delays
environment_id      TEXT                          -- FK to environments
version             INT NOT NULL DEFAULT 1
created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
UNIQUE (project_id, slug)
```

**job_runs**
```sql
id                  TEXT PRIMARY KEY              -- UUIDv7
job_id              TEXT NOT NULL REFERENCES jobs(id)
project_id          TEXT NOT NULL
status              TEXT NOT NULL DEFAULT 'queued' -- 13 states incl. dead_letter
attempt             INT NOT NULL DEFAULT 1
payload             JSONB
result              JSONB
metadata            JSONB NOT NULL DEFAULT '{}'   -- key-value annotations
error               TEXT
triggered_by        TEXT NOT NULL DEFAULT 'manual' -- manual, cron, spawn, workflow
scheduled_at        TIMESTAMPTZ
started_at          TIMESTAMPTZ
finished_at         TIMESTAMPTZ
heartbeat_at        TIMESTAMPTZ
next_retry_at       TIMESTAMPTZ
expires_at          TIMESTAMPTZ
parent_run_id       TEXT REFERENCES job_runs(id)
priority            INT NOT NULL DEFAULT 0
idempotency_key     TEXT
job_version         INT NOT NULL DEFAULT 1
workflow_step_run_id TEXT
execution_trace     JSONB                         -- timing breakdown
debug_mode          BOOLEAN NOT NULL DEFAULT FALSE
continuation_of     TEXT                          -- FK for continuation lineage
lineage_depth       INT NOT NULL DEFAULT 0        -- depth in continuation chain
created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

**run_usage**
```sql
id                  TEXT PRIMARY KEY              -- UUIDv7
run_id              TEXT NOT NULL REFERENCES job_runs(id)
provider            TEXT NOT NULL                 -- e.g. openai, anthropic
model               TEXT NOT NULL                 -- e.g. gpt-4o
prompt_tokens       INT NOT NULL DEFAULT 0
completion_tokens   INT NOT NULL DEFAULT 0
total_tokens        INT NOT NULL DEFAULT 0
cost_microusd       BIGINT NOT NULL DEFAULT 0     -- cost in micro-USD (1/1,000,000 USD)
created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

**environments**
```sql
id                  TEXT PRIMARY KEY              -- UUIDv7
project_id          TEXT NOT NULL
name                TEXT NOT NULL
slug                TEXT NOT NULL
parent_id           TEXT                          -- FK for variable inheritance
created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
UNIQUE (project_id, slug)
```

**project_quotas**
```sql
project_id            TEXT PRIMARY KEY
max_queued_runs       INT NOT NULL DEFAULT 1000
max_executing_runs    INT NOT NULL DEFAULT 100
max_jobs              INT NOT NULL DEFAULT 100
timezone              TEXT NOT NULL DEFAULT 'UTC'
max_cost_per_run_microusd  BIGINT NOT NULL DEFAULT 0  -- 0 = unlimited
max_daily_cost_microusd    BIGINT NOT NULL DEFAULT 0  -- 0 = unlimited
```

**run_events**
```sql
id         TEXT PRIMARY KEY                 -- UUIDv7
run_id     TEXT NOT NULL REFERENCES job_runs(id)
type       TEXT NOT NULL                    -- log, state_change, error, progress
level      TEXT                             -- info, warn, error, debug
message    TEXT
data       JSONB NOT NULL DEFAULT '{}'
created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

**Key Indexes**
- `idx_runs_queue`: Partial index on `status = 'queued'` for dequeue performance.
- `idx_runs_priority`: `(priority DESC, created_at ASC) WHERE status = 'queued'`.
- `idx_runs_idempotency`: Unique partial index on `(job_id, idempotency_key) WHERE idempotency_key IS NOT NULL`.
- `idx_runs_heartbeat`: Index on `heartbeat_at WHERE status = 'executing'` for stale run detection.
- `idx_runs_retry`: Index on `next_retry_at WHERE status = 'queued' AND next_retry_at IS NOT NULL`.
- `idx_runs_dead_letter`: Index on `status = 'dead_letter'` for DLQ queries.
- `idx_run_usage_run_id`: Index on `run_usage(run_id)` for per-run cost aggregation.
- `idx_run_usage_created_at`: Index on `run_usage(created_at)` for daily cost queries.
- `idx_runs_continuation`: Index on `continuation_of` for lineage tree traversal.
- `idx_runs_debug`: Partial index on `debug_mode = TRUE`.

### 4. Queue Mechanics

The orchestrator uses PostgreSQL as a message queue to minimize operational complexity and ensure transactional consistency.

- **SELECT FOR UPDATE SKIP LOCKED**: This allows multiple workers to poll the same table simultaneously without blocking each other. Each worker locks only the rows it is currently processing.
- **Atomic Dequeue**: A single dequeue operation uses a subquery to select a row with `SKIP LOCKED` and an outer `UPDATE` to transition the status to `dequeued` atomically.
- **Batch Dequeue (DequeueN)**: Uses a Common Table Expression (CTE) to claim up to N rows in a single database round-trip.
- **Priority and Ordering**: The queue respects job priority (`ORDER BY priority DESC`) and then follows a FIFO approach (`created_at ASC`).
- **Delayed and Retry Gating**: The dequeue query includes filters for `scheduled_at` and `next_retry_at`, ensuring jobs are not picked up before they are ready.
- **Visibility**: The transition from `queued` to `dequeued` acts as the claim mechanism, so no separate visibility timeout is required.

### 5. Finite State Machine

The lifecycle of a job run is managed by a Finite State Machine (FSM) with 13 possible states.

```
                    ┌─────────┐
                    │ delayed │
                    └────┬────┘
                         │ scheduled_at <= NOW
                         v
   ┌────────────────┬─────────┬────────────────────┐
   │                │ queued  │                     │
   │                └────┬────┘                     │
   │                     │ dequeue                  │
   │                     v                          │
   │              ┌──────────┐                      │
   │              │ dequeued │──────────┐            │
   │              └────┬─────┘          │            │
   │                   │ start          │ system     │
   │                   v                │ failure    │
   │    retry    ┌───────────┐          │            │
   │  ┌─────────>│ executing │          │            │
   │  │          └─────┬─────┘          │            │
   │  │  ┌─────────┬───┴───┬─────────┐ │            │
   │  │  │         │       │         │ │            │
   │  │  v         v       v         v v            │
   │  │ completed failed timed_out system_failed    │
   │  │                │                             │
   │  │                v (max attempts reached       │
   │  │          ┌─────────────┐  + FF_RUN_DLQ)     │
   │  │          │ dead_letter │                     │
   │  │          └─────────────┘                     │
   │  │                                              │
   │  └── (attempt < max_attempts) ──────────────────┘
   │                                                 │
   │  canceled <── (any non-terminal) ──────────────┘
   │  expired  <── (delayed, queued with expires_at) ┘
```

**Valid Transitions**
- **delayed**: -> `queued`, `canceled`, `expired`
- **queued**: -> `dequeued`, `canceled`, `expired`
- **dequeued**: -> `executing`, `queued`, `canceled`, `system_failed`
- **executing**: -> `completed`, `failed`, `timed_out`, `crashed`, `canceled`, `waiting`, `queued`, `system_failed`, `dead_letter`
- **waiting**: -> `executing`, `completed`, `failed`, `canceled`, `timed_out`
- **dead_letter**: -> `queued` (via DLQ replay)
- **Terminal States**: `completed`, `failed`, `timed_out`, `crashed`, `system_failed`, `canceled`, `expired` (no outgoing transitions except DLQ replay for `dead_letter`).

### 6. Authentication

The system implements two distinct authentication schemes:

**Internal Secret Auth**
Used for the management API (`/v1/*` endpoints). It requires an `Authorization: Bearer <INTERNAL_SECRET>` header. This is a simple string comparison used for job CRUD, triggering, and run management.

**Run Token Auth**
Used for the SDK API (`/sdk/v1/*` endpoints). It uses JWT HS256 signed with `JWT_SIGNING_KEY`. The token contains claims such as `sub` (runID), `exp` (timeout + 60s), and `iat`. The token is generated when a job is triggered and must be provided by the SDK to interact with that specific run.

### 7. Execution Lifecycle

1. **Trigger**: The API receives a POST request to `/v1/jobs/{id}/trigger`. If `FF_COST_BUDGETS` is enabled, the daily project cost budget is checked before enqueuing.
2. **Enqueue**: A `job_run` is created with `status=queued` (or `delayed`). A JWT is generated for the run.
3. **Dequeue**: A worker calls `DequeueN`, claiming the run via `SKIP LOCKED` and updating its status to `dequeued`.
4. **Execute**: The execution is submitted to the worker pool using `context.WithoutCancel`.
5. **Job Lookup**: The executor retrieves the job's configuration (endpoint, timeout, retry strategy, etc.).
6. **Environment Resolution**: If `FF_ENVIRONMENTS` is enabled and the job has an `environment_id`, the executor resolves environment variables. If an `ENDPOINT_URL` variable is defined, it overrides the job's `endpoint_url` after SSRF validation.
7. **Status Transition**: The run status transitions from `dequeued` to `executing` using an optimistic lock.
8. **Heartbeat**: A background goroutine starts sending periodic heartbeats to `job_runs`.
9. **Dispatch**: The executor sends an HTTP POST to the resolved endpoint URL with the payload and metadata headers (`X-Run-ID`, `X-Job-ID`, `X-Attempt`).
10. **Execution Tracing**: If `FF_EXECUTION_TRACING` is enabled, timing breakdowns (queue wait, dequeue, connect, TTFB, transfer, total) are captured and stored as JSONB on the run.
11. **Result Handling**:
    - 2xx response: Status becomes `completed` and the result is stored.
    - Non-2xx response: The run is scheduled for retry using the job's `retry_strategy` (defaulting to exponential) or marked as `failed`.
    - Timeout: Handled as a retry or marked as `timed_out`.
    - If `FF_ADAPTIVE_TIMEOUT` is enabled, the timeout may be dynamically adjusted based on historical execution data.
12. **DLQ**: If `FF_RUN_DLQ` is enabled and the run has exhausted all retry attempts, it transitions to `dead_letter` instead of `failed`.
13. **Usage Budget**: During execution, the SDK can report usage via the `/sdk/v1/runs/{runID}/usage` endpoint. If `FF_COST_BUDGETS` is enabled, the per-run cost limit is checked on each usage report. Budget exceeded → run fails gracefully.
14. **Webhook**: Upon reaching a terminal state, a webhook is optionally dispatched to the job's `webhook_url`.
15. **PubSub**: State changes are published to Redis for real-time SSE updates.

### 8. Retry and Backoff Strategies

The system supports four retry strategies, configurable per job via the `retry_strategy` field:

#### Exponential (default)

- **Formula**: `base × 2^(attempt-1)` ± 20% jitter, capped at 1 hour.
- **Behavior**: attempt 1 → ~1s, attempt 2 → ~2s, attempt 3 → ~4s, attempt 4 → ~8s.
- **Use case**: General-purpose retry for transient failures and rate limits.

#### Linear

- **Formula**: `base × attempt` ± 20% jitter, capped at 1 hour.
- **Behavior**: attempt 1 → ~1s, attempt 2 → ~2s, attempt 3 → ~3s.
- **Use case**: Predictable, gradually increasing delays.

#### Fixed

- **Formula**: `base` ± 20% jitter (constant).
- **Behavior**: Every retry waits ~1s.
- **Use case**: Polling-style retries with constant interval.

#### Custom

- **Formula**: User-provided delays via `retry_delays_secs` array.
- **Behavior**: `retry_delays_secs: [1, 5, 30, 120, 600]` → attempt 1 waits 1s, attempt 2 waits 5s, etc. Last value repeats for any additional attempts.
- **Use case**: Full control over the exact delay sequence.

**Common properties across all strategies**:
- ±20% jitter is applied to prevent thundering herd effects.
- A minimum floor of 1 second is enforced (guards against negative or zero custom delays).
- Maximum delay is capped at 1 hour.
- The `next_retry_at` column prevents the job from being dequeued until the backoff period has elapsed.
- Terminal failure: If `attempt >= max_attempts`, the run transitions to `failed` (or `dead_letter` if `FF_RUN_DLQ` is enabled).

**Input validation**: The API validates `retry_strategy` against the set of known strategies and ensures `retry_delays_secs` values are positive integers. Custom strategy requires at least one delay value.

### 9. Graceful Shutdown

The shutdown sequence is designed to protect in-flight work:
1. `signal.NotifyContext` captures termination signals (SIGINT, SIGTERM).
2. Cancellation propagates through the `errgroup`, causing the worker's polling loop to exit.
3. In-flight executions continue because they use `context.WithoutCancel(ctx)`.
4. `pool.Shutdown()` blocks until all active goroutines in the worker pool have finished.
5. The scheduler stops the cron ticker, delayed poller, and reaper.
6. The HTTP server is shut down with a 10-second grace period to drain connections.
7. Database and Redis connections are closed.

The use of `context.WithoutCancel` ensures that once a job has started its HTTP dispatch, it is allowed to complete and record its result, even if the orchestrator process is terminating.

### 10. Observability

**Tracing**
OpenTelemetry is used for distributed tracing, with spans covering store methods, queue operations, and the executor lifecycle. The `otelchi` middleware traces incoming HTTP requests. When `FF_EXECUTION_TRACING` is enabled, per-run execution traces capture granular timing breakdowns (queue wait, dequeue, connect, TTFB, transfer, total) stored as JSONB on the run record.

**Metrics**
Prometheus metrics are exposed at `GET /metrics`. Key metrics include:
- `orchestrator.run.transitions`: A counter of FSM state changes.
- `orchestrator.dispatch.duration`: A histogram of HTTP dispatch latency.
- `orchestrator.dequeue.duration`: A histogram of queue polling latency.

**Logging**
Structured JSON logging is implemented via `log/slog`. It captures key events such as job dequeue, dispatch, completion, failure, and webhook delivery.

**Health Scoring**
When `FF_JOB_HEALTH_SCORING` is enabled, the `GET /v1/jobs/{jobID}/health` endpoint computes aggregated health metrics over a configurable time window (1h, 1d, 7d, 30d). The health score is a composite metric factoring in success rate, timeout rate, crash rate, and latency stability, providing an at-a-glance view of job reliability.

**Debug Bundles**
When `FF_DEBUG_BUNDLE` is enabled, the `GET /v1/runs/{runID}/debug-bundle` endpoint assembles a comprehensive diagnostic payload including the run record, all events, checkpoints, usage records, execution trace, and job configuration — useful for troubleshooting production issues without manual data assembly.

### 11. Core Engine Features

#### Adaptive Timeout (FF_ADAPTIVE_TIMEOUT)

When enabled, the executor dynamically adjusts the HTTP dispatch timeout based on historical execution data for the job. Jobs that consistently complete quickly receive shorter timeouts, improving resource recovery. Jobs with variable execution times receive longer timeouts, preventing premature failures. The adjustment uses historical p95 and average durations to compute an appropriate timeout ceiling.

#### Run Dead Letter Queue (FF_RUN_DLQ)

When enabled, runs that permanently fail after exhausting all retry attempts transition to `dead_letter` status instead of `failed`. This provides:
- **Inspection**: Dead-lettered runs are queryable via `GET /v1/runs/dlq?project_id=X`.
- **Replay**: Individual runs can be replayed via `POST /v1/runs/{runID}/dlq-replay`, which resets the run to `queued` with attempt counter reset.
- **Separation**: DLQ runs are excluded from normal run listings, preventing noise in operational views.
- **Index**: A partial index on `status = 'dead_letter'` ensures efficient DLQ queries.

#### Execution Tracing (FF_EXECUTION_TRACING)

When enabled, the executor captures detailed timing breakdowns for each run execution:
- `queue_wait_ms`: Time from run creation to dequeue.
- `dequeue_ms`: Time spent in the dequeue operation.
- `connect_ms`: TCP + TLS connection establishment time.
- `ttfb_ms`: Time to first byte from the endpoint.
- `transfer_ms`: Response body transfer time.
- `total_ms`: End-to-end execution time.

Traces are stored as JSONB in the `execution_trace` column on `job_runs`.

#### Debug Bundles (FF_DEBUG_BUNDLE)

When enabled, debug mode can be activated per-run via `POST /v1/runs/{runID}/debug`. The `GET /v1/runs/{runID}/debug-bundle` endpoint assembles a comprehensive diagnostic payload including: the run record, all events, checkpoints, usage records, and the job configuration.

#### Run Continuation (FF_RUN_CONTINUATION)

When enabled, runs can create continuation runs via the SDK `POST /sdk/v1/runs/{runID}/continue` endpoint. Continuations form a lineage chain tracked via:
- `continuation_of`: References the parent run ID.
- `lineage_depth`: Tracks how deep in the continuation chain a run sits.
- Parent run waits until all descendants reach a terminal state.
- The lineage tree is queryable via `GET /v1/runs/{runID}/lineage`.

This supports long-running AI workflows that need to split work across multiple runs while maintaining a single logical execution chain.

#### Job Health Score (FF_JOB_HEALTH_SCORING)

The `GET /v1/jobs/{jobID}/health?window=7d` endpoint computes aggregated health metrics:
- **Counts**: total, completed, failed, timed_out, crashed, canceled, expired runs.
- **Rates**: success_rate (completed/total).
- **Latency**: avg_duration_secs, p95_duration_secs.
- **Composite**: health_score — a weighted score factoring in success rate, timeout rate, crash rate, and latency stability.

Supported time windows: `1h`, `1d`, `7d`, `30d` (default: `7d`).

#### Environment Endpoint Overrides (FF_ENVIRONMENTS)

Environments provide per-project, named configurations with key-value variables:
- Variables support inheritance from parent environments.
- Jobs link to an environment via `environment_id`.
- At dispatch time, the executor resolves environment variables and checks for an `ENDPOINT_URL` override.
- The override URL passes through SSRF validation (blocks private/loopback addresses) in both the API server and the worker.
- This enables routing the same job definition to different endpoints (staging, production, canary) without modifying the job.

#### Cost Budgets (FF_COST_BUDGETS)

Cost budget enforcement uses two limits defined in `project_quotas`:
- **Per-run**: `max_cost_per_run_microusd` — checked on each SDK usage report. Budget check occurs BEFORE recording the usage, so the violating report is rejected.
- **Daily**: `max_daily_cost_microusd` — checked at trigger time and uses project timezone for day boundary calculation.

Cost is tracked in micro-USD (1/1,000,000 USD) for integer precision. Performance indexes on `run_usage(run_id)` and `run_usage(created_at)` ensure efficient aggregation queries.

### 12. Feature Flags

All new engine capabilities are gated behind feature flags (environment variables defaulting to `false`). This allows:
- **Progressive rollout**: Enable features individually in production.
- **Safe testing**: Test new behaviors without affecting existing workloads.
- **Independent lifecycle**: Each feature can be enabled/disabled without code changes.

See the Configuration section in README.md for the complete feature flag reference.

### 13. Design Decisions

1. **Postgres as Queue**: Chosen to avoid the operational overhead of an external broker. `SKIP LOCKED` provides the necessary performance and reliability for typical workloads.
2. **Optimistic Locking**: Using `WHERE status = $from` in updates prevents race conditions during state transitions without requiring complex distributed locks.
3. **Raw SQL over ORM**: `pgx/v5` with hand-written SQL provides full control over performance-critical queries like `SKIP LOCKED` and CTEs.
4. **Interface Segregation**: Components define small, specific interfaces for the store methods they require, improving testability and decoupling.
5. **UUIDv7 Primary Keys**: Time-ordered UUIDs provide a natural sort order and avoid sequence contention while being stored as `TEXT`.
6. **Embedded Migrations**: SQL migrations are embedded in the binary using `go:embed` and run automatically on startup.
7. **Single Binary, Three Modes**: Simplifies deployment and development while allowing independent scaling of API and worker processes in production.
8. **context.WithoutCancel for In-flight Jobs**: Prioritizes job completion over immediate process exit during shutdown.
9. **Fire-and-forget Webhooks**: Webhooks are dispatched asynchronously to keep the execution path simple, accepting the trade-off of no built-in retries for webhooks.
10. **Build Tag for Integration Tests**: The `//go:build integration` tag separates fast unit tests from slower tests that require a Docker environment.
11. **Feature Flags for Progressive Rollout**: All new engine capabilities are gated behind boolean feature flags, enabling safe production rollout and independent feature lifecycle management.
12. **Micro-USD Cost Tracking**: Using integer micro-USD (1/1,000,000 USD) avoids floating-point precision issues in financial calculations while supporting sub-cent granularity.
13. **SSRF Validation in Both Layers**: Endpoint URL validation happens in both the API server (job creation/update) and the worker (environment override resolution) to prevent bypasses via environment variable injection.
14. **Budget Check Before Recording**: Cost budget validation occurs before persisting usage data, ensuring the violating request is rejected without inflating recorded costs.
15. **Dead Letter as FSM State**: DLQ is modeled as a first-class FSM state (`dead_letter`) rather than a separate table, allowing standard run queries and transitions while maintaining FSM invariants.
16. **Jitter on All Retry Strategies**: Even fixed and linear strategies apply ±20% jitter to prevent synchronized retry storms across multiple workers.
