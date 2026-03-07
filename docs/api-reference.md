# API Reference

All `/v1/*` endpoints require authentication via `Authorization: Bearer <token>` header (internal secret or API key). SDK `/sdk/v1/*` endpoints require a run token JWT issued by the trigger response. See [Authentication](authentication.md) for details.

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
| `GET` | `/v1/workflows/{workflowID}/graph` | Get DAG visualization |

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
| `POST` | `/v1/workflow-runs/{workflowRunID}/pause` | Pause a running workflow |
| `POST` | `/v1/workflow-runs/{workflowRunID}/resume` | Resume a paused workflow |
| `POST` | `/v1/workflow-runs/{workflowRunID}/retry` | Retry from first failed step |
| `POST` | `/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/approve` | Approve an approval step |
| `POST` | `/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/skip` | Skip a pending/waiting step |
| `POST` | `/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/force-complete` | Force-complete a step |

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
