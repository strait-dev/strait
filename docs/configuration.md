# Configuration

All configuration is via environment variables. See the main [README](../README.md) for quick start.

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

## Environment File Example

```env
DATABASE_URL=postgres://user:pass@localhost:5432/dbname
REDIS_URL=redis://localhost:6379
INTERNAL_SECRET=your-internal-secret
JWT_SIGNING_KEY=your-jwt-signing-key-at-least-32-chars
```
