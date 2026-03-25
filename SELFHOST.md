# Self-Hosting Strait

Run Strait on your own infrastructure with a single command.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/) v2+
- 2 GB RAM minimum (4 GB recommended)
- Ports 8080 (API), 5432 (Postgres), 6379 (Redis), 7376 (Sequin) available

## Quick Start

```bash
# 1. Clone the repository.
git clone https://github.com/strait-dev/strait.git
cd strait

# 2. Generate secrets (only needed once).
./scripts/selfhost-init.sh

# 3. Start all services.
docker compose -f docker-compose.selfhost.yml up -d

# 4. Verify.
curl http://localhost:8080/health
# {"edition":"community","status":"ok"}
```

The init script prints your `INTERNAL_SECRET` -- save it. You need it to create projects and API keys.

## Creating Your First Job

```bash
SECRET="<your INTERNAL_SECRET from step 2>"

# Create a project.
curl -X POST http://localhost:8080/v1/projects \
  -H "Content-Type: application/json" \
  -H "X-Internal-Secret: $SECRET" \
  -d '{"id": "my-project", "org_id": "my-org", "name": "My Project"}'

# Create an API key.
API_KEY=$(curl -s -X POST http://localhost:8080/v1/api-keys \
  -H "Content-Type: application/json" \
  -H "X-Internal-Secret: $SECRET" \
  -d '{"project_id": "my-project", "name": "dev-key"}' | jq -r '.key')

echo "API Key: $API_KEY"

# Create a job.
curl -X POST http://localhost:8080/v1/jobs \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "project_id": "my-project",
    "name": "My First Job",
    "slug": "my-first-job",
    "endpoint_url": "https://httpbin.org/post",
    "max_attempts": 3,
    "timeout_secs": 30
  }'

# Trigger it.
JOB_ID="<job id from above>"
curl -X POST "http://localhost:8080/v1/jobs/$JOB_ID/trigger" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"payload": {"hello": "world"}}'
```

## What Gets Deployed

| Service | Image | Purpose |
|---|---|---|
| `strait` | `ghcr.io/strait-dev/strait:latest` | API server + worker |
| `postgres` | `postgres:18-alpine` | Primary database |
| `redis` | `redis:8-alpine` | Pub/sub, caching |
| `sequin` | `sequin/sequin:latest` | CDC (Change Data Capture) |

## Community Edition

The self-hosted version runs the **community edition** which includes:

- Job creation, scheduling, and HTTP dispatch
- Cron scheduling with overlap policies
- Retry strategies (exponential, linear, fixed, custom)
- Workflow DAG orchestration with dependencies
- Webhook subscriptions and delivery
- SSE real-time streaming
- API key management and RBAC
- Run lifecycle management (cancel, replay, pause, resume)
- Dead letter queue

Cloud-only features (managed container execution, advanced analytics, multi-region) are available on [strait.dev](https://strait.dev).

## Environment Overrides

Override any default by adding variables to `.env.selfhost` or passing them in the `environment` section of `docker-compose.selfhost.yml`.

Common overrides:

| Variable | Default | Description |
|---|---|---|
| `WORKER_CONCURRENCY` | `25` | Max concurrent job executions |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `RATE_LIMIT_REQUESTS` | `100` | Global rate limit per minute per IP |
| `DEFAULT_JOB_TIMEOUT_SECS` | `300` | Default job timeout |
| `DEFAULT_JOB_MAX_ATTEMPTS` | `3` | Default retry attempts |

## Upgrading

```bash
docker compose -f docker-compose.selfhost.yml pull
docker compose -f docker-compose.selfhost.yml up -d
```

Migrations run automatically on startup.

## Backup

Back up the Postgres data:

```bash
docker exec strait-postgres-1 pg_dump -U strait strait > backup.sql
```

Restore:

```bash
docker exec -i strait-postgres-1 psql -U strait strait < backup.sql
```

## Load Testing

Test your deployment with the packaged stress test:

```bash
docker run --rm --network host \
  -e STRAIT_URL=http://localhost:8080 \
  -e INTERNAL_SECRET="<your secret>" \
  -e ITERATIONS=1000 \
  ghcr.io/strait-dev/strait-loadtest
```
