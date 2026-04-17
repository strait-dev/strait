# Self-Hosting Strait

Strait offers two self-hosting paths. Pick whichever matches how you want to run the dashboard.

| | Option 1 — Cloudflare dashboard | Option 2 — Full Docker stack |
|---|---|---|
| Setup | One click | `make selfhost` |
| Dashboard runs on | Your own Cloudflare account | Your own hardware |
| API runs on | Your own infrastructure (anywhere reachable) | Docker Compose on the same host |
| Postgres | Neon / Supabase / Fly PG / self-hosted | Bundled `postgres:18-alpine` |
| Best for | Zero-ops setups, teams already on Cloudflare | Air-gapped, on-prem, purist Docker users |

Both options run the community edition with all open-source features.

---

## Option 1 — Deploy the dashboard to Cloudflare

[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=https://github.com/strait-dev/strait)

Clicking the button forks the repo and takes you through a Cloudflare Workers Builds import. Because this repo is a Bun monorepo and Cloudflare does not yet support Bun workspace resolution in one-click deploys, you will need to set **Root directory** to `apps/app` and provide a custom **Build command** during import — the Hyperdrive binding, non-secret variables, and secret prompts are predeclared in `apps/app/wrangler.jsonc` so the rest of the flow is guided.

See [apps/app/README.md](apps/app/README.md#deploy-to-cloudflare) for the detailed walkthrough, including the exact build command string and the list of secrets you must set in the Cloudflare dashboard after the first deploy.

**You still need the Strait API somewhere reachable by the Worker.** The easiest setup: run the API locally with Option 2 below (or on any VPS/Kubernetes/Fly.io host), expose it via `cloudflared tunnel` or a public hostname, and point `STRAIT_API_URL` at it in the Cloudflare Worker's Variables.

---

## Option 2 — Run the full stack locally with Docker Compose

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/) v2+
- 2 GB RAM minimum (4 GB recommended)
- Ports 3000 (Dashboard), 8080 (API), 5432 (Postgres), 6379 (Redis), 7376 (Sequin) available

### Quick Start

```bash
# 1. Clone the repository.
git clone https://github.com/strait-dev/strait.git
cd strait

# 2. Generate secrets and start all services.
make selfhost

# 3. Verify.
curl http://localhost:8080/health
# {"edition":"community","status":"ok"}

# 4. Open the dashboard.
open http://localhost:3000
```

If you prefer the underlying commands, `make selfhost` runs `./packages/scripts/selfhost-init.sh` and then starts `docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml up -d`.

To stop or reset the stack:

```bash
make selfhost-down
make selfhost-reset
```

## Creating Your First Job

1. Open **http://localhost:3000** and sign up
2. A workspace is created automatically for you
3. Click **Create project** in the getting-started wizard
4. Follow the on-screen instructions to create and trigger your first job

The dashboard guides you through installing the SDK, deploying a job, and triggering your first run.

### Advanced: API-only usage

If you prefer using the API directly without the dashboard:

```bash
SECRET="<your INTERNAL_SECRET from .env.selfhost>"

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

# Create and trigger a job.
JOB=$(curl -s -X POST http://localhost:8080/v1/jobs \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"project_id":"my-project","name":"My Job","slug":"my-job","endpoint_url":"https://httpbin.org/post","max_attempts":3,"timeout_secs":30}')

curl -X POST "http://localhost:8080/v1/jobs/$(echo $JOB | jq -r .id)/trigger" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"payload": {"hello": "world"}}'
```

## What Gets Deployed

| Service | Image | Port | Purpose |
|---|---|---|---|
| `strait` | `ghcr.io/strait-dev/strait:latest` | 8080 | API server + worker |
| `strait-app` | `ghcr.io/strait-dev/strait-app:latest` | 3000 | Dashboard UI |
| `postgres` | `postgres:18-alpine` | 5432 | Primary database |
| `redis` | `redis:8-alpine` | 6379 | Pub/sub, caching |
| `sequin` | `sequin/sequin:latest` | 7376 | CDC (Change Data Capture) |

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

## Real-Time Updates (CDC)

Strait uses [Sequin](https://sequinstream.com) for Change Data Capture. Sequin monitors four Postgres tables and pushes changes to Strait via an HTTP Pull Consumer:

| Table | Purpose |
|---|---|
| `job_runs` | Job execution status changes |
| `workflow_runs` | Workflow orchestration updates |
| `workflow_step_runs` | Individual step completions |
| `event_triggers` | Event-driven trigger status |

CDC is **auto-configured** -- the Sequin consumer is provisioned via `packages/configs/sequin.yml` which is mounted into the Sequin container. No manual setup needed.

To verify CDC is working:
```bash
curl http://localhost:7376/health
# Should return healthy
```

CDC enables SSE real-time streaming (`GET /v1/runs/{runID}/stream`) and instant dashboard updates.

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
docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml pull
docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml up -d
```

Migrations run automatically on startup.

## Backup

Use the included backup script:

```bash
# Create a backup.
./packages/scripts/selfhost-backup.sh
# Backup saved to ./backups/strait_20260325_120000.sql.gz

# Schedule daily backups (add to crontab).
# 0 3 * * * cd /path/to/strait && ./packages/scripts/selfhost-backup.sh
```

Backups older than 30 days are automatically cleaned up.

Restore from backup:

```bash
gunzip -c backups/strait_20260325_120000.sql.gz | docker exec -i strait-postgres-1 psql -U strait strait
```

## API Reference

Interactive API documentation is available via Scalar:

- **Self-hosted**: http://localhost:8080/reference
- **Cloud**: https://api.strait.dev/reference

The OpenAPI 3.0 spec is served at `/reference/openapi.json`.

## Monitoring

Strait exposes a Prometheus-compatible `/metrics` endpoint with 50+ metrics (queue depth, dispatch latency, error rates, worker concurrency). Point your existing Prometheus/Datadog/New Relic at `http://localhost:8080/metrics`.

For advanced analytics dashboards, cost tracking, log search, and alerting -- use [Strait Cloud](https://strait.dev).

## Using the SDKs

Trigger jobs programmatically from your application:

**TypeScript** ([github.com/strait-dev/strait-ts](https://github.com/strait-dev/strait-ts)):
```bash
npm install @strait/ts
```
```typescript
import { Strait } from "@strait/ts";

const strait = new Strait({
  baseUrl: "http://localhost:8080",
  apiKey: "strait_...",
});

const run = await strait.jobs.trigger("my-job", {
  payload: { userId: "123", action: "process" },
});
```

**Python** ([github.com/strait-dev/python-sdk](https://github.com/strait-dev/python-sdk)):
```bash
pip install strait
```
```python
from strait import Strait

client = Strait(base_url="http://localhost:8080", api_key="strait_...")
run = client.jobs.trigger("my-job", payload={"userId": "123"})
```

**Go** ([github.com/strait-dev/go-sdk](https://github.com/strait-dev/go-sdk)):
```bash
go get github.com/strait-dev/go-sdk
```
```go
client := strait.New("http://localhost:8080", "strait_...")
run, _ := client.Jobs.Trigger(ctx, "my-job", map[string]any{"userId": "123"})
```

Or use the REST API directly:
```bash
curl -X POST http://localhost:8080/v1/jobs/my-job/trigger \
  -H "Authorization: Bearer strait_..." \
  -H "Content-Type: application/json" \
  -d '{"payload": {"userId": "123"}}'
```

## Resetting

Start fresh by wiping all data and secrets:

```bash
./packages/scripts/selfhost-init.sh --reset
./packages/scripts/selfhost-init.sh
docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml up -d
```

## Troubleshooting

**Strait API won't start:**
```bash
docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml logs strait
```
Common causes: Postgres not ready (wait and retry), invalid secrets in `.env.selfhost` (run `--reset`).

**Dashboard can't reach the API:**
Both services must share the same `INTERNAL_SECRET` from `.env.selfhost`. Restart both: `docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml restart`

**Sequin not starting:**
```bash
docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml logs sequin
```
Sequin needs Postgres to be fully ready with logical replication enabled. The Alpine Postgres image enables it by default.

**Migrations failing:**
Migrations run automatically on startup. Check logs: `docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml logs strait | grep migration`

**Contributing:**
See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup.
