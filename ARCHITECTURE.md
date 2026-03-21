# Architecture

## System Overview

Strait is a job orchestration platform that schedules, executes, and monitors background jobs and workflows. The Go service (`apps/strait/`) handles API requests, job execution, webhook delivery, workflow orchestration, and analytics.

```
                                Internet
                                   |
                          Fly.io (iad region)
                     +-----------+-----------+
                     |                       |
               strait app              strait-otel-collector
               (Go service)            (OTel Collector)
                     |                       |
          +----------+----------+    +-------+-------+
          |          |          |    |       |       |
       Postgres    Redis    ClickHouse   Grafana   Loki
       (primary)  (pub/sub)  (analytics)  Cloud    Cloud
                                   |
                                Sentry
                             (error tracking)
```

## Core Components

### API Server (`internal/api/`)

Chi router with middleware chain: CORS, security headers, request ID, rate limiting, auth, metrics, logging. Serves REST API on port 8080.

### Worker (`internal/worker/`)

Dequeues jobs from Postgres, executes via HTTP dispatch or managed containers (Fly Machines / Docker), handles retries, timeouts, and dead-letter queue.

### Scheduler (`internal/scheduler/`)

Cron scheduler for periodic jobs and workflows. Poller for delayed runs. Reaper for expired runs, stale heartbeats, and retention cleanup.

### Workflow Engine (`internal/workflow/`)

DAG-based workflow execution with step dependencies, approvals, fan-out/fan-in, and nested sub-workflows.

### Webhook Delivery (`internal/webhook/`)

Async webhook delivery with circuit breakers, retry with exponential backoff, and per-endpoint health scoring.

## Data Stores

### PostgreSQL (primary database)

All operational data: jobs, runs, workflows, API keys, secrets, webhook subscriptions, event triggers, notification channels. Connected via `pgx/v5` with pool tuning and OTel tracing.

### Redis

Pub/sub for real-time SSE streams, per-project rate limiting, webhook circuit breaker state. Optional -- app degrades gracefully without it.

### ClickHouse (analytics)

Optional analytics backend for historical data. Never required for operational correctness. Two data paths:

**Path 1: OTel Collector (traces)**
- Distributed traces from the app flow to `otel_traces` table
- 90-day TTL

**Path 2: Custom async exporter (analytics)**
- Batches records in memory, flushes every 5s or at 1000 records
- Backpressure: drops oldest if buffer exceeds 10x batch size
- Retry: requeues failed batches up to 2 times, then drops with metric

**ClickHouse Tables (12):**

| Table | Purpose | TTL | Populated by |
|-------|---------|-----|-------------|
| `run_analytics` | One row per completed run (status, duration, cost, tags) | 365d | ClickHouseSubscriber |
| `run_events` | Individual log/progress events per run | 90d | ClickHouseSubscriber |
| `run_usage_events` | AI model usage per run (provider, model, tokens, cost) | 365d | ClickHouseSubscriber |
| `compute_usage` | Container execution costs (machine, duration, cost) | 365d | ClickHouseSubscriber |
| `job_metadata` | Denormalized job slugs (ReplacingMergeTree) | none | API handlers on job create/update |
| `webhook_delivery_events` | Webhook delivery success/failure/latency | 365d | Webhook delivery worker |
| `workflow_run_analytics` | Workflow-level analytics (status, duration, step count) | 365d | publishWorkflowRunHook |
| `workflow_step_analytics` | Per-step timing and status within workflows | 365d | StepCallback |
| `workflow_approval_events` | Approval create/resolve records | 365d | OnApprovalChanged hook |
| `event_trigger_events` | Event trigger create/receive/timeout with timing | 365d | API handlers + reaper |
| `run_stats_daily` | Pre-aggregated daily run stats (materialized view target) | 365d | MV from run_analytics |
| `cost_daily` | Pre-aggregated daily costs (materialized view target) | 365d | MV from run_usage_events |

## Observability Stack

### Metrics (Prometheus + Grafana Cloud)

80+ custom metrics exposed at `/metrics` via OTel SDK Prometheus exporter. OTel Collector scrapes every 15s and forwards to Grafana Cloud via `prometheusremotewrite`.

**Key metric categories:**
- HTTP: request duration (with status label), in-flight requests
- Queue: depth by status, dequeue duration
- Worker pool: running/waiting/submitted/completed/failed/dropped
- Dispatch: duration, errors, run transitions
- Run lifecycle: duration histogram, latency anomalies, snooze count
- Webhooks: deliveries, duration, retries, circuit breaker, health score, backlog
- Workflows: triggers, step progressions, stalled runs, dependency waits
- Events: triggers created/received/timed out, wait duration
- Scheduler: cron triggers by status, cron drift
- DB pool: acquired/idle/total/max connections
- ClickHouse: exporter pending, dropped records, flush failures
- Notifications: delivery success/error
- Log drains: events success/error
- Pub/sub: publish errors

### Logs (Grafana Cloud Loki)

Structured JSON logs via `slog`. TeeHandler fans to stdout, Sentry (error-level), and OTLP HTTP. OTel Collector forwards to Grafana Cloud Loki.

**Labels available for LogQL queries:**
- `service_name="strait"`, `detected_level`, `method`, `path`, `status`, `user_agent`

### Traces (ClickHouse via OTel Collector)

OpenTelemetry tracing with `otlptracehttp` exporter. Automatic HTTP instrumentation via `otelchi`. Database query tracing via `otelpgx`. Custom spans for executor, webhook delivery, and store operations.

### Errors (Sentry)

ERROR-level slog records sent to Sentry with stack traces. Comprehensive sanitization: connection strings, bearer tokens, API keys, request headers/bodies are scrubbed. Known transient errors (context canceled, connection refused) are ignored.

