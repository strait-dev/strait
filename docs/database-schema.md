# Database Schema

All primary keys are UUIDv7 stored as `TEXT`. Schema is managed by 47 migrations that run automatically on startup.

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
| `workflow_versions` | Workflow step definition snapshots per version |
| `workflow_run_labels` | Key-value labels on workflow runs |
| `workflow_step_approvals` | Step approval tracking (approvers, status, timestamps) |

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
