# Architecture

This document provides a deep technical overview of the job orchestrator's internals, design decisions, and component interactions.

### 1. System Overview

The system is designed as a distributed job orchestrator that uses PostgreSQL as its primary state store and message queue.

```
                    ┌──────────────────────────────────┐
                    │           API Server              │
                    │  (Chi router + middleware)         │
                    │                                    │
                    │  /v1/jobs/* ── Job CRUD            │
                    │  /v1/jobs/{id}/trigger ── Enqueue  │
    HTTP ──────────>│  /v1/runs/* ── Run management     │
    clients         │  /sdk/v1/* ── SDK (JWT auth)      │
                    │  /metrics ── Prometheus            │
                    └──────────┬───────────────────────┘
                               │ Enqueue
                               v
                    ┌──────────────────────────────────┐
                    │         PostgreSQL                 │
                    │                                    │
                    │  jobs ── job definitions           │
                    │  job_runs ── run state + queue     │
                    │  run_events ── log entries         │
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
                    │  Execute ─> HTTP POST to endpoint  │
                    │  Handle ─> Success/Failure/Timeout │
                    │  Retry ─> Re-enqueue with backoff  │
                    └──────────┬───────────────────────┘
                               │ Webhook / PubSub
                               v
                    ┌──────────────────────────────────┐
                    │  Scheduler         │  Redis       │
                    │  - Cron ticker     │  - PubSub    │
                    │  - Delayed poller  │  - SSE       │
                    │  - Stale reaper    │  streaming   │
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
URL validation includes SSRF protection to block private and loopback IP addresses.

**config**
Handles environment variable loading using Viper. It manages 20 configuration fields with sensible defaults and performs validation (e.g., `DATABASE_URL` and `INTERNAL_SECRET` are required, `JWT_SIGNING_KEY` must be at least 32 characters).

**domain**
Defines core types such as `Job` (15 fields), `JobRun` (19 fields), and `RunEvent`. It includes a `RunStatus` enum with 12 states and structured error types like `TransitionError`, `UnknownStatusError`, `EndpointError`, `FieldError`, and `ConfigError`, along with sentinel errors like `ErrJobNotFound`.

**store**
Provides raw SQL access via `pgx/v5`. It uses interface segregation with `JobStore`, `RunStore`, and `EventStore`. The `Queries` struct accepts a `DBTX` interface, allowing it to work with both connection pools and transactions. `UpdateRunStatus` implements optimistic locking using `WHERE id = $2 AND status = $3`. All methods are instrumented with OTel spans.

**queue**
The `PostgresQueue` implements the `Queue` interface. It uses `SELECT FOR UPDATE SKIP LOCKED` for atomic job claiming, eliminating the need for an external message broker. `DequeueN` uses a CTE pattern for batch claims. Jobs are ordered by `priority DESC, created_at ASC`. Filters ensure that delayed or retrying jobs are only dequeued after their scheduled time.

**worker**
The executor polls the queue at configurable intervals. It uses a semaphore-based worker pool (buffered channel + WaitGroup). For each execution, it fetches job configuration, transitions the state from `dequeued` to `executing`, dispatches an HTTP POST to the job endpoint, and handles the result. In-flight jobs use `context.WithoutCancel` to survive process shutdown. A background goroutine sends heartbeats for each active run.

**scheduler**
Consists of three background components:
- **CronScheduler**: Uses `robfig/cron/v3` to tick matching jobs and enqueue new runs.
- **DelayedPoller**: Finds runs with `status=delayed` that are ready to be queued.
- **Reaper**: Identifies stale `executing` runs (based on heartbeat), stale `dequeued` runs, and expired runs.

**pubsub**
Defines a `Publisher` interface for real-time updates. The `RedisPublisher` implementation uses `go-redis/v9` to support SSE streaming via channels named `run:{runID}`.

**dbscan**
Provides shared row scanning logic. `ScanRun` handles 19 columns in a fixed order, using helpers like `NilIfEmptyString` to preserve SQL NULL semantics.

**telemetry**
Sets up the OpenTelemetry `TracerProvider` (OTLP HTTP) and `MeterProvider` (Prometheus). It tracks metrics such as `RunTransitions` (counter), `DispatchDuration` (histogram), and `DequeueDuration` (histogram).

### 3. Data Model

The system relies on three primary tables in PostgreSQL:

**jobs**
```sql
id            TEXT PRIMARY KEY              -- UUIDv7
project_id    TEXT NOT NULL
name          TEXT NOT NULL
slug          TEXT NOT NULL
description   TEXT
cron          TEXT                          -- 5-field cron expression
payload_schema JSONB
endpoint_url  TEXT NOT NULL
max_attempts  INT NOT NULL DEFAULT 3
timeout_secs  INT NOT NULL DEFAULT 300
enabled       BOOLEAN NOT NULL DEFAULT TRUE
webhook_url   TEXT
webhook_secret TEXT
created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
UNIQUE (project_id, slug)
```

**job_runs**
```sql
id              TEXT PRIMARY KEY            -- UUIDv7
job_id          TEXT NOT NULL REFERENCES jobs(id)
project_id      TEXT NOT NULL
status          TEXT NOT NULL DEFAULT 'queued'
attempt         INT NOT NULL DEFAULT 1
payload         JSONB
result          JSONB
error           TEXT
triggered_by    TEXT NOT NULL DEFAULT 'manual'  -- manual, cron, spawn
scheduled_at    TIMESTAMPTZ
started_at      TIMESTAMPTZ
finished_at     TIMESTAMPTZ
heartbeat_at    TIMESTAMPTZ
next_retry_at   TIMESTAMPTZ
expires_at      TIMESTAMPTZ
parent_run_id   TEXT REFERENCES job_runs(id)
priority        INT NOT NULL DEFAULT 0
idempotency_key TEXT
created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
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