### Uptime (Better Stack)

Two HTTP monitors:
- `strait.dev` -- marketing site status
- `strait.fly.dev/health` -- API health endpoint (60s interval)

### Alerting (Grafana Cloud + Better Stack)

19 Prometheus alert rules in `ops/monitoring/alerts-strait-core.yaml` and `ops/monitoring/alerts-authz-rbac.yaml`. Rules are imported into Grafana Cloud Alerting. Alert routing to Better Stack via webhook (requires paid plan -- setup script at `ops/monitoring/setup-alert-routing.sh`).

### Dashboards (Grafana Cloud)

3 dashboards at `https://strait.grafana.net`:

**Strait Overview** (`/d/strait-overview`): Health Score gauge (0-5), HTTP request rate/latency, queue depth, DLQ, worker pool, DB pool, dispatch errors, run duration, cron drift, reaper throughput, recent error logs.

**Strait Jobs & Workflows** (`/d/strait-jobs`): Run transitions, cron triggers, workflow triggers/steps, event triggers, latency anomalies, bulk operations.

**Strait Webhooks & Infrastructure** (`/d/strait-infra`): Webhook deliveries/latency/circuit breakers, managed machines, reaper operations, permission cache, Go GC.

### Health Checks

- `/health` -- basic liveness
- `/health/ready` -- database connectivity, Redis ping, worker pool, queue depth, migration status, scheduler freshness

## Analytics API (32 endpoints)

All analytics endpoints require `ScopeStatsRead` permission. Time range params use RFC3339 format with 90-day max window. ClickHouse is the primary backend with automatic Postgres fallback.

### Existing (7)
- `GET /v1/analytics/performance` -- slowest jobs, throughput, health summary
- `GET /v1/analytics/costs` -- AI + compute costs, by model, by job
- `GET /v1/analytics/costs/trends` -- cost trends over time
- `GET /v1/analytics/costs/top` -- top N most expensive jobs
- `GET /v1/analytics/compute` -- compute cost by machine preset
- `GET /v1/analytics/cost-insights` -- cost outliers
- `GET /v1/analytics/approvals` -- approval stats

### Run Analytics (5)
- `GET /v1/analytics/runs/timeline` -- run count over time by status
- `GET /v1/analytics/runs/duration-distribution` -- duration histogram
- `GET /v1/analytics/runs/failure-reasons` -- top error patterns
- `GET /v1/analytics/runs/summary` -- total/completed/failed/success rate
- `GET /v1/analytics/runs/by-trigger` -- breakdown by trigger type

### Job Analytics (6)
- `GET /v1/analytics/jobs/{jobID}/history` -- per-job performance over time
- `GET /v1/analytics/jobs/comparison` -- multi-job side-by-side comparison
- `GET /v1/analytics/jobs/reliability` -- reliability ranking
- `GET /v1/analytics/jobs/by-version` -- runs grouped by job version
- `GET /v1/analytics/jobs/cost-ranking` -- jobs ranked by cost
- `GET /v1/analytics/jobs/top-failing` -- top failing jobs

### Tag Analytics (3)
- `GET /v1/analytics/tags/summary` -- runs grouped by tag key/value
- `GET /v1/analytics/tags/top-failing` -- most failing tags
- `GET /v1/analytics/tags/cost` -- cost by tag

### Workflow Analytics (3)
- `GET /v1/analytics/workflows/{workflowID}/step-durations` -- per-step breakdown
- `GET /v1/analytics/workflows/completion-rates` -- completion vs failure over time
- `GET /v1/analytics/workflows/summary` -- workflow totals

### Webhook Analytics (3)
- `GET /v1/analytics/webhooks/delivery-stats` -- per-endpoint success/latency
- `GET /v1/analytics/webhooks/endpoint-health` -- health over time
- `GET /v1/analytics/webhooks/top-failing` -- most failing endpoints

### Event Analytics (2)
- `GET /v1/analytics/events/volume` -- event volume over time
- `GET /v1/analytics/events/latency` -- wait duration stats

### Cost Analytics (3)
- `GET /v1/analytics/costs/forecast` -- monthly cost projection
- `GET /v1/analytics/costs/by-trigger` -- cost by trigger type
- `GET /v1/analytics/costs/by-machine` -- cost by machine preset

## Infrastructure

### Fly.io Apps

| App | Region | Purpose | VM |
|-----|--------|---------|-----|
| `strait` | iad | Go service (API + worker + scheduler) | shared 2 CPU, 2GB |
| `strait-otel-collector` | iad | OTel Collector (traces/metrics/logs routing) | shared 1 CPU, 512MB |
| `strait-sequin` | iad | CDC consumer (Sequin) | - |

### Secrets Management

All secrets managed via Doppler (project: `strait`, configs: `dev`, `stg`, `prd`). Doppler syncs to Fly via integration. OTel Collector secrets set directly on Fly (`CLICKHOUSE_ENDPOINT`, `GRAFANA_*`).

### External Services

| Service | Purpose | Config |
|---------|---------|--------|
| Grafana Cloud | Metrics (Prometheus), Logs (Loki), Dashboards, Alerting | `GRAFANA_*` env vars |
| ClickHouse Cloud | Traces (OTel), Custom analytics (12 tables) | `CLICKHOUSE_*` env vars |
| Sentry | Error tracking (ERROR-level logs) | `SENTRY_DSN` |
| Better Stack | Uptime monitoring, alert routing, on-call | Web UI config |
| Polar | Billing/subscriptions | `POLAR_*` env vars |
| Resend | Transactional email | `RESEND_*` env vars |