### 4. Queue Mechanics

The orchestrator uses PostgreSQL as a message queue to minimize operational complexity and ensure transactional consistency.

- **SELECT FOR UPDATE SKIP LOCKED**: This allows multiple workers to poll the same table simultaneously without blocking each other. Each worker locks only the rows it is currently processing.
- **Atomic Dequeue**: A single dequeue operation uses a subquery to select a row with `SKIP LOCKED` and an outer `UPDATE` to transition the status to `dequeued` atomically.
- **Batch Dequeue (DequeueN)**: Uses a Common Table Expression (CTE) to claim up to N rows in a single database round-trip.
- **Priority and Ordering**: The queue respects job priority (`ORDER BY priority DESC`) and then follows a FIFO approach (`created_at ASC`).
- **Delayed and Retry Gating**: The dequeue query includes filters for `scheduled_at` and `next_retry_at`, ensuring jobs are not picked up before they are ready.
- **Visibility**: The transition from `queued` to `dequeued` acts as the claim mechanism, so no separate visibility timeout is required.

### 5. Finite State Machine

The lifecycle of a job run is managed by a Finite State Machine (FSM) with 12 possible states.

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
- **executing**: -> `completed`, `failed`, `timed_out`, `crashed`, `canceled`, `waiting`, `queued`, `system_failed`
- **waiting**: -> `executing`, `completed`, `failed`, `canceled`, `timed_out`
- **Terminal States**: `completed`, `failed`, `timed_out`, `crashed`, `system_failed`, `canceled`, `expired` (no outgoing transitions).

### 6. Authentication

The system implements two distinct authentication schemes:

**Internal Secret Auth**
Used for the management API (`/v1/*` endpoints). It requires an `Authorization: Bearer <INTERNAL_SECRET>` header. This is a simple string comparison used for job CRUD, triggering, and run management.

**Run Token Auth**
Used for the SDK API (`/sdk/v1/*` endpoints). It uses JWT HS256 signed with `JWT_SIGNING_KEY`. The token contains claims such as `sub` (runID), `exp` (timeout + 60s), and `iat`. The token is generated when a job is triggered and must be provided by the SDK to interact with that specific run.

### 7. Execution Lifecycle

1. **Trigger**: The API receives a POST request to `/v1/jobs/{id}/trigger`.
2. **Enqueue**: A `job_run` is created with `status=queued` (or `delayed`). A JWT is generated for the run.
3. **Dequeue**: A worker calls `DequeueN`, claiming the run via `SKIP LOCKED` and updating its status to `dequeued`.
4. **Execute**: The execution is submitted to the worker pool using `context.WithoutCancel`.
5. **Job Lookup**: The executor retrieves the job's configuration (endpoint, timeout, etc.).
6. **Status Transition**: The run status transitions from `dequeued` to `executing` using an optimistic lock.
7. **Heartbeat**: A background goroutine starts sending periodic heartbeats to `job_runs`.
8. **Dispatch**: The executor sends an HTTP POST to the job's `endpoint_url` with the payload and metadata headers (`X-Run-ID`, `X-Job-ID`, `X-Attempt`).
9. **Result Handling**:
   - 2xx response: Status becomes `completed` and the result is stored.
   - Non-2xx response: The run is scheduled for retry or marked as `failed`.
   - Timeout: Handled as a retry or marked as `timed_out`.
10. **Webhook**: Upon reaching a terminal state, a webhook is optionally dispatched to the job's `webhook_url`.
11. **PubSub**: State changes are published to Redis for real-time SSE updates.

### 8. Retry and Backoff Strategy

The system employs an exponential backoff strategy with jitter:
- **Backoff**: `base * 2^(attempt-1)` where the base is 1 second.
- **Jitter**: A random variation of +/- 20% is applied to the backoff.
- **Flow**: When a retry is triggered, the run transitions back to `queued`, the `attempt` count is incremented, and `next_retry_at` is set.
- **Gating**: The `next_retry_at` column prevents the job from being dequeued until the backoff period has elapsed.
- **Terminal Failure**: If `attempt >= max_attempts`, the run transitions to a final state (`failed` or `timed_out`).

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
OpenTelemetry is used for distributed tracing, with spans covering store methods, queue operations, and the executor lifecycle. The `otelchi` middleware traces incoming HTTP requests.

**Metrics**
Prometheus metrics are exposed at `GET /metrics`. Key metrics include:
- `orchestrator.run.transitions`: A counter of FSM state changes.
- `orchestrator.dispatch.duration`: A histogram of HTTP dispatch latency.
- `orchestrator.dequeue.duration`: A histogram of queue polling latency.

**Logging**
Structured JSON logging is implemented via `log/slog`. It captures key events such as job dequeue, dispatch, completion, failure, and webhook delivery.

### 11. Design Decisions

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